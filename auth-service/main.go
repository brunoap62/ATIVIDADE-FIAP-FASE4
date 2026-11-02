package main

import (
	"database/sql"
	// "fmt"
	"log"
	"net/http"
	"os"

	_ "github.com/jackc/pgx/v4/stdlib" // Adicione o underline (_) exatamente assim
	"github.com/joho/godotenv"
)

// App struct (para injeção de dependência)
type App struct {
	DB         *sql.DB
	MasterKey  string
}

func main() {
	// Carrega o .env para desenvolvimento local. Em produção, isso não fará nada.
	_ = godotenv.Load()

	// --- Configuração ---
	port := os.Getenv("PORT")
	if port == "" {
		port = "8001" // Porta padrão
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL deve ser definida")
	}

	masterKey := os.Getenv("MASTER_KEY")
	if masterKey == "" {
		log.Fatal("MASTER_KEY deve ser definida")
	}

	// --- Conexão com o Banco ---
	db, err := connectDB(databaseURL)
	if err != nil {
		log.Fatalf("Não foi possível conectar ao banco de dados: %v", err)
	}
	defer db.Close()

	// --- Inicialização Automática de Tabelas ---
	if err := initDB(db); err != nil {
		log.Fatalf("Erro ao inicializar tabelas do banco de dados: %v", err)
	}

	app := &App{
		DB:         db,
		MasterKey:  masterKey,
	}

	// --- Rotas da API ---
	mux := http.NewServeMux()
	mux.HandleFunc("/health", app.healthHandler)

	// Endpoint público para validar uma chave
	mux.HandleFunc("/validate", app.validateKeyHandler)

	// Endpoints de "admin" para criar/gerenciar chaves
	// Eles são protegidos pelo middleware de autenticação
	mux.Handle("/admin/keys", app.masterKeyAuthMiddleware(http.HandlerFunc(app.createKeyHandler)))

	log.Printf("Serviço de Autenticação (Go) rodando na porta %s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}

// connectDB inicializa e testa a conexão com o PostgreSQL
func connectDB(databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}

	if err = db.Ping(); err != nil {
		return nil, err
	}

	log.Println("Conectado ao PostgreSQL com sucesso!")
	return db, nil
}

// initDB cria a estrutura inicial de tabelas e seeds caso não existam
func initDB(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS api_keys (
		id SERIAL PRIMARY KEY,
		name VARCHAR(100) NOT NULL,
		key_hash VARCHAR(64) NOT NULL UNIQUE, 
		is_active BOOLEAN DEFAULT true,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);
	`
	if _, err := db.Exec(query); err != nil {
		return err
	}

	// Faz o seed dinâmico da chave do evaluation-service a partir do segredo injetado (SSM/ESO),
	// evitando qualquer chave ou hash hardcoded no repositório.
	if serviceAPIKey := os.Getenv("SERVICE_API_KEY"); serviceAPIKey != "" {
		keyHash := hashAPIKey(serviceAPIKey)
		seedQuery := `
		INSERT INTO api_keys (name, key_hash) 
		VALUES ('evaluation-service-key', $1)
		ON CONFLICT (key_hash) DO NOTHING;
		`
		if _, err := db.Exec(seedQuery, keyHash); err != nil {
			return err
		}
	}

	log.Println("Tabela 'api_keys' e chaves padrão verificadas/criadas com sucesso!")
	return nil
}





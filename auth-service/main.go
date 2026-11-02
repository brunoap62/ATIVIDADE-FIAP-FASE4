package main

import (
	"context"
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
	defer func() { _ = db.Close() }()

	// --- Inicialização Automática de Tabelas ---
	if err := initDB(db); err != nil {
		log.Fatalf("Erro ao inicializar tabelas do banco de dados: %v", err)
	}

	// --- Inicialização do OpenTelemetry (Traces & Logs OTLP) ---
	ctx := context.Background()
	serviceName := os.Getenv("OTEL_SERVICE_NAME")
	if serviceName == "" {
		serviceName = "auth-service"
	}
	shutdownTelemetry := initTelemetry(ctx, serviceName)
	defer func() { _ = shutdownTelemetry(ctx) }()

	app := &App{
		DB:         db,
		MasterKey:  masterKey,
	}

	// --- Rotas da API (Instrumentadas com OpenTelemetry) ---
	mux := http.NewServeMux()
	mux.HandleFunc("/health", app.otelMiddleware("/health", app.healthHandler))
	mux.HandleFunc("/validate", app.otelMiddleware("/validate", app.validateKeyHandler))
	mux.HandleFunc("/chaos", app.otelMiddleware("/chaos", app.chaosHandler))

	// Endpoints de "admin" para criar/gerenciar chaves
	// Eles são protegidos pelo middleware de autenticação
	adminHandler := app.masterKeyAuthMiddleware(http.HandlerFunc(app.createKeyHandler))
	mux.HandleFunc("/admin/keys", app.otelMiddleware("/admin/keys", adminHandler.ServeHTTP))

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
	svcKey := os.Getenv("SERVICE_API_KEY")
	if svcKey != "" {
		keyHash := hashAPIKey(svcKey)
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





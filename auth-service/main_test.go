package main

import (
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestConnectDB_InvalidURL(t *testing.T) {
	// Tenta conectar em uma porta onde não há PostgreSQL rodando
	_, err := connectDB("postgres://127.0.0.1:54333/invalid_db?sslmode=disable&connect_timeout=1")
	if err == nil {
		t.Error("esperava erro ao conectar em URL de banco inválida, obteve nil")
	}
}

func TestInitDB_WithServiceAPIKey(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("erro ao criar sqlmock: %v", err)
	}
	defer db.Close()

	testKey := "tm_key_test_dynamic_seed"
	t.Setenv("SERVICE_API_KEY", testKey)

	mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE IF NOT EXISTS api_keys")).
		WillReturnResult(sqlmock.NewResult(0, 0))

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO api_keys (name, key_hash)")).
		WithArgs(hashAPIKey(testKey)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := initDB(db); err != nil {
		t.Errorf("initDB retornou erro inesperado: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectativas do sqlmock não atendidas: %v", err)
	}
}

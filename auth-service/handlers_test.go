package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestHealthHandler(t *testing.T) {
	app := &App{}

	req, err := http.NewRequest(http.MethodGet, "/health", nil)
	if err != nil {
		t.Fatalf("falha ao criar requisição: %v", err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(app.healthHandler)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status code incorreto: obteve %d, esperado %d", rr.Code, http.StatusOK)
	}

	contentType := rr.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("header Content-Type incorreto: obteve %s, esperado application/json", contentType)
	}

	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("falha ao deserializar resposta JSON: %v", err)
	}

	if !strings.HasPrefix(body["status"], "ok") {
		t.Errorf("campo status incorreto: obteve %q, esperado prefixo 'ok'", body["status"])
	}
}

func TestValidateKeyHandler(t *testing.T) {
	tests := []struct {
		name           string
		authHeader     string
		mockSetup      func(mock sqlmock.Sqlmock)
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "sem header de autorização",
			authHeader:     "",
			mockSetup:      func(mock sqlmock.Sqlmock) {},
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   "Authorization header não encontrado",
		},
		{
			name:           "header de autorização com bearer vazio",
			authHeader:     "Bearer ",
			mockSetup:      func(mock sqlmock.Sqlmock) {},
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   "Authorization header não encontrado",
		},
		{
			name:       "chave não encontrada no banco (inativa ou inexistente)",
			authHeader: "Bearer invalid_token",
			mockSetup: func(mock sqlmock.Sqlmock) {
				tokenVal := "invalid_token"
				hash := hashAPIKey(tokenVal)
				mock.ExpectQuery(regexp.QuoteMeta("SELECT id FROM api_keys WHERE key_hash = $1 AND is_active = true")).
					WithArgs(hash).
					WillReturnError(sql.ErrNoRows)
			},
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   "Chave de API inválida ou inativa",
		},
		{
			name:       "erro no banco de dados durante a validação",
			authHeader: "Bearer error_token",
			mockSetup: func(mock sqlmock.Sqlmock) {
				tokenVal := "error_token"
				hash := hashAPIKey(tokenVal)
				mock.ExpectQuery(regexp.QuoteMeta("SELECT id FROM api_keys WHERE key_hash = $1 AND is_active = true")).
					WithArgs(hash).
					WillReturnError(errors.New("db connection timeout"))
			},
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   "Chave de API inválida ou inativa",
		},
		{
			name:       "chave válida encontrada e ativa",
			authHeader: "Bearer valid_token",
			mockSetup: func(mock sqlmock.Sqlmock) {
				tokenVal := "valid_token"
				hash := hashAPIKey(tokenVal)
				rows := sqlmock.NewRows([]string{"id"}).AddRow(1)
				mock.ExpectQuery(regexp.QuoteMeta("SELECT id FROM api_keys WHERE key_hash = $1 AND is_active = true")).
					WithArgs(hash).
					WillReturnRows(rows)
			},
			expectedStatus: http.StatusOK,
			expectedBody:   `"message":"Chave válida"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("erro ao criar sqlmock: %v", err)
			}
			defer func() { _ = db.Close() }()

			tt.mockSetup(mock)

			app := &App{DB: db}

			req, err := http.NewRequest(http.MethodGet, "/validate", nil)
			if err != nil {
				t.Fatalf("falha ao criar requisição: %v", err)
			}
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			rr := httptest.NewRecorder()
			handler := http.HandlerFunc(app.validateKeyHandler)
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("status code: obteve %d, esperado %d", rr.Code, tt.expectedStatus)
			}

			if !strings.Contains(rr.Body.String(), tt.expectedBody) {
				t.Errorf("corpo da resposta: obteve %q, esperado conter %q", rr.Body.String(), tt.expectedBody)
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("expectativas do mock não foram cumpridas: %v", err)
			}
		})
	}
}

func TestCreateKeyHandler(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		body           interface{}
		rawBody        string
		mockSetup      func(mock sqlmock.Sqlmock)
		expectedStatus int
		checkResponse  func(t *testing.T, rr *httptest.ResponseRecorder)
	}{
		{
			name:           "método inválido GET",
			method:         http.MethodGet,
			body:           nil,
			mockSetup:      func(mock sqlmock.Sqlmock) {},
			expectedStatus: http.StatusMethodNotAllowed,
			checkResponse: func(t *testing.T, rr *httptest.ResponseRecorder) {
				if !strings.Contains(rr.Body.String(), "Método não permitido") {
					t.Errorf("esperado 'Método não permitido', obteve %s", rr.Body.String())
				}
			},
		},
		{
			name:           "corpo JSON inválido",
			method:         http.MethodPost,
			rawBody:        "{ invalid json",
			mockSetup:      func(mock sqlmock.Sqlmock) {},
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, rr *httptest.ResponseRecorder) {
				if !strings.Contains(rr.Body.String(), "Corpo da requisição inválido") {
					t.Errorf("esperado 'Corpo da requisição inválido', obteve %s", rr.Body.String())
				}
			},
		},
		{
			name:           "campo name vazio",
			method:         http.MethodPost,
			body:           CreateKeyRequest{Name: ""},
			mockSetup:      func(mock sqlmock.Sqlmock) {},
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, rr *httptest.ResponseRecorder) {
				if !strings.Contains(rr.Body.String(), "O campo 'name' é obrigatório") {
					t.Errorf("esperado 'O campo 'name' é obrigatório', obteve %s", rr.Body.String())
				}
			},
		},
		{
			name:   "erro ao salvar no banco",
			method: http.MethodPost,
			body:   CreateKeyRequest{Name: "servico-pedidos"},
			mockSetup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO api_keys (name, key_hash) VALUES ($1, $2) RETURNING id")).
					WithArgs("servico-pedidos", sqlmock.AnyArg()).
					WillReturnError(errors.New("db insert error"))
			},
			expectedStatus: http.StatusInternalServerError,
			checkResponse: func(t *testing.T, rr *httptest.ResponseRecorder) {
				if !strings.Contains(rr.Body.String(), "Erro ao salvar a chave") {
					t.Errorf("esperado 'Erro ao salvar a chave', obteve %s", rr.Body.String())
				}
			},
		},
		{
			name:   "sucesso na criação de chave",
			method: http.MethodPost,
			body:   CreateKeyRequest{Name: "servico-pedidos"},
			mockSetup: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id"}).AddRow(1)
				mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO api_keys (name, key_hash) VALUES ($1, $2) RETURNING id")).
					WithArgs("servico-pedidos", sqlmock.AnyArg()).
					WillReturnRows(rows)
			},
			expectedStatus: http.StatusCreated,
			checkResponse: func(t *testing.T, rr *httptest.ResponseRecorder) {
				var resp CreateKeyResponse
				if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
					t.Fatalf("falha ao deserializar resposta: %v", err)
				}
				if resp.Name != "servico-pedidos" {
					t.Errorf("esperado Name 'servico-pedidos', obteve '%s'", resp.Name)
				}
				if !strings.HasPrefix(resp.Key, "tm_key_") {
					t.Errorf("esperado prefixo 'tm_key_', obteve '%s'", resp.Key)
				}
				if resp.Message == "" {
					t.Errorf("mensagem de sucesso não deveria estar vazia")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("erro ao criar sqlmock: %v", err)
			}
			defer func() { _ = db.Close() }()

			tt.mockSetup(mock)

			app := &App{DB: db}

			var reqBody []byte
			if tt.rawBody != "" {
				reqBody = []byte(tt.rawBody)
			} else if tt.body != nil {
				reqBody, _ = json.Marshal(tt.body)
			}

			req, err := http.NewRequest(tt.method, "/admin/keys", bytes.NewBuffer(reqBody))
			if err != nil {
				t.Fatalf("falha ao criar requisição: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			handler := http.HandlerFunc(app.createKeyHandler)
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("status code: obteve %d, esperado %d", rr.Code, tt.expectedStatus)
			}

			tt.checkResponse(t, rr)

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("expectativas do mock não foram cumpridas: %v", err)
			}
		})
	}
}

func TestMasterKeyAuthMiddleware(t *testing.T) {
	app := &App{
		MasterKey: "super-secret-master-key",
	}

	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("acesso permitido"))
	})

	protectedHandler := app.masterKeyAuthMiddleware(dummyHandler)

	tests := []struct {
		name           string
		authHeader     string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "sem header Authorization",
			authHeader:     "",
			expectedStatus: http.StatusForbidden,
			expectedBody:   "Acesso não autorizado",
		},
		{
			name:           "header com chave mestre incorreta",
			authHeader:     "Bearer chave-errada",
			expectedStatus: http.StatusForbidden,
			expectedBody:   "Acesso não autorizado",
		},
		{
			name:           "header com chave mestre correta",
			authHeader:     "Bearer super-secret-master-key",
			expectedStatus: http.StatusOK,
			expectedBody:   "acesso permitido",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, "/admin/keys", nil)
			if err != nil {
				t.Fatalf("falha ao criar requisição: %v", err)
			}
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			rr := httptest.NewRecorder()
			protectedHandler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("status code: obteve %d, esperado %d", rr.Code, tt.expectedStatus)
			}

			if !strings.Contains(rr.Body.String(), tt.expectedBody) {
				t.Errorf("corpo da resposta: obteve %q, esperado conter %q", rr.Body.String(), tt.expectedBody)
			}
		})
	}
}

package server

import (
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jing332/tts-server-go/internal/tokenstore"
)

func TestTokenAdminLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tokens.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	schema, err := os.ReadFile("../internal/tokenstore/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
	manager, err := tokenstore.OpenManager(path)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	validator, err := tokenstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer validator.Close()
	logger, err := NewAPILogger(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := &GracefulServer{AdminToken: strings.Repeat("a", 43), TokenManager: manager, TokenValidator: validator, APILogger: logger}
	s.HandleFunc()
	handler := s.logAPI(s.serveMux)
	call := func(method, admin, token, body string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, "/api/tokens", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+admin)
		r.Header.Set("Token", token)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	input := `{"username":"alice","expires_at":"2125-10-19T16:14:41+08:00"}`
	for _, secret := range []string{"", "wrong", "ordinary-user-token"} {
		if w := call("POST", secret, "", input); w.Code != 401 {
			t.Fatalf("unauthorized: %d", w.Code)
		}
	}
	for _, body := range []string{`{}`, `{"username":"alice","expires_at":"2020-01-01T00:00:00Z"}`, input + `{}`, `{"username":"alice","expires_at":"not-a-date"}`} {
		if w := call("POST", s.AdminToken, "", body); w.Code != 400 {
			t.Fatalf("invalid: %d %s", w.Code, w.Body.String())
		}
	}
	w := call("POST", s.AdminToken, "", input)
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var created struct {
		Token     string    `json:"token"`
		Username  string    `json:"username"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if len(created.Token) < 32 || created.Username != "alice" {
		t.Fatal("invalid token response")
	}
	if ok, err := validator.Valid(t.Context(), created.Token); err != nil || !ok {
		t.Fatal("new token rejected", err)
	}
	if ok, _ := validator.Valid(t.Context(), s.AdminToken); ok {
		t.Fatal("admin credential must not be a user token")
	}
	w = call("GET", s.AdminToken, created.Token, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "alice") || !strings.Contains(w.Body.String(), "2125-10-19T08:14:41Z") {
		t.Fatalf("lookup: %s", w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("token response may be cached")
	}
	var stored string
	if err := db.QueryRow(`SELECT token_hash FROM tokens`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == created.Token || len(stored) != 64 {
		t.Fatal("token stored without hashing")
	}
	w = call("DELETE", s.AdminToken, created.Token, "")
	if w.Code != 204 {
		t.Fatalf("delete: %d", w.Code)
	}
	if ok, err := validator.Valid(t.Context(), created.Token); err != nil || ok {
		t.Fatal("deleted token still accepted", err)
	}
	if w := call("GET", s.AdminToken, created.Token, ""); w.Code != 404 {
		t.Fatalf("missing: %d", w.Code)
	}
	if w := call("DELETE", s.AdminToken, created.Token, ""); w.Code != 404 {
		t.Fatalf("missing delete: %d", w.Code)
	}
	files, _ := filepath.Glob(filepath.Join(logger.dir, "*.jsonl"))
	for _, file := range files {
		data, _ := os.ReadFile(file)
		if strings.Contains(string(data), created.Token) || strings.Contains(string(data), s.AdminToken) {
			t.Fatal("credential leaked in audit log")
		}
	}
}

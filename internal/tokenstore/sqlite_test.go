package tokenstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

//go:embed schema.sql
var schema string

func TestExternalIssuerUpdates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tokens.db")
	issuer, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer issuer.Close()
	if _, err = issuer.Exec(schema); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	check := func(token string, want bool) {
		t.Helper()
		got, err := store.Valid(context.Background(), token)
		if err != nil || got != want {
			t.Fatalf("valid=%v want=%v err=%v", got, want, err)
		}
	}
	check("", false)
	check("issued", false)
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte("issued")))
	if _, err = issuer.Exec(`INSERT INTO tokens(token_hash,user_id) VALUES (?,?)`, hash, "alice"); err != nil {
		t.Fatal(err)
	}
	check("issued", true)
	check("wrong", false)
	for _, tc := range []struct {
		query string
		valid bool
	}{
		{`UPDATE tokens SET enabled=0`, false},
		{`UPDATE tokens SET enabled=1, expires_at=unixepoch()-1`, false},
		{`UPDATE tokens SET expires_at=unixepoch()+3600`, true},
		{`DELETE FROM tokens`, false},
	} {
		if _, err = issuer.Exec(tc.query); err != nil {
			t.Fatal(err)
		}
		check("issued", tc.valid)
	}
	if _, err = store.db.Exec(`DELETE FROM tokens`); err == nil {
		t.Fatal("validator must be read-only")
	}
	issuer.Close()
	store.Close()
	if _, err = store.Valid(context.Background(), "issued"); err == nil {
		t.Fatal("closed database must return error")
	}
}

func TestMissingDatabaseFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.db")
	if s, err := Open(path); err == nil {
		s.Close()
		t.Fatal("accepted missing database")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("created missing database: %v", err)
	}
}

// Package tokenstore validates tokens issued by an external application.
package tokenstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/url"
	"path/filepath"
)

type Store struct{ db *sql.DB }

// Open opens an existing database read-only. Schema and issuance belong to the issuer.
func Open(path string) (*Store, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: absolute}
	q := url.Values{"mode": {"ro"}, "_pragma": {"busy_timeout(3000)"}}
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, fmt.Errorf("SQLite driver unavailable on this platform: %w", err)
	}
	db.SetMaxOpenConns(4)
	// Verify schema at startup; never silently disable authentication.
	rows, err := db.Query(`SELECT token_hash, user_id, enabled, expires_at FROM tokens LIMIT 0`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("open token database: %w", err)
	}
	rows.Close()
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Valid(ctx context.Context, token string) (bool, error) {
	if token == "" {
		return false, nil
	}
	hash := sha256.Sum256([]byte(token))
	var valid bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS (
 SELECT 1 FROM tokens WHERE token_hash = ? AND enabled = 1
 AND (expires_at IS NULL OR expires_at > unixepoch())
 )`, hex.EncodeToString(hash[:])).Scan(&valid)
	return valid, err
}

// UserID identifies known tokens, including expired or disabled tokens, for audit logs.
func (s *Store) UserID(ctx context.Context, token string) (string, error) {
	if token == "" {
		return "", nil
	}
	hash := sha256.Sum256([]byte(token))
	var user string
	err := s.db.QueryRowContext(ctx, `SELECT user_id FROM tokens WHERE token_hash = ?`, hex.EncodeToString(hash[:])).Scan(&user)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return user, err
}

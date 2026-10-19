package tokenstore

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/url"
	"path/filepath"
	"time"
)

var ErrNotFound = errors.New("token not found")

type TokenInfo struct {
	Username  string     `json:"username"`
	ExpiresAt *time.Time `json:"expires_at"`
	Enabled   bool       `json:"enabled"`
}

type Manager struct{ db *sql.DB }

// OpenManager requires an existing schema; only enabled admin APIs get a writer.
func OpenManager(path string) (*Manager, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: absolute}
	u.RawQuery = url.Values{"mode": {"rw"}, "_pragma": {"busy_timeout(3000)"}}.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	rows, err := db.Query(`SELECT token_hash,user_id,expires_at,enabled FROM tokens LIMIT 0`)
	if err != nil {
		db.Close()
		return nil, err
	}
	rows.Close()
	return &Manager{db: db}, nil
}
func (m *Manager) Close() error { return m.db.Close() }
func tokenHash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
func (m *Manager) Create(ctx context.Context, username string, expires time.Time) (string, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(secret)
	_, err := m.db.ExecContext(ctx, `INSERT INTO tokens(token_hash,user_id,expires_at) VALUES (?,?,?)`, tokenHash(token), username, expires.Unix())
	if err != nil {
		return "", err
	}
	return token, nil
}
func (m *Manager) Info(ctx context.Context, token string) (TokenInfo, error) {
	var info TokenInfo
	var expiry sql.NullInt64
	err := m.db.QueryRowContext(ctx, `SELECT user_id,expires_at,enabled FROM tokens WHERE token_hash=?`, tokenHash(token)).Scan(&info.Username, &expiry, &info.Enabled)
	if err == sql.ErrNoRows {
		return info, ErrNotFound
	}
	if expiry.Valid {
		t := time.Unix(expiry.Int64, 0).UTC()
		info.ExpiresAt = &t
	}
	return info, err
}
func (m *Manager) Delete(ctx context.Context, token string) error {
	result, err := m.db.ExecContext(ctx, `DELETE FROM tokens WHERE token_hash=?`, tokenHash(token))
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count == 0 {
		return ErrNotFound
	}
	return err
}

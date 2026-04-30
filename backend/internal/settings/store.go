// Package settings persists key/value runtime configuration that admins can
// edit from the web UI (TG bot token, mode, webhook URL, etc.).
package settings

import (
	"context"
	"database/sql"
	"errors"

	appdb "github.com/ripyc/rip-lgbt-submission-tg/internal/db"
)

// Known setting keys.
const (
	KeyBotToken         = "bot_token"
	KeyBotMode          = "bot_mode"     // "polling" or "webhook"
	KeyBotWebhook       = "bot_webhook_url"
	KeyBotWebhookSecret = "bot_webhook_secret"
	KeyBotUsername      = "bot_username"
	KeySiteName         = "site_name"
)

type Store struct{ DB *appdb.DB }

func NewStore(db *appdb.DB) *Store { return &Store{DB: db} }

// Get returns the value or "" if unset.
func (s *Store) Get(ctx context.Context, key string) (string, error) {
	var v string
	err := s.DB.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// Set upserts a key/value.
func (s *Store) Set(ctx context.Context, key, value string) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO settings(key, value) VALUES(?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// All returns the full settings map.
func (s *Store) All(ctx context.Context) (map[string]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// SetMany applies a batch of changes within a single transaction.
func (s *Store) SetMany(ctx context.Context, kv map[string]string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for k, v := range kv {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO settings(key, value) VALUES(?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value`, k, v); err != nil {
			return err
		}
	}
	return tx.Commit()
}

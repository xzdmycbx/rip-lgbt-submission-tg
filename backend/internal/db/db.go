// Package db owns the SQLite connection, migrations, and a few small helpers
// shared by the rest of the app. Other packages should depend on *DB rather
// than reaching for *sql.DB directly.
package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// DB wraps *sql.DB with helpers and migration management.
type DB struct {
	*sql.DB
	path string
}

// Open opens (or creates) a SQLite database at path, configures pragmas, and
// applies any pending migrations.
func Open(ctx context.Context, path string) (*DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)", path)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	conn.SetMaxOpenConns(1) // SQLite is happiest with a serialized writer; reads are still fine.
	conn.SetMaxIdleConns(1)
	conn.SetConnMaxLifetime(0)

	if err := conn.PingContext(ctx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	d := &DB{DB: conn, path: path}
	if err := d.migrate(ctx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return d, nil
}

// Now returns the canonical timestamp format used in the DB (RFC3339 in UTC).
func Now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func (d *DB) migrate(ctx context.Context) error {
	if _, err := d.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	files, err := listMigrations()
	if err != nil {
		return err
	}
	for _, f := range files {
		var existing string
		err := d.QueryRowContext(ctx, `SELECT version FROM schema_migrations WHERE version = ?`, f.name).Scan(&existing)
		if err == nil {
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("check migration %s: %w", f.name, err)
		}
		if err := d.applyMigration(ctx, f); err != nil {
			return err
		}
	}
	return nil
}

type migrationFile struct {
	name string
	body string
}

func listMigrations() ([]migrationFile, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}
	out := make([]migrationFile, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		body, err := fs.ReadFile(migrationsFS, "migrations/"+e.Name())
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), err)
		}
		out = append(out, migrationFile{name: e.Name(), body: string(body)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out, nil
}

func (d *DB) applyMigration(ctx context.Context, f migrationFile) error {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx for %s: %w", f.name, err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, f.body); err != nil {
		return fmt.Errorf("apply %s: %w", f.name, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?)`, f.name, Now()); err != nil {
		return fmt.Errorf("record %s: %w", f.name, err)
	}
	return tx.Commit()
}

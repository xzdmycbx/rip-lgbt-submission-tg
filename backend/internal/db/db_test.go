package db

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpenAndMigrateFreshDB(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	d, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	rows, err := d.Query(`SELECT name FROM sqlite_master WHERE type='table' ORDER BY name`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	defer rows.Close()

	want := map[string]bool{
		"admins": false, "admin_passkeys": false, "admin_sessions": false, "admin_login_links": false,
		"settings": false, "memorials": false, "memorial_assets": false,
		"flowers": false, "flower_events": false, "comments": false,
		"drafts": false, "draft_assets": false, "draft_messages": false,
		"submission_events": false, "schema_migrations": false,
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if _, ok := want[name]; ok {
			want[name] = true
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("expected table %q to be created", name)
		}
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	d, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	d.Close()

	d2, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open second time: %v", err)
	}
	defer d2.Close()

	var count int
	if err := d2.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	// We currently ship 2 migrations; bump this when adding more. The
	// invariant under test is that running Open twice does not duplicate
	// any of them.
	if count != 2 {
		t.Fatalf("expected 2 migrations recorded, got %d", count)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(context.Background(), filepath.Join(dir, "settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	if _, err := d.Exec(`INSERT INTO settings(key, value) VALUES (?, ?)`, "bot_token", "abc"); err != nil {
		t.Fatalf("insert setting: %v", err)
	}
	var v string
	if err := d.QueryRow(`SELECT value FROM settings WHERE key = ?`, "bot_token").Scan(&v); err != nil {
		t.Fatalf("read setting: %v", err)
	}
	if v != "abc" {
		t.Fatalf("expected abc got %q", v)
	}
}

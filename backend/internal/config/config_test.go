package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestEnsureSecretsCreatesFreshFile(t *testing.T) {
	dir := t.TempDir()
	s, err := ensureSecrets(dir, "https://example.com")
	if err != nil {
		t.Fatalf("ensureSecrets: %v", err)
	}
	if s.JWTSecret == "" || s.CSRFSecret == "" || s.IPHashPepper == "" || s.PreviewSignKey == "" {
		t.Errorf("expected all secrets to be populated, got %+v", s)
	}
	if s.WebAuthnRPID != "example.com" {
		t.Errorf("expected RPID to be example.com, got %q", s.WebAuthnRPID)
	}
	if s.SessionCookieName == "" {
		t.Error("expected default session cookie name")
	}
	info, err := os.Stat(filepath.Join(dir, "secrets.json"))
	if err != nil {
		t.Fatalf("stat secrets.json: %v", err)
	}
	// Windows / NTFS does not honor Unix mode bits, so only assert on Unix-likes.
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("expected 0600, got %v", info.Mode().Perm())
	}
}

func TestEnsureSecretsIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	first, err := ensureSecrets(dir, "https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	second, err := ensureSecrets(dir, "https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	if first.JWTSecret != second.JWTSecret {
		t.Error("JWTSecret rotated on reload; expected stable value")
	}
	if first.IPHashPepper != second.IPHashPepper {
		t.Error("IPHashPepper rotated on reload")
	}
}

func TestEnsureSecretsFillsMissingFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secrets.json")
	// Pre-write a partial secrets.json (missing PreviewSignKey).
	if err := os.WriteFile(path, []byte(`{"jwt_secret":"abc","ip_hash_pepper":"def"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := ensureSecrets(dir, "https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	if s.JWTSecret != "abc" {
		t.Errorf("expected existing jwt_secret preserved, got %q", s.JWTSecret)
	}
	if s.PreviewSignKey == "" {
		t.Error("expected PreviewSignKey to be filled in")
	}
}

func TestDeriveRPID(t *testing.T) {
	cases := map[string]string{
		"https://rip.lgbt":          "rip.lgbt",
		"http://localhost:8080":     "localhost",
		"https://example.com:443/x": "example.com",
		"":                          "localhost",
	}
	for in, want := range cases {
		got := deriveRPID(in)
		if got != want {
			t.Errorf("deriveRPID(%q) = %q, want %q", in, got, want)
		}
	}
}

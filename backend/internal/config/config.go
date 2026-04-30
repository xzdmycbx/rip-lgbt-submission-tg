package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config holds runtime configuration sourced from environment variables and
// the secrets file generated on first launch.
type Config struct {
	SuperadminUsername string
	SuperadminPassword string
	SiteURL            string
	ListenAddr         string
	DataDir            string

	Secrets Secrets
}

// Secrets holds values generated on first launch and persisted under DataDir.
type Secrets struct {
	JWTSecret           string `json:"jwt_secret"`
	CSRFSecret          string `json:"csrf_secret"`
	IPHashPepper        string `json:"ip_hash_pepper"`
	PreviewSignKey      string `json:"preview_sign_key"`
	WebAuthnRPID        string `json:"webauthn_rp_id"`
	SessionCookieName   string `json:"session_cookie_name"`
}

// Load reads environment variables and ensures the secrets file exists.
func Load() (*Config, error) {
	cfg := &Config{
		SuperadminUsername: getenv("SUPERADMIN_USERNAME", "admin"),
		SiteURL:            strings.TrimRight(getenv("SITE_URL", "http://localhost:8080"), "/"),
		ListenAddr:         getenv("LISTEN_ADDR", ":8080"),
		DataDir:            getenv("DATA_DIR", "./data"),
	}

	password, err := loadSuperadminPassword()
	if err != nil {
		return nil, err
	}
	cfg.SuperadminPassword = password

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("ensure data dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(cfg.DataDir, "uploads"), 0o755); err != nil {
		return nil, fmt.Errorf("ensure uploads dir: %w", err)
	}

	secrets, err := ensureSecrets(cfg.DataDir, cfg.SiteURL)
	if err != nil {
		return nil, err
	}
	cfg.Secrets = secrets

	return cfg, nil
}

func loadSuperadminPassword() (string, error) {
	if path := os.Getenv("SUPERADMIN_PASSWORD_FILE"); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read SUPERADMIN_PASSWORD_FILE: %w", err)
		}
		return strings.TrimSpace(string(data)), nil
	}
	pw := strings.TrimSpace(os.Getenv("SUPERADMIN_PASSWORD"))
	if pw == "" {
		return "", errors.New("SUPERADMIN_PASSWORD or SUPERADMIN_PASSWORD_FILE must be set")
	}
	return pw, nil
}

func ensureSecrets(dataDir, siteURL string) (Secrets, error) {
	path := filepath.Join(dataDir, "secrets.json")
	if data, err := os.ReadFile(path); err == nil {
		var s Secrets
		if err := json.Unmarshal(data, &s); err != nil {
			return Secrets{}, fmt.Errorf("decode secrets: %w", err)
		}
		// fill missing fields without rotating existing ones
		changed := false
		if s.JWTSecret == "" {
			s.JWTSecret = randomHex(32)
			changed = true
		}
		if s.CSRFSecret == "" {
			s.CSRFSecret = randomHex(32)
			changed = true
		}
		if s.IPHashPepper == "" {
			s.IPHashPepper = randomHex(32)
			changed = true
		}
		if s.PreviewSignKey == "" {
			s.PreviewSignKey = randomHex(32)
			changed = true
		}
		if s.WebAuthnRPID == "" {
			s.WebAuthnRPID = deriveRPID(siteURL)
			changed = true
		}
		if s.SessionCookieName == "" {
			s.SessionCookieName = "rip_session"
			changed = true
		}
		if changed {
			if err := writeSecrets(path, s); err != nil {
				return Secrets{}, err
			}
		}
		return s, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return Secrets{}, fmt.Errorf("read secrets: %w", err)
	}

	s := Secrets{
		JWTSecret:         randomHex(32),
		CSRFSecret:        randomHex(32),
		IPHashPepper:      randomHex(32),
		PreviewSignKey:    randomHex(32),
		WebAuthnRPID:      deriveRPID(siteURL),
		SessionCookieName: "rip_session",
	}
	if err := writeSecrets(path, s); err != nil {
		return Secrets{}, err
	}
	return s, nil
}

func writeSecrets(path string, s Secrets) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write secrets: %w", err)
	}
	return nil
}

func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buf)
}

func deriveRPID(siteURL string) string {
	host := siteURL
	for _, prefix := range []string{"https://", "http://"} {
		host = strings.TrimPrefix(host, prefix)
	}
	if i := strings.Index(host, "/"); i >= 0 {
		host = host[:i]
	}
	if i := strings.Index(host, ":"); i >= 0 {
		host = host[:i]
	}
	if host == "" {
		host = "localhost"
	}
	return host
}

func getenv(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

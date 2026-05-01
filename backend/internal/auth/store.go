package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	appdb "github.com/ripyc/rip-lgbt-submission-tg/internal/db"
)

// Admin is the canonical representation of an administrator account.
type Admin struct {
	ID            int64
	Username      string // may be empty for TG-only admins
	PasswordHash  string // argon2id; may be empty
	TelegramID    int64  // 0 means unset
	DisplayName   string
	IsSuper       bool
	TOTPSecret    string
	TOTPConfirmed bool
	MustSetup2FA  bool
	HasPasskey    bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Session represents an active admin web session.
type Session struct {
	ID        string
	AdminID   int64
	CreatedAt time.Time
	ExpiresAt time.Time
	UA        string
	IP        string
}

// Store wraps DB queries used by the auth subsystem.
type Store struct{ DB *appdb.DB }

func NewStore(db *appdb.DB) *Store { return &Store{DB: db} }

// EnsureSuperadmin creates or updates the configured superadmin so that the
// password from the environment is always honored. The flag must_setup_2fa is
// preserved if already false, otherwise initialized to true.
func (s *Store) EnsureSuperadmin(ctx context.Context, username, plaintextPassword string) (*Admin, error) {
	if username == "" || plaintextPassword == "" {
		return nil, errors.New("superadmin username/password required")
	}
	hash, err := HashPassword(plaintextPassword)
	if err != nil {
		return nil, err
	}

	now := appdb.Now()
	existing, err := s.GetAdminByUsername(ctx, username)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if existing == nil {
		res, err := s.DB.ExecContext(ctx, `
			INSERT INTO admins(username, password_hash, display_name, is_super, totp_confirmed, must_setup_2fa, created_at, updated_at)
			VALUES(?, ?, ?, 1, 0, 1, ?, ?)`,
			username, hash, username, now, now,
		)
		if err != nil {
			return nil, fmt.Errorf("seed superadmin: %w", err)
		}
		id, _ := res.LastInsertId()
		return s.GetAdminByID(ctx, id)
	}

	if _, err := s.DB.ExecContext(ctx, `
		UPDATE admins SET password_hash = ?, is_super = 1, updated_at = ? WHERE id = ?`,
		hash, now, existing.ID); err != nil {
		return nil, fmt.Errorf("update superadmin password: %w", err)
	}
	return s.GetAdminByID(ctx, existing.ID)
}

// GetAdminByID returns the admin or sql.ErrNoRows.
func (s *Store) GetAdminByID(ctx context.Context, id int64) (*Admin, error) {
	return s.scanOneAdmin(ctx, `WHERE a.id = ?`, id)
}

// GetAdminByUsername returns the admin or sql.ErrNoRows.
func (s *Store) GetAdminByUsername(ctx context.Context, username string) (*Admin, error) {
	if username == "" {
		return nil, sql.ErrNoRows
	}
	return s.scanOneAdmin(ctx, `WHERE a.username = ?`, username)
}

// GetAdminByTelegramID returns the admin or sql.ErrNoRows.
func (s *Store) GetAdminByTelegramID(ctx context.Context, tgID int64) (*Admin, error) {
	if tgID == 0 {
		return nil, sql.ErrNoRows
	}
	return s.scanOneAdmin(ctx, `WHERE a.telegram_id = ?`, tgID)
}

// ListAdmins returns all admins ordered by ID.
func (s *Store) ListAdmins(ctx context.Context) ([]*Admin, error) {
	rows, err := s.DB.QueryContext(ctx, adminSelect+` ORDER BY a.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Admin
	for rows.Next() {
		a, err := scanAdmin(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// CreateAdmin inserts a new admin record. Either Username or TelegramID must be set.
func (s *Store) CreateAdmin(ctx context.Context, a *Admin) (int64, error) {
	if a.Username == "" && a.TelegramID == 0 {
		return 0, errors.New("either username or telegram_id required")
	}
	now := appdb.Now()
	must2FA := 1
	if a.PasswordHash == "" {
		// TG-only admin can be added without password; 2FA setup happens later.
		must2FA = 0
	}
	res, err := s.DB.ExecContext(ctx, `
		INSERT INTO admins(username, password_hash, telegram_id, display_name, is_super, totp_confirmed, must_setup_2fa, created_at, updated_at)
		VALUES(NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, 0), ?, ?, 0, ?, ?, ?)`,
		a.Username, a.PasswordHash, a.TelegramID, a.DisplayName, boolToInt(a.IsSuper), must2FA, now, now,
	)
	if err != nil {
		return 0, fmt.Errorf("insert admin: %w", err)
	}
	id, _ := res.LastInsertId()
	return id, nil
}

// UpdateAdminPassword sets a new argon2id hash and clears must_setup_2fa
// when it was set only because the admin had no password.
func (s *Store) UpdateAdminPassword(ctx context.Context, adminID int64, hash string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE admins SET password_hash = ?, updated_at = ? WHERE id = ?`,
		hash, appdb.Now(), adminID)
	return err
}

// UpdateAdminProfile mutates the editable profile fields for an admin.
// Empty pointers leave the column untouched. The caller is responsible
// for enforcing field-level rules (e.g. "username only when empty").
func (s *Store) UpdateAdminProfile(ctx context.Context, adminID int64, displayName, username *string, telegramID *int64) error {
	cols := []string{}
	args := []any{}
	if displayName != nil {
		cols = append(cols, "display_name = ?")
		args = append(args, strings.TrimSpace(*displayName))
	}
	if username != nil {
		v := SanitizeUsername(*username)
		if v == "" {
			cols = append(cols, "username = NULL")
		} else {
			cols = append(cols, "username = ?")
			args = append(args, v)
		}
	}
	if telegramID != nil {
		if *telegramID == 0 {
			cols = append(cols, "telegram_id = NULL")
		} else {
			cols = append(cols, "telegram_id = ?")
			args = append(args, *telegramID)
		}
	}
	if len(cols) == 0 {
		return nil
	}
	cols = append(cols, "updated_at = ?")
	args = append(args, appdb.Now(), adminID)
	q := "UPDATE admins SET " + strings.Join(cols, ", ") + " WHERE id = ?"
	_, err := s.DB.ExecContext(ctx, q, args...)
	if err != nil {
		// SQLite uniqueness violations come back as "constraint failed".
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			if username != nil && strings.Contains(err.Error(), "username") {
				return errors.New("username_taken")
			}
			if telegramID != nil && strings.Contains(err.Error(), "telegram_id") {
				return errors.New("telegram_id_taken")
			}
			return errors.New("conflict")
		}
		return err
	}
	return nil
}

// UpdateAdminTOTP saves an unconfirmed TOTP secret.
func (s *Store) UpdateAdminTOTP(ctx context.Context, adminID int64, secret string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE admins SET totp_secret = ?, totp_confirmed = 0, updated_at = ? WHERE id = ?`,
		secret, appdb.Now(), adminID)
	return err
}

// ConfirmAdminTOTP marks the secret as verified.
func (s *Store) ConfirmAdminTOTP(ctx context.Context, adminID int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE admins SET totp_confirmed = 1, must_setup_2fa = 0, updated_at = ? WHERE id = ?`,
		appdb.Now(), adminID)
	return err
}

// DisableAdminTOTP clears the totp secret + confirmed flag.
func (s *Store) DisableAdminTOTP(ctx context.Context, adminID int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE admins SET totp_secret = NULL, totp_confirmed = 0, updated_at = ? WHERE id = ?`,
		appdb.Now(), adminID)
	return err
}

// ClearMustSetup2FA is used after a successful 2FA bind.
func (s *Store) ClearMustSetup2FA(ctx context.Context, adminID int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE admins SET must_setup_2fa = 0, updated_at = ? WHERE id = ?`,
		appdb.Now(), adminID)
	return err
}

// ListAdminPasskeys returns the credentials registered for an admin.
type AdminPasskey struct {
	ID         int64
	Transports string
	CreatedAt  string
}

func (s *Store) ListAdminPasskeys(ctx context.Context, adminID int64) ([]AdminPasskey, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, transports, created_at FROM admin_passkeys WHERE admin_id = ? ORDER BY id`, adminID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AdminPasskey
	for rows.Next() {
		var p AdminPasskey
		if err := rows.Scan(&p.ID, &p.Transports, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeleteAdminPasskey removes a credential row for an admin (by passkey id).
func (s *Store) DeleteAdminPasskey(ctx context.Context, adminID, passkeyID int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM admin_passkeys WHERE id = ? AND admin_id = ?`, passkeyID, adminID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("passkey not found for this admin")
	}
	return nil
}

// DeleteAdmin removes an admin and (cascade) their passkeys, sessions, login links.
func (s *Store) DeleteAdmin(ctx context.Context, adminID int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM admins WHERE id = ?`, adminID)
	return err
}

// CreateSession persists a new session and returns it. The cookie value
// returned is opaque (random) and stored in the DB as the primary key.
func (s *Store) CreateSession(ctx context.Context, adminID int64, ttl time.Duration, ua, ip string) (*Session, error) {
	id, err := randomToken(32)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	expires := now.Add(ttl)
	if _, err := s.DB.ExecContext(ctx, `
		INSERT INTO admin_sessions(id, admin_id, created_at, expires_at, ua, ip)
		VALUES(?, ?, ?, ?, ?, ?)`,
		id, adminID, now.Format(time.RFC3339Nano), expires.Format(time.RFC3339Nano), ua, ip,
	); err != nil {
		return nil, err
	}
	return &Session{ID: id, AdminID: adminID, CreatedAt: now, ExpiresAt: expires, UA: ua, IP: ip}, nil
}

// GetSession returns the session if it exists and has not expired.
func (s *Store) GetSession(ctx context.Context, id string) (*Session, error) {
	if id == "" {
		return nil, sql.ErrNoRows
	}
	var sess Session
	var created, expires string
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, admin_id, created_at, expires_at, ua, ip FROM admin_sessions WHERE id = ?`, id).
		Scan(&sess.ID, &sess.AdminID, &created, &expires, &sess.UA, &sess.IP)
	if err != nil {
		return nil, err
	}
	sess.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	sess.ExpiresAt, _ = time.Parse(time.RFC3339Nano, expires)
	if time.Now().UTC().After(sess.ExpiresAt) {
		_, _ = s.DB.ExecContext(ctx, `DELETE FROM admin_sessions WHERE id = ?`, id)
		return nil, sql.ErrNoRows
	}
	return &sess, nil
}

// DeleteSession invalidates a session.
func (s *Store) DeleteSession(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM admin_sessions WHERE id = ?`, id)
	return err
}

// CleanupSessions removes expired sessions and login links.
func (s *Store) CleanupSessions(ctx context.Context) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM admin_sessions WHERE expires_at < ?`, now); err != nil {
		return err
	}
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM admin_login_links WHERE expires_at < ?`, now); err != nil {
		return err
	}
	return nil
}

// CreateLoginLink generates a single-use TG login link.
func (s *Store) CreateLoginLink(ctx context.Context, adminID int64, ttl time.Duration) (string, error) {
	tok, err := randomToken(24)
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	if _, err := s.DB.ExecContext(ctx, `
		INSERT INTO admin_login_links(token, admin_id, created_at, expires_at)
		VALUES(?, ?, ?, ?)`,
		tok, adminID, now.Format(time.RFC3339Nano), now.Add(ttl).Format(time.RFC3339Nano),
	); err != nil {
		return "", err
	}
	return tok, nil
}

// ConsumeLoginLink atomically marks a token as used and returns its admin_id
// if it was valid (unused, unexpired).
func (s *Store) ConsumeLoginLink(ctx context.Context, token string) (int64, error) {
	if token == "" {
		return 0, sql.ErrNoRows
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.DB.ExecContext(ctx, `
		UPDATE admin_login_links SET used_at = ?
		WHERE token = ? AND used_at IS NULL AND expires_at > ?`,
		now, token, now,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return 0, sql.ErrNoRows
	}
	var adminID int64
	if err := s.DB.QueryRowContext(ctx, `SELECT admin_id FROM admin_login_links WHERE token = ?`, token).Scan(&adminID); err != nil {
		return 0, err
	}
	return adminID, nil
}

const adminSelect = `
SELECT a.id, COALESCE(a.username,''), COALESCE(a.password_hash,''), COALESCE(a.telegram_id, 0),
       a.display_name, a.is_super, COALESCE(a.totp_secret,''), a.totp_confirmed, a.must_setup_2fa,
       (SELECT COUNT(*) FROM admin_passkeys p WHERE p.admin_id = a.id) AS passkeys,
       a.created_at, a.updated_at
FROM admins a
`

func (s *Store) scanOneAdmin(ctx context.Context, where string, args ...any) (*Admin, error) {
	row := s.DB.QueryRowContext(ctx, adminSelect+" "+where, args...)
	return scanAdmin(row)
}

type scanner interface {
	Scan(dest ...any) error
}

func scanAdmin(r scanner) (*Admin, error) {
	var a Admin
	var created, updated string
	var passkeys int
	if err := r.Scan(
		&a.ID, &a.Username, &a.PasswordHash, &a.TelegramID,
		&a.DisplayName, &a.IsSuper, &a.TOTPSecret, &a.TOTPConfirmed, &a.MustSetup2FA,
		&passkeys, &created, &updated,
	); err != nil {
		return nil, err
	}
	a.HasPasskey = passkeys > 0
	a.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	a.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return &a, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func randomToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// HexFromBase64URL converts a base64url token back to a hex string for logging,
// without leaking the underlying token value (truncated).
func HexFromBase64URL(token string) string {
	if len(token) < 8 {
		return "..."
	}
	raw, err := base64.RawURLEncoding.DecodeString(token[:8])
	if err != nil {
		return "..."
	}
	return hex.EncodeToString(raw) + "..."
}

// SanitizeUsername normalizes admin usernames before storage.
func SanitizeUsername(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	appdb "github.com/ripyc/rip-lgbt-submission-tg/internal/db"
)

// PasskeyManager wraps go-webauthn for our single-tenant relying party.
type PasskeyManager struct {
	web *webauthn.WebAuthn
	db  *appdb.DB
}

// NewPasskeyManager configures the WebAuthn relying party using the given
// identifier (typically the site's hostname) and origin URL.
func NewPasskeyManager(rpID, rpDisplayName, origin string, db *appdb.DB) (*PasskeyManager, error) {
	cfg := &webauthn.Config{
		RPID:          rpID,
		RPDisplayName: rpDisplayName,
		RPOrigins:     []string{origin},
	}
	w, err := webauthn.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("init webauthn: %w", err)
	}
	return &PasskeyManager{web: w, db: db}, nil
}

// passkeyUser adapts an Admin into webauthn.User.
type passkeyUser struct {
	id          []byte
	name        string
	displayName string
	creds       []webauthn.Credential
}

func (u *passkeyUser) WebAuthnID() []byte                         { return u.id }
func (u *passkeyUser) WebAuthnName() string                       { return u.name }
func (u *passkeyUser) WebAuthnDisplayName() string                { return u.displayName }
func (u *passkeyUser) WebAuthnIcon() string                       { return "" }
func (u *passkeyUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

func (m *PasskeyManager) loadUser(ctx context.Context, admin *Admin) (*passkeyUser, error) {
	rows, err := m.db.QueryContext(ctx, `
		SELECT credential_id, public_key, sign_count, transports, attestation
		FROM admin_passkeys WHERE admin_id = ? ORDER BY id`, admin.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	user := &passkeyUser{
		id:          adminUserHandle(admin.ID),
		name:        userLabel(admin),
		displayName: adminDisplay(admin),
	}
	for rows.Next() {
		var cred webauthn.Credential
		var transports, attestation string
		if err := rows.Scan(&cred.ID, &cred.PublicKey, &cred.Authenticator.SignCount, &transports, &attestation); err != nil {
			return nil, err
		}
		if transports != "" {
			_ = json.Unmarshal([]byte(transports), &cred.Transport)
		}
		user.creds = append(user.creds, cred)
	}
	return user, rows.Err()
}

// BeginRegistration starts a new passkey registration ceremony for an admin.
// The returned creation options should be sent to the browser; sessionData
// must be persisted server-side for FinishRegistration.
func (m *PasskeyManager) BeginRegistration(ctx context.Context, admin *Admin) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
	user, err := m.loadUser(ctx, admin)
	if err != nil {
		return nil, nil, err
	}
	options, sessionData, err := m.web.BeginRegistration(user)
	if err != nil {
		return nil, nil, err
	}
	return options, sessionData, nil
}

// FinishRegistration validates the response and persists the new credential.
func (m *PasskeyManager) FinishRegistration(ctx context.Context, admin *Admin, sessionData *webauthn.SessionData, response *protocol.ParsedCredentialCreationData) error {
	user, err := m.loadUser(ctx, admin)
	if err != nil {
		return err
	}
	cred, err := m.web.CreateCredential(user, *sessionData, response)
	if err != nil {
		return fmt.Errorf("create credential: %w", err)
	}
	transports, _ := json.Marshal(cred.Transport)
	if _, err := m.db.ExecContext(ctx, `
		INSERT INTO admin_passkeys(admin_id, credential_id, public_key, sign_count, transports, attestation, user_handle, created_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
		admin.ID, cred.ID, cred.PublicKey, cred.Authenticator.SignCount,
		string(transports), string(cred.AttestationType), user.id, appdb.Now(),
	); err != nil {
		return fmt.Errorf("save passkey: %w", err)
	}
	return nil
}

// BeginLogin starts a passkey assertion ceremony.
func (m *PasskeyManager) BeginLogin(ctx context.Context, admin *Admin) (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
	user, err := m.loadUser(ctx, admin)
	if err != nil {
		return nil, nil, err
	}
	if len(user.creds) == 0 {
		return nil, nil, errors.New("no passkeys registered")
	}
	return m.web.BeginLogin(user)
}

// FinishLogin verifies the assertion and updates the sign counter.
func (m *PasskeyManager) FinishLogin(ctx context.Context, admin *Admin, sessionData *webauthn.SessionData, response *protocol.ParsedCredentialAssertionData) error {
	user, err := m.loadUser(ctx, admin)
	if err != nil {
		return err
	}
	cred, err := m.web.ValidateLogin(user, *sessionData, response)
	if err != nil {
		return err
	}
	if _, err := m.db.ExecContext(ctx, `UPDATE admin_passkeys SET sign_count = ? WHERE credential_id = ?`,
		cred.Authenticator.SignCount, cred.ID); err != nil {
		return err
	}
	return nil
}

// HasPasskey returns whether the admin has at least one credential.
func (m *PasskeyManager) HasPasskey(ctx context.Context, adminID int64) (bool, error) {
	var n int
	if err := m.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM admin_passkeys WHERE admin_id = ?`, adminID).Scan(&n); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return n > 0, nil
}

// BeginDiscoverableLogin starts a passwordless / "just tap your passkey"
// ceremony. The browser uses platform-stored credentials; the user does
// not need to type a username first.
func (m *PasskeyManager) BeginDiscoverableLogin() (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
	return m.web.BeginDiscoverableLogin()
}

// FinishDiscoverableLogin validates an assertion when the admin is identified
// only by the credential / userHandle returned by the authenticator. Returns
// the matched admin or an error.
func (m *PasskeyManager) FinishDiscoverableLogin(ctx context.Context, sessionData *webauthn.SessionData, response *protocol.ParsedCredentialAssertionData) (*Admin, error) {
	var resolvedAdmin *Admin
	handler := func(rawID, userHandle []byte) (webauthn.User, error) {
		admin, err := m.LookupAdminByPasskey(ctx, rawID, userHandle)
		if err != nil {
			return nil, err
		}
		user, err := m.loadUser(ctx, admin)
		if err != nil {
			return nil, err
		}
		resolvedAdmin = admin
		return user, nil
	}
	cred, err := m.web.ValidateDiscoverableLogin(handler, *sessionData, response)
	if err != nil {
		return nil, err
	}
	if resolvedAdmin == nil {
		return nil, errors.New("could not identify admin from passkey")
	}
	if _, err := m.db.ExecContext(ctx, `UPDATE admin_passkeys SET sign_count = ? WHERE credential_id = ?`,
		cred.Authenticator.SignCount, cred.ID); err != nil {
		return nil, err
	}
	return resolvedAdmin, nil
}

// LookupAdminByPasskey resolves the admin owning a passkey credential. Used by
// FinishDiscoverableLogin and exposed for tests.
func (m *PasskeyManager) LookupAdminByPasskey(ctx context.Context, rawID, userHandle []byte) (*Admin, error) {
	var adminID int64
	err := m.db.QueryRowContext(ctx, `SELECT admin_id FROM admin_passkeys WHERE user_handle = ? LIMIT 1`, userHandle).Scan(&adminID)
	if errors.Is(err, sql.ErrNoRows) {
		err = m.db.QueryRowContext(ctx, `SELECT admin_id FROM admin_passkeys WHERE credential_id = ? LIMIT 1`, rawID).Scan(&adminID)
	}
	if err != nil {
		return nil, fmt.Errorf("lookup credential: %w", err)
	}
	store := &Store{DB: m.db}
	return store.GetAdminByID(ctx, adminID)
}

func userLabel(a *Admin) string {
	if a.Username != "" {
		return a.Username
	}
	if a.TelegramID != 0 {
		return fmt.Sprintf("tg:%d", a.TelegramID)
	}
	return fmt.Sprintf("admin-%d", a.ID)
}

func adminDisplay(a *Admin) string {
	if a.DisplayName != "" {
		return a.DisplayName
	}
	return userLabel(a)
}

// adminUserHandle returns a stable 16-byte handle for the admin so authenticators
// can correlate registration and assertion ceremonies.
func adminUserHandle(adminID int64) []byte {
	out := make([]byte, 16)
	for i := 0; i < 8; i++ {
		out[i] = byte(adminID >> (8 * i))
	}
	out[15] = 0x42 // small marker so handles are obviously synthetic
	_ = time.Now
	return out
}

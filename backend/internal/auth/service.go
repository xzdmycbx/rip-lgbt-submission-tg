package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-webauthn/webauthn/protocol"
)

// Service ties the storage layer, TOTP, and WebAuthn together and exposes
// HTTP handlers for the admin auth flow.
type Service struct {
	Store          *Store
	Passkeys       *PasskeyManager
	pending        *webauthnPending
	cookieName     string
	cookieSecure   bool
	siteURL        string
	siteIssuer     string
	loginLinkBase  string
	loginLinkTTL   time.Duration
	sessionTTL     time.Duration
}

// ServiceConfig configures auth flow parameters.
type ServiceConfig struct {
	CookieName    string
	CookieSecure  bool
	SiteURL       string
	SiteIssuer    string // shown in TOTP apps
	LoginLinkTTL  time.Duration
	SessionTTL    time.Duration
}

// NewService wires the auth service.
func NewService(store *Store, passkeys *PasskeyManager, cfg ServiceConfig) *Service {
	if cfg.LoginLinkTTL == 0 {
		cfg.LoginLinkTTL = 10 * time.Minute
	}
	if cfg.SessionTTL == 0 {
		cfg.SessionTTL = CookieMaxAge
	}
	if cfg.CookieName == "" {
		cfg.CookieName = "rip_session"
	}
	if cfg.SiteIssuer == "" {
		cfg.SiteIssuer = "rip.lgbt"
	}
	return &Service{
		Store:         store,
		Passkeys:      passkeys,
		pending:       newWebauthnPending(),
		cookieName:    cfg.CookieName,
		cookieSecure:  cfg.CookieSecure,
		siteURL:       strings.TrimRight(cfg.SiteURL, "/"),
		siteIssuer:    cfg.SiteIssuer,
		loginLinkBase: strings.TrimRight(cfg.SiteURL, "/") + "/admin/login",
		loginLinkTTL:  cfg.LoginLinkTTL,
		sessionTTL:    cfg.SessionTTL,
	}
}

// Routes returns the auth-related sub-router mounted at /api/auth.
func (s *Service) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/login", s.handleLogin)
	r.Post("/login/tg", s.handleTGLogin)
	r.Post("/logout", s.handleLogout)
	r.Get("/me", s.handleMe)

	r.Route("/2fa", func(r chi.Router) {
		r.Use(RequireLogin)
		r.Post("/totp/begin", s.handleTOTPBegin)
		r.Post("/totp/confirm", s.handleTOTPConfirm)
		r.Post("/passkey/register/begin", s.handlePasskeyRegisterBegin)
		r.Post("/passkey/register/finish", s.handlePasskeyRegisterFinish)
	})

	r.Post("/passkey/login/begin", s.handlePasskeyLoginBegin)
	r.Post("/passkey/login/finish", s.handlePasskeyLoginFinish)
	r.Post("/passkey/login/discoverable/begin", s.handlePasskeyDiscoverableBegin)
	r.Post("/passkey/login/discoverable/finish", s.handlePasskeyDiscoverableFinish)
	return r
}

// CookieName returns the session cookie name (used by the http app to wire middleware).
func (s *Service) CookieName() string { return s.cookieName }

func (s *Service) issueSession(ctx context.Context, w http.ResponseWriter, r *http.Request, adminID int64) error {
	sess, err := s.Store.CreateSession(ctx, adminID, s.sessionTTL, r.UserAgent(), r.RemoteAddr)
	if err != nil {
		return err
	}
	IssueCookie(w, s.cookieName, sess.ID, s.cookieSecure, s.sessionTTL)
	return nil
}

// --- Login ---

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	TOTP     string `json:"totp,omitempty"`
}

type loginResponse struct {
	OK            bool         `json:"ok"`
	Admin         *adminDTO    `json:"admin,omitempty"`
	Need          *needHints   `json:"need,omitempty"`
	Error         string       `json:"error,omitempty"`
}

type needHints struct {
	TOTP        bool `json:"totp"`
	PasskeySetup bool `json:"passkey_setup"`
	TOTPSetup    bool `json:"totp_setup"`
	MustSetup2FA bool `json:"must_setup_2fa"`
}

type adminDTO struct {
	ID            int64  `json:"id"`
	Username      string `json:"username,omitempty"`
	TelegramID    int64  `json:"telegram_id,omitempty"`
	DisplayName   string `json:"display_name"`
	IsSuper       bool   `json:"is_super"`
	HasPasskey    bool   `json:"has_passkey"`
	TOTPConfirmed bool   `json:"totp_confirmed"`
	MustSetup2FA  bool   `json:"must_setup_2fa"`
}

func toAdminDTO(a *Admin) *adminDTO {
	return &adminDTO{
		ID: a.ID, Username: a.Username, TelegramID: a.TelegramID,
		DisplayName: a.DisplayName, IsSuper: a.IsSuper,
		HasPasskey: a.HasPasskey, TOTPConfirmed: a.TOTPConfirmed, MustSetup2FA: a.MustSetup2FA,
	}
}

func (s *Service) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, loginResponse{Error: "bad_request"})
		return
	}
	username := SanitizeUsername(req.Username)
	if username == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, loginResponse{Error: "missing_credentials"})
		return
	}
	admin, err := s.Store.GetAdminByUsername(r.Context(), username)
	if errors.Is(err, sql.ErrNoRows) || admin.PasswordHash == "" {
		writeJSON(w, http.StatusUnauthorized, loginResponse{Error: "invalid_credentials"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, loginResponse{Error: "server_error"})
		return
	}
	if err := VerifyPassword(admin.PasswordHash, req.Password); err != nil {
		writeJSON(w, http.StatusUnauthorized, loginResponse{Error: "invalid_credentials"})
		return
	}

	// If the admin already has a TOTP confirmed, we require a code on this request.
	if admin.TOTPConfirmed {
		if req.TOTP == "" {
			writeJSON(w, http.StatusOK, loginResponse{Need: &needHints{TOTP: true}})
			return
		}
		if err := VerifyTOTP(admin.TOTPSecret, req.TOTP); err != nil {
			writeJSON(w, http.StatusUnauthorized, loginResponse{Error: "invalid_totp"})
			return
		}
	}

	// Issue a session, but if the admin still owes 2FA setup we force them
	// through the setup flow before any privileged action.
	if err := s.issueSession(r.Context(), w, r, admin.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, loginResponse{Error: "session_failed"})
		return
	}

	resp := loginResponse{OK: true, Admin: toAdminDTO(admin)}
	if admin.MustSetup2FA || (!admin.TOTPConfirmed && !admin.HasPasskey && admin.PasswordHash != "") {
		resp.Need = &needHints{
			MustSetup2FA: true,
			TOTPSetup:    !admin.TOTPConfirmed,
			PasskeySetup: !admin.HasPasskey,
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// --- TG one-shot login ---

type tgLoginRequest struct {
	Token string `json:"token"`
}

func (s *Service) handleTGLogin(w http.ResponseWriter, r *http.Request) {
	var req tgLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, loginResponse{Error: "bad_request"})
		return
	}
	adminID, err := s.Store.ConsumeLoginLink(r.Context(), req.Token)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, loginResponse{Error: "invalid_token"})
		return
	}
	admin, err := s.Store.GetAdminByID(r.Context(), adminID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, loginResponse{Error: "server_error"})
		return
	}
	if err := s.issueSession(r.Context(), w, r, admin.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, loginResponse{Error: "session_failed"})
		return
	}
	resp := loginResponse{OK: true, Admin: toAdminDTO(admin)}
	if admin.MustSetup2FA && admin.PasswordHash != "" {
		resp.Need = &needHints{
			MustSetup2FA: true,
			TOTPSetup:    !admin.TOTPConfirmed,
			PasskeySetup: !admin.HasPasskey,
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// IssueLoginLink builds a URL the bot can DM to a TG admin.
func (s *Service) IssueLoginLink(ctx context.Context, adminID int64) (string, error) {
	tok, err := s.Store.CreateLoginLink(ctx, adminID, s.loginLinkTTL)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s?token=%s", s.loginLinkBase, tok), nil
}

// --- Logout / Me ---

func (s *Service) handleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(s.cookieName)
	if err == nil && cookie.Value != "" {
		_ = s.Store.DeleteSession(r.Context(), cookie.Value)
	}
	ClearCookie(w, s.cookieName, s.cookieSecure)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Service) handleMe(w http.ResponseWriter, r *http.Request) {
	admin := FromContext(r.Context())
	if admin == nil {
		writeJSON(w, http.StatusOK, map[string]any{"admin": nil})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"admin": toAdminDTO(admin)})
}

// --- TOTP setup ---

type totpBeginResponse struct {
	OK      bool   `json:"ok"`
	Secret  string `json:"secret"`
	Otpauth string `json:"otpauth"`
}

func (s *Service) handleTOTPBegin(w http.ResponseWriter, r *http.Request) {
	admin := FromContext(r.Context())
	if admin == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	account := admin.Username
	if account == "" {
		account = adminDisplay(admin)
	}
	secret, otpauth, err := GenerateTOTPSecret(s.siteIssuer, account)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if err := s.Store.UpdateAdminTOTP(r.Context(), admin.ID, secret); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, totpBeginResponse{OK: true, Secret: secret, Otpauth: otpauth})
}

type totpConfirmRequest struct {
	Code string `json:"code"`
}

func (s *Service) handleTOTPConfirm(w http.ResponseWriter, r *http.Request) {
	admin := FromContext(r.Context())
	var req totpConfirmRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bad_request"})
		return
	}
	fresh, err := s.Store.GetAdminByID(r.Context(), admin.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if fresh.TOTPSecret == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "no_pending_totp"})
		return
	}
	if err := VerifyTOTP(fresh.TOTPSecret, req.Code); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_code"})
		return
	}
	if err := s.Store.ConfirmAdminTOTP(r.Context(), admin.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- Passkey registration ---

func (s *Service) handlePasskeyRegisterBegin(w http.ResponseWriter, r *http.Request) {
	admin := FromContext(r.Context())
	options, sessionData, err := s.Passkeys.BeginRegistration(r.Context(), admin)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	cookie, err := r.Cookie(s.cookieName)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "no_session"})
		return
	}
	s.pending.put(cookie.Value, "register", sessionData)
	writeJSON(w, http.StatusOK, options)
}

func (s *Service) handlePasskeyRegisterFinish(w http.ResponseWriter, r *http.Request) {
	admin := FromContext(r.Context())
	cookie, err := r.Cookie(s.cookieName)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "no_session"})
		return
	}
	sessionData, ok := s.pending.take(cookie.Value, "register")
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "no_pending_registration"})
		return
	}
	parsed, err := protocol.ParseCredentialCreationResponseBody(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if err := s.Passkeys.FinishRegistration(r.Context(), admin, sessionData, parsed); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	// If both factors are now bound, clear must_setup_2fa.
	fresh, _ := s.Store.GetAdminByID(r.Context(), admin.ID)
	if fresh != nil && fresh.TOTPConfirmed && fresh.HasPasskey {
		_ = s.Store.ClearMustSetup2FA(r.Context(), admin.ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- Passkey login (second factor) ---

type passkeyLoginBeginRequest struct {
	Username string `json:"username"`
}

func (s *Service) handlePasskeyLoginBegin(w http.ResponseWriter, r *http.Request) {
	var req passkeyLoginBeginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bad_request"})
		return
	}
	admin, err := s.Store.GetAdminByUsername(r.Context(), SanitizeUsername(req.Username))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "no_such_admin"})
		return
	}
	options, sessionData, err := s.Passkeys.BeginLogin(r.Context(), admin)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	tok, err := randomToken(16)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	s.pending.put("login:"+tok, "login", sessionData)
	writeJSON(w, http.StatusOK, map[string]any{"options": options, "challenge_token": tok, "admin_id": admin.ID})
}

type passkeyLoginFinishRequest struct {
	AdminID        int64           `json:"admin_id"`
	ChallengeToken string          `json:"challenge_token"`
	Response       json.RawMessage `json:"response"`
}

func (s *Service) handlePasskeyLoginFinish(w http.ResponseWriter, r *http.Request) {
	var req passkeyLoginFinishRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bad_request"})
		return
	}
	sessionData, ok := s.pending.take("login:"+req.ChallengeToken, "login")
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "no_pending_login"})
		return
	}
	admin, err := s.Store.GetAdminByID(r.Context(), req.AdminID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "no_such_admin"})
		return
	}
	parsed, err := protocol.ParseCredentialRequestResponseBody(strings.NewReader(string(req.Response)))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if err := s.Passkeys.FinishLogin(r.Context(), admin, sessionData, parsed); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
		return
	}
	if err := s.issueSession(r.Context(), w, r, admin.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "admin": toAdminDTO(admin)})
}

// --- Passkey discoverable / passwordless login ---

func (s *Service) handlePasskeyDiscoverableBegin(w http.ResponseWriter, r *http.Request) {
	if s.Passkeys == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "passkeys_unavailable"})
		return
	}
	options, sessionData, err := s.Passkeys.BeginDiscoverableLogin()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	tok, err := randomToken(16)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	s.pending.put("disco:"+tok, "discoverable", sessionData)
	writeJSON(w, http.StatusOK, map[string]any{"options": options, "challenge_token": tok})
}

type passkeyDiscoverableFinishRequest struct {
	ChallengeToken string          `json:"challenge_token"`
	Response       json.RawMessage `json:"response"`
}

func (s *Service) handlePasskeyDiscoverableFinish(w http.ResponseWriter, r *http.Request) {
	if s.Passkeys == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "passkeys_unavailable"})
		return
	}
	var req passkeyDiscoverableFinishRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bad_request"})
		return
	}
	sessionData, ok := s.pending.take("disco:"+req.ChallengeToken, "discoverable")
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "no_pending_login"})
		return
	}
	parsed, err := protocol.ParseCredentialRequestResponseBody(strings.NewReader(string(req.Response)))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	admin, err := s.Passkeys.FinishDiscoverableLogin(r.Context(), sessionData, parsed)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
		return
	}
	if err := s.issueSession(r.Context(), w, r, admin.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "admin": toAdminDTO(admin)})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("content-type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

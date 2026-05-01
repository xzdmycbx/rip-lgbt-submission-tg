package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
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
	loginPending   *webauthnPending // re-uses the same expiring-bucket implementation
	cookieName     string
	siteURL        string
	siteIssuer     string
	loginLinkBase  string
	loginLinkTTL   time.Duration
	sessionTTL     time.Duration
}

// ServiceConfig configures auth flow parameters.
type ServiceConfig struct {
	CookieName    string
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
		loginPending:  newWebauthnPending(),
		cookieName:    cfg.CookieName,
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
	r.Post("/login/totp", s.handleLoginTOTP)
	r.Post("/login/tg", s.handleTGLogin)
	r.Post("/logout", s.handleLogout)
	r.Get("/me", s.handleMe)

	r.Group(func(r chi.Router) {
		r.Use(RequireLogin)
		r.Patch("/me", s.handleUpdateMe)
		r.Post("/me/password", s.handleChangePassword)
	})

	r.Route("/2fa", func(r chi.Router) {
		r.Use(RequireLogin)
		r.Post("/totp/begin", s.handleTOTPBegin)
		r.Post("/totp/confirm", s.handleTOTPConfirm)
		r.Post("/totp/disable", s.handleTOTPDisable)
		r.Post("/passkey/register/begin", s.handlePasskeyRegisterBegin)
		r.Post("/passkey/register/finish", s.handlePasskeyRegisterFinish)
		r.Get("/passkeys", s.handleListPasskeys)
		r.Delete("/passkeys/{id}", s.handleDeletePasskey)
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
	IssueCookie(w, r, s.cookieName, sess.ID, s.sessionTTL)
	return nil
}

// --- Login ---

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	OK            bool       `json:"ok"`
	Admin         *adminDTO  `json:"admin,omitempty"`
	Need          *needHints `json:"need,omitempty"`
	PendingToken  string     `json:"pending_token,omitempty"`
	Error         string     `json:"error,omitempty"`
}

type needHints struct {
	TOTP         bool `json:"totp"`
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
	// MustSetup2FA is the union of the persisted flag (set on creation
	// for the superadmin / for password-bearing admins) and the dynamic
	// check "has a password but no second factor". Without the dynamic
	// component, an admin who claims a password later in their lifecycle
	// would never be nudged to bind 2FA.
	mustSetup := a.MustSetup2FA
	if a.PasswordHash != "" && !a.TOTPConfirmed && !a.HasPasskey {
		mustSetup = true
	}
	return &adminDTO{
		ID: a.ID, Username: a.Username, TelegramID: a.TelegramID,
		DisplayName: a.DisplayName, IsSuper: a.IsSuper,
		HasPasskey: a.HasPasskey, TOTPConfirmed: a.TOTPConfirmed, MustSetup2FA: mustSetup,
	}
}

// handleLogin verifies username + password. If the admin has a confirmed
// TOTP, no session is issued — instead the response includes a short-lived
// pending_token the client passes to /api/auth/login/totp along with the
// 6-digit code on a separate page.
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
	if errors.Is(err, sql.ErrNoRows) || (admin != nil && admin.PasswordHash == "") {
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

	if admin.TOTPConfirmed {
		// Issue a short-lived pending token. The client redirects to
		// /admin/login/totp and submits the code along with this token.
		tok, err := randomToken(24)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, loginResponse{Error: "server_error"})
			return
		}
		s.loginPending.putAdmin("login:"+tok, "totp", admin.ID, 5*time.Minute)
		writeJSON(w, http.StatusOK, loginResponse{
			Need:         &needHints{TOTP: true},
			PendingToken: tok,
		})
		return
	}

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

type loginTOTPRequest struct {
	PendingToken string `json:"pending_token"`
	Code         string `json:"code"`
}

func (s *Service) handleLoginTOTP(w http.ResponseWriter, r *http.Request) {
	var req loginTOTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, loginResponse{Error: "bad_request"})
		return
	}
	adminID, ok := s.loginPending.takeAdmin("login:"+req.PendingToken, "totp")
	if !ok {
		writeJSON(w, http.StatusBadRequest, loginResponse{Error: "expired_or_unknown_token"})
		return
	}
	admin, err := s.Store.GetAdminByID(r.Context(), adminID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, loginResponse{Error: "server_error"})
		return
	}
	if !admin.TOTPConfirmed || admin.TOTPSecret == "" {
		writeJSON(w, http.StatusBadRequest, loginResponse{Error: "totp_unbound"})
		return
	}
	if err := VerifyTOTP(admin.TOTPSecret, req.Code); err != nil {
		// Re-issue a fresh pending token so the user does not have to
		// type the password again on a single mistake.
		tok, _ := randomToken(24)
		s.loginPending.putAdmin("login:"+tok, "totp", admin.ID, 5*time.Minute)
		writeJSON(w, http.StatusUnauthorized, loginResponse{
			Error: "invalid_totp", PendingToken: tok, Need: &needHints{TOTP: true},
		})
		return
	}
	if err := s.issueSession(r.Context(), w, r, admin.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, loginResponse{Error: "session_failed"})
		return
	}
	resp := loginResponse{OK: true, Admin: toAdminDTO(admin)}
	if admin.MustSetup2FA {
		resp.Need = &needHints{MustSetup2FA: true, PasskeySetup: !admin.HasPasskey}
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
	ClearCookie(w, r, s.cookieName)
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

// updateMeRequest is the payload for PATCH /api/auth/me. Pointers
// distinguish "field not provided" from "explicitly empty" so a user
// can clear their telegram_id but leave their display_name unchanged.
type updateMeRequest struct {
	DisplayName *string `json:"display_name,omitempty"`
	Username    *string `json:"username,omitempty"`
	TelegramID  *int64  `json:"telegram_id,omitempty"`
}

func (s *Service) handleUpdateMe(w http.ResponseWriter, r *http.Request) {
	admin := FromContext(r.Context())
	var req updateMeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bad_request"})
		return
	}

	// Reload the latest admin row to enforce per-field policy.
	fresh, err := s.Store.GetAdminByID(r.Context(), admin.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "server_error"})
		return
	}

	// username: only if not yet set; once chosen, the user cannot
	// rename it (that would invalidate their muscle memory and cause
	// log-trail confusion). Superadmins can rename via /admin/admins.
	if req.Username != nil && fresh.Username != "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error":   "username_locked",
			"message": "用户名已设置，无法修改。如需更改请联系超级管理员。",
		})
		return
	}
	if req.Username != nil {
		v := SanitizeUsername(*req.Username)
		if v == "" {
			req.Username = nil // treat empty same as missing on a first-set request
		} else if !validUsername(v) {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": "invalid_username",
				"message": "用户名仅允许英文、数字、下划线、短横线，长度 3-32。",
			})
			return
		} else {
			req.Username = &v
		}
	}

	// telegram_id: anyone can update their own. We don't currently do a
	// bot-side ownership challenge — superadmins should treat self-set
	// telegram_id as an aspiration until verified.
	if req.TelegramID != nil && *req.TelegramID < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error":   "invalid_telegram_id",
			"message": "Telegram ID 必须是正整数。",
		})
		return
	}

	if err := s.Store.UpdateAdminProfile(r.Context(), admin.ID, req.DisplayName, req.Username, req.TelegramID); err != nil {
		switch err.Error() {
		case "username_taken":
			writeJSON(w, http.StatusConflict, map[string]any{"error": "username_taken", "message": "该用户名已被使用。"})
		case "telegram_id_taken":
			writeJSON(w, http.StatusConflict, map[string]any{"error": "telegram_id_taken", "message": "该 Telegram ID 已被另一个管理员占用。"})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		}
		return
	}

	updated, err := s.Store.GetAdminByID(r.Context(), admin.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "server_error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "admin": toAdminDTO(updated)})
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// handleChangePassword lets an admin set or change their own password.
// If they already have one, current_password is required to change it;
// if not (e.g. TG-only admin claiming password login), current_password
// is ignored. Setting a password requires a username — bare telegram_id
// admins must first claim a username via /api/auth/me.
func (s *Service) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	admin := FromContext(r.Context())
	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bad_request"})
		return
	}
	if len(req.NewPassword) < 8 {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "password_too_short", "message": "新密码至少 8 个字符。",
		})
		return
	}
	fresh, err := s.Store.GetAdminByID(r.Context(), admin.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "server_error"})
		return
	}
	if fresh.Username == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "username_required",
			"message": "请先在「账号信息」处设置一个用户名，再设置密码。",
		})
		return
	}
	if fresh.PasswordHash != "" {
		if err := VerifyPassword(fresh.PasswordHash, req.CurrentPassword); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": "invalid_current_password",
				"message": "当前密码不正确。",
			})
			return
		}
	}
	hash, err := HashPassword(req.NewPassword)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "server_error"})
		return
	}
	if err := s.Store.UpdateAdminPassword(r.Context(), admin.ID, hash); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func validUsername(s string) bool {
	if len(s) < 3 || len(s) > 32 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-':
		default:
			return false
		}
	}
	return true
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

type totpDisableRequest struct {
	Code string `json:"code"`
}

// handleTOTPDisable removes a confirmed TOTP binding. We require the admin
// to enter the current code (or pass an empty code if TOTP was never
// confirmed) so that someone with a stolen session cookie still cannot
// silently turn off 2FA.
func (s *Service) handleTOTPDisable(w http.ResponseWriter, r *http.Request) {
	admin := FromContext(r.Context())
	fresh, err := s.Store.GetAdminByID(r.Context(), admin.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if fresh.TOTPConfirmed {
		var req totpDisableRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if err := VerifyTOTP(fresh.TOTPSecret, req.Code); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_code"})
			return
		}
	}
	if err := s.Store.DisableAdminTOTP(r.Context(), admin.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Service) handleListPasskeys(w http.ResponseWriter, r *http.Request) {
	admin := FromContext(r.Context())
	keys, err := s.Store.ListAdminPasskeys(r.Context(), admin.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	out := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		out = append(out, map[string]any{
			"id":         k.ID,
			"transports": k.Transports,
			"created_at": k.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"passkeys": out})
}

func (s *Service) handleDeletePasskey(w http.ResponseWriter, r *http.Request) {
	admin := FromContext(r.Context())
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bad_id"})
		return
	}
	if err := s.Store.DeleteAdminPasskey(r.Context(), admin.ID, id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
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

// Package admin implements administrator-management and settings HTTP
// endpoints used by the web back office.
package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/ripyc/rip-lgbt-submission-tg/internal/auth"
	"github.com/ripyc/rip-lgbt-submission-tg/internal/settings"
)

// Service mounts /api/admin/* routes (excluding draft review which lives in
// the submission package).
type Service struct {
	Auth     *auth.Service
	Settings *settings.Store
	Store    *auth.Store
}

func NewService(authStore *auth.Store, authSvc *auth.Service, settings *settings.Store) *Service {
	return &Service{Auth: authSvc, Settings: settings, Store: authStore}
}

// Register hooks the admin/superadmin endpoints onto a chi router. The
// caller is expected to already have auth middleware in place; this method
// wraps the relevant subtree with auth.RequireLogin / RequireSuperadmin.
func (s *Service) Register(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireLogin, auth.RequireSuperadmin)
		r.Get("/admins", s.handleListAdmins)
		r.Post("/admins", s.handleCreateAdmin)
		r.Delete("/admins/{id}", s.handleDeleteAdmin)
		r.Post("/admins/{id}/login-link", s.handleIssueLoginLink)
		r.Get("/settings", s.handleGetSettings)
		r.Put("/settings", s.handleUpdateSettings)
	})
}

type adminCreateRequest struct {
	Username    string `json:"username,omitempty"`
	Password    string `json:"password,omitempty"`
	TelegramID  int64  `json:"telegram_id,omitempty"`
	DisplayName string `json:"display_name"`
	IsSuper     bool   `json:"is_super,omitempty"`
}

func (s *Service) handleListAdmins(w http.ResponseWriter, r *http.Request) {
	admins, err := s.Store.ListAdmins(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	out := make([]map[string]any, 0, len(admins))
	for _, a := range admins {
		out = append(out, map[string]any{
			"id":             a.ID,
			"username":       a.Username,
			"telegram_id":    a.TelegramID,
			"display_name":   a.DisplayName,
			"is_super":       a.IsSuper,
			"has_passkey":    a.HasPasskey,
			"totp_confirmed": a.TOTPConfirmed,
			"must_setup_2fa": a.MustSetup2FA,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"admins": out})
}

func (s *Service) handleCreateAdmin(w http.ResponseWriter, r *http.Request) {
	var req adminCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bad_request"})
		return
	}
	username := auth.SanitizeUsername(req.Username)
	if username == "" && req.TelegramID == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing_identifier"})
		return
	}
	a := &auth.Admin{
		Username:    username,
		TelegramID:  req.TelegramID,
		DisplayName: req.DisplayName,
		IsSuper:     req.IsSuper,
	}
	if req.Password != "" {
		hash, err := auth.HashPassword(req.Password)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		a.PasswordHash = hash
	}
	id, err := s.Store.CreateAdmin(r.Context(), a)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "id": id})
}

func (s *Service) handleDeleteAdmin(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bad_id"})
		return
	}
	caller := auth.FromContext(r.Context())
	if caller != nil && caller.ID == id {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "cannot_delete_self"})
		return
	}
	if err := s.Store.DeleteAdmin(r.Context(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Service) handleIssueLoginLink(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bad_id"})
		return
	}
	url, err := s.Auth.IssueLoginLink(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "url": url, "expires_in_seconds": 600})
}

func (s *Service) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	all, err := s.Settings.All(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	// Mask the token so it isn't echoed wholesale on every page load.
	if tok := all[settings.KeyBotToken]; tok != "" {
		all[settings.KeyBotToken+"_set"] = "1"
		all[settings.KeyBotToken] = maskToken(tok)
	}
	if sec := all[settings.KeyBotWebhookSecret]; sec != "" {
		all[settings.KeyBotWebhookSecret+"_set"] = "1"
		all[settings.KeyBotWebhookSecret] = maskToken(sec)
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": all})
}

type settingsUpdate struct {
	BotToken         *string `json:"bot_token,omitempty"`
	BotMode          *string `json:"bot_mode,omitempty"`
	BotWebhookURL    *string `json:"bot_webhook_url,omitempty"`
	BotWebhookSecret *string `json:"bot_webhook_secret,omitempty"`
	BotUsername      *string `json:"bot_username,omitempty"`
	SiteName         *string `json:"site_name,omitempty"`
}

func (s *Service) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req settingsUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bad_request"})
		return
	}
	updates := map[string]string{}
	if req.BotToken != nil {
		updates[settings.KeyBotToken] = *req.BotToken
	}
	if req.BotMode != nil {
		mode := *req.BotMode
		if mode != "polling" && mode != "webhook" && mode != "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_mode"})
			return
		}
		updates[settings.KeyBotMode] = mode
	}
	if req.BotWebhookURL != nil {
		updates[settings.KeyBotWebhook] = *req.BotWebhookURL
	}
	if req.BotWebhookSecret != nil {
		secret := *req.BotWebhookSecret
		if secret != "" && !validSecretToken(secret) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_secret",
				"message": "secret token 仅允许 A-Z a-z 0-9 _ - 1-256 字符"})
			return
		}
		updates[settings.KeyBotWebhookSecret] = secret
	}
	if req.BotUsername != nil {
		updates[settings.KeyBotUsername] = *req.BotUsername
	}
	if req.SiteName != nil {
		updates[settings.KeySiteName] = *req.SiteName
	}
	if len(updates) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	if err := s.Settings.SetMany(r.Context(), updates); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// validSecretToken matches Telegram's allowed character set (A-Z, a-z, 0-9, _, -)
// and length 1-256 inclusive, per the Bot API spec for setWebhook secret_token.
func validSecretToken(s string) bool {
	if len(s) < 1 || len(s) > 256 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-':
		default:
			return false
		}
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("content-type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func maskToken(token string) string {
	if len(token) <= 6 {
		return "***"
	}
	return token[:4] + "…" + token[len(token)-3:]
}

// for debugging, never reach
var _ = errors.New

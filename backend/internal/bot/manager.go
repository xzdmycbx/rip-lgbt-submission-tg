// Package bot implements the Telegram submission bot. It can be started,
// stopped, and reloaded while the rest of the server keeps running, so that
// admins can configure the token / mode / webhook URL from the web UI.
package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/ripyc/rip-lgbt-submission-tg/internal/auth"
	"github.com/ripyc/rip-lgbt-submission-tg/internal/settings"
	"github.com/ripyc/rip-lgbt-submission-tg/internal/submission"
)

// Manager owns the bot lifecycle. Call Reload after settings change.
type Manager struct {
	logger      *slog.Logger
	settings    *settings.Store
	drafts      *submission.Store
	authStore   *auth.Store
	authService *auth.Service
	preview     PreviewCapturer
	siteURL     string
	internalURL string
	webhookPath string

	mu       sync.Mutex
	cancel   context.CancelFunc
	running  bool
	bot      *gotgbot.Bot
	updater  *ext.Updater
	mode     string
}

// PreviewCapturer renders a draft preview to PNG.
type PreviewCapturer interface {
	CaptureDraft(ctx context.Context, internalBaseURL, draftID string) ([]byte, error)
}

// Config wires the bot dependencies.
type Config struct {
	Logger      *slog.Logger
	Settings    *settings.Store
	Drafts      *submission.Store
	AuthStore   *auth.Store
	AuthService *auth.Service
	Preview     PreviewCapturer
	SiteURL     string
	InternalURL string // e.g. "http://127.0.0.1:8080"
	// WebhookPath is the URL path the chi router exposes for incoming TG
	// updates (no leading slash needed). The full URL Telegram is told to
	// hit is `{bot_webhook_url}/{WebhookPath}` from the settings store.
	WebhookPath string
}

// New creates a manager that is not yet running.
func New(cfg Config) *Manager {
	if cfg.WebhookPath == "" {
		cfg.WebhookPath = "tg"
	}
	return &Manager{
		logger:      cfg.Logger,
		settings:    cfg.Settings,
		drafts:      cfg.Drafts,
		authStore:   cfg.AuthStore,
		authService: cfg.AuthService,
		preview:     cfg.Preview,
		siteURL:     cfg.SiteURL,
		internalURL: cfg.InternalURL,
		webhookPath: cfg.WebhookPath,
	}
}

// Reload (re)reads settings, stops the existing bot if any, and starts a new one.
func (m *Manager) Reload(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	if m.updater != nil {
		_ = m.updater.Stop()
		m.updater = nil
	}
	m.running = false

	token, err := m.settings.Get(ctx, settings.KeyBotToken)
	if err != nil {
		return err
	}
	if token == "" {
		m.logger.Info("bot token not set; bot is offline")
		return nil
	}
	mode, _ := m.settings.Get(ctx, settings.KeyBotMode)
	if mode == "" {
		mode = "polling"
	}

	bot, err := gotgbot.NewBot(token, nil)
	if err != nil {
		return fmt.Errorf("init bot: %w", err)
	}

	disp := ext.NewDispatcher(&ext.DispatcherOpts{
		Error: func(b *gotgbot.Bot, ctx *ext.Context, err error) ext.DispatcherAction {
			m.logger.Error("dispatcher error", "err", err)
			return ext.DispatcherActionNoop
		},
	})
	m.registerHandlers(disp)

	updater := ext.NewUpdater(disp, nil)
	m.bot = bot
	m.updater = updater
	m.mode = mode

	switch mode {
	case "polling":
		if err := updater.StartPolling(bot, &ext.PollingOpts{
			DropPendingUpdates: true,
		}); err != nil {
			return fmt.Errorf("start polling: %w", err)
		}
		m.logger.Info("bot started", "mode", "polling", "username", bot.Username)
	case "webhook":
		hookURL, _ := m.settings.Get(ctx, settings.KeyBotWebhook)
		if hookURL == "" {
			return errors.New("bot_webhook_url is empty")
		}
		secret, _ := m.settings.Get(ctx, settings.KeyBotWebhookSecret)
		if err := updater.AddWebhook(bot, m.webhookPath, &ext.AddWebhookOpts{SecretToken: secret}); err != nil {
			return fmt.Errorf("add webhook: %w", err)
		}
		fullURL := strings.TrimRight(hookURL, "/") + "/" + strings.TrimLeft(m.webhookPath, "/")
		if _, err := bot.SetWebhook(fullURL, &gotgbot.SetWebhookOpts{SecretToken: secret}); err != nil {
			return fmt.Errorf("set webhook: %w", err)
		}
		m.logger.Info("bot started", "mode", "webhook", "url", fullURL, "secret_set", secret != "")
	default:
		return fmt.Errorf("unknown bot mode %q", mode)
	}

	m.running = true
	return nil
}

// Stop halts polling/webhook.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	if m.updater != nil {
		_ = m.updater.Stop()
		m.updater = nil
	}
	m.running = false
}

// Bot exposes the underlying gotgbot.Bot for callers that need to send messages.
func (m *Manager) Bot() *gotgbot.Bot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.bot
}

// Notify sends a plain text message to a Telegram chat (best-effort).
func (m *Manager) Notify(chatID int64, text string) error {
	bot := m.Bot()
	if bot == nil {
		return errors.New("bot offline")
	}
	_, err := bot.SendMessage(chatID, text, nil)
	return err
}

// SiteURL returns the configured site URL (used for login link prefixes).
func (m *Manager) SiteURL() string { return strings.TrimRight(m.siteURL, "/") }

// WebhookHandler returns an HTTP handler that delegates to the underlying
// gotgbot updater. Returns 503 when the bot is not running in webhook mode.
// pathPrefix should match the chi mount prefix so gotgbot can correctly
// strip the URL prefix and locate the bot.
func (m *Manager) WebhookHandler(pathPrefix string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		updater := m.updater
		mode := m.mode
		m.mu.Unlock()
		if updater == nil || mode != "webhook" {
			http.Error(w, "bot not in webhook mode", http.StatusServiceUnavailable)
			return
		}
		updater.GetHandlerFunc(pathPrefix).ServeHTTP(w, r)
	}
}

// WebhookPath returns the URL path slug used after the prefix.
func (m *Manager) WebhookPath() string { return m.webhookPath }

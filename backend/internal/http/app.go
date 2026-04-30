package http

import (
	"context"
	"embed"
	"io/fs"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ripyc/rip-lgbt-submission-tg/internal/admin"
	"github.com/ripyc/rip-lgbt-submission-tg/internal/auth"
	"github.com/ripyc/rip-lgbt-submission-tg/internal/bot"
	"github.com/ripyc/rip-lgbt-submission-tg/internal/config"
	appdb "github.com/ripyc/rip-lgbt-submission-tg/internal/db"
	"github.com/ripyc/rip-lgbt-submission-tg/internal/markdown"
	"github.com/ripyc/rip-lgbt-submission-tg/internal/memorial"
	"github.com/ripyc/rip-lgbt-submission-tg/internal/preview"
	"github.com/ripyc/rip-lgbt-submission-tg/internal/settings"
	"github.com/ripyc/rip-lgbt-submission-tg/internal/submission"
)

//go:embed all:static
var staticFS embed.FS

// App wires routes, dependencies, and lifecycle.
type App struct {
	cfg          *config.Config
	logger       *slog.Logger
	router       chi.Router
	db           *appdb.DB
	authStore    *auth.Store
	authService  *auth.Service
	memorialSvc  *memorial.Service
	adminSvc     *admin.Service
	settingStore *settings.Store
	drafts       *submission.Store
	draftAdmin   *submission.AdminService
	botManager   *bot.Manager
	preview      *preview.Service
	stopJanitor  func()
}

// NewApp opens the database, runs migrations, seeds the superadmin, and wires
// the HTTP router with all subsystems mounted.
func NewApp(cfg *config.Config, logger *slog.Logger) (*App, error) {
	ctx := context.Background()

	db, err := appdb.Open(ctx, dbPath(cfg))
	if err != nil {
		return nil, err
	}

	store := auth.NewStore(db)
	if _, err := store.EnsureSuperadmin(ctx, cfg.SuperadminUsername, cfg.SuperadminPassword); err != nil {
		_ = db.Close()
		return nil, err
	}

	passkeys, err := auth.NewPasskeyManager(cfg.Secrets.WebAuthnRPID, "rip.lgbt", cfg.SiteURL, db)
	if err != nil {
		logger.Warn("passkey manager init failed; passkey routes will error", "err", err)
	}

	cookieSecure := strings.HasPrefix(cfg.SiteURL, "https://")
	svc := auth.NewService(store, passkeys, auth.ServiceConfig{
		CookieName:   cfg.Secrets.SessionCookieName,
		CookieSecure: cookieSecure,
		SiteURL:      cfg.SiteURL,
		SiteIssuer:   "rip.lgbt",
	})

	settingsStore := settings.NewStore(db)
	uploadsDir := filepath.Join(cfg.DataDir, "uploads")
	drafts := submission.NewStore(db, uploadsDir)

	previewSvc := preview.NewService(preview.Config{
		Logger:  logger,
		SignKey: cfg.Secrets.PreviewSignKey,
		SiteURL: cfg.SiteURL,
	})

	internalURL := "http://127.0.0.1" + parseListenPort(cfg.ListenAddr)

	botMgr := bot.New(bot.Config{
		Logger:      logger,
		Settings:    settingsStore,
		Drafts:      drafts,
		AuthStore:   store,
		AuthService: svc,
		Preview:     previewSvc,
		SiteURL:     cfg.SiteURL,
		InternalURL: internalURL,
		WebhookPath: "tg",
	})
	if err := botMgr.Reload(ctx); err != nil {
		logger.Warn("bot start failed; configure token in /admin/settings to retry", "err", err)
	}

	// Register the markdown preview renderer using the memorial markdown engine.
	submission.SetPreviewRenderer(func(d *submission.Draft) string {
		body := strings.TrimSpace(strings.Join([]string{
			"## 简介", d.GetString("intro"),
			"## 生平与记忆", d.GetString("life"),
			"## 离世", d.GetString("death"),
			"## 念想", d.GetString("remembrance"),
		}, "\n\n"))
		return markdown.Render(body, d.GetString("entry_id"))
	})

	a := &App{
		cfg:          cfg,
		logger:       logger,
		db:           db,
		authStore:    store,
		authService:  svc,
		memorialSvc:  memorial.NewService(db, cfg.Secrets.IPHashPepper),
		settingStore: settingsStore,
		adminSvc:     admin.NewService(store, svc, settingsStore),
		drafts:       drafts,
		draftAdmin:   submission.NewAdminService(drafts, botMgr),
		botManager:   botMgr,
		preview:      previewSvc,
	}
	a.stopJanitor = startJanitor(logger, store, drafts)
	a.router = a.buildRouter()
	return a, nil
}

func (a *App) Router() http.Handler { return a.router }

// Close releases dependencies.
func (a *App) Close() error {
	if a.stopJanitor != nil {
		a.stopJanitor()
	}
	if a.botManager != nil {
		a.botManager.Stop()
	}
	if a.preview != nil {
		a.preview.Close()
	}
	if a.db != nil {
		return a.db.Close()
	}
	return nil
}

func parseListenPort(addr string) string {
	if addr == "" {
		return ":8080"
	}
	if strings.HasPrefix(addr, ":") {
		return addr
	}
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		return addr[i:]
	}
	return ":8080"
}

func (a *App) buildRouter() chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(loggingMiddleware(a.logger))
	r.Use(a.authStore.Middleware(a.authService.CookieName()))

	r.Route("/api", func(r chi.Router) {
		r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("content-type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		})
		r.Mount("/auth", a.authService.Routes())
		r.Route("/admin", func(r chi.Router) {
			a.adminSvc.Register(r)
			a.draftAdmin.Register(r)
			r.With(auth.RequireLogin, auth.RequireSuperadmin).
				Post("/settings/reload-bot", func(w http.ResponseWriter, req *http.Request) {
					if err := a.botManager.Reload(req.Context()); err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
						return
					}
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{"ok":true}`))
				})
		})
		r.Route("/bot", func(r chi.Router) {
			// Telegram webhook endpoint. The actual sub-path (e.g. /tg) is
			// owned by the bot manager so gotgbot can identify the right bot.
			prefix := "/api/bot/webhook"
			r.Handle("/webhook/*", a.botManager.WebhookHandler(prefix))
		})
		r.Mount("/", a.memorialSvc.Routes())
	})

	a.mountMedia(r)
	a.mountStatic(r)
	return r
}

// mountMedia serves files from <DataDir>/uploads at /media/...
func (a *App) mountMedia(r chi.Router) {
	uploads := filepath.Join(a.cfg.DataDir, "uploads")
	fs := http.FileServer(http.Dir(uploads))
	r.Handle("/media/*", http.StripPrefix("/media/", fs))
}

func (a *App) mountStatic(r chi.Router) {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		a.logger.Warn("static subfs", "err", err)
		return
	}
	fileServer := http.FileServer(http.FS(sub))
	r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
		path := strings.TrimPrefix(req.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(sub, path); err != nil {
			req2 := req.Clone(req.Context())
			req2.URL.Path = "/"
			fileServer.ServeHTTP(w, req2)
			return
		}
		fileServer.ServeHTTP(w, req)
	})
}

func loggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			logger.Info("http",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"remote", r.RemoteAddr,
			)
		})
	}
}

func dbPath(cfg *config.Config) string {
	return cfg.DataDir + "/app.db"
}

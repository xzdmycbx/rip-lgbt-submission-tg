// Package preview wraps chromedp to render a draft page (in the same Go
// server) and capture a full-page PNG screenshot. The TG bot uses this to
// send admins a "real" preview image of the submission.
package preview

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

// Service drives a single shared headless Chrome session.
type Service struct {
	logger    *slog.Logger
	signKey   string
	siteURL   string
	chromeBin string

	mu      sync.Mutex
	allocCancel context.CancelFunc
	browserCtx  context.Context
	browserCancel context.CancelFunc
}

// Config configures the preview service.
type Config struct {
	Logger    *slog.Logger
	SignKey   string
	SiteURL   string // public site URL (used only for TLS-friendly defaults)
	ChromeBin string // optional override; falls back to env CHROMIUM_PATH or auto.
}

// NewService initializes the chromedp pool lazily on first use.
func NewService(cfg Config) *Service {
	bin := cfg.ChromeBin
	if bin == "" {
		bin = os.Getenv("CHROMIUM_PATH")
	}
	return &Service{
		logger:    cfg.Logger,
		signKey:   cfg.SignKey,
		siteURL:   strings.TrimRight(cfg.SiteURL, "/"),
		chromeBin: bin,
	}
}

func (s *Service) ensureBrowser(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.browserCtx != nil {
		return nil
	}
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.Flag("headless", "new"),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("hide-scrollbars", true),
	)
	if s.chromeBin != "" {
		opts = append(opts, chromedp.ExecPath(s.chromeBin))
	}
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	if err := chromedp.Run(browserCtx); err != nil {
		allocCancel()
		browserCancel()
		return fmt.Errorf("start chromium: %w", err)
	}
	s.allocCancel = allocCancel
	s.browserCtx = browserCtx
	s.browserCancel = browserCancel
	return nil
}

// Close shuts down the browser.
func (s *Service) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.browserCancel != nil {
		s.browserCancel()
		s.browserCancel = nil
	}
	if s.allocCancel != nil {
		s.allocCancel()
		s.allocCancel = nil
	}
	s.browserCtx = nil
}

// CaptureDraft navigates to the internal preview URL for the draft and
// returns a full-page PNG.
func (s *Service) CaptureDraft(ctx context.Context, internalBaseURL, draftID string) ([]byte, error) {
	if err := s.ensureBrowser(ctx); err != nil {
		return nil, err
	}
	tabCtx, cancel := chromedp.NewContext(s.browserCtx)
	defer cancel()

	timeoutCtx, cancelTimeout := context.WithTimeout(tabCtx, 25*time.Second)
	defer cancelTimeout()

	url := fmt.Sprintf("%s/admin/preview/%s?token=%s",
		strings.TrimRight(internalBaseURL, "/"), draftID, s.SignToken(draftID, time.Now().Add(2*time.Minute)))

	var buf []byte
	if err := chromedp.Run(timeoutCtx,
		chromedp.EmulateViewport(1024, 1400),
		chromedp.Navigate(url),
		chromedp.Sleep(800*time.Millisecond),
		chromedp.FullScreenshot(&buf, 90),
	); err != nil {
		return nil, fmt.Errorf("screenshot: %w", err)
	}
	return buf, nil
}

// SignToken returns an HMAC token authorizing access to a draft preview
// until expires.
func (s *Service) SignToken(draftID string, expires time.Time) string {
	exp := strconv.FormatInt(expires.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(s.signKey))
	mac.Write([]byte(draftID + "|" + exp))
	return exp + "." + hex.EncodeToString(mac.Sum(nil))
}

// VerifyToken returns nil iff the token is well-formed and not expired.
func (s *Service) VerifyToken(draftID, token string) error {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return errors.New("malformed token")
	}
	exp, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return errors.New("malformed expiry")
	}
	if time.Now().Unix() > exp {
		return errors.New("token expired")
	}
	mac := hmac.New(sha256.New, []byte(s.signKey))
	mac.Write([]byte(draftID + "|" + parts[0]))
	want := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(parts[1])) {
		return errors.New("bad signature")
	}
	return nil
}

// PreviewMiddleware permits requests that are either authenticated as an
// admin or carry a valid HMAC preview token. It is intended to wrap the
// /admin/preview/{id} SPA route or any preview API.
func (s *Service) PreviewMiddleware(isAuthed func(*http.Request) bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isAuthed != nil && isAuthed(r) {
				next.ServeHTTP(w, r)
				return
			}
			id := r.URL.Path
			i := strings.LastIndex(id, "/")
			if i >= 0 {
				id = id[i+1:]
			}
			if id == "" {
				http.Error(w, "no draft id", http.StatusBadRequest)
				return
			}
			tok := r.URL.Query().Get("token")
			if err := s.VerifyToken(id, tok); err != nil {
				http.Error(w, err.Error(), http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

package auth

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// CookieMaxAge is the lifetime of a session cookie.
const CookieMaxAge = 24 * time.Hour

// IsSecureRequest returns true when the incoming request was served over
// HTTPS — either directly (r.TLS != nil) or via a trusted reverse proxy
// (X-Forwarded-Proto: https). The result determines whether to mark a
// cookie as Secure so it survives across requests.
func IsSecureRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	proto := r.Header.Get("X-Forwarded-Proto")
	if proto == "" {
		// Some proxies use X-Forwarded-Ssl=on instead.
		if strings.EqualFold(r.Header.Get("X-Forwarded-Ssl"), "on") {
			return true
		}
		return false
	}
	// X-Forwarded-Proto can be a comma-separated list when there are
	// multiple proxies; the first hop's scheme is what matters.
	if i := strings.IndexByte(proto, ','); i >= 0 {
		proto = proto[:i]
	}
	return strings.EqualFold(strings.TrimSpace(proto), "https")
}

// IssueCookie writes the session id as an HTTP-only cookie. Secure is
// derived from the request, not from the static config, so the same
// binary works correctly when accessed via plain http://localhost
// during development and via https://example.com behind a proxy.
func IssueCookie(w http.ResponseWriter, r *http.Request, name, value string, maxAge time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   IsSecureRequest(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(maxAge.Seconds()),
	})
}

// ClearCookie removes the session cookie.
func ClearCookie(w http.ResponseWriter, r *http.Request, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   IsSecureRequest(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// ctxKey is the context key for the authenticated admin.
type ctxKey int

const adminCtxKey ctxKey = 1

// FromContext returns the authenticated admin (or nil if unauthenticated).
func FromContext(ctx context.Context) *Admin {
	v, _ := ctx.Value(adminCtxKey).(*Admin)
	return v
}

// WithAdmin attaches an admin to a request context.
func WithAdmin(ctx context.Context, a *Admin) context.Context {
	return context.WithValue(ctx, adminCtxKey, a)
}

// Middleware loads the admin (if any) from the session cookie.
// It does NOT enforce authentication; routes that need login should also
// chain RequireLogin or RequireSuperadmin.
func (s *Store) Middleware(cookieName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(cookieName)
			if err != nil || cookie.Value == "" {
				next.ServeHTTP(w, r)
				return
			}
			sess, err := s.GetSession(r.Context(), cookie.Value)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			admin, err := s.GetAdminByID(r.Context(), sess.AdminID)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			r = r.WithContext(WithAdmin(r.Context(), admin))
			next.ServeHTTP(w, r)
		})
	}
}

// RequireLogin returns 401 if no admin is on the context.
func RequireLogin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if FromContext(r.Context()) == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireSuperadmin returns 403 unless the current admin has is_super.
func RequireSuperadmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a := FromContext(r.Context())
		if a == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if !a.IsSuper {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

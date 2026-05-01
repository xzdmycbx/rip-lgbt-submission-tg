package auth

import (
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

// webauthnPending stores transient SessionData (or any opaque pointer-typed
// payload) between BeginXxx and FinishXxx HTTP turns, keyed by an opaque
// token. The same bucket is reused for the password→TOTP intermediate
// "pending login" step which only needs to remember an admin id.
type webauthnPending struct {
	mu      sync.Mutex
	entries map[string]webauthnPendingEntry
}

type webauthnPendingEntry struct {
	kind    string
	data    *webauthn.SessionData // nil for non-WebAuthn entries
	adminID int64                 // populated for password→TOTP transitions
	expires time.Time
}

func newWebauthnPending() *webauthnPending {
	wp := &webauthnPending{entries: map[string]webauthnPendingEntry{}}
	go wp.gc()
	return wp
}

func (w *webauthnPending) gc() {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for now := range t.C {
		w.mu.Lock()
		for k, v := range w.entries {
			if now.After(v.expires) {
				delete(w.entries, k)
			}
		}
		w.mu.Unlock()
	}
}

func (w *webauthnPending) put(key, kind string, data *webauthn.SessionData) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.entries[key] = webauthnPendingEntry{
		kind:    kind,
		data:    data,
		expires: time.Now().Add(5 * time.Minute),
	}
}

func (w *webauthnPending) putAdmin(key, kind string, adminID int64, ttl time.Duration) {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.entries[key] = webauthnPendingEntry{
		kind:    kind,
		adminID: adminID,
		expires: time.Now().Add(ttl),
	}
}

func (w *webauthnPending) take(key, kind string) (*webauthn.SessionData, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	v, ok := w.entries[key]
	if !ok || v.kind != kind || time.Now().After(v.expires) {
		return nil, false
	}
	delete(w.entries, key)
	return v.data, true
}

func (w *webauthnPending) takeAdmin(key, kind string) (int64, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	v, ok := w.entries[key]
	if !ok || v.kind != kind || time.Now().After(v.expires) {
		return 0, false
	}
	delete(w.entries, key)
	return v.adminID, true
}

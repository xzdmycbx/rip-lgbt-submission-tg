package auth

import (
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

// webauthnPending stores transient SessionData between BeginXxx and FinishXxx.
// Keyed by the admin session id. Records expire after 5 minutes.
type webauthnPending struct {
	mu      sync.Mutex
	entries map[string]webauthnPendingEntry
}

type webauthnPendingEntry struct {
	kind       string // "register" or "login"
	data       *webauthn.SessionData
	expires    time.Time
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

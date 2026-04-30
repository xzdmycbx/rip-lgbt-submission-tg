package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	appdb "github.com/ripyc/rip-lgbt-submission-tg/internal/db"
)

func newTestAuthEnv(t *testing.T) (*Service, *Store, *PasskeyManager, func()) {
	t.Helper()
	dir := t.TempDir()
	db, err := appdb.Open(context.Background(), filepath.Join(dir, "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	if _, err := store.EnsureSuperadmin(context.Background(), "admin", "test1234"); err != nil {
		t.Fatal(err)
	}
	passkeys, err := NewPasskeyManager("localhost", "rip.lgbt", "http://localhost:8080", db)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(store, passkeys, ServiceConfig{
		CookieName: "rip_session", SiteURL: "http://localhost:8080",
	})
	cleanup := func() { db.Close() }
	return svc, store, passkeys, cleanup
}

// seedPasskey installs a synthetic passkey row for an admin so that
// LookupAdminByPasskey can resolve it. Mirrors what FinishRegistration
// would write under a real ceremony.
func seedPasskey(t *testing.T, db *appdb.DB, adminID int64, credID, userHandle []byte) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO admin_passkeys(admin_id, credential_id, public_key, sign_count, transports, attestation, user_handle, created_at)
		VALUES(?, ?, ?, 0, '', '', ?, datetime('now'))`,
		adminID, credID, []byte("dummy-public-key"), userHandle,
	); err != nil {
		t.Fatal(err)
	}
}

func TestLookupAdminByPasskey_PrefersUserHandle(t *testing.T) {
	_, store, passkeys, cleanup := newTestAuthEnv(t)
	defer cleanup()

	admin, err := store.GetAdminByUsername(context.Background(), "admin")
	if err != nil {
		t.Fatal(err)
	}
	credID := []byte("credA")
	handle := adminUserHandle(admin.ID)
	seedPasskey(t, store.DB, admin.ID, credID, handle)

	// Lookup by user handle even if the rawID is wrong.
	got, err := passkeys.LookupAdminByPasskey(context.Background(), []byte("wrong-cred"), handle)
	if err != nil {
		t.Fatalf("LookupAdminByPasskey: %v", err)
	}
	if got.ID != admin.ID {
		t.Errorf("expected admin %d, got %d", admin.ID, got.ID)
	}
}

func TestLookupAdminByPasskey_FallbackToCredentialID(t *testing.T) {
	_, store, passkeys, cleanup := newTestAuthEnv(t)
	defer cleanup()

	admin, _ := store.GetAdminByUsername(context.Background(), "admin")
	credID := []byte("credB")
	handle := adminUserHandle(admin.ID)
	seedPasskey(t, store.DB, admin.ID, credID, handle)

	// Provide an unknown user_handle but a known credential_id; should still resolve.
	got, err := passkeys.LookupAdminByPasskey(context.Background(), credID, []byte("unknown-handle"))
	if err != nil {
		t.Fatalf("LookupAdminByPasskey: %v", err)
	}
	if got.ID != admin.ID {
		t.Errorf("expected admin %d, got %d", admin.ID, got.ID)
	}
}

func TestLookupAdminByPasskey_NoMatch(t *testing.T) {
	_, _, passkeys, cleanup := newTestAuthEnv(t)
	defer cleanup()
	if _, err := passkeys.LookupAdminByPasskey(context.Background(), []byte("nope"), []byte("nope")); err == nil {
		t.Fatal("expected error when nothing matches")
	}
}

func TestDiscoverableBeginIssuesChallengeToken(t *testing.T) {
	svc, _, _, cleanup := newTestAuthEnv(t)
	defer cleanup()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/passkey/login/discoverable/begin", nil)
	svc.handlePasskeyDiscoverableBegin(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		Options        map[string]any `json:"options"`
		ChallengeToken string         `json:"challenge_token"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.ChallengeToken == "" {
		t.Error("expected challenge_token to be issued")
	}
	if _, ok := body.Options["publicKey"]; !ok {
		t.Errorf("expected publicKey in options, got %v", body.Options)
	}
}

func TestDiscoverableFinishRejectsUnknownChallenge(t *testing.T) {
	svc, _, _, cleanup := newTestAuthEnv(t)
	defer cleanup()

	payload, _ := json.Marshal(map[string]any{
		"challenge_token": "totally-bogus",
		"response":        map[string]any{},
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/passkey/login/discoverable/finish",
		bytes.NewReader(payload))
	svc.handlePasskeyDiscoverableFinish(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown challenge, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestDiscoverableFinishRejectsMalformedResponse(t *testing.T) {
	svc, _, _, cleanup := newTestAuthEnv(t)
	defer cleanup()

	// Begin first, capture token.
	rr := httptest.NewRecorder()
	svc.handlePasskeyDiscoverableBegin(rr, httptest.NewRequest(http.MethodPost, "/x", nil))
	var begin struct {
		ChallengeToken string `json:"challenge_token"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &begin); err != nil {
		t.Fatal(err)
	}

	// Send finish with an obviously broken response body.
	payload, _ := json.Marshal(map[string]any{
		"challenge_token": begin.ChallengeToken,
		"response":        map[string]any{"id": "not-a-real-credential"},
	})
	rr2 := httptest.NewRecorder()
	svc.handlePasskeyDiscoverableFinish(rr2, httptest.NewRequest(http.MethodPost, "/x",
		bytes.NewReader(payload)))

	if rr2.Code < 400 {
		t.Fatalf("expected 4xx for malformed response, got %d body=%s", rr2.Code, rr2.Body.String())
	}
}

func TestDiscoverableFinishConsumesChallengeOnce(t *testing.T) {
	svc, _, _, cleanup := newTestAuthEnv(t)
	defer cleanup()

	rr := httptest.NewRecorder()
	svc.handlePasskeyDiscoverableBegin(rr, httptest.NewRequest(http.MethodPost, "/x", nil))
	var begin struct {
		ChallengeToken string `json:"challenge_token"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &begin)

	payload, _ := json.Marshal(map[string]any{
		"challenge_token": begin.ChallengeToken,
		"response":        map[string]any{"id": "x"},
	})
	rrA := httptest.NewRecorder()
	svc.handlePasskeyDiscoverableFinish(rrA, httptest.NewRequest(http.MethodPost, "/x",
		bytes.NewReader(payload)))

	// Replay must fail at the challenge gate (token was consumed).
	rrB := httptest.NewRecorder()
	svc.handlePasskeyDiscoverableFinish(rrB, httptest.NewRequest(http.MethodPost, "/x",
		bytes.NewReader(payload)))

	if rrB.Code != http.StatusBadRequest {
		t.Fatalf("expected replay to be rejected with 400, got %d body=%s", rrB.Code, rrB.Body.String())
	}
}

func TestAdminUserHandleIsStable(t *testing.T) {
	a := adminUserHandle(42)
	b := adminUserHandle(42)
	if string(a) != string(b) {
		t.Errorf("expected stable handle for same id, got %x vs %x", a, b)
	}
	c := adminUserHandle(43)
	if string(a) == string(c) {
		t.Errorf("expected different handles for different ids, got %x", a)
	}
}

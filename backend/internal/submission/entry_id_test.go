package submission

import (
	"context"
	"path/filepath"
	"testing"

	appdb "github.com/ripyc/rip-lgbt-submission-tg/internal/db"
)

func newTestDB(t *testing.T) *appdb.DB {
	t.Helper()
	dir := t.TempDir()
	d, err := appdb.Open(context.Background(), filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestEntryIDFormat(t *testing.T) {
	d := newTestDB(t)
	cases := map[string]EntryIDStatus{
		"valid_id":          EntryIDOK,
		"abc-123":           EntryIDOK,
		"with space and 1":  EntryIDOK,
		"a":                 EntryIDInvalid,
		"":                  EntryIDInvalid,
		"非法":                EntryIDInvalid,
		"foo!bar":           EntryIDInvalid,
	}
	for in, want := range cases {
		got, err := CheckEntryID(context.Background(), d, in, "")
		if err != nil {
			t.Fatalf("CheckEntryID(%q): %v", in, err)
		}
		if got != want {
			t.Errorf("CheckEntryID(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestEntryIDDuplicateMemorial(t *testing.T) {
	d := newTestDB(t)
	if _, err := d.Exec(`
		INSERT INTO memorials(id, display_name, slug, status, created_at, updated_at)
		VALUES('alex', 'Alex', 'alex', 'published', datetime('now'), datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	got, err := CheckEntryID(context.Background(), d, "alex", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != EntryIDTakenByMemorial {
		t.Errorf("expected EntryIDTakenByMemorial, got %v", got)
	}
}

func TestEntryIDDuplicateDraft(t *testing.T) {
	d := newTestDB(t)
	if _, err := d.Exec(`
		INSERT INTO drafts(id, submitter_telegram_id, submitter_chat_id, status, payload_json, created_at, updated_at)
		VALUES('draft-1', 1, 1, 'collecting', '{"entry_id":"shared"}', datetime('now'), datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	// Different draft using the same id → taken.
	got, err := CheckEntryID(context.Background(), d, "shared", "draft-2")
	if err != nil {
		t.Fatal(err)
	}
	if got != EntryIDTakenByDraft {
		t.Errorf("expected EntryIDTakenByDraft, got %v", got)
	}
	// Same draft re-checking its own id → ok.
	got, err = CheckEntryID(context.Background(), d, "shared", "draft-1")
	if err != nil {
		t.Fatal(err)
	}
	if got != EntryIDOK {
		t.Errorf("expected EntryIDOK when excluding self, got %v", got)
	}
}

func TestEntryIDIgnoresSoftDeletedDrafts(t *testing.T) {
	d := newTestDB(t)
	if _, err := d.Exec(`
		INSERT INTO drafts(id, submitter_telegram_id, submitter_chat_id, status, payload_json, created_at, updated_at, deleted_at)
		VALUES('old', 1, 1, 'rejected', '{"entry_id":"freed"}', datetime('now'), datetime('now'), datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	got, _ := CheckEntryID(context.Background(), d, "freed", "")
	if got != EntryIDOK {
		t.Errorf("soft-deleted draft should free the id, got %v", got)
	}
}

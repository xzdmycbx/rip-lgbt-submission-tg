package memorial

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	appdb "github.com/ripyc/rip-lgbt-submission-tg/internal/db"
)

func newTestService(t *testing.T) (*Service, func()) {
	t.Helper()
	dir := t.TempDir()
	db, err := appdb.Open(context.Background(), filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(db, "pepper")
	return svc, func() { db.Close() }
}

func seedMemorial(t *testing.T, svc *Service, id string) {
	t.Helper()
	facts, _ := json.Marshal([]Fact{{Label: "出生", Value: "1999"}})
	sites, _ := json.Marshal([]Site{{Label: "twitter", URL: "https://x.com/u"}})
	if _, err := svc.db.Exec(`
		INSERT INTO memorials(id, display_name, slug, description, death_date, status, facts_json, websites_json, markdown_full, created_at, updated_at)
		VALUES(?, ?, ?, ?, ?, 'published', ?, ?, ?, ?, ?)`,
		id, "示例", id, "一句话简介", "2025-06-19", string(facts), string(sites),
		"## 简介\n\nHello world.", appdb.Now(), appdb.Now(),
	); err != nil {
		t.Fatal(err)
	}
}

func TestListAndGetProfile(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	seedMemorial(t, svc, "alex")

	people, err := svc.List(context.Background())
	if err != nil || len(people) != 1 {
		t.Fatalf("List = %v err=%v", people, err)
	}
	if people[0].ID != "alex" {
		t.Errorf("expected alex, got %+v", people[0])
	}

	prof, err := svc.GetProfile(context.Background(), "alex")
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if !strings.Contains(prof.ContentHTML, "<h3>简介</h3>") {
		t.Errorf("expected rendered markdown, got %s", prof.ContentHTML)
	}
}

func TestCommentCooldown(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	seedMemorial(t, svc, "alex")
	ctx := context.Background()

	if _, err := svc.AddComment(ctx, "alex", "Aki", "晚安", "ip1"); err != nil {
		t.Fatalf("first comment: %v", err)
	}
	if _, err := svc.AddComment(ctx, "alex", "Aki", "再见", "ip1"); !IsCooldown(err) {
		t.Fatalf("expected cooldown error, got %v", err)
	}
	// Different ip is OK.
	if _, err := svc.AddComment(ctx, "alex", "Aki", "另一条", "ip2"); err != nil {
		t.Fatalf("second ip: %v", err)
	}
}

func TestAddCommentRejectsEmpty(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	seedMemorial(t, svc, "alex")
	if _, err := svc.AddComment(context.Background(), "alex", "Aki", "   ", "ip"); !IsEmptyContent(err) {
		t.Fatalf("expected empty-content, got %v", err)
	}
}

func TestAddFlowerCooldown(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	seedMemorial(t, svc, "alex")
	ctx := context.Background()

	counted, total, err := svc.AddFlower(ctx, "alex", "ip1")
	if err != nil || !counted || total != 1 {
		t.Fatalf("first flower: counted=%v total=%d err=%v", counted, total, err)
	}
	counted, total, err = svc.AddFlower(ctx, "alex", "ip1")
	if err != nil || counted {
		t.Fatalf("expected throttled flower: counted=%v err=%v", counted, err)
	}
	if total != 1 {
		t.Errorf("expected total to stay at 1, got %d", total)
	}
}

func TestNotFound(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()
	if _, err := svc.GetEngagement(context.Background(), "missing"); !IsNotFound(err) {
		t.Errorf("expected not_found, got %v", err)
	}
}

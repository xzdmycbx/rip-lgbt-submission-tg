package submission

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// osMkdirAll / osRename / osRemoveAll exist as variables so tests can stub
// the FS layer if they ever want to. In production they map to os.* directly.
var (
	osMkdirAll  = os.MkdirAll
	osRename    = os.Rename
	osRemoveAll = os.RemoveAll
)

func storeAbs(store *Store, rel string) string {
	return filepath.Join(store.UploadDir, filepath.FromSlash(rel))
}

// BuildMarkdown generates the .md bundle the admins see, mirroring
// buildSubmissionMarkdown() in frontend.js — minus the uploaded-attachment
// listing, which is administrative metadata that has no business in the
// published markdown body.
func BuildMarkdown(d *Draft) string {
	var b strings.Builder
	displayName := d.GetString("display_name")
	if displayName == "" {
		displayName = "(未填写)"
	}
	fmt.Fprintf(&b, "# 勿忘我投稿:%s\n\n", displayName)

	b.WriteString("## 基础信息\n\n")
	for _, p := range []struct{ Label, Key string }{
		{"条目ID", "entry_id"},
		{"展示名", "display_name"},
		{"一句话简介", "description"},
		{"地区", "location"},
		{"出生日期", "birth_date"},
		{"逝世日期", "death_date"},
		{"昵称", "alias"},
		{"年龄", "age"},
		{"身份表述", "identity"},
		{"代词", "pronouns"},
		{"内容提醒", "content_warnings"},
		{"投稿人联系方式", "submitter_contact"},
	} {
		v := d.GetString(p.Key)
		if v != "" {
			fmt.Fprintf(&b, "- %s:%s\n", p.Label, v)
		}
	}

	for _, p := range []struct{ Label, Key string }{
		{"简介", "intro"},
		{"生平与记忆", "life"},
		{"离世", "death"},
		{"念想", "remembrance"},
		{"公开链接", "links"},
		{"作品", "works"},
		{"资料来源", "sources"},
		{"自选附加项", "custom"},
	} {
		v := d.GetString(p.Key)
		if v != "" {
			fmt.Fprintf(&b, "\n## %s\n\n%s\n", p.Label, v)
		}
	}

	fmt.Fprintf(&b, "\n---\n\n提交时间:%s\n", time.Now().UTC().Format(time.RFC3339))
	return b.String()
}

// AcceptDraft promotes a draft into the memorials table and copies assets to
// the published folder. Caller is responsible for triggering bot notifications.
func AcceptDraft(ctx context.Context, store *Store, d *Draft, reviewerID int64) error {
	entryID := strings.TrimSpace(d.GetString("entry_id"))
	if entryID == "" {
		return fmt.Errorf("entry_id is empty")
	}

	// Re-derive markdown_full / facts / websites server-side via the
	// shared memorial generator so the published memorial matches what
	// the admin would see in the edit form right after acceptance.
	displayName := d.GetString("display_name")
	avatar := d.GetString("avatar")
	if avatar == "" || strings.EqualFold(avatar, "none") {
		// Use the post-relocation memorial path so avatar_url stays valid
		// after the draft directory is cleaned up.
		avatar = ""
		for _, a := range d.Assets {
			if a.Role == "avatar" {
				avatar = "/media/memorials/" + entryID + "/" + a.Filename
				break
			}
		}
	}

	gen := generatePublishedShape(d, avatar)
	tx, err := store.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO memorials(id, display_name, slug, avatar_url, description, location, birth_date, death_date,
		                     alias, age, identity, pronouns, content_warnings,
		                     intro, life, death, remembrance, links_md, works_md, sources_md, custom_md,
		                     status, facts_json, websites_json, markdown_full, created_at, updated_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'published', ?, ?, ?, datetime('now'), datetime('now'))
	`,
		entryID, displayName, entryID, avatar, d.GetString("description"),
		d.GetString("location"), d.GetString("birth_date"), d.GetString("death_date"),
		d.GetString("alias"), d.GetString("age"), d.GetString("identity"), d.GetString("pronouns"),
		"[]",
		d.GetString("intro"), d.GetString("life"), d.GetString("death"), d.GetString("remembrance"),
		d.GetString("links"), d.GetString("works"), d.GetString("sources"), d.GetString("custom"),
		gen.factsJSON, gen.websitesJSON, gen.markdown,
	); err != nil {
		return fmt.Errorf("insert memorial: %w", err)
	}

	// Mark draft accepted.
	if _, err := tx.ExecContext(ctx, `UPDATE drafts SET status = ?, reviewer_admin_id = ?, updated_at = datetime('now') WHERE id = ?`,
		StatusAccepted, reviewerID, d.ID); err != nil {
		return err
	}

	// Move uploaded draft assets into the published memorial's directory
	// so /media/memorials/{id}/... is the canonical location going
	// forward. Best-effort: storage failures don't roll back the row.
	if err := tx.Commit(); err != nil {
		return err
	}
	if store.UploadDir != "" {
		_ = relocateDraftAssets(ctx, store, d, entryID)
	}
	return nil
}

type publishedShape struct {
	markdown      string
	factsJSON     string
	websitesJSON  string
}

// generatePublishedShape is a thin shim that lets us reuse the memorial
// generator without an import cycle. The actual generators live in
// internal/memorial; we wire them through the package-level variables
// below so the http app can plug them in at startup.
func generatePublishedShape(d *Draft, avatar string) publishedShape {
	if generateMarkdownPublic == nil {
		// Fallback to the older bot-internal renderer; only triggered
		// before the http app wires the generator (i.e. tests).
		return publishedShape{
			markdown:     BuildMarkdown(d),
			factsJSON:    "[]",
			websitesJSON: "[]",
		}
	}
	out := generateMarkdownPublic(d, avatar)
	return publishedShape{
		markdown:     out.Markdown,
		factsJSON:    out.FactsJSON,
		websitesJSON: out.WebsitesJSON,
	}
}

// PublishedShape is the public-facing struct injected by the http app.
type PublishedShape struct {
	Markdown     string
	FactsJSON    string
	WebsitesJSON string
}

var generateMarkdownPublic func(d *Draft, avatar string) PublishedShape

// SetPublishedShapeGenerator wires the memorial markdown/facts generator
// into AcceptDraft so the published memorial matches the admin edit
// form exactly.
func SetPublishedShapeGenerator(fn func(d *Draft, avatar string) PublishedShape) {
	generateMarkdownPublic = fn
}

// relocateDraftAssets moves files from data/uploads/drafts/<id>/ to
// data/uploads/memorials/<entry_id>/ so they survive draft cleanup.
// It also populates memorial_assets so published memorials own their assets
// independently of the draft record.
func relocateDraftAssets(ctx context.Context, store *Store, d *Draft, entryID string) error {
	if len(d.Assets) == 0 {
		return nil
	}
	srcDir := store.DraftDir(d.ID)
	if srcDir == "" {
		return nil
	}
	dstRel := "memorials/" + entryID
	dstDir := storeAbs(store, dstRel)
	if err := osMkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	for _, a := range d.Assets {
		newRel := dstRel + "/" + a.Filename
		if !strings.HasPrefix(a.Path, "memorials/") {
			fromPath := store.AssetPath(&a)
			toPath := storeAbs(store, newRel)
			if err := osRename(fromPath, toPath); err != nil {
				continue
			}
			_, _ = store.DB.ExecContext(ctx, `UPDATE draft_assets SET path = ? WHERE id = ?`, newRel, a.ID)
		}
		insertMemorialAsset(ctx, store, entryID, a, newRel)
	}
	_ = osRemoveAll(srcDir)
	return nil
}

// insertMemorialAsset adds an asset record to memorial_assets (idempotent).
func insertMemorialAsset(ctx context.Context, store *Store, entryID string, a Asset, path string) {
	_, _ = store.DB.ExecContext(ctx, `
		INSERT INTO memorial_assets(memorial_id, role, filename, path, content_type, size, sort)
		SELECT ?, ?, ?, ?, ?, ?, ?
		WHERE NOT EXISTS (
			SELECT 1 FROM memorial_assets WHERE memorial_id = ? AND filename = ?
		)`,
		entryID, a.Role, a.Filename, path, a.ContentType, a.Size, a.Sort,
		entryID, a.Filename)
}

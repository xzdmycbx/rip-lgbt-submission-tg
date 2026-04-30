package submission

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// BuildMarkdown generates the .md bundle the admins see, mirroring
// buildSubmissionMarkdown() in frontend.js.
func BuildMarkdown(d *Draft) string {
	var b strings.Builder
	displayName := d.GetString("display_name")
	if displayName == "" {
		displayName = "(未填写)"
	}
	fmt.Fprintf(&b, "# 勿忘我投稿：%s\n\n", displayName)

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
			fmt.Fprintf(&b, "- %s：%s\n", p.Label, v)
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

	if len(d.Assets) > 0 {
		b.WriteString("\n## 上传附件\n\n")
		for _, a := range d.Assets {
			fmt.Fprintf(&b, "- %s（%s · %d KB · %s）\n", a.Filename, a.Role, a.Size/1024, a.ContentType)
		}
	}

	fmt.Fprintf(&b, "\n---\n\n提交时间：%s\n", time.Now().UTC().Format(time.RFC3339))
	return b.String()
}

// AcceptDraft promotes a draft into the memorials table and copies assets to
// the published folder. Caller is responsible for triggering bot notifications.
func AcceptDraft(ctx context.Context, store *Store, d *Draft, reviewerID int64) error {
	// Defensive copy / extract
	entryID := strings.TrimSpace(d.GetString("entry_id"))
	if entryID == "" {
		return fmt.Errorf("entry_id is empty")
	}

	// Insert into memorials
	displayName := d.GetString("display_name")
	avatar := d.GetString("avatar")
	if avatar == "" {
		// Try first asset of role 'avatar'
		for _, a := range d.Assets {
			if a.Role == "avatar" {
				avatar = "/media/" + a.Path
				break
			}
		}
	}
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
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'published', '[]', '[]', ?, datetime('now'), datetime('now'))
	`,
		entryID, displayName, entryID, avatar, d.GetString("description"),
		d.GetString("location"), d.GetString("birth_date"), d.GetString("death_date"),
		d.GetString("alias"), d.GetString("age"), d.GetString("identity"), d.GetString("pronouns"),
		"[]",
		d.GetString("intro"), d.GetString("life"), d.GetString("death"), d.GetString("remembrance"),
		d.GetString("links"), d.GetString("works"), d.GetString("sources"), d.GetString("custom"),
		BuildMarkdown(d),
	); err != nil {
		return fmt.Errorf("insert memorial: %w", err)
	}

	// Mark draft accepted
	if _, err := tx.ExecContext(ctx, `UPDATE drafts SET status = ?, reviewer_admin_id = ?, updated_at = datetime('now') WHERE id = ?`,
		StatusAccepted, reviewerID, d.ID); err != nil {
		return err
	}
	return tx.Commit()
}

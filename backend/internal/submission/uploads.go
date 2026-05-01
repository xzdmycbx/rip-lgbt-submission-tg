package submission

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// UploadTokenTTL is a safety-net expiry. Tokens are normally revoked
// when the draft transitions out of the collecting / revising state
// (i.e. submitted / accepted / rejected / cancelled), so under normal
// flow the user has the link active for as long as they are actively
// editing. The TTL here only matters for orphaned tokens belonging to
// drafts that were never submitted.
const UploadTokenTTL = 30 * 24 * time.Hour

// UploadAPI exposes the public upload endpoints used by the web image
// uploader. Routes are NOT auth-gated — possession of the (random) token
// is the credential.
type UploadAPI struct {
	Store   *Store
	SiteURL string
}

func NewUploadAPI(store *Store, siteURL string) *UploadAPI {
	return &UploadAPI{Store: store, SiteURL: strings.TrimRight(siteURL, "/")}
}

// Routes mounts /api/uploads/{token}/...
func (a *UploadAPI) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/{token}/state", a.handleState)
	r.Post("/{token}/file", a.handleUpload)
	r.Delete("/{token}/file/{assetID}", a.handleDelete)
	return r
}

// IssueUploadToken creates a fresh token for a draft, replacing any
// previously-issued token for the same draft so the most recent link is
// the only valid one. Returns the absolute URL the user should visit.
func (a *UploadAPI) IssueUploadToken(ctx context.Context, draftID string) (token, url string, expires time.Time, err error) {
	buf := make([]byte, 24)
	if _, err = rand.Read(buf); err != nil {
		return "", "", time.Time{}, err
	}
	token = base64.RawURLEncoding.EncodeToString(buf)
	now := time.Now().UTC()
	expires = now.Add(UploadTokenTTL)

	if _, err = a.Store.DB.ExecContext(ctx,
		`UPDATE draft_upload_tokens SET revoked_at = ? WHERE draft_id = ? AND revoked_at IS NULL`,
		now.Format(time.RFC3339Nano), draftID); err != nil {
		return "", "", time.Time{}, err
	}
	if _, err = a.Store.DB.ExecContext(ctx,
		`INSERT INTO draft_upload_tokens(token, draft_id, created_at, expires_at) VALUES(?, ?, ?, ?)`,
		token, draftID, now.Format(time.RFC3339Nano), expires.Format(time.RFC3339Nano)); err != nil {
		return "", "", time.Time{}, err
	}
	url = fmt.Sprintf("%s/upload/%s", a.SiteURL, token)
	return token, url, expires, nil
}

// resolveToken returns the draft id for a valid (unexpired, unrevoked) token.
func (a *UploadAPI) resolveToken(ctx context.Context, token string) (string, error) {
	if token == "" {
		return "", sql.ErrNoRows
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var draftID string
	err := a.Store.DB.QueryRowContext(ctx, `
		SELECT draft_id FROM draft_upload_tokens
		WHERE token = ? AND revoked_at IS NULL AND expires_at > ?`,
		token, now).Scan(&draftID)
	return draftID, err
}

// uploadCategory describes the user-facing buckets in the web uploader,
// mirroring the bot's image step roles.
type uploadCategory struct {
	Role     string `json:"role"`
	Title    string `json:"title"`
	Multiple bool   `json:"multiple"`
}

// UploadCategories returns the categories the upload page renders.
func UploadCategories() []uploadCategory {
	return []uploadCategory{
		{Role: "avatar", Title: "头像（1 张）", Multiple: false},
		{Role: "intro", Title: "简介图片", Multiple: true},
		{Role: "life", Title: "生平与记忆图片", Multiple: true},
		{Role: "death", Title: "离世图片（如必要）", Multiple: true},
		{Role: "remembrance", Title: "念想图片", Multiple: true},
		{Role: "works", Title: "作品图片", Multiple: true},
		{Role: "custom", Title: "自选附加项图片", Multiple: true},
	}
}

func (a *UploadAPI) handleState(w http.ResponseWriter, r *http.Request) {
	tok := chi.URLParam(r, "token")
	draftID, err := a.resolveToken(r.Context(), tok)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "token_invalid"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	d, err := a.Store.Get(r.Context(), draftID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
		return
	}
	assets := []map[string]any{}
	for _, asset := range d.Assets {
		assets = append(assets, map[string]any{
			"id":       asset.ID,
			"role":     asset.Role,
			"filename": asset.Filename,
			"url":      "/media/" + asset.Path,
			"size":     asset.Size,
		})
	}
	// Build a profile-shaped preview so the upload page can render the
	// same look as the eventual public memorial detail. We use the
	// memorial markdown engine via the SetMarkdownRenderer hook.
	personPath := d.GetString("entry_id")
	if personPath == "" {
		personPath = d.ID
	}
	contentHTML := buildPreviewContentHTML(d, personPath)

	avatar := d.GetString("avatar")
	if avatar == "none" {
		avatar = ""
	}
	for _, asset := range d.Assets {
		if asset.Role == "avatar" {
			avatar = "/media/" + asset.Path
			break
		}
	}

	facts := []map[string]string{}
	for _, p := range []struct{ label, key string }{
		{"地区", "location"},
		{"出生日期", "birth_date"},
		{"逝世日期", "death_date"},
		{"昵称", "alias"},
		{"年龄", "age"},
		{"身份表述", "identity"},
		{"代词", "pronouns"},
	} {
		if v := d.GetString(p.key); v != "" {
			facts = append(facts, map[string]string{"label": p.label, "value": v})
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"draft": map[string]any{
			"id":           d.ID,
			"display_name": d.GetString("display_name"),
			"description":  d.GetString("description"),
			"entry_id":     d.GetString("entry_id"),
			"current_step": d.CurrentStep,
			"status":       d.Status,
		},
		"profile": map[string]any{
			"name":        d.GetString("display_name"),
			"desc":        d.GetString("description"),
			"departure":   d.GetString("death_date"),
			"profileUrl":  avatar,
			"facts":       facts,
			"contentHtml": contentHTML,
		},
		"categories": UploadCategories(),
		"assets":     assets,
	})
}

// buildPreviewContentHTML stitches the body sections of the draft into
// markdown and renders via the memorial engine.
func buildPreviewContentHTML(d *Draft, personPath string) string {
	var b strings.Builder
	for _, p := range []struct{ Title, Key string }{
		{"简介", "intro"},
		{"生平与记忆", "life"},
		{"离世", "death"},
		{"念想", "remembrance"},
	} {
		v := strings.TrimSpace(d.GetString(p.Key))
		if v == "" {
			continue
		}
		fmt.Fprintf(&b, "## %s\n\n%s\n\n", p.Title, v)
	}
	if renderMarkdownToHTML == nil {
		return ""
	}
	return renderMarkdownToHTML(b.String(), personPath)
}

func (a *UploadAPI) handleUpload(w http.ResponseWriter, r *http.Request) {
	tok := chi.URLParam(r, "token")
	draftID, err := a.resolveToken(r.Context(), tok)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "token_invalid"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bad_multipart"})
		return
	}
	role := strings.TrimSpace(r.FormValue("role"))
	if !validUploadRole(role) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_role"})
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing_file"})
		return
	}
	defer file.Close()
	if header.Size <= 0 || header.Size > 12*1024*1024 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "size_out_of_range"})
		return
	}
	contentType := header.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "must_be_image"})
		return
	}
	ext := filepath.Ext(header.Filename)
	if ext == "" {
		switch contentType {
		case "image/jpeg":
			ext = ".jpg"
		case "image/png":
			ext = ".png"
		case "image/webp":
			ext = ".webp"
		case "image/gif":
			ext = ".gif"
		default:
			ext = ".bin"
		}
	}

	d, err := a.Store.Get(r.Context(), draftID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
		return
	}

	// Avatar is single — replace any prior asset of role "avatar".
	if role == "avatar" {
		for _, asset := range d.Assets {
			if asset.Role == "avatar" {
				_ = removeAsset(r.Context(), a.Store, asset.ID, asset)
			}
		}
	}

	dir := a.Store.DraftDir(draftID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	name := role + "_" + strconv.FormatInt(time.Now().UnixNano(), 36) + ext
	target := filepath.Join(dir, name)
	out, err := os.Create(target)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	n, err := io.Copy(out, file)
	out.Close()
	if err != nil {
		_ = os.Remove(target)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	asset, err := a.Store.AddAsset(r.Context(), draftID, role, name, contentType, n)
	if err != nil {
		_ = os.Remove(target)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	// If the user is currently sitting on an avatar / image step in the
	// bot, advance the draft's step to the next non-image step so the
	// bot prompt updates next time they tap a button.
	updateDraftAfterUpload(r.Context(), a.Store, d, role)

	writeJSON(w, http.StatusCreated, map[string]any{
		"ok":    true,
		"asset": map[string]any{
			"id":       asset.ID,
			"role":     asset.Role,
			"url":      "/media/" + asset.Path,
			"filename": asset.Filename,
			"size":     asset.Size,
		},
	})
}

func (a *UploadAPI) handleDelete(w http.ResponseWriter, r *http.Request) {
	tok := chi.URLParam(r, "token")
	draftID, err := a.resolveToken(r.Context(), tok)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "token_invalid"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	idStr := chi.URLParam(r, "assetID")
	assetID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bad_id"})
		return
	}
	// Make sure the asset belongs to this token's draft before deleting.
	var asset Asset
	if err := a.Store.DB.QueryRowContext(r.Context(), `
		SELECT id, draft_id, role, filename, path, content_type, size, sort
		FROM draft_assets WHERE id = ? AND draft_id = ?`, assetID, draftID).Scan(
		&asset.ID, &asset.DraftID, &asset.Role, &asset.Filename, &asset.Path, &asset.ContentType, &asset.Size, &asset.Sort,
	); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
		return
	}
	if err := removeAsset(r.Context(), a.Store, asset.ID, asset); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func validUploadRole(role string) bool {
	for _, c := range UploadCategories() {
		if c.Role == role {
			return true
		}
	}
	return false
}

func removeAsset(ctx context.Context, store *Store, id int64, asset Asset) error {
	if asset.Path != "" {
		_ = os.Remove(store.AssetPath(&asset))
	}
	_, err := store.DB.ExecContext(ctx, `DELETE FROM draft_assets WHERE id = ?`, id)
	return err
}

// updateDraftAfterUpload nudges the current_step forward when the user
// finishes the avatar step or signals that they are done with an images
// step (best-effort — the bot will re-render its main message on next
// interaction). This is a write-only side-effect; callers ignore errors.
func updateDraftAfterUpload(ctx context.Context, store *Store, d *Draft, role string) {
	step, ok := FindStep(d.CurrentStep)
	if !ok {
		return
	}
	if step.Kind != StepImage {
		return
	}
	if step.AssetRole != role {
		return
	}
	d.CurrentStep = NextStep(step.Key).Key
	_ = store.Save(ctx, d)
}
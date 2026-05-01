package memorial

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ripyc/rip-lgbt-submission-tg/internal/auth"
	appdb "github.com/ripyc/rip-lgbt-submission-tg/internal/db"
	"github.com/ripyc/rip-lgbt-submission-tg/internal/markdown"
)

// AdminService exposes CRUD endpoints for already-published memorials.
// It lives next to the public service so that data shapes and rendering
// stay in sync.
type AdminService struct {
	DB *appdb.DB
}

func NewAdminService(db *appdb.DB) *AdminService { return &AdminService{DB: db} }

// Register installs /memorials/* admin routes onto the parent router.
func (a *AdminService) Register(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireLogin)
		r.Get("/memorials", a.handleList)
		r.Get("/memorials/{id}", a.handleGet)
		r.Put("/memorials/{id}", a.handleUpdate)
		r.Delete("/memorials/{id}", a.handleDelete)
		r.Get("/check-entry-id", a.handleCheckEntry)
	})
}

// AdminMemorial captures the full memorial row admins can edit.
type AdminMemorial struct {
	ID              string   `json:"id"`
	DisplayName     string   `json:"display_name"`
	AvatarURL       string   `json:"avatar_url"`
	Description     string   `json:"description"`
	Location        string   `json:"location"`
	BirthDate       string   `json:"birth_date"`
	DeathDate       string   `json:"death_date"`
	Alias           string   `json:"alias"`
	Age             string   `json:"age"`
	Identity        string   `json:"identity"`
	Pronouns        string   `json:"pronouns"`
	ContentWarnings []string `json:"content_warnings"`
	Intro           string   `json:"intro"`
	Life            string   `json:"life"`
	Death           string   `json:"death"`
	Remembrance     string   `json:"remembrance"`
	LinksMD         string   `json:"links_md"`
	WorksMD         string   `json:"works_md"`
	SourcesMD       string   `json:"sources_md"`
	CustomMD        string   `json:"custom_md"`
	EffectsMD       string   `json:"effects_md"`
	MarkdownFull    string   `json:"markdown_full"`
	Status          string   `json:"status"`
	Facts           []Fact   `json:"facts"`
	Websites        []Site   `json:"websites"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
}

func (a *AdminService) handleList(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	args := []any{}
	where := []string{"1=1"}
	if status != "" {
		where = append(where, "status = ?")
		args = append(args, status)
	}
	if q != "" {
		where = append(where, "(id LIKE ? OR display_name LIKE ?)")
		like := "%" + q + "%"
		args = append(args, like, like)
	}
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT id, display_name, description, death_date, status, created_at, updated_at
		FROM memorials WHERE `+strings.Join(where, " AND ")+`
		ORDER BY death_date DESC, id ASC`, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name, desc, death, st, c, u string
		if err := rows.Scan(&id, &name, &desc, &death, &st, &c, &u); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		out = append(out, map[string]any{
			"id": id, "display_name": name, "description": desc,
			"death_date": death, "status": st, "created_at": c, "updated_at": u,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"memorials": out, "count": len(out)})
}

func (a *AdminService) handleGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	row := a.DB.QueryRowContext(r.Context(), `
		SELECT id, display_name, avatar_url, description, location, birth_date, death_date,
		       alias, age, identity, pronouns, content_warnings,
		       intro, life, death, remembrance,
		       links_md, works_md, sources_md, custom_md, effects_md,
		       markdown_full, status, facts_json, websites_json,
		       created_at, updated_at
		FROM memorials WHERE id = ?`, id)
	m, err := scanAdminMemorial(row)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	// Re-render content for preview.
	contentHTML := markdown.Render(markdown.CleanMemorial(markdown.StripFrontmatter(m.MarkdownFull)), m.ID)
	writeJSON(w, http.StatusOK, map[string]any{"memorial": m, "content_html": contentHTML})
}

func (a *AdminService) handleUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req AdminMemorial
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bad_request"})
		return
	}
	// Always re-derive markdown_full / facts / websites from the
	// structured fields so admins cannot drift them apart by hand.
	req.MarkdownFull = GenerateMarkdown(&req)
	req.Facts = GenerateFacts(&req)
	req.Websites = GenerateWebsites(req.LinksMD)

	cw, _ := json.Marshal(req.ContentWarnings)
	facts, _ := json.Marshal(req.Facts)
	sites, _ := json.Marshal(req.Websites)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := a.DB.ExecContext(r.Context(), `
		UPDATE memorials SET display_name = ?, avatar_url = ?, description = ?, location = ?,
		    birth_date = ?, death_date = ?, alias = ?, age = ?, identity = ?, pronouns = ?,
		    content_warnings = ?, intro = ?, life = ?, death = ?, remembrance = ?,
		    links_md = ?, works_md = ?, sources_md = ?, custom_md = ?, effects_md = ?,
		    markdown_full = ?, status = ?, facts_json = ?, websites_json = ?, updated_at = ?
		WHERE id = ?`,
		req.DisplayName, req.AvatarURL, req.Description, req.Location,
		req.BirthDate, req.DeathDate, req.Alias, req.Age, req.Identity, req.Pronouns,
		string(cw), req.Intro, req.Life, req.Death, req.Remembrance,
		req.LinksMD, req.WorksMD, req.SourcesMD, req.CustomMD, req.EffectsMD,
		req.MarkdownFull, defaultStatus(req.Status), string(facts), string(sites), now,
		id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"markdown_full": req.MarkdownFull,
		"facts":         req.Facts,
		"websites":      req.Websites,
	})
}

func (a *AdminService) handleDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	hard := r.URL.Query().Get("hard") == "1"
	if hard {
		if _, err := a.DB.ExecContext(r.Context(), `DELETE FROM memorials WHERE id = ?`, id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
	} else {
		// Soft: flip status to "archived" so the public list ignores it
		// while the row (and its assets / engagement counters) remain.
		if _, err := a.DB.ExecContext(r.Context(), `
			UPDATE memorials SET status = 'archived', updated_at = ? WHERE id = ?`,
			time.Now().UTC().Format(time.RFC3339Nano), id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleCheckEntry exposes the same dedup logic the bot uses, so the admin
// edit form can verify an id before saving.
func (a *AdminService) handleCheckEntry(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	exclude := r.URL.Query().Get("exclude_draft")
	// We can't import submission here without circular imports, so inline
	// the check inline. (Only the small CheckEntryID logic is duplicated.)
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing id"})
		return
	}
	var n int
	if err := a.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM memorials WHERE id = ?`, id).Scan(&n); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if n > 0 {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "reason": "memorial"})
		return
	}
	args := []any{id, "collecting", "review", "revising"}
	q := `SELECT COUNT(*) FROM drafts WHERE deleted_at IS NULL
		AND status IN (?, ?, ?)
		AND json_extract(payload_json, '$.entry_id') = ?`
	args = []any{}
	args = append(args, "collecting", "review", "revising", id)
	q = `SELECT COUNT(*) FROM drafts WHERE deleted_at IS NULL
		AND status IN (?, ?, ?)
		AND json_extract(payload_json, '$.entry_id') = ?`
	if exclude != "" {
		q += " AND id != ?"
		args = append(args, exclude)
	}
	if err := a.DB.QueryRowContext(r.Context(), q, args...).Scan(&n); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if n > 0 {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "reason": "draft"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func scanAdminMemorial(row *sql.Row) (*AdminMemorial, error) {
	var m AdminMemorial
	var contentWarnings, factsJSON, sitesJSON string
	if err := row.Scan(
		&m.ID, &m.DisplayName, &m.AvatarURL, &m.Description, &m.Location,
		&m.BirthDate, &m.DeathDate, &m.Alias, &m.Age, &m.Identity, &m.Pronouns,
		&contentWarnings, &m.Intro, &m.Life, &m.Death, &m.Remembrance,
		&m.LinksMD, &m.WorksMD, &m.SourcesMD, &m.CustomMD, &m.EffectsMD,
		&m.MarkdownFull, &m.Status, &factsJSON, &sitesJSON,
		&m.CreatedAt, &m.UpdatedAt,
	); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(contentWarnings), &m.ContentWarnings)
	_ = json.Unmarshal([]byte(factsJSON), &m.Facts)
	_ = json.Unmarshal([]byte(sitesJSON), &m.Websites)
	return &m, nil
}

func defaultStatus(s string) string {
	if s == "" {
		return "published"
	}
	return s
}

// MarkdownPreview renders the content markdown to HTML for the admin preview pane.
func MarkdownPreview(raw, personPath string) string {
	return markdown.Render(markdown.CleanMemorial(markdown.StripFrontmatter(raw)), personPath)
}

// dummyContextKey silences unused-import warnings if we drop the auth import later.
var _ context.Context
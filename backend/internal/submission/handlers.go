package submission

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ripyc/rip-lgbt-submission-tg/internal/auth"
)

// AdminService exposes draft endpoints used by /api/admin/drafts.
type AdminService struct {
	Store    *Store
	Notifier Notifier
}

// Notifier delivers user-visible messages (typically through the bot).
type Notifier interface {
	NotifyUser(ctx context.Context, draft *Draft, kind, message string) error
	NotifyAdmins(ctx context.Context, draft *Draft, kind, message string) error
}

func NewAdminService(store *Store, notifier Notifier) *AdminService {
	return &AdminService{Store: store, Notifier: notifier}
}

// Register hooks /drafts/* endpoints onto a chi router. The caller is
// responsible for adding any prefix; this method only wraps with RequireLogin.
func (s *AdminService) Register(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireLogin)
		r.Get("/drafts", s.handleList)
		r.Get("/drafts/{id}", s.handleGet)
		r.Get("/drafts/{id}/preview", s.handlePreviewData)
		r.Post("/drafts/{id}/accept", s.handleAccept)
		r.Post("/drafts/{id}/reject", s.handleReject)
		r.Post("/drafts/{id}/request-revision", s.handleRequestRevision)
	})
}

func (s *AdminService) handleList(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status == "" {
		status = StatusReview
	}
	drafts, err := s.Store.ListByStatus(r.Context(), status, false)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	out := make([]map[string]any, 0, len(drafts))
	for _, d := range drafts {
		out = append(out, map[string]any{
			"id":                    d.ID,
			"status":                d.Status,
			"submitter_telegram_id": d.SubmitterTelegramID,
			"display_name":          d.GetString("display_name"),
			"entry_id":              d.GetString("entry_id"),
			"created_at":            d.CreatedAt,
			"updated_at":            d.UpdatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"drafts": out})
}

func (s *AdminService) handleGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	d, err := s.Store.Get(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"draft": map[string]any{
			"id":                    d.ID,
			"status":                d.Status,
			"submitter_telegram_id": d.SubmitterTelegramID,
			"current_step":          d.CurrentStep,
			"payload":               d.Payload,
			"assets":                d.Assets,
			"markdown_full":         BuildMarkdown(d),
			"created_at":            d.CreatedAt,
			"updated_at":            d.UpdatedAt,
		},
	})
}

func (s *AdminService) handlePreviewData(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	d, err := s.Store.Get(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"draft": map[string]any{
			"id":           d.ID,
			"display_name": d.GetString("display_name"),
			"description":  d.GetString("description"),
		},
		"profile": map[string]any{
			"name":        d.GetString("display_name"),
			"desc":        d.GetString("description"),
			"contentHtml": markdownPreviewHTML(d),
		},
	})
}

func (s *AdminService) handleAccept(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	caller := auth.FromContext(r.Context())
	d, err := s.Store.Get(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
		return
	}
	var reviewerID int64
	if caller != nil {
		reviewerID = caller.ID
	}
	if err := AcceptDraft(r.Context(), s.Store, d, reviewerID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if s.Notifier != nil {
		_ = s.Notifier.NotifyUser(r.Context(), d, "accepted",
			"你的投稿已被接受并发布，感谢你为这位逝者留下名字。")
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type rejectRequest struct {
	Reason string `json:"reason"`
}

func (s *AdminService) handleReject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req rejectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bad_request"})
		return
	}
	d, err := s.Store.Get(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
		return
	}
	d.Status = StatusRejected
	d.RejectionReason = req.Reason
	if err := s.Store.Save(r.Context(), d); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	_ = s.Store.SoftDelete(r.Context(), d.ID)
	if s.Notifier != nil {
		message := "你的投稿未被接受。"
		if req.Reason != "" {
			message += " 维护者留言：\n" + req.Reason
		}
		_ = s.Notifier.NotifyUser(r.Context(), d, "rejected", message)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type revisionRequest struct {
	Section string `json:"section"`
	Note    string `json:"note"`
}

func (s *AdminService) handleRequestRevision(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req revisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bad_request"})
		return
	}
	if _, ok := FindStep(req.Section); !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unknown_section"})
		return
	}
	d, err := s.Store.Get(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
		return
	}
	d.Status = StatusRevising
	d.RevisingSection = req.Section
	d.CurrentStep = req.Section
	if err := s.Store.Save(r.Context(), d); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if s.Notifier != nil {
		step, _ := FindStep(req.Section)
		message := "管理员请你修改这一节：" + step.Title
		if req.Note != "" {
			message += "\n备注：" + req.Note
		}
		message += "\n\n直接发送新的内容给机器人即可。"
		_ = s.Notifier.NotifyUser(r.Context(), d, "revision", message)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// markdownPreviewHTML renders a basic preview by rebuilding the markdown
// blob from the draft and feeding it through the memorial markdown engine.
// Implemented as a function variable so the server can wire it to
// internal/markdown without importing it into this package directly.
var markdownPreviewHTML = func(d *Draft) string { return "" }

// SetPreviewRenderer overrides the default no-op renderer.
func SetPreviewRenderer(fn func(d *Draft) string) {
	if fn != nil {
		markdownPreviewHTML = fn
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("content-type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// for compile
var _ = errors.New

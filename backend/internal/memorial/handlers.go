package memorial

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Routes returns the public sub-router.
func (s *Service) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/memorials", s.handleList)
	r.Get("/memorials/{id}", s.handleGet)
	r.Get("/memorials/{id}/engagement", s.handleEngagement)
	r.Post("/memorials/{id}/flowers", s.handleAddFlower)
	r.Get("/memorials/{id}/comments", s.handleEngagement) // alias
	r.Post("/memorials/{id}/comments", s.handleAddComment)
	return r
}

func (s *Service) handleList(w http.ResponseWriter, r *http.Request) {
	people, err := s.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"site": "勿忘我", "count": len(people), "people": people})
}

func (s *Service) handleGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, err := s.GetProfile(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Service) handleEngagement(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sum, err := s.GetEngagement(r.Context(), id)
	if errors.Is(err, errNoMemorial) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, sum)
}

type commentRequest struct {
	Author  string `json:"author"`
	Content string `json:"content"`
	Website string `json:"website,omitempty"`
}

func (s *Service) handleAddComment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req commentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bad_request"})
		return
	}
	if req.Website != "" {
		writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "ignored": true})
		return
	}
	c, err := s.AddComment(r.Context(), id, req.Author, req.Content, s.HashVisitor(r))
	switch {
	case errors.Is(err, errNoMemorial):
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
		return
	case errors.Is(err, errEmptyContent):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "empty_content", "message": "留言不能为空。"})
		return
	case errors.Is(err, errCooldown):
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": "too_fast", "message": "留言太快了，请稍后再试。"})
		return
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	sum, _ := s.GetEngagement(r.Context(), id)
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "comment": c, "summary": sum})
}

func (s *Service) handleAddFlower(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	counted, total, err := s.AddFlower(r.Context(), id, s.HashVisitor(r))
	if errors.Is(err, errNoMemorial) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "counted": counted, "flowers": total})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("content-type", "application/json; charset=utf-8")
	w.Header().Set("cache-control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

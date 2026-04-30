// Package memorial implements the public-facing memorial features:
// listing, detail, comments and flowers (engagement).
package memorial

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	appdb "github.com/ripyc/rip-lgbt-submission-tg/internal/db"
	"github.com/ripyc/rip-lgbt-submission-tg/internal/markdown"
)

// Limits ported from frontend.js.
const (
	CommentLimit         = 1000
	AuthorLimit          = 40
	CommentCooldown      = 30 * time.Second
	FlowerCooldown       = 24 * time.Hour
	SubmissionCooldown   = 120 * time.Second
	commentsPerMemorial  = 200
)

// Person is the summary form returned by the list endpoint and embedded inside Profile.
type Person struct {
	ID          string  `json:"id"`
	Path        string  `json:"path"`
	Name        string  `json:"name"`
	Desc        string  `json:"desc"`
	Departure   string  `json:"departure"`
	ProfileURL  string  `json:"profileUrl"`
	Facts       []Fact  `json:"facts"`
	Websites    []Site  `json:"websites"`
}

// Fact is one row in the public-info table on the detail page.
type Fact struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// Site is one external link.
type Site struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// Profile is the full detail-page payload.
type Profile struct {
	Person
	ContentHTML string `json:"contentHtml"`
}

// EngagementSummary bundles flowers + comments for a memorial.
type EngagementSummary struct {
	Flowers  int       `json:"flowers"`
	Comments []Comment `json:"comments"`
}

// Comment is one user message.
type Comment struct {
	ID        string `json:"id"`
	Author    string `json:"author"`
	Content   string `json:"content"`
	CreatedAt string `json:"createdAt"`
}

// Service exposes memorial features over HTTP.
type Service struct {
	db     *appdb.DB
	pepper string
}

// NewService wires the public memorial service.
func NewService(db *appdb.DB, ipPepper string) *Service {
	return &Service{db: db, pepper: ipPepper}
}

// --- Listing ---

// List returns published memorials ordered by death_date desc.
func (s *Service) List(ctx context.Context) ([]Person, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, display_name, description, death_date, avatar_url, facts_json, websites_json
		FROM memorials WHERE status = 'published'
		ORDER BY death_date DESC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Person
	for rows.Next() {
		var p Person
		var factsJSON, sitesJSON string
		if err := rows.Scan(&p.ID, &p.Name, &p.Desc, &p.Departure, &p.ProfileURL, &factsJSON, &sitesJSON); err != nil {
			return nil, err
		}
		p.Path = p.ID
		_ = json.Unmarshal([]byte(factsJSON), &p.Facts)
		_ = json.Unmarshal([]byte(sitesJSON), &p.Websites)
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetProfile returns the detail-page payload, including rendered markdown.
func (s *Service) GetProfile(ctx context.Context, id string) (*Profile, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, display_name, description, death_date, avatar_url, facts_json, websites_json, markdown_full
		FROM memorials WHERE id = ? AND status = 'published'`, id)
	var p Profile
	var factsJSON, sitesJSON, raw string
	if err := row.Scan(&p.ID, &p.Name, &p.Desc, &p.Departure, &p.ProfileURL, &factsJSON, &sitesJSON, &raw); err != nil {
		return nil, err
	}
	p.Path = p.ID
	_ = json.Unmarshal([]byte(factsJSON), &p.Facts)
	_ = json.Unmarshal([]byte(sitesJSON), &p.Websites)
	p.ContentHTML = markdown.Render(markdown.CleanMemorial(markdown.StripFrontmatter(raw)), p.Path)
	return &p, nil
}

// --- Engagement ---

// GetEngagement returns flowers count + last 200 comments.
func (s *Service) GetEngagement(ctx context.Context, memorialID string) (*EngagementSummary, error) {
	if err := s.assertMemorial(ctx, memorialID); err != nil {
		return nil, err
	}
	out := &EngagementSummary{}
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(total, 0) FROM flowers WHERE memorial_id = ?`, memorialID).Scan(&out.Flowers); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, author, content, created_at
		FROM comments
		WHERE memorial_id = ? AND hidden_at IS NULL
		ORDER BY created_at DESC LIMIT ?`, memorialID, commentsPerMemorial)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.Author, &c.Content, &c.CreatedAt); err != nil {
			return nil, err
		}
		out.Comments = append(out.Comments, c)
	}
	return out, rows.Err()
}

// AddComment inserts a comment subject to length / cooldown / honeypot rules.
func (s *Service) AddComment(ctx context.Context, memorialID, author, content, ipHash string) (*Comment, error) {
	if err := s.assertMemorial(ctx, memorialID); err != nil {
		return nil, err
	}
	author = cleanAuthor(author)
	content = cleanContent(content)
	if content == "" {
		return nil, errEmptyContent
	}

	var lastAt sql.NullString
	if err := s.db.QueryRowContext(ctx, `
		SELECT created_at FROM comments WHERE memorial_id = ? AND ip_hash = ?
		ORDER BY created_at DESC LIMIT 1`, memorialID, ipHash).Scan(&lastAt); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if lastAt.Valid {
		if t, err := time.Parse(time.RFC3339Nano, lastAt.String); err == nil && time.Since(t) < CommentCooldown {
			return nil, errCooldown
		}
	}

	c := &Comment{
		ID:        uuid.NewString(),
		Author:    author,
		Content:   content,
		CreatedAt: appdb.Now(),
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO comments(id, memorial_id, author, content, ip_hash, created_at)
		VALUES(?, ?, ?, ?, ?, ?)`, c.ID, memorialID, c.Author, c.Content, ipHash, c.CreatedAt); err != nil {
		return nil, err
	}
	return c, nil
}

// AddFlower atomically increments the flower counter, with one bouquet per ip / 24h.
func (s *Service) AddFlower(ctx context.Context, memorialID, ipHash string) (counted bool, total int, err error) {
	if err = s.assertMemorial(ctx, memorialID); err != nil {
		return false, 0, err
	}
	var lastAt sql.NullString
	if err = s.db.QueryRowContext(ctx, `
		SELECT created_at FROM flower_events WHERE memorial_id = ? AND ip_hash = ?
		ORDER BY created_at DESC LIMIT 1`, memorialID, ipHash).Scan(&lastAt); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, 0, err
	}
	now := appdb.Now()
	if lastAt.Valid {
		if t, err := time.Parse(time.RFC3339Nano, lastAt.String); err == nil && time.Since(t) < FlowerCooldown {
			err = nil
			total, _ = s.flowerTotal(ctx, memorialID)
			return false, total, nil
		}
	}

	if _, err = s.db.ExecContext(ctx, `INSERT INTO flower_events(memorial_id, ip_hash, created_at) VALUES(?, ?, ?)`,
		memorialID, ipHash, now); err != nil {
		return false, 0, err
	}
	if _, err = s.db.ExecContext(ctx, `
		INSERT INTO flowers(memorial_id, total, updated_at) VALUES(?, 1, ?)
		ON CONFLICT(memorial_id) DO UPDATE SET total = total + 1, updated_at = excluded.updated_at`,
		memorialID, now); err != nil {
		return false, 0, err
	}

	total, err = s.flowerTotal(ctx, memorialID)
	return err == nil, total, err
}

func (s *Service) flowerTotal(ctx context.Context, memorialID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(total, 0) FROM flowers WHERE memorial_id = ?`, memorialID).Scan(&n)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	return n, nil
}

// HashVisitor produces a stable visitor identifier from the request IP using
// the configured pepper. Mirrors hashVisitor() in frontend.js.
func (s *Service) HashVisitor(r *http.Request) string {
	ip := r.RemoteAddr
	if forward := r.Header.Get("CF-Connecting-IP"); forward != "" {
		ip = forward
	} else if forward := r.Header.Get("X-Forwarded-For"); forward != "" {
		parts := strings.Split(forward, ",")
		ip = strings.TrimSpace(parts[0])
	}
	host, _, err := net.SplitHostPort(ip)
	if err == nil {
		ip = host
	}
	sum := sha256.Sum256([]byte(s.pepper + "|" + ip))
	return hex.EncodeToString(sum[:])
}

// --- helpers ---

var (
	errEmptyContent = errors.New("empty_content")
	errCooldown     = errors.New("too_fast")
	errNoMemorial   = errors.New("not_found")
)

// IsCooldown reports whether the error came from a rate limiter.
func IsCooldown(err error) bool { return errors.Is(err, errCooldown) }

// IsNotFound reports whether the error came from a missing memorial.
func IsNotFound(err error) bool { return errors.Is(err, errNoMemorial) }

// IsEmptyContent reports whether the error came from an empty comment.
func IsEmptyContent(err error) bool { return errors.Is(err, errEmptyContent) }

func (s *Service) assertMemorial(ctx context.Context, id string) error {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memorials WHERE id = ? AND status = 'published'`, id).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return errNoMemorial
	}
	return nil
}

func cleanAuthor(value string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		v = "访客"
	}
	r := []rune(v)
	if len(r) > AuthorLimit {
		r = r[:AuthorLimit]
	}
	return string(r)
}

func cleanContent(value string) string {
	v := strings.TrimSpace(value)
	r := []rune(v)
	if len(r) > CommentLimit {
		r = r[:CommentLimit]
	}
	return string(r)
}

func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

// for tests
var _ = fmt.Sprintf

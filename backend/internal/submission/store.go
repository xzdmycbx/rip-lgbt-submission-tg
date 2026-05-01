// Package submission owns drafts: storage, state machine helpers, and the
// flow used by both the TG bot and the admin web UI.
package submission

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	appdb "github.com/ripyc/rip-lgbt-submission-tg/internal/db"
)

// Draft status values.
const (
	StatusCollecting = "collecting"
	StatusReview     = "review"
	StatusRevising   = "revising"
	StatusAccepted   = "accepted"
	StatusRejected   = "rejected"
)

// Draft is the storage representation of an in-progress submission.
type Draft struct {
	ID                  string         `json:"id"`
	SubmitterTelegramID int64          `json:"submitter_telegram_id"`
	SubmitterChatID     int64          `json:"submitter_chat_id"`
	Status              string         `json:"status"`
	CurrentStep         string         `json:"current_step"`
	Payload             map[string]any `json:"payload"`
	RejectionReason     string         `json:"rejection_reason"`
	RevisingSection     string         `json:"revising_section"`
	ReviewerAdminID     sql.NullInt64  `json:"-"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	DeletedAt           sql.NullString `json:"-"`
	Assets              []Asset        `json:"assets,omitempty"`
}

// Asset is a single uploaded file.
type Asset struct {
	ID          int64  `json:"id"`
	DraftID     string `json:"draft_id"`
	Role        string `json:"role"`
	Filename    string `json:"filename"`
	Path        string `json:"path"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	Sort        int    `json:"sort"`
}

// Store wraps draft persistence.
type Store struct {
	DB        *appdb.DB
	UploadDir string
}

func NewStore(db *appdb.DB, uploadDir string) *Store {
	return &Store{DB: db, UploadDir: uploadDir}
}

// FindOpenByTelegram returns the latest collecting/revising draft for a user, if any.
func (s *Store) FindOpenByTelegram(ctx context.Context, tgID int64) (*Draft, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT id, submitter_telegram_id, submitter_chat_id, status, current_step, payload_json,
		       rejection_reason, revising_section, reviewer_admin_id, created_at, updated_at, deleted_at
		FROM drafts
		WHERE submitter_telegram_id = ? AND status IN (?, ?) AND deleted_at IS NULL
		ORDER BY updated_at DESC LIMIT 1`, tgID, StatusCollecting, StatusRevising)
	return scanDraft(row)
}

// Get fetches a draft by id (including soft-deleted).
func (s *Store) Get(ctx context.Context, id string) (*Draft, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT id, submitter_telegram_id, submitter_chat_id, status, current_step, payload_json,
		       rejection_reason, revising_section, reviewer_admin_id, created_at, updated_at, deleted_at
		FROM drafts WHERE id = ?`, id)
	d, err := scanDraft(row)
	if err != nil {
		return nil, err
	}
	d.Assets, err = s.assetsForDraft(ctx, d.ID)
	if err != nil {
		return nil, err
	}
	return d, nil
}

// ListByStatus returns drafts in the given status (excluding soft-deleted unless asked).
func (s *Store) ListByStatus(ctx context.Context, status string, includeDeleted bool) ([]*Draft, error) {
	q := `
		SELECT id, submitter_telegram_id, submitter_chat_id, status, current_step, payload_json,
		       rejection_reason, revising_section, reviewer_admin_id, created_at, updated_at, deleted_at
		FROM drafts WHERE status = ?`
	if !includeDeleted {
		q += ` AND deleted_at IS NULL`
	}
	q += ` ORDER BY updated_at DESC`
	rows, err := s.DB.QueryContext(ctx, q, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Draft
	for rows.Next() {
		d, err := scanDraft(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Create starts a new collecting draft for a TG user.
func (s *Store) Create(ctx context.Context, tgID, chatID int64) (*Draft, error) {
	d := &Draft{
		ID:                  uuid.NewString(),
		SubmitterTelegramID: tgID,
		SubmitterChatID:     chatID,
		Status:              StatusCollecting,
		CurrentStep:         "",
		Payload:             map[string]any{},
		CreatedAt:           time.Now().UTC(),
		UpdatedAt:           time.Now().UTC(),
	}
	if _, err := s.DB.ExecContext(ctx, `
		INSERT INTO drafts(id, submitter_telegram_id, submitter_chat_id, status, current_step, payload_json,
			created_at, updated_at)
		VALUES(?, ?, ?, ?, '', '{}', ?, ?)`,
		d.ID, tgID, chatID, StatusCollecting, appdb.Now(), appdb.Now()); err != nil {
		return nil, err
	}
	return d, nil
}

// Save updates payload, status, current_step, and timestamps.
func (s *Store) Save(ctx context.Context, d *Draft) error {
	payloadJSON, err := json.Marshal(d.Payload)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `
		UPDATE drafts SET status = ?, current_step = ?, payload_json = ?, rejection_reason = ?,
		                  revising_section = ?, updated_at = ?
		WHERE id = ?`,
		d.Status, d.CurrentStep, string(payloadJSON), d.RejectionReason, d.RevisingSection, appdb.Now(),
		d.ID,
	)
	if err == nil {
		// Tokens stay valid only while the draft is actively being
		// edited. Once it moves into review / accepted / rejected we
		// pre-emptively revoke any outstanding upload links.
		switch d.Status {
		case StatusReview, StatusAccepted, StatusRejected:
			_ = s.RevokeUploadTokens(ctx, d.ID)
		}
	}
	return err
}

// RevokeUploadTokens marks every active token for a draft as revoked.
func (s *Store) RevokeUploadTokens(ctx context.Context, draftID string) error {
	_, err := s.DB.ExecContext(ctx,
		`UPDATE draft_upload_tokens SET revoked_at = ? WHERE draft_id = ? AND revoked_at IS NULL`,
		appdb.Now(), draftID)
	return err
}

// SoftDelete marks a draft as deleted.
func (s *Store) SoftDelete(ctx context.Context, id string) error {
	if _, err := s.DB.ExecContext(ctx, `UPDATE drafts SET deleted_at = ?, updated_at = ? WHERE id = ?`,
		appdb.Now(), appdb.Now(), id); err != nil {
		return err
	}
	return s.RevokeUploadTokens(ctx, id)
}

// PurgeOlderThan permanently deletes drafts soft-deleted before cutoff.
func (s *Store) PurgeOlderThan(ctx context.Context, cutoff time.Time) (int, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id FROM drafts WHERE deleted_at IS NOT NULL AND deleted_at < ?`,
		cutoff.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		ids = append(ids, id)
	}
	for _, id := range ids {
		if dir := s.draftDir(id); dir != "" {
			_ = os.RemoveAll(dir)
		}
		if _, err := s.DB.ExecContext(ctx, `DELETE FROM drafts WHERE id = ?`, id); err != nil {
			return 0, err
		}
	}
	return len(ids), nil
}

// AddAsset persists a new asset (file already on disk).
func (s *Store) AddAsset(ctx context.Context, draftID, role, filename, contentType string, size int64) (*Asset, error) {
	dir := s.draftDir(draftID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	rel := filepath.ToSlash(filepath.Join(s.relDraftDir(draftID), filename))
	res, err := s.DB.ExecContext(ctx, `
		INSERT INTO draft_assets(draft_id, role, filename, path, content_type, size, sort)
		VALUES(?, ?, ?, ?, ?, ?, 0)`, draftID, role, filename, rel, contentType, size)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &Asset{ID: id, DraftID: draftID, Role: role, Filename: filename, Path: rel, ContentType: contentType, Size: size}, nil
}

// AssetPath returns the absolute path on disk for a stored asset.
func (s *Store) AssetPath(a *Asset) string { return filepath.Join(s.UploadDir, filepath.FromSlash(a.Path)) }

// DraftDir returns the directory where a draft's assets live.
func (s *Store) DraftDir(draftID string) string { return s.draftDir(draftID) }

func (s *Store) draftDir(draftID string) string {
	if s.UploadDir == "" || draftID == "" {
		return ""
	}
	return filepath.Join(s.UploadDir, "drafts", draftID)
}

func (s *Store) relDraftDir(draftID string) string {
	return filepath.ToSlash(filepath.Join("drafts", draftID))
}

// Bot message tracking
func (s *Store) RecordMessage(ctx context.Context, draftID string, chatID, messageID int64, kind string) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO draft_messages(draft_id, telegram_chat_id, telegram_message_id, kind, created_at)
		VALUES(?, ?, ?, ?, ?)`,
		draftID, chatID, messageID, kind, appdb.Now())
	return err
}

// LatestMainMessage returns the latest "main" bot message for the draft, used to
// know which message to edit on the next step.
func (s *Store) LatestMainMessage(ctx context.Context, draftID string) (chatID, messageID int64, err error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT telegram_chat_id, telegram_message_id FROM draft_messages
		WHERE draft_id = ? AND kind = 'main' ORDER BY id DESC LIMIT 1`, draftID)
	err = row.Scan(&chatID, &messageID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, nil
	}
	return chatID, messageID, err
}

// ReplaceMainMessage replaces (kind='main') with a new message id.
func (s *Store) ReplaceMainMessage(ctx context.Context, draftID string, chatID, messageID int64) error {
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM draft_messages WHERE draft_id = ? AND kind = 'main'`, draftID); err != nil {
		return err
	}
	return s.RecordMessage(ctx, draftID, chatID, messageID, "main")
}

func (s *Store) assetsForDraft(ctx context.Context, draftID string) ([]Asset, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, draft_id, role, filename, path, content_type, size, sort
		FROM draft_assets WHERE draft_id = ? ORDER BY id`, draftID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Asset
	for rows.Next() {
		var a Asset
		if err := rows.Scan(&a.ID, &a.DraftID, &a.Role, &a.Filename, &a.Path, &a.ContentType, &a.Size, &a.Sort); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func scanDraft(r interface{ Scan(...any) error }) (*Draft, error) {
	var d Draft
	var payload string
	var created, updated string
	var deleted sql.NullString
	if err := r.Scan(
		&d.ID, &d.SubmitterTelegramID, &d.SubmitterChatID, &d.Status, &d.CurrentStep, &payload,
		&d.RejectionReason, &d.RevisingSection, &d.ReviewerAdminID, &created, &updated, &deleted,
	); err != nil {
		return nil, err
	}
	if payload != "" {
		_ = json.Unmarshal([]byte(payload), &d.Payload)
	}
	if d.Payload == nil {
		d.Payload = map[string]any{}
	}
	d.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	d.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	d.DeletedAt = deleted
	return &d, nil
}

// SetStringField stores a string value into payload.
func (d *Draft) SetStringField(key, value string) {
	if d.Payload == nil {
		d.Payload = map[string]any{}
	}
	d.Payload[key] = strings.TrimSpace(value)
}

// GetString fetches a string field (empty if missing).
func (d *Draft) GetString(key string) string {
	if d.Payload == nil {
		return ""
	}
	if s, ok := d.Payload[key].(string); ok {
		return s
	}
	return ""
}

// for tests
var _ = fmt.Sprintf

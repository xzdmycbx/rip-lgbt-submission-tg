package submission

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"

	appdb "github.com/ripyc/rip-lgbt-submission-tg/internal/db"
)

// EntryIDPattern matches the format originally enforced by frontend.js:
// 2–80 chars of letters, digits, dashes, underscores, or spaces.
var EntryIDPattern = regexp.MustCompile(`^[A-Za-z0-9_ -]{2,80}$`)

// EntryIDStatus reports the result of a uniqueness check.
type EntryIDStatus int

const (
	EntryIDOK EntryIDStatus = iota
	EntryIDInvalid
	EntryIDTakenByMemorial
	EntryIDTakenByDraft
)

// CheckEntryID validates the format and uniqueness of an entry id, optionally
// excluding a specific draft (so a draft can re-confirm its own id without
// tripping on itself).
func CheckEntryID(ctx context.Context, db *appdb.DB, raw, excludeDraftID string) (EntryIDStatus, error) {
	id := strings.TrimSpace(raw)
	if !EntryIDPattern.MatchString(id) {
		return EntryIDInvalid, nil
	}

	// Check published memorials.
	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM memorials WHERE id = ?`, id).Scan(&n); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return EntryIDOK, err
		}
	}
	if n > 0 {
		return EntryIDTakenByMemorial, nil
	}

	// Check active drafts. We look at any draft that hasn't been
	// soft-deleted and isn't the current one.
	args := []any{id, StatusCollecting, StatusReview, StatusRevising}
	q := `SELECT COUNT(*) FROM drafts
		WHERE deleted_at IS NULL
		  AND status IN (?, ?, ?)
		  AND json_extract(payload_json, '$.entry_id') = ?`
	args = append(args[:0], StatusCollecting, StatusReview, StatusRevising, id)
	if excludeDraftID != "" {
		q += ` AND id != ?`
		args = append(args, excludeDraftID)
	}
	if err := db.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return EntryIDOK, err
		}
	}
	if n > 0 {
		return EntryIDTakenByDraft, nil
	}
	return EntryIDOK, nil
}

// EntryIDStatusMessage returns a Chinese message describing the status.
func EntryIDStatusMessage(s EntryIDStatus) string {
	switch s {
	case EntryIDOK:
		return ""
	case EntryIDInvalid:
		return "条目 ID 仅允许英文、数字、下划线、短横线或空格，长度 2–80。"
	case EntryIDTakenByMemorial:
		return "这个条目 ID 已经被某个已发布的纪念条目使用，请换一个。"
	case EntryIDTakenByDraft:
		return "这个条目 ID 已经被另一份正在审核 / 收集的投稿占用，请换一个。"
	default:
		return "条目 ID 不可用。"
	}
}

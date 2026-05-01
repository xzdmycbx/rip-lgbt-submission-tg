package submission

import (
	"context"
	"log/slog"
	"strings"
)

// MigrateAcceptedDraftAssets is a startup migration that repairs memorials
// created before the relocation fix. It is safe to call on every startup
// because all operations are idempotent:
//
//   - Files still under drafts/ are moved to memorials/<entry_id>/
//   - memorial_assets rows are inserted for any missing entries
//   - memorials.avatar_url is corrected if it still points to a drafts/ path
func MigrateAcceptedDraftAssets(ctx context.Context, store *Store, logger *slog.Logger) {
	if store.UploadDir == "" {
		return
	}

	rows, err := store.DB.QueryContext(ctx, `
		SELECT d.id, json_extract(d.payload_json, '$.entry_id')
		FROM drafts d
		WHERE d.status = 'accepted'
		  AND d.deleted_at IS NULL
		  AND json_extract(d.payload_json, '$.entry_id') IS NOT NULL
		  AND json_extract(d.payload_json, '$.entry_id') != ''
	`)
	if err != nil {
		logger.Warn("migrate draft assets: query", "err", err)
		return
	}
	defer rows.Close()

	type entry struct{ draftID, entryID string }
	var all []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.draftID, &e.entryID); err != nil {
			logger.Warn("migrate draft assets: scan", "err", err)
			return
		}
		all = append(all, e)
	}
	if err := rows.Err(); err != nil {
		logger.Warn("migrate draft assets: rows", "err", err)
		return
	}
	rows.Close()

	fixed := 0
	for _, e := range all {
		if err := migrateOneDraft(ctx, store, e.draftID, e.entryID); err != nil {
			logger.Warn("migrate draft assets: one draft", "draft_id", e.draftID, "entry_id", e.entryID, "err", err)
		} else {
			fixed++
		}
	}
	if fixed > 0 {
		logger.Info("migrate draft assets: done", "count", fixed)
	}
}

func migrateOneDraft(ctx context.Context, store *Store, draftID, entryID string) error {
	assets, err := store.assetsForDraft(ctx, draftID)
	if err != nil || len(assets) == 0 {
		return err
	}

	dstRel := "memorials/" + entryID
	dstDir := storeAbs(store, dstRel)
	if err := osMkdirAll(dstDir, 0o755); err != nil {
		return err
	}

	var avatarNewPath string
	for _, a := range assets {
		currentPath := a.Path

		if strings.HasPrefix(currentPath, "drafts/") {
			newRel := dstRel + "/" + a.Filename
			if err := osRename(storeAbs(store, currentPath), storeAbs(store, newRel)); err == nil {
				_, _ = store.DB.ExecContext(ctx,
					`UPDATE draft_assets SET path = ? WHERE id = ?`, newRel, a.ID)
				currentPath = newRel
			}
		}

		insertMemorialAsset(ctx, store, entryID, a, currentPath)

		if a.Role == "avatar" && avatarNewPath == "" && strings.HasPrefix(currentPath, "memorials/") {
			avatarNewPath = currentPath
		}
	}

	// Fix avatar_url only when it still points to the old draft location.
	if avatarNewPath != "" {
		_, _ = store.DB.ExecContext(ctx, `
			UPDATE memorials SET avatar_url = ?
			WHERE id = ? AND avatar_url LIKE '/media/drafts/%'`,
			"/media/"+avatarNewPath, entryID)
	}
	return nil
}

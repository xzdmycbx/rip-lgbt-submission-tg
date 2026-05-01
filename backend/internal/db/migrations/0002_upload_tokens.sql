CREATE TABLE IF NOT EXISTS draft_upload_tokens (
  token       TEXT PRIMARY KEY,
  draft_id    TEXT NOT NULL REFERENCES drafts(id) ON DELETE CASCADE,
  created_at  TEXT NOT NULL,
  expires_at  TEXT NOT NULL,
  revoked_at  TEXT
);
CREATE INDEX IF NOT EXISTS idx_upload_tokens_draft ON draft_upload_tokens(draft_id);
CREATE INDEX IF NOT EXISTS idx_upload_tokens_expires ON draft_upload_tokens(expires_at);

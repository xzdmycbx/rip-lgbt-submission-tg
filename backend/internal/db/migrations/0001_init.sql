-- 0001_init.sql

CREATE TABLE IF NOT EXISTS admins (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  username        TEXT UNIQUE,
  password_hash   TEXT,
  telegram_id     INTEGER UNIQUE,
  display_name    TEXT NOT NULL DEFAULT '',
  is_super        INTEGER NOT NULL DEFAULT 0,
  totp_secret     TEXT,
  totp_confirmed  INTEGER NOT NULL DEFAULT 0,
  must_setup_2fa  INTEGER NOT NULL DEFAULT 0,
  created_at      TEXT NOT NULL,
  updated_at      TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_admins_telegram ON admins(telegram_id);

CREATE TABLE IF NOT EXISTS admin_passkeys (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  admin_id        INTEGER NOT NULL REFERENCES admins(id) ON DELETE CASCADE,
  credential_id   BLOB NOT NULL UNIQUE,
  public_key      BLOB NOT NULL,
  sign_count      INTEGER NOT NULL DEFAULT 0,
  transports      TEXT NOT NULL DEFAULT '',
  attestation     TEXT NOT NULL DEFAULT '',
  user_handle     BLOB,
  created_at      TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_passkeys_admin ON admin_passkeys(admin_id);

CREATE TABLE IF NOT EXISTS admin_sessions (
  id          TEXT PRIMARY KEY,
  admin_id    INTEGER NOT NULL REFERENCES admins(id) ON DELETE CASCADE,
  created_at  TEXT NOT NULL,
  expires_at  TEXT NOT NULL,
  ua          TEXT NOT NULL DEFAULT '',
  ip          TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_sessions_admin ON admin_sessions(admin_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON admin_sessions(expires_at);

CREATE TABLE IF NOT EXISTS admin_login_links (
  token       TEXT PRIMARY KEY,
  admin_id    INTEGER NOT NULL REFERENCES admins(id) ON DELETE CASCADE,
  created_at  TEXT NOT NULL,
  expires_at  TEXT NOT NULL,
  used_at     TEXT
);
CREATE INDEX IF NOT EXISTS idx_login_links_admin ON admin_login_links(admin_id);

CREATE TABLE IF NOT EXISTS settings (
  key    TEXT PRIMARY KEY,
  value  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS memorials (
  id                TEXT PRIMARY KEY,
  display_name      TEXT NOT NULL,
  slug              TEXT NOT NULL,
  avatar_url        TEXT NOT NULL DEFAULT '',
  description       TEXT NOT NULL DEFAULT '',
  location          TEXT NOT NULL DEFAULT '',
  birth_date        TEXT NOT NULL DEFAULT '',
  death_date        TEXT NOT NULL DEFAULT '',
  alias             TEXT NOT NULL DEFAULT '',
  age               TEXT NOT NULL DEFAULT '',
  identity          TEXT NOT NULL DEFAULT '',
  pronouns          TEXT NOT NULL DEFAULT '',
  content_warnings  TEXT NOT NULL DEFAULT '[]',
  intro             TEXT NOT NULL DEFAULT '',
  life              TEXT NOT NULL DEFAULT '',
  death             TEXT NOT NULL DEFAULT '',
  remembrance       TEXT NOT NULL DEFAULT '',
  links_md          TEXT NOT NULL DEFAULT '',
  works_md          TEXT NOT NULL DEFAULT '',
  sources_md        TEXT NOT NULL DEFAULT '',
  custom_md         TEXT NOT NULL DEFAULT '',
  effects_md        TEXT NOT NULL DEFAULT '',
  markdown_full     TEXT NOT NULL DEFAULT '',
  status            TEXT NOT NULL DEFAULT 'published',
  facts_json        TEXT NOT NULL DEFAULT '[]',
  websites_json     TEXT NOT NULL DEFAULT '[]',
  created_at        TEXT NOT NULL,
  updated_at        TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_memorials_status ON memorials(status, death_date DESC);

CREATE TABLE IF NOT EXISTS memorial_assets (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  memorial_id   TEXT NOT NULL REFERENCES memorials(id) ON DELETE CASCADE,
  role          TEXT NOT NULL,
  filename      TEXT NOT NULL,
  path          TEXT NOT NULL,
  content_type  TEXT NOT NULL,
  size          INTEGER NOT NULL,
  sort          INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_memorial_assets_memorial ON memorial_assets(memorial_id);

CREATE TABLE IF NOT EXISTS flowers (
  memorial_id  TEXT PRIMARY KEY REFERENCES memorials(id) ON DELETE CASCADE,
  total        INTEGER NOT NULL DEFAULT 0,
  updated_at   TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS flower_events (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  memorial_id  TEXT NOT NULL REFERENCES memorials(id) ON DELETE CASCADE,
  ip_hash      TEXT NOT NULL,
  created_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_flower_events_lookup
  ON flower_events(memorial_id, ip_hash, created_at);

CREATE TABLE IF NOT EXISTS comments (
  id           TEXT PRIMARY KEY,
  memorial_id  TEXT NOT NULL REFERENCES memorials(id) ON DELETE CASCADE,
  author       TEXT NOT NULL,
  content      TEXT NOT NULL,
  ip_hash      TEXT NOT NULL,
  created_at   TEXT NOT NULL,
  hidden_at    TEXT
);
CREATE INDEX IF NOT EXISTS idx_comments_visible
  ON comments(memorial_id, hidden_at, created_at);
CREATE INDEX IF NOT EXISTS idx_comments_ip
  ON comments(memorial_id, ip_hash, created_at);

CREATE TABLE IF NOT EXISTS drafts (
  id                      TEXT PRIMARY KEY,
  submitter_telegram_id   INTEGER NOT NULL,
  submitter_chat_id       INTEGER NOT NULL DEFAULT 0,
  status                  TEXT NOT NULL,
  current_step            TEXT NOT NULL DEFAULT '',
  payload_json            TEXT NOT NULL DEFAULT '{}',
  rejection_reason        TEXT NOT NULL DEFAULT '',
  revising_section        TEXT NOT NULL DEFAULT '',
  reviewer_admin_id       INTEGER REFERENCES admins(id) ON DELETE SET NULL,
  created_at              TEXT NOT NULL,
  updated_at              TEXT NOT NULL,
  deleted_at              TEXT
);
CREATE INDEX IF NOT EXISTS idx_drafts_status ON drafts(status, deleted_at);
CREATE INDEX IF NOT EXISTS idx_drafts_submitter ON drafts(submitter_telegram_id, status);

CREATE TABLE IF NOT EXISTS draft_assets (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  draft_id      TEXT NOT NULL REFERENCES drafts(id) ON DELETE CASCADE,
  role          TEXT NOT NULL,
  filename      TEXT NOT NULL,
  path          TEXT NOT NULL,
  content_type  TEXT NOT NULL,
  size          INTEGER NOT NULL,
  sort          INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_draft_assets_draft ON draft_assets(draft_id);

CREATE TABLE IF NOT EXISTS draft_messages (
  id                    INTEGER PRIMARY KEY AUTOINCREMENT,
  draft_id              TEXT NOT NULL REFERENCES drafts(id) ON DELETE CASCADE,
  telegram_chat_id      INTEGER NOT NULL,
  telegram_message_id   INTEGER NOT NULL,
  kind                  TEXT NOT NULL,
  created_at            TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_draft_messages_draft ON draft_messages(draft_id);

CREATE TABLE IF NOT EXISTS submission_events (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  ip_hash     TEXT NOT NULL,
  created_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_submission_events_ip ON submission_events(ip_hash, created_at);

CREATE TABLE IF NOT EXISTS accounts (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    provider TEXT NOT NULL DEFAULT 'caldav',
    email TEXT NOT NULL DEFAULT '',
    caldav_url TEXT NOT NULL DEFAULT '',
    username TEXT NOT NULL DEFAULT '',
    password TEXT NOT NULL DEFAULT '',
    color TEXT NOT NULL DEFAULT '#7d6b96',
    is_active INTEGER NOT NULL DEFAULT 1,
    last_synced_at INTEGER,
    sync_error TEXT DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

ALTER TABLE events ADD COLUMN account_id TEXT DEFAULT '';
ALTER TABLE events ADD COLUMN remote_uid TEXT DEFAULT '';

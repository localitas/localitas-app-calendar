CREATE TABLE IF NOT EXISTS calendars (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    href TEXT NOT NULL DEFAULT '',
    color TEXT NOT NULL DEFAULT '#7d6b96',
    is_visible INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

ALTER TABLE events ADD COLUMN calendar_id TEXT DEFAULT '';

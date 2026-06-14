CREATE TABLE IF NOT EXISTS reminders (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL,
    minutes_before INTEGER NOT NULL DEFAULT 15,
    notified INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL
);

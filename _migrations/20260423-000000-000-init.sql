CREATE TABLE IF NOT EXISTS events (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    description TEXT DEFAULT '',
    start_time INTEGER NOT NULL,
    end_time INTEGER NOT NULL,
    all_day INTEGER DEFAULT 0,
    location TEXT DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_events_start_time ON events(start_time);
CREATE INDEX IF NOT EXISTS idx_events_end_time ON events(end_time);

CREATE VIRTUAL TABLE IF NOT EXISTS events_fts USING fts5(
    id,
    title,
    description,
    location,
    content='events',
    content_rowid='rowid',
    tokenize='porter unicode61'
);

CREATE TRIGGER IF NOT EXISTS events_ai AFTER INSERT ON events BEGIN
    INSERT INTO events_fts(rowid, id, title, description, location)
    VALUES (new.rowid, new.id, new.title, new.description, new.location);
END;

CREATE TRIGGER IF NOT EXISTS events_ad AFTER DELETE ON events BEGIN
    INSERT INTO events_fts(events_fts, rowid, id, title, description, location)
    VALUES ('delete', old.rowid, old.id, old.title, old.description, old.location);
END;

CREATE TRIGGER IF NOT EXISTS events_au AFTER UPDATE ON events BEGIN
    INSERT INTO events_fts(events_fts, rowid, id, title, description, location)
    VALUES ('delete', old.rowid, old.id, old.title, old.description, old.location);
    INSERT INTO events_fts(rowid, id, title, description, location)
    VALUES (new.rowid, new.id, new.title, new.description, new.location);
END;

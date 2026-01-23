-- Email cache table
CREATE TABLE IF NOT EXISTS emails (
    id TEXT PRIMARY KEY,
    from_email TEXT NOT NULL,
    from_name TEXT,
    to_email TEXT,
    subject TEXT,
    date INTEGER NOT NULL,
    size INTEGER NOT NULL,
    labels TEXT,  -- JSON array
    snippet TEXT,
    last_synced INTEGER NOT NULL
);

-- Indexes for fast queries
CREATE INDEX IF NOT EXISTS idx_from_email ON emails(from_email);
CREATE INDEX IF NOT EXISTS idx_date ON emails(date);
CREATE INDEX IF NOT EXISTS idx_size ON emails(size);
CREATE INDEX IF NOT EXISTS idx_last_synced ON emails(last_synced);

-- Sync metadata table
CREATE TABLE IF NOT EXISTS sync_metadata (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);

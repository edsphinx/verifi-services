-- Sync state table to track indexer progress
CREATE TABLE IF NOT EXISTS sync_state (
    key VARCHAR(255) PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Initialize with starting version (set to 0 or current version)
INSERT INTO sync_state (key, value, updated_at)
VALUES ('last_indexed_version', '0', NOW())
ON CONFLICT (key) DO NOTHING;

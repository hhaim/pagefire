-- +goose Up
CREATE TABLE home_ingestion_key (
    id TEXT PRIMARY KEY CHECK (id = 'default'),
    owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash BLOB NOT NULL,
    prefix TEXT NOT NULL,
    created_at INTEGER NOT NULL
);

-- +goose Down
DROP TABLE home_ingestion_key;

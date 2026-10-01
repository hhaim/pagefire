-- +goose Up
ALTER TABLE home_ingestion_key ADD COLUMN encrypted_token TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE home_ingestion_key DROP COLUMN encrypted_token;

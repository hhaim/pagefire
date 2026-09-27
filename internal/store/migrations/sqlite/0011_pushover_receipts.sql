-- +goose Up
CREATE TABLE home_pushover_receipts (
    alert_id TEXT PRIMARY KEY REFERENCES home_alerts(id) ON DELETE CASCADE,
    receipt TEXT NOT NULL,
    expires_at INTEGER NOT NULL,
    last_checked_at INTEGER NOT NULL DEFAULT 0,
    acknowledged_at INTEGER,
    canceled_at INTEGER
);

-- +goose Down
DROP TABLE home_pushover_receipts;

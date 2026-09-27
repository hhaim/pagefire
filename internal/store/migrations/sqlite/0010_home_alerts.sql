-- +goose Up
CREATE TABLE home_alerts (
    id TEXT PRIMARY KEY,
    source TEXT NOT NULL,
    incident_key TEXT NOT NULL,
    severity TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    summary TEXT NOT NULL,
    details TEXT NOT NULL DEFAULT '',
    repeat_interval_seconds INTEGER NOT NULL DEFAULT 300,
    next_repeat_at INTEGER,
    acknowledged_by TEXT,
    acknowledged_at INTEGER,
    started_at INTEGER NOT NULL,
    stopped_at INTEGER
);
CREATE UNIQUE INDEX home_alerts_active_key ON home_alerts(source, incident_key) WHERE status = 'active';
CREATE INDEX home_alerts_repeats ON home_alerts(next_repeat_at) WHERE status = 'active';

CREATE TABLE home_events (
    id TEXT PRIMARY KEY,
    source TEXT NOT NULL,
    client_event_id TEXT,
    alert_id TEXT REFERENCES home_alerts(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    severity TEXT NOT NULL DEFAULT 'low',
    summary TEXT NOT NULL DEFAULT '',
    details TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL
);
CREATE UNIQUE INDEX home_events_client_id ON home_events(source, client_event_id) WHERE client_event_id IS NOT NULL;
CREATE INDEX home_events_alert ON home_events(alert_id, created_at);

CREATE TABLE home_plugins (
    kind TEXT PRIMARY KEY,
    destination TEXT NOT NULL,
    secret TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    updated_at INTEGER NOT NULL
);

CREATE TABLE home_deliveries (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL REFERENCES home_events(id) ON DELETE CASCADE,
    plugin_kind TEXT NOT NULL,
    message TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at INTEGER NOT NULL,
    sent_at INTEGER,
    error TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    UNIQUE(event_id, plugin_kind)
);
CREATE INDEX home_deliveries_pending ON home_deliveries(status, next_attempt_at);

-- +goose Down
DROP TABLE home_deliveries;
DROP TABLE home_plugins;
DROP TABLE home_events;
DROP TABLE home_alerts;

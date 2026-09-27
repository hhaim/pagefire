-- +goose Up
ALTER TABLE home_events ADD COLUMN incident_key TEXT NOT NULL DEFAULT '';
UPDATE home_events
SET incident_key = COALESCE((SELECT incident_key FROM home_alerts WHERE id = home_events.alert_id), '')
WHERE alert_id IS NOT NULL;

-- +goose Down
ALTER TABLE home_events DROP COLUMN incident_key;

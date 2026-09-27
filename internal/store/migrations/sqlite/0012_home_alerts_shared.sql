-- +goose Up
ALTER TABLE alerts ADD COLUMN severity TEXT NOT NULL DEFAULT 'high';
INSERT OR IGNORE INTO escalation_policies(id,name,description,repeat)
SELECT 'home-events-policy','Home Events','Managed by the home event worker',0 WHERE EXISTS (SELECT 1 FROM home_alerts);
INSERT OR IGNORE INTO services(id,name,description,escalation_policy_id)
SELECT 'home-events','Home Events','Alerts created by the event API and playground','home-events-policy' WHERE EXISTS (SELECT 1 FROM home_alerts);

INSERT OR IGNORE INTO alerts(id,service_id,status,summary,details,source,dedup_key,group_key,escalation_policy_snapshot,next_escalation_at,acknowledged_at,resolved_at,created_at)
SELECT id,'home-events',
       CASE WHEN status='stopped' THEN 'resolved' WHEN acknowledged_at IS NOT NULL THEN 'acknowledged' ELSE 'triggered' END,
       summary,details,'home',source||':'||incident_key,incident_key,'{}',NULL,
       CASE WHEN acknowledged_at IS NOT NULL THEN datetime(acknowledged_at,'unixepoch') END,
       CASE WHEN stopped_at IS NOT NULL THEN datetime(stopped_at,'unixepoch') END,
       datetime(started_at,'unixepoch')
FROM home_alerts;
UPDATE alerts SET severity=(SELECT severity FROM home_alerts WHERE home_alerts.id=alerts.id) WHERE source='home';

-- +goose Down
DELETE FROM alerts WHERE service_id='home-events';
DELETE FROM services WHERE id='home-events';
DELETE FROM escalation_policies WHERE id='home-events-policy';

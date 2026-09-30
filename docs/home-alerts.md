# Home Events — Design

**Status:** Implemented in this PageFire checkout. The Alerts view is the main dashboard.

## Purpose

Accept `start`, `stop`, and `info` events from the Event Playground or an external sender. Both submit the same JSON to `POST /api/v1/events`, pass through the same lifecycle code, and appear in the same Home Events dashboard. No Service, Escalation Policy, Schedule, Team, or Incident setup is needed.

The lifecycle and notifications run without an LLM.

## Main UX

The sidebar has one **Alerts** entry. The root page and `/alerts` show the same Home Events dashboard. The old on-call, service, escalation, schedule, team, and incident API routes are removed. Existing historical database rows remain untouched but are no longer ingested or displayed.

The dashboard provides:

- An **Open alerts** list showing every active start without a matching stop, independent of the event-feed time window, with a **Force close** action.
- Window presets: 1 hour, 1 day, 1 week, 1 month.
- Type, class, source, client, open (all/true/false), and message regex filters.
- A stacked event chart by class, counts by event type, and summary cards.
- Event details, JSON, delivery status, and pagination.
- Double-click on an open Home Event to open its alert in the playground as a prepared stop event.

Force close records a generated stop event and queues the same all-clear notification as an incoming `stop`. It is available to the signed-in user who owns the alert.

The open count uses distinct alert IDs, so multiple repeat events for one active alert count as one open alert. Home Event timestamps are Unix seconds; the UI converts them to JavaScript milliseconds for relative time display.

The **Event Playground** has one generator. It builds and edits the exact JSON sent to `POST /api/v1/events`. Resending an `event_id` shows idempotency; sending as new generates a new ID. The stop picker and double-click flow fill the active alert's incident key, summary, and details into the JSON.

The **Notification Plugins** page shows Telegram and Pushover in collapsed accordions. Each header shows the plugin name and status. Opening it reveals the destination, token, Enabled control, Save, test, and remove actions. Plugin credentials are encrypted at rest and redacted from read APIs.

## Single ingestion key

The **Event Ingestion** settings page creates one long lived key for external senders. The key has no automatic expiration and is shown only when created or rotated. Rotation immediately invalidates the old key; revocation disables ingestion until another key is created. The server stores a SHA-256 hash of the random key, not the raw value. The key is scoped to `POST /api/v1/events`; it cannot read alerts or manage settings. Its owner is the event source, so use the same key for matching start and stop events.

An administrator creates the key at **Event Ingestion → Create key**, then copies the generated value. The settings page displays ready to copy `curl` commands. Every event has an incident key. A start and its stop use different event IDs and the same incident key:

```bash
curl -X POST 'http://YOUR_PAGEFIRE_HOST:PORT/api/v1/events' \
  -H 'Authorization: Bearer YOUR_INGESTION_KEY' \
  -H 'Content-Type: application/json' \
  -d '{"event_id":"demo-start-001","event":"start","incident_key":"demo-water","severity":"high","summary":"Water leak","details":"Boiler room sensor"}'

curl -X POST 'http://YOUR_PAGEFIRE_HOST:PORT/api/v1/events' \
  -H 'Authorization: Bearer YOUR_INGESTION_KEY' \
  -H 'Content-Type: application/json' \
  -d '{"event_id":"demo-stop-001","event":"stop","incident_key":"demo-water"}'

curl -X POST 'http://YOUR_PAGEFIRE_HOST:PORT/api/v1/events' \
  -H 'Authorization: Bearer YOUR_INGESTION_KEY' \
  -H 'Content-Type: application/json' \
  -d '{"event_id":"demo-info-001","event":"info","incident_key":"demo-water","severity":"high","summary":"Water sensor update","details":"Reading 99"}'
```

The playground uses the signed-in session. The event-only key is the external ingestion workflow.

## Event lifecycle

| Event | State change | Notification |
|---|---|---|
| `start` | Creates one Home Alert for `(source, incident_key)`; repeated active starts are ignored. | Sends severity, summary, and details. |
| `stop` | Resolves the matching alert immediately and clears future repeats. | Sends `✅ DOWN — all clear` with the original summary and details unless the request supplies replacements. |
| `info` | Records a standalone event with an incident key, without opening an alert. | Sends once. |
| `repeat` | Runs for an active, unacknowledged high alert when the stored interval is due. | Sends the current alert message. |

Every inbound event needs a caller-supplied `event_id`, unique per source, and a nonempty `incident_key`. Retrying the same ID returns `already_seen` without another delivery. High starts use a repeat interval of at least 300 seconds, defaulting to 300. Mid and low events are one-shot. Stop does not wait for acknowledgement and still sends an all-clear after acknowledgement.

An active start lives in `home_alerts`. Stop marks it stopped; Pushover or Telegram acknowledgement records acknowledgement and stops repeats. `home_events` stores the event history and `home_deliveries` stores plugin delivery attempts. The dashboard reads these Home Events tables directly.

## Notification and acknowledgement

Telegram receives the full message, including `incident_key` and `details`. A high start includes an inline acknowledgement button. The worker polls callbacks and acknowledges the Home Alert, which stops PageFire repeats.

Every high Home Event, including high `info` and high `stop`, is sent to Pushover with emergency priority 2. High starts use an emergency receipt that the worker polls. Acknowledging that receipt in Pushover acknowledges the same Home Alert and stops PageFire repeats. The UI has no separate PageFire acknowledgement button. If a stop arrives before Pushover acknowledgement, the worker cancels the outstanding emergency receipt. High info and stop notifications are short emergency deliveries and do not create an alert to acknowledge in PageFire.

Pushover's emergency notification handles provider retries while its receipt is pending. PageFire does not send a duplicate repeat to Pushover during that period. If the receipt expires while the alert is still active and unacknowledged, a later repeat starts another emergency. Delivery is durable and at least once; after a crash, a provider may receive a duplicate if SQLite did not record its success.

## API and storage

| Route | Purpose |
|---|---|
| `POST /api/v1/events` | Shared playground and external ingestion path. |
| `GET /api/v1/events` and `GET /api/v1/events/{event_id}` | Recent events and delivery details for the signed-in user. |
| `GET /api/v1/alert-events` | Filtered Home Events feed used by the dashboard. |
| `GET /api/v1/home-alerts/active` and `GET /api/v1/home-alerts/{id}` | Active Home Alerts and a stop target for the playground. |
| `POST /api/v1/home-alerts/{id}/close` | Force close an active alert by recording a stop event. |
| `GET/POST/DELETE /api/v1/event-ingestion-key` | Key status, create/rotate, revoke. Writes require admin. |
| `GET/PUT/DELETE /api/v1/home-plugins` | Plugin settings. Writes require admin. |
| `GET /api/v1/home-stats` | Event and delivery counts. |

SQLite stores lifecycle timing in `home_alerts`, event history in `home_events`, durable plugin delivery work in `home_deliveries`, Pushover start receipts in `home_pushover_receipts`, encrypted plugin settings in `home_plugins`, and the single ingestion-key hash in `home_ingestion_key`. Event and alert changes use one transaction. Stopped Home Event history is retained for six calendar months. Back up `home-alerts.key` with the database or encrypted plugin tokens cannot be recovered.

## Five-minute demo

1. Configure Pushover or Telegram in **Notification Plugins**, and create an ingestion key if sending from outside PageFire.
2. Send a high `start` with `repeat_interval_seconds: 300` and nonempty `details`.
3. Verify one open alert in **Alerts** and a notification containing the details.
4. Wait five minutes without acknowledging, or acknowledge in Pushover to demonstrate that repeats stop.
5. Double-click the open Home Event in **Alerts**; the playground prepares a `stop` with the same incident key and message. Send it with a new event ID.
6. Verify the Home Alert is stopped, the Open count decreases, and the plugin sends `✅ DOWN — all clear`.

## Current limits

- The key is global, owned by the administrator who creates it. Start and stop must use that key or a session for the same user to share the source identity.
- Pushover emergency receipts are polled, so PageFire acknowledgement follows the next worker poll rather than the instant the button is tapped.
- Mid severity currently sends once. Weekly statistics run Monday at 09:00 UTC through enabled plugins.

# Notes: Home alert implementation

## Repository findings
- Go HTTP API uses chi in `internal/api/router.go`, with session or token auth.
- SQLite migrations are embedded under `internal/store/migrations/sqlite`.
- Notification delivery uses a dispatcher and periodic engine, but home alerts need independent repeat and provider configuration.
- Frontend is Preact with Vite and pages under `web/src/pages`.

## Implementation notes
- Preserve client event IDs for inbound start/stop/info. Generate IDs for internal repeats and acknowledgements.
- Keep plugin secrets server-side and redact them from responses.
- Use a durable queue so API success does not depend on provider availability.
- Home plugin tokens are encrypted in SQLite using `home-alerts.key` in the configured data directory.
- Weekly summary defaults to Monday at 09:00 UTC for the prior Monday–Sunday period.
- Mid severity currently sends once and does not repeat or require acknowledgement.
- The event playground posts to the real `/api/v1/events` endpoint and shows delivery status.
- Provider metadata supplies the settings page labels; adding a provider registers its kind and fields without changing the page.
- Telegram polls callbacks from a dedicated bot and acknowledges high alerts through inline buttons.

## Verification
- Go lifecycle tests cover duplicate events, repeats, cancellation on acknowledgement, stop/info delivery, weekly report idempotency, and six-month cleanup.
- HTTP test covers the authenticated event, acknowledgement, timeline, stats, and plugin configuration routes.
- Frontend production build and existing component tests pass.
- Live Telegram/Pushover calls cannot be exercised without user credentials.

## Follow-up research (2026-09-27)
- DNS Inventory reference at `http://frigate:8090/`, Events view: window, type, class, source, device, message-regex filters; a severity stacked time chart; counts; searchable event table; double-click event JSON. The live view offered 1h/1d/7d, while the requested PageFire view also needs 1w/1m.
- PageFire `TimeAgo` gives numeric Unix seconds to JavaScript `Date`, which expects milliseconds; this explains the dashboard's 56-year display.
- Home events live in `home_events` and are scoped by authenticated user; normal alerts live in `alerts`. Current home event list returns only 20 records without time filters.
- Pushover start-high uses emergency priority 2 and stores a receipt. Receipt polling records a separate Pushover ack but does not call PageFire `Acknowledge`, so app repeats continue.
- Pushover's official API requires `priority=2`, `retry>=30`, and `expire<=10800` for emergency messages; receipt polling reports acknowledgement. See https://pushover.net/api and https://pushover.net/api/receipts.

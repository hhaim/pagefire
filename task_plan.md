# Task Plan: Home Events only

## Goal
Make Home Events the only ingestion and dashboard data path. Remove the legacy service, escalation, on-call, incident, schedule, team, and per-user notification code.

## Phases
- [x] Map runtime and database dependencies.
- [x] Move the Home Events lifecycle, feed, and playground off legacy alert/service rows.
- [x] Remove legacy UI, API, worker, store, provider, and documentation code.
- [x] Require incident keys for info events and verify high info uses Pushover emergency priority.
- [x] Verify Go tests, web build, OpenAPI parsing, and diff formatting.

## Decisions
- Retain old SQLite migration files and historical legacy tables so existing databases can upgrade without losing past data.
- Keep browser sessions for the UI and one event ingestion key for external senders.
- Use `home_alerts` for active lifecycle state and `home_events` for event history and dashboard filters.

## Verification
- `go test ./...` passed with local loopback access for the Telegram test.
- `npm run build` passed.
- The OpenAPI YAML parsed and `git diff --check` passed.

## Status
Complete.

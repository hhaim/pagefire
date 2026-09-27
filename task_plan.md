# Task Plan: Home alert API, plugins, and event playground

## Goal
Implement the home alert design in this PageFire checkout with persistent start/stop/info events, Telegram and Pushover delivery, plugin settings UI, and a JSON event playground.

## Phases
- [x] Phase 1: Establish scope and inspect repository conventions.
- [x] Phase 2: Implement SQLite model, event lifecycle, plugins, and background work.
- [x] Phase 3: Add authenticated API and UI pages.
- [x] Phase 4: Verify critical flows and update documentation.

## Decisions Made
- Use the existing app, SQLite connection, router, and Preact UI.
- Build home alerts as a distinct subsystem so existing on-call behavior remains intact.
- Use global destinations and the same event endpoint for real clients and the playground.

## Open Questions
- Mid severity: use a conservative one-shot policy until specified.
- Weekly report schedule: choose a simple default and document it.
- External plugin acknowledgement: implement direct Telegram action if feasible; otherwise keep the API action and document the gap.

## Errors Encountered
- The HTTP stats test exposed a half-open range ending at the current Unix second; moved the default end one second ahead so new events appear immediately.
- Preview startup with a new data directory failed before migrations because the directory did not exist; app startup now creates it before opening SQLite.
- The sandbox refused a local TCP bind for a live browser preview. Production frontend build and API tests remain available and passed.

## Status
Original home alert implementation complete. Follow-up work below is active.

## Follow-up: Events view and acknowledgements

### Goal
Fix dashboard time display; build an Events-style alerts view with time, type, class, source, client, and message regex filters plus grouped counts and event details; make every high home event critical in Pushover; use Pushover acknowledgement as the PageFire acknowledgement.

### Phases
- [x] Inspect Frigate Events and the current PageFire data model/API.
- [ ] Implement timestamp and alerts view changes.
- [ ] Implement high-priority Pushover delivery and shared acknowledgement state.
- [ ] Verify behavior and document remaining limitations in `demo-verification.md`.

### Status
Implementation in progress. The combined view will map Home Event kinds to Type, severity to Class, incident key to Client; service alerts map to Type `alert`, Class `error`, and service name as Client. Open reflects current active state.

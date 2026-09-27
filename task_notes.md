# Home Events only cleanup notes

- User explicitly chose Home Events only; ordinary service alert ingestion and dashboard entries can be removed.
- Existing Home Events currently create a legacy `services` and `escalation_policies` row only because they mirror into `alerts`. The Home Events tables already hold their lifecycle and delivery history.
- Historical migrations must remain immutable for existing databases. A new migration can decouple runtime Home Events from the legacy tables.
- Playground currently reads legacy `/alerts` to populate active stop targets; add a Home Events active-alert endpoint and use it there.
- Dashboard feed joins legacy `alerts` for status and unions ordinary service alerts. Use `home_alerts` status/acknowledgement directly and remove the service query.
- Home worker cleanup and ACK still update legacy `alerts`; remove those updates after switching the read path.
- Runtime app starts legacy escalation/notification/cleanup processors and mounts service integration APIs; remove that wiring.
- Removed on-call/service/escalation API handlers, store implementations, workers, and old provider code. Browser sessions and one global event ingestion key remain; per-user API token creation was removed.
- Migration 0014 stores `incident_key` on every `home_events` row and backfills historical linked events.
- The old SQLite migration history stays in place so existing installations can upgrade; legacy tables and historical rows remain but are not part of runtime ingestion or the dashboard.

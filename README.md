# PageFire

PageFire is a self-hosted Home Events dashboard. Send `start`, `stop`, or `info` events through one API, inspect them in the dashboard, and deliver notifications through Telegram or Pushover.

## Features

- One event ingestion key for external senders; the Event Playground uses the same API with your signed-in session.
- Start/stop lifecycle keyed by `incident_key`, with idempotency keyed by `event_id`.
- Time window, type, class, source, client, open state, and message regex filters.
- Stacked event chart, event counts, delivery details, and a stop action prepared by double-clicking an open event.
- Telegram and Pushover plugins. High events, including `info`, use Pushover emergency priority 2. High starts repeat until acknowledged or stopped.
- SQLite storage and a single Go server with an embedded web UI.

## Run

With Docker:

~~~bash
docker compose up -d
~~~

Or build from source with Go 1.26.1 or newer and Node.js/npm:

~~~bash
make build
./bin/pagefire serve
~~~

Open `http://localhost:3001` with the included Docker Compose file, or `http://localhost:3000` when running the binary. Create the first admin account, configure a destination in **Notification Plugins**, then create one key in **Event Ingestion**.

## Send events

Each request needs a new `event_id` and a nonempty `incident_key`. Use the same incident key for the matching start and stop.

~~~bash
curl -X POST http://localhost:3000/api/v1/events \
  -H 'Authorization: Bearer YOUR_INGESTION_KEY' \
  -H 'Content-Type: application/json' \
  -d '{"event_id":"water-start-001","event":"start","incident_key":"boiler-water","severity":"high","summary":"Water leak","details":"Boiler room sensor"}'

curl -X POST http://localhost:3000/api/v1/events \
  -H 'Authorization: Bearer YOUR_INGESTION_KEY' \
  -H 'Content-Type: application/json' \
  -d '{"event_id":"water-stop-001","event":"stop","incident_key":"boiler-water"}'
~~~

An `info` event also requires `incident_key`. The dashboard and the Event Playground are at `/` and `/home-events`.

## Configuration

| Variable | Default | Purpose |
|---|---|---|
| `PAGEFIRE_PORT` | `3000` | HTTP listen port |
| `PAGEFIRE_DATABASE_URL` | `./pagefire.db` | SQLite database path |
| `PAGEFIRE_DATA_DIR` | `.` | Directory for the plugin encryption key |
| `PAGEFIRE_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error` |
| `PAGEFIRE_ENGINE_INTERVAL_SECONDS` | `5` | Worker polling interval |

Back up the database and `home-alerts.key` together. The key decrypts stored plugin credentials.

See [Home Events design](docs/home-alerts.md), [API specification](docs/openapi.yaml), and [Docker guide](docs/docker.md).

## License

[MIT](LICENSE)

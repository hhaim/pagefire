# Self-Hosting PageFire with Docker

## Quick Start

```bash
cd pagefire-home
docker compose up -d --build
```

Open **http://localhost:3001**. On first launch you will see a setup wizard to create your admin account. The container still listens on port 3000 internally.

To use the pre-built image instead of building locally, edit `docker-compose.yml` and replace the `build: .` line with:

```yaml
image: ghcr.io/pagefire/pagefire:latest
```

## Configuration

All configuration is done through environment variables with the `PAGEFIRE_` prefix. Uncomment and set them in `docker-compose.yml` under the `environment` key.

### Core

| Variable | Default | Description |
|---|---|---|
| `PAGEFIRE_PORT` | `3000` | HTTP listen port |
| `PAGEFIRE_DATA_DIR` | `/data` | Directory for SQLite database and data files |
| `PAGEFIRE_DATABASE_URL` | `<data_dir>/pagefire.db` | Database path (SQLite) or connection string (Postgres) |
| `PAGEFIRE_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |

### Engine

| Variable | Default | Description |
|---|---|---|
| `PAGEFIRE_ENGINE_INTERVAL_SECONDS` | `5` | How often the Home Events worker checks deliveries, receipts, and repeats |

## Data Persistence

PageFire stores its SQLite database and the `home-alerts.key` plugin encryption key in the `/data` directory inside the container. The `docker-compose.yml` maps this to a named Docker volume (`pagefire-data`), so your data survives container restarts and recreations.

At startup, the container sets the mounted data directory's ownership for PageFire and then runs the server as an unprivileged user. This also prepares a fresh or previously root-owned volume for SQLite migrations.

To use a bind mount instead (useful for backups):

```yaml
volumes:
  - ./data:/data
```

The container prepares the mounted directory for its internal `pagefire` user before starting. With a bind mount, this changes ownership of files under `./data` to that container user so SQLite can write its database and WAL files.

### Backups

Back up the database and `home-alerts.key` together. Use SQLite's online backup command or stop the container before copying database files, including any WAL file. Restoring the database without `home-alerts.key` makes saved plugin credentials unreadable.

## Creating an Admin User via CLI

If you need to create an admin user non-interactively (e.g. in CI or scripted setup):

```bash
docker compose exec pagefire pagefire admin create \
  --email admin@example.com \
  --name "Admin" \
  --password "your-secure-password"
```

## Updating

Pull the latest image and recreate the container. Your data is preserved in the volume.

```bash
docker compose pull        # if using a pre-built image
docker compose up -d --build  # if building from source
```

To pin a specific version:

```yaml
image: ghcr.io/pagefire/pagefire:v0.3.0
```

## Running Behind a Reverse Proxy

In production, run PageFire behind a reverse proxy (Caddy, nginx, Traefik) that terminates TLS. Example with Caddy added to the compose file:

```yaml
services:
  caddy:
    image: caddy:2
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile
      - caddy-data:/data
    depends_on:
      - pagefire

  pagefire:
    build: .
    # no need to expose port 3000 to the host
    expose:
      - "3000"
    # ... rest of config
```

With a `Caddyfile`:

```
pagefire.example.com {
    reverse_proxy pagefire:3000
}
```

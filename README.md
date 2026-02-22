# Magnetar

A lightweight DHT torrent indexer built for Sonarr, Radarr, and Stremio debrid addons. Crawls the DHT network, classifies media torrents, matches them against TMDB/TVDB, and serves everything over a Torznab-compatible API.

Think of it as a leaner alternative to Bitmagnet — same DHT crawling, ~89% less storage. A couple million media torrents fit comfortably in under 2GB with SQLite.

## What it does

- **Crawls the DHT network** for new torrents using BEP 5/9
- **Filters out non-media** at ingest — only movies, TV, and anime make it to the database
- **Matches metadata** against TMDB (and optionally TVDB) with smart caching via Valkey
- **Serves a Torznab API** that plugs directly into Prowlarr, Sonarr, and Radarr
- **Hash lookup API** for debrid/Stremio addon integration
- **Web UI** for browsing, searching, and monitoring

## Quick start

```bash
# Clone and build
git clone https://github.com/VibingForFreedom/magnetar.git
cd magnetar
make build

# Configure (minimum: just a TMDB key for metadata matching)
cp .env.example .env
# Edit .env with your TMDB API key

# Run
./magnetar serve
```

The web UI will be at `http://localhost:8080`.

### Docker

```yaml
services:
  magnetar:
    image: ghcr.io/vibingforfreedom/magnetar:latest
    ports:
      - "8080:8080"
      - "6881:6881/udp"
    volumes:
      - ./data:/data
    environment:
      - MAGNETAR_DB_PATH=/data/magnetar.db
      - MAGNETAR_TMDB_API_KEY=your_key_here
    restart: unless-stopped
```

## Configuration

All configuration is via environment variables (or a `.env` file):

| Variable | Default | Description |
|----------|---------|-------------|
| `MAGNETAR_PORT` | `8080` | HTTP server port |
| `MAGNETAR_DB_BACKEND` | `sqlite` | `sqlite` or `mariadb` |
| `MAGNETAR_DB_PATH` | `data/magnetar.db` | SQLite database path |
| `MAGNETAR_CRAWL_ENABLED` | `true` | Enable DHT crawler |
| `MAGNETAR_CRAWL_PORT` | `6881` | UDP port for DHT |
| `MAGNETAR_MATCH_ENABLED` | `true` | Enable background metadata matching |
| `MAGNETAR_TMDB_API_KEY` | | TMDB API key (free at themoviedb.org) |
| `MAGNETAR_API_KEY` | | Optional API key to protect endpoints |

See `.env.example` for the full list.

## Connecting to Sonarr / Radarr

Add Magnetar as an indexer in Prowlarr (or directly in Sonarr/Radarr):

- **URL**: `http://your-server:8080/api/torznab`
- **API Key**: whatever you set in `MAGNETAR_API_KEY` (leave blank if none)
- **Categories**: Movies (2000), TV (5000), Anime (5070)

## Database options

**SQLite** (default) — zero config, single file, good for most setups. Handles millions of torrents without breaking a sweat.

**MariaDB** — available if you need it. Switch with `MAGNETAR_DB_BACKEND=mariadb` and provide a DSN. A built-in migration tool moves data between backends:

```bash
magnetar migrate --from sqlite --from-path ./data/magnetar.db \
                 --to mariadb --to-dsn "user:pass@tcp(localhost:3306)/magnetar"
```

## System monitoring

The `/system` page in the web UI shows database stats, background task status (WAL checkpoints, integrity checks, purge jobs, anime DB refresh), and process info — all auto-refreshing.

## Credits & acknowledgments

Magnetar wouldn't exist without these projects:

- **[Bitmagnet](https://github.com/bitmagnet-io/bitmagnet)** — the DHT crawler code is adapted from Bitmagnet's implementation (MIT licensed). Huge thanks to the Bitmagnet team for building such a solid crawler.
- **[anacrolix/torrent](https://github.com/anacrolix/torrent)** — Go BitTorrent library powering the DHT and metadata protocols
- **[Valkey](https://valkey.io/)** via **[valkey-go](https://github.com/valkey-io/valkey-go)** — high-performance caching for TMDB/TVDB API responses
- **[mattn/go-sqlite3](https://github.com/mattn/go-sqlite3)** — CGo SQLite3 driver with FTS5 support
- **[go-sql-driver/mysql](https://github.com/go-sql-driver/mysql)** — MariaDB/MySQL driver
- **[SvelteKit](https://kit.svelte.dev/)** + **[Tailwind CSS](https://tailwindcss.com/)** + **[Lucide](https://lucide.dev/)** — the web frontend
- **[TMDB](https://www.themoviedb.org/)** — movie and TV metadata (this product uses the TMDB API but is not endorsed or certified by TMDB)

## License

MIT — see [LICENSE](LICENSE) for details.

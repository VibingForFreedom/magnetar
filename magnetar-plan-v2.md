# Magnetar v2 — Lightweight DHT Torrent Indexer

## Vision

A stripped-down, space-efficient alternative to Bitmagnet focused on serving Sonarr/Radarr (Torznab) and Stremio debrid addons. Same DHT crawling capability, ~89% less storage. Media-only (movies, TV, anime).

## Problem

Bitmagnet uses ~14GB for 2M torrents. ~12GB is unnecessary for arr/debrid use cases. A purpose-built indexer stores the same useful data in ~1.6GB compressed.

## Tech Stack

- **Language**: Go (shares toolchain with Nova)
- **Database**: SQLite (default) or MariaDB (optional upgrade path)
- **Frontend**: SvelteKit (embedded static build in Go binary)
- **Deployment**: Single Go binary (SQLite) or Go binary + MariaDB container

---

## Database Strategy

### Dual-Backend Architecture

Magnetar ships with SQLite as the default and supports MariaDB as a drop-in alternative. A `Store` interface abstracts all database access so the application layer never touches SQL directly. Both backends are tested against the same integration test suite.

```
┌──────────────────────────────────┐
│           Application            │
│  (Crawler, Matcher, API, etc.)   │
└──────────────┬───────────────────┘
               │
         Store Interface
               │
       ┌───────┴───────┐
       │               │
  ┌────▼────┐   ┌──────▼──────┐
  │ SQLite  │   │  MariaDB    │
  │ Backend │   │  Backend    │
  └─────────┘   └─────────────┘
```

### Store Interface (Go)

```go
package store

import "context"

type Store interface {
    // Torrent CRUD
    UpsertTorrent(ctx context.Context, t *Torrent) error
    UpsertTorrents(ctx context.Context, ts []*Torrent) error
    GetTorrent(ctx context.Context, infoHash []byte) (*Torrent, error)
    BulkLookup(ctx context.Context, hashes [][]byte) ([]*Torrent, error)
    DeleteTorrent(ctx context.Context, infoHash []byte) error

    // Search
    SearchByName(ctx context.Context, query string, opts SearchOpts) (*SearchResult, error)
    SearchByExternalID(ctx context.Context, id ExternalID) ([]*Torrent, error)

    // Matching queue
    FetchUnmatched(ctx context.Context, limit int) ([]*Torrent, error)
    UpdateMatchResult(ctx context.Context, infoHash []byte, m MatchResult) error

    // Stats
    Stats(ctx context.Context) (*DBStats, error)

    // Lifecycle
    Migrate(ctx context.Context) error
    Close() error
    Checkpoint(ctx context.Context) error // WAL checkpoint (SQLite), no-op for MariaDB
    Analyze(ctx context.Context) error    // Run ANALYZE / optimizer hints
}

type SearchOpts struct {
    Categories []Category
    MinYear    int
    MaxYear    int
    Quality    []Quality
    Limit      int
    Offset     int
}

type ExternalID struct {
    Type  string // "imdb", "tmdb", "tvdb", "anilist", "kitsu"
    Value string
}

type MatchResult struct {
    Status    MatchStatus
    IMDBID    string
    TMDBID    int
    TVDBID    int
    AniListID int
    KitsuID   int
    Year      int
}
```

### Configuration

```env
# Database backend: "sqlite" (default) or "mariadb"
MAGNETAR_DB_BACKEND=sqlite

# SQLite settings
MAGNETAR_DB_PATH=/data/magnetar.db
MAGNETAR_DB_CACHE_SIZE=64000          # pages (64MB at 1KB/page)
MAGNETAR_DB_MMAP_SIZE=268435456       # 256MB

# MariaDB settings (used when MAGNETAR_DB_BACKEND=mariadb)
MAGNETAR_DB_DSN=magnetar:pass@tcp(localhost:3306)/magnetar
MAGNETAR_DB_MAX_OPEN_CONNS=25
MAGNETAR_DB_MAX_IDLE_CONNS=10
```

### Migration Tool (SQLite ↔ MariaDB)

Built into the binary as a subcommand:

```bash
# Export SQLite → MariaDB
magnetar migrate --from sqlite --from-path /data/magnetar.db \
                 --to mariadb --to-dsn "magnetar:pass@tcp(localhost:3306)/magnetar"

# Export MariaDB → SQLite
magnetar migrate --from mariadb --from-dsn "..." \
                 --to sqlite --to-path /data/magnetar.db
```

Migration strategy:
- Opens both stores simultaneously
- Streams rows in batches of 5,000
- Writes using `UpsertTorrents` (idempotent, safe to restart)
- Rebuilds FTS index after completion (SQLite target)
- Prints progress: `[142,000 / 2,340,000] 6.1% — 12,400 rows/sec`
- Verifies row count matches at the end

---

## Data Model

### Core Types

```go
type Torrent struct {
    InfoHash     []byte    // 20 bytes (binary)
    Name         string
    Size         int64
    Category     Category
    Quality      Quality
    Files        []File    // compressed JSON in DB
    IMDBID       string
    TMDBID       int
    TVDBID       int
    AniListID    int
    KitsuID      int
    MediaYear    int
    MatchStatus  MatchStatus
    MatchAttempts int
    MatchAfter   int64     // unix timestamp — exponential backoff
    Seeders      int
    Leechers     int
    Source       Source
    DiscoveredAt int64
    UpdatedAt    int64
}

type File struct {
    Path string `json:"path"`
    Size int64  `json:"size"`
}

type Category int
const (
    CategoryMovie Category = iota
    CategoryTV
    CategoryAnime
)

type Quality int
const (
    QualityUnknown Quality = iota
    QualitySD              // 480p and below
    QualityHD              // 720p
    QualityFHD             // 1080p
    QualityUHD             // 2160p/4K
)

type MatchStatus int
const (
    MatchUnmatched MatchStatus = iota
    MatchMatched
    MatchFailed
)

type Source int
const (
    SourceDHT Source = iota
    SourceBitmagnet
)
```

### SQLite Schema

```sql
-- Core table
CREATE TABLE torrents (
    info_hash      BLOB PRIMARY KEY,     -- 20 bytes binary
    name           TEXT NOT NULL,
    size           INTEGER,
    category       INTEGER NOT NULL,     -- 0=movie 1=tv 2=anime
    quality        INTEGER DEFAULT 0,    -- 0=unknown 1=sd 2=hd 3=fhd 4=uhd
    files          BLOB,                 -- zstd-compressed JSON

    imdb_id        TEXT,
    tmdb_id        INTEGER,
    tvdb_id        INTEGER,
    anilist_id     INTEGER,
    kitsu_id       INTEGER,
    media_year     INTEGER,
    match_status   INTEGER DEFAULT 0,
    match_attempts INTEGER DEFAULT 0,
    match_after    INTEGER DEFAULT 0,    -- unix timestamp for backoff

    seeders        INTEGER DEFAULT 0,
    leechers       INTEGER DEFAULT 0,

    source         INTEGER DEFAULT 0,
    discovered_at  INTEGER NOT NULL,
    updated_at     INTEGER
) STRICT;

-- FTS5 for name search
CREATE VIRTUAL TABLE torrents_fts USING fts5(
    name,
    content='torrents',
    content_rowid='rowid',
    tokenize='unicode61 remove_diacritics 2'
);

-- Sync triggers
CREATE TRIGGER torrents_fts_insert AFTER INSERT ON torrents BEGIN
    INSERT INTO torrents_fts(rowid, name) VALUES (new.rowid, new.name);
END;

CREATE TRIGGER torrents_fts_delete AFTER DELETE ON torrents BEGIN
    INSERT INTO torrents_fts(torrents_fts, rowid, name) VALUES ('delete', old.rowid, old.name);
END;

CREATE TRIGGER torrents_fts_update AFTER UPDATE OF name ON torrents BEGIN
    INSERT INTO torrents_fts(torrents_fts, rowid, name) VALUES ('delete', old.rowid, old.name);
    INSERT INTO torrents_fts(rowid, name) VALUES (new.rowid, new.name);
END;

-- Indexes
CREATE INDEX idx_imdb       ON torrents(imdb_id)       WHERE imdb_id IS NOT NULL;
CREATE INDEX idx_tmdb       ON torrents(tmdb_id)       WHERE tmdb_id IS NOT NULL;
CREATE INDEX idx_tvdb       ON torrents(tvdb_id)       WHERE tvdb_id IS NOT NULL;
CREATE INDEX idx_anilist    ON torrents(anilist_id)     WHERE anilist_id IS NOT NULL;
CREATE INDEX idx_kitsu      ON torrents(kitsu_id)       WHERE kitsu_id IS NOT NULL;
CREATE INDEX idx_category   ON torrents(category, quality, media_year);
CREATE INDEX idx_discovered ON torrents(discovered_at DESC);
CREATE INDEX idx_match_queue ON torrents(match_status, match_after)
    WHERE match_status != 1; -- exclude already-matched rows
```

### MariaDB Schema

```sql
CREATE TABLE torrents (
    info_hash      BINARY(20) PRIMARY KEY,
    name           VARCHAR(1024) NOT NULL,
    size           BIGINT,
    category       TINYINT NOT NULL,
    quality        TINYINT DEFAULT 0,
    files          MEDIUMBLOB,            -- zstd-compressed JSON (same as SQLite)

    imdb_id        VARCHAR(12),
    tmdb_id        INT,
    tvdb_id        INT,
    anilist_id     INT,
    kitsu_id       INT,
    media_year     SMALLINT,
    match_status   TINYINT DEFAULT 0,
    match_attempts TINYINT DEFAULT 0,
    match_after    INT DEFAULT 0,

    seeders        INT DEFAULT 0,
    leechers       INT DEFAULT 0,

    source         TINYINT DEFAULT 0,
    discovered_at  INT NOT NULL,
    updated_at     INT,

    FULLTEXT INDEX ft_name (name),
    INDEX idx_imdb (imdb_id),
    INDEX idx_tmdb (tmdb_id),
    INDEX idx_tvdb (tvdb_id),
    INDEX idx_anilist (anilist_id),
    INDEX idx_kitsu (kitsu_id),
    INDEX idx_category (category, quality, media_year),
    INDEX idx_discovered (discovered_at),
    INDEX idx_match_queue (match_status, match_after)
) ENGINE=InnoDB ROW_FORMAT=COMPRESSED KEY_BLOCK_SIZE=8;
```

Both schemas store `files` as a zstd-compressed blob — the application layer handles compression/decompression identically regardless of backend.

---

## Corruption Prevention & Data Safety

### SQLite Hardening

```go
func initSQLite(path string) (*sql.DB, error) {
    db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
    if err != nil {
        return nil, err
    }

    // Connection pragmas — run on every connection
    pragmas := []string{
        "PRAGMA journal_mode = WAL",
        "PRAGMA synchronous = NORMAL",        // safe with WAL, fsync on checkpoint only
        "PRAGMA wal_autocheckpoint = 1000",   // checkpoint every 1000 pages
        "PRAGMA cache_size = -65536",         // 64MB page cache
        "PRAGMA mmap_size = 268435456",       // 256MB mmap window
        "PRAGMA temp_store = MEMORY",         // temp tables in RAM
        "PRAGMA foreign_keys = ON",
        "PRAGMA busy_timeout = 5000",         // 5s wait on lock contention
    }
    for _, p := range pragmas {
        if _, err := db.Exec(p); err != nil {
            return nil, fmt.Errorf("pragma %s: %w", p, err)
        }
    }

    // Writer connection pool: exactly 1 writer
    // Reader connection pool: up to GOMAXPROCS readers
    db.SetMaxOpenConns(1) // only for writer pool
    // Create separate read-only pool (see connection strategy below)

    return db, nil
}
```

### Two-Pool Connection Strategy (SQLite)

SQLite's WAL mode allows concurrent reads but serializes writes. Using a single `*sql.DB` with `MaxOpenConns > 1` can cause "database is locked" errors because Go's pool doesn't distinguish readers from writers.

Solution: two separate `*sql.DB` instances.

```go
type SQLiteStore struct {
    writer *sql.DB  // MaxOpenConns=1, used for INSERT/UPDATE/DELETE
    reader *sql.DB  // MaxOpenConns=N, used for SELECT, opened with ?mode=ro
}
```

- **Writer pool**: `MaxOpenConns=1` — serializes all writes through one connection, eliminates lock contention
- **Reader pool**: `MaxOpenConns=runtime.NumCPU()` — concurrent reads, opened with `?mode=ro` for safety
- All `SELECT` queries go through `reader`
- All `INSERT`/`UPDATE`/`DELETE` go through `writer`

### Integrity Checks

```go
// Run on startup
func (s *SQLiteStore) IntegrityCheck(ctx context.Context) error {
    var result string
    err := s.reader.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result)
    if err != nil {
        return fmt.Errorf("integrity check failed: %w", err)
    }
    if result != "ok" {
        return fmt.Errorf("database corruption detected: %s", result)
    }
    return nil
}

// Run daily via background goroutine
func (s *SQLiteStore) QuickCheck(ctx context.Context) error {
    var result string
    err := s.reader.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&result)
    // quick_check is faster, skips content verification
    ...
}
```

### Backup Strategy

```go
// Online backup using SQLite's backup API (doesn't block readers or writers)
func (s *SQLiteStore) Backup(ctx context.Context, destPath string) error {
    _, err := s.reader.ExecContext(ctx, "VACUUM INTO ?", destPath)
    return err
}
```

- `VACUUM INTO` creates a compacted copy without disrupting the live database
- Schedule nightly via config: `MAGNETAR_BACKUP_PATH=/data/backups/magnetar-%Y%m%d.db`
- Keep last N backups, configurable via `MAGNETAR_BACKUP_RETAIN=7`

### Write Safety Rules

1. All batch inserts wrapped in explicit transactions
2. Transaction batches capped at 5,000 rows (limits WAL growth and rollback journal size)
3. `synchronous=NORMAL` with WAL — writes survive process crashes, only lose data on OS crash + power loss (acceptable trade-off for ~10x write throughput)
4. If absolute durability required: `MAGNETAR_DB_SYNC_FULL=true` sets `synchronous=FULL`
5. `ANALYZE` runs after every 50,000 inserts to keep the query planner informed

### MariaDB Safety

When using the MariaDB backend:
- `innodb_flush_log_at_trx_commit=1` (default, full durability)
- Connection pool with health checks and automatic reconnection
- All batch inserts use `INSERT ... ON DUPLICATE KEY UPDATE`
- Transaction isolation: `READ COMMITTED` (matches SQLite WAL behavior)

---

## Torznab Category System

### Newznab Category Mapping

Sonarr, Radarr, and Prowlarr use [Newznab categories](https://newznab.readthedocs.io/en/latest/misc/api/#predefined-categories). Magnetar must map its internal `(category, quality)` tuple to the correct Newznab category IDs.

### Category Constants

```go
package torznab

// Newznab category IDs that Magnetar supports
const (
    // Movies (2000-2099)
    CatMovies      = 2000
    CatMoviesSD    = 2030
    CatMoviesHD    = 2040
    CatMoviesUHD   = 2045
    CatMoviesBluRay = 2050
    CatMoviesWebDL = 2070
    CatMoviesOther = 2020

    // TV (5000-5099)
    CatTV          = 5000
    CatTVSD        = 5030
    CatTVHD        = 5040
    CatTVUHD       = 5045
    CatTVWebDL     = 5010
    CatTVAnime     = 5070
    CatTVOther     = 5050
)
```

### Internal → Newznab Mapping

When a torrent is ingested, the media filter parses the name and sets `category` and `quality`. The Torznab API maps these to Newznab IDs at query time:

```go
// MapToNewznab converts internal category+quality+source info to Newznab category IDs.
// Returns multiple IDs since a torrent can belong to a parent + sub-category.
func MapToNewznab(t *Torrent, parsed *ParsedName) []int {
    var cats []int

    switch t.Category {
    case CategoryMovie:
        cats = append(cats, CatMovies) // always include parent
        switch {
        case parsed.IsBluRay:
            cats = append(cats, CatMoviesBluRay)
        case parsed.IsWebDL:
            cats = append(cats, CatMoviesWebDL)
        default:
            switch t.Quality {
            case QualitySD:
                cats = append(cats, CatMoviesSD)
            case QualityHD:
                cats = append(cats, CatMoviesHD)
            case QualityFHD:
                cats = append(cats, CatMoviesHD)  // 1080p maps to HD
            case QualityUHD:
                cats = append(cats, CatMoviesUHD)
            default:
                cats = append(cats, CatMoviesOther)
            }
        }

    case CategoryTV:
        cats = append(cats, CatTV)
        switch {
        case parsed.IsWebDL:
            cats = append(cats, CatTVWebDL)
        default:
            switch t.Quality {
            case QualitySD:
                cats = append(cats, CatTVSD)
            case QualityHD, QualityFHD:
                cats = append(cats, CatTVHD)
            case QualityUHD:
                cats = append(cats, CatTVUHD)
            default:
                cats = append(cats, CatTVOther)
            }
        }

    case CategoryAnime:
        cats = append(cats, CatTV, CatTVAnime)  // anime is a TV sub-category
    }

    return cats
}
```

### Newznab → Query Filter (Reverse Mapping)

When Sonarr/Radarr sends `&cat=2040,2045` (HD + UHD movies), Magnetar converts this back to a store query:

```go
// ParseNewznabCategories converts Newznab category IDs from a Torznab request
// into internal filter criteria for the store.
func ParseNewznabCategories(catIDs []int) SearchOpts {
    opts := SearchOpts{}
    catSet := make(map[int]bool)
    for _, id := range catIDs {
        catSet[id] = true
    }

    // Check parent categories first
    if catSet[CatMovies] {
        opts.Categories = append(opts.Categories, CategoryMovie)
    }
    if catSet[CatTV] {
        // TV parent includes anime unless specifically excluded
        opts.Categories = append(opts.Categories, CategoryTV, CategoryAnime)
    }

    // Sub-categories narrow quality/type
    if catSet[CatMoviesHD] {
        opts.Quality = append(opts.Quality, QualityHD, QualityFHD)
    }
    if catSet[CatMoviesUHD] {
        opts.Quality = append(opts.Quality, QualityUHD)
    }
    if catSet[CatTVAnime] {
        // If only anime requested, override categories
        opts.Categories = []Category{CategoryAnime}
    }
    // ... etc.

    return opts
}
```

### Caps Response

The `/api/torznab?t=caps` endpoint declares supported categories so Prowlarr knows what to query:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<caps>
  <server version="1.0" title="Magnetar" />
  <searching>
    <search available="yes" supportedParams="q" />
    <tv-search available="yes" supportedParams="q,season,ep,imdbid,tvdbid" />
    <movie-search available="yes" supportedParams="q,imdbid,tmdbid" />
  </searching>
  <categories>
    <category id="2000" name="Movies">
      <subcat id="2020" name="Movies/Other" />
      <subcat id="2030" name="Movies/SD" />
      <subcat id="2040" name="Movies/HD" />
      <subcat id="2045" name="Movies/UHD" />
      <subcat id="2050" name="Movies/BluRay" />
      <subcat id="2070" name="Movies/WEB-DL" />
    </category>
    <category id="5000" name="TV">
      <subcat id="5010" name="TV/WEB-DL" />
      <subcat id="5030" name="TV/SD" />
      <subcat id="5040" name="TV/HD" />
      <subcat id="5045" name="TV/UHD" />
      <subcat id="5050" name="TV/Other" />
      <subcat id="5070" name="TV/Anime" />
    </category>
  </categories>
</caps>
```

### Torznab Query Parameters

Full set of supported query parameters:

| Parameter | Description | Example |
|-----------|-------------|---------|
| `t` | Query type | `search`, `tvsearch`, `movie`, `caps` |
| `q` | Free text search | `Oppenheimer 2023` |
| `cat` | Newznab category IDs (comma-separated) | `2040,2045` |
| `imdbid` | IMDB ID (with or without `tt` prefix) | `tt15398776` or `15398776` |
| `tmdbid` | TMDB ID | `872585` |
| `tvdbid` | TVDB ID | `371572` |
| `season` | Season number | `1` |
| `ep` | Episode number | `5` |
| `limit` | Max results | `100` |
| `offset` | Pagination offset | `0` |
| `apikey` | API key (if configured) | `abc123` |

### Torznab Response

Each result includes `<attr>` tags with Newznab category IDs:

```xml
<item>
  <title>Movie.Name.2024.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-GROUP</title>
  <guid>abc123def456...</guid>
  <size>15032385536</size>
  <attr name="category" value="2000" />
  <attr name="category" value="2045" />
  <attr name="category" value="2070" />
  <attr name="imdb" value="tt15398776" />
  <attr name="tmdb" value="872585" />
  <attr name="seeders" value="142" />
  <attr name="peers" value="23" />
  <enclosure url="magnet:?xt=urn:btih:abc123..." length="15032385536" type="application/x-bittorrent" />
</item>
```

---

## Torrent Name Parsing & Classification

### ParsedName Structure

```go
type ParsedName struct {
    Title       string
    Year        int
    Season      int    // -1 if not present
    Episode     int    // -1 if not present
    Quality     Quality
    Source      string // "bluray", "web-dl", "webrip", "hdtv", "dvdrip", etc.
    Codec       string // "h264", "h265", "hevc", "av1", "xvid"
    Audio       string // "aac", "dd5.1", "dts-hd", "atmos", "truehd"
    HDR         string // "hdr", "hdr10", "hdr10+", "dolby-vision", "dv"
    Group       string
    IsRemux     bool
    IsBluRay    bool
    IsWebDL     bool
    IsWebRip    bool
    IMDBID      string // extracted from name if present
    Resolution  string // "480p", "720p", "1080p", "2160p"
    SubGroup    string // fansub group for anime [SubGroup]
}
```

### Quality Detection

```go
func detectQuality(name string) Quality {
    n := strings.ToLower(name)
    switch {
    case contains(n, "2160p", "4k", "uhd"):
        return QualityUHD
    case contains(n, "1080p", "1080i"):
        return QualityFHD
    case contains(n, "720p"):
        return QualityHD
    case contains(n, "480p", "dvdrip", "sdtv", "xvid"):
        return QualitySD
    default:
        return QualityUnknown
    }
}
```

### Category Detection (Media Filter)

Two-pass approach for maximum coverage:

```go
func classifyTorrent(name string, files []File) (Category, bool) {
    // Pass 1: Regex patterns
    if isAnime(name) {
        return CategoryAnime, true
    }
    if isTV(name) {
        return CategoryTV, true
    }
    if isMovie(name) {
        return CategoryMovie, true
    }

    // Pass 2: File extension heuristic
    if hasMediaFiles(files) {
        // If >80% of total size is video files, it's probably media
        // Use file count and naming for movie vs TV
        return guessFromFiles(files)
    }

    // Discard: not classifiable as media
    return 0, false
}

func isAnime(name string) bool {
    // [SubGroup] pattern (fansub releases)
    // Known fansub groups list
    // Nyaa-style naming: [Group] Title - Episode [Quality]
    // CRC32 checksums in brackets: [ABCD1234]
    ...
}

func isTV(name string) bool {
    // S01E01, S01E01-E03, S01, Season 1, Complete Series
    // Daily shows: 2024.01.15
    // Anime episode patterns: - 01, - 01v2, Episode 01
    ...
}

func isMovie(name string) bool {
    // Has quality tags but no season/episode markers
    // Year in name + quality + source = almost certainly a movie
    // Known movie release patterns
    ...
}

func hasMediaFiles(files []File) bool {
    videoExts := map[string]bool{
        ".mkv": true, ".mp4": true, ".avi": true,
        ".wmv": true, ".ts": true, ".m2ts": true,
    }
    var videoSize, totalSize int64
    for _, f := range files {
        totalSize += f.Size
        ext := strings.ToLower(filepath.Ext(f.Path))
        if videoExts[ext] {
            videoSize += f.Size
        }
    }
    return totalSize > 0 && float64(videoSize)/float64(totalSize) > 0.80
}
```

---

## Architecture

```
┌──────────────────┐
│   DHT Crawler    │──── discovers info_hashes from the DHT network
│   (BEP 5)       │
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│ Metadata Fetcher │──── fetches torrent name, size, files via BEP 9
│    (BEP 9)       │
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│  Name Parser     │──── extracts title, year, season, episode, quality, source
│  + Media Filter  │──── discards non-media, sets category + quality
└────────┬─────────┘
         │
         ▼
┌──────────────────────────────────────────────────────┐
│                    Store Interface                     │
│                                                       │
│  ┌─────────┐                        ┌──────────────┐ │
│  │ SQLite  │  (switch via config)   │   MariaDB    │ │
│  └─────────┘                        └──────────────┘ │
└────────────────────────┬─────────────────────────────┘
                         │
         ┌───────────────┼───────────────────┐
         │               │                   │
         ▼               ▼                   ▼
   ┌──────────┐   ┌─────────────┐   ┌──────────────┐
   │ Torznab  │   │ Hash Lookup │   │ Web Frontend │
   │ API      │   │ API         │   │ (embedded)   │
   └──────────┘   └─────────────┘   └──────────────┘
         │
    Sonarr / Radarr        ◄────── Background Matcher
    Prowlarr                       (TMDB/TVDB/AniList)
```

---

## API Endpoints

### Torznab (Sonarr/Radarr/Prowlarr compatible)

```
GET /api/torznab?t=caps
GET /api/torznab?t=search&q=...&cat=...&limit=100
GET /api/torznab?t=tvsearch&q=...&season=1&ep=1&imdbid=...&tvdbid=...
GET /api/torznab?t=movie&q=...&imdbid=...&tmdbid=...
```

**Prowlarr compatibility notes:**
- Accept `imdbid` with and without `tt` prefix
- Return `<attr name="category">` for every applicable Newznab ID
- Return `<attr name="imdb">` with just the numeric part (no `tt`)
- Support `offset` and `limit` for pagination
- Enclosure URL is a magnet link (no .torrent hosting)

### Hash Lookup (debrid addons)

```
GET  /api/torrents/:info_hash
POST /api/torrents/lookup  — {"hashes": ["abc123...", ...]}
```

Response includes files (decompressed from zstd blob):
```json
{
  "info_hash": "abc123...",
  "name": "Movie.Name.2024.2160p...",
  "size": 15032385536,
  "category": "movie",
  "quality": "uhd",
  "files": [
    {"path": "Movie.Name.2024.2160p.../Movie.Name.mkv", "size": 14998000000},
    {"path": "Movie.Name.2024.2160p.../Subs/English.srt", "size": 85000}
  ],
  "imdb_id": "tt15398776",
  "tmdb_id": 872585,
  "seeders": 142,
  "leechers": 23
}
```

### Health & Stats

```
GET /health        — 200 OK + DB integrity status
GET /api/stats     — counts, crawl rate, match progress, DB size, uptime
```

---

## DHT Crawler Design

### Approach
- Extracted from Bitmagnet source (MIT licensed), stripped of PostgreSQL/job queue dependencies
- BEP 5 (DHT Protocol) + BEP 9 (Metadata Extension)
- Maintain routing table of DHT nodes
- Listen for `get_peers` and `announce_peer` to discover info_hashes
- Fetch metadata from announcing peers for each new hash
- Classify immediately — discard non-media before DB insert
- Configurable crawl rate (torrents/hour)

### Implementation: Extract from Bitmagnet
Bitmagnet's DHT crawler (MIT licensed) is purpose-built for exactly this use case. Rather than writing from scratch or pulling in heavy libraries like `anacrolix/dht`, extract and adapt Bitmagnet's crawler code:

- **What to extract**: DHT routing table, `get_peers`/`announce_peer` handling, BEP 9 metadata fetcher
- **What to strip**: Bitmagnet's job queue integration, PostgreSQL storage calls, classification pipeline
- **What to replace**: swap their storage layer for Magnetar's `Store` interface, swap their classifier for Magnetar's media filter
- **Key files** (as of Bitmagnet v0.9.x): `internal/dht/`, `internal/metainfo/`, `internal/protocol/`
- Bitmagnet's crawler is already well-optimized for throughput and handles peer connection pooling, timeout management, and UDP socket multiplexing

### Resource Budget
- CPU: 1 core for ~1,000 torrents/hour
- RAM: ~100-200MB (routing table + in-flight fetches)
- Network: moderate UDP traffic (configurable)

---

## Background Metadata Matcher

### Match Queue Strategy

Uses `match_status` + `match_after` columns as a built-in queue with exponential backoff:

```go
// Backoff schedule: 0 → 1h → 6h → 24h → give up
func nextRetryDelay(attempts int) time.Duration {
    switch attempts {
    case 0: return 0
    case 1: return 1 * time.Hour
    case 2: return 6 * time.Hour
    case 3: return 24 * time.Hour
    default: return 0 // won't be called, status set to Failed at 4
    }
}
```

### Query (both backends)

```sql
-- SQLite
SELECT * FROM torrents
WHERE match_status IN (0, 2)      -- unmatched or failed (retrying)
  AND match_after < unixepoch()
  AND match_attempts < 4
ORDER BY discovered_at DESC
LIMIT 100;

-- MariaDB
SELECT * FROM torrents
WHERE match_status IN (0, 2)
  AND match_after < UNIX_TIMESTAMP()
  AND match_attempts < 4
ORDER BY discovered_at DESC
LIMIT 100;
```

### Match Pipeline

1. Parse torrent name → title, year, season, episode
2. If IMDB ID in name → use directly
3. Query TMDB Search API with title + year
4. Cross-reference with TVDB for TV shows
5. For anime: query AniList/Kitsu by title
6. On match: update row with all external IDs, set `match_status = 1`
7. On failure: increment `match_attempts`, set `match_after` with backoff, set `match_status = 2`
8. After 4 failures: set `match_status = 2` permanently (no more `match_after`)

### Rate Limiting

- TMDB: 40 requests/10 seconds (their limit)
- TVDB: configurable, default 20 req/s
- AniList: 90 requests/minute (their limit)
- Internal token bucket rate limiter per API

---

## Performance Tuning

### SQLite-Specific

| Setting | Value | Reason |
|---------|-------|--------|
| `journal_mode` | WAL | concurrent reads + writes |
| `synchronous` | NORMAL | safe with WAL, 10x faster writes |
| `cache_size` | -65536 (64MB) | keeps hot pages in memory |
| `mmap_size` | 268435456 (256MB) | OS page cache accelerates reads |
| `temp_store` | MEMORY | temp tables in RAM |
| `wal_autocheckpoint` | 1000 | prevent WAL bloat |
| `page_size` | 4096 | matches OS page size |

### Scheduled Maintenance

```go
// Background maintenance goroutine
func (s *SQLiteStore) maintenanceLoop(ctx context.Context) {
    // Every 5 minutes: checkpoint WAL
    // Every 50,000 inserts: ANALYZE
    // Every 24 hours: PRAGMA quick_check
    // Weekly (configurable): VACUUM INTO backup path
}
```

### Query Optimization Notes

1. **FTS5 with category filter**: join FTS results with main table, don't put category in FTS
2. **Bulk hash lookup**: use `WHERE info_hash IN (?, ?, ...)` not loop (one B-tree scan)
3. **Partial indexes**: `WHERE imdb_id IS NOT NULL` saves ~40% index size on sparse columns
4. **STRICT mode**: catches type errors at insert time, prevents silent data corruption
5. **BLOB primary key**: 20-byte `info_hash` is ~50% smaller than 40-char hex, faster comparisons

### MariaDB-Specific

| Setting | Value | Reason |
|---------|-------|--------|
| `innodb_buffer_pool_size` | 1-2GB | main performance lever |
| `innodb_log_file_size` | 256MB | larger = fewer checkpoints |
| `innodb_flush_log_at_trx_commit` | 1 | full durability |
| `innodb_read_io_threads` | 4 | parallel reads |
| `key_block_size` | 8 | InnoDB page compression |
| `max_connections` | 50 | Magnetar + shared users |

---

## Configuration (Complete)

```env
# Core
MAGNETAR_PORT=3333
MAGNETAR_API_KEY=                          # optional, empty = no auth
MAGNETAR_LOG_LEVEL=info

# Database
MAGNETAR_DB_BACKEND=sqlite                 # "sqlite" or "mariadb"
MAGNETAR_DB_PATH=/data/magnetar.db         # SQLite path
MAGNETAR_DB_CACHE_SIZE=65536               # SQLite page cache (KB)
MAGNETAR_DB_MMAP_SIZE=268435456            # SQLite mmap (bytes)
MAGNETAR_DB_SYNC_FULL=false                # SQLite synchronous=FULL if true
MAGNETAR_DB_DSN=                           # MariaDB DSN
MAGNETAR_DB_MAX_OPEN_CONNS=25              # MariaDB connection pool
MAGNETAR_DB_MAX_IDLE_CONNS=10

# DHT Crawler
MAGNETAR_CRAWL_ENABLED=true
MAGNETAR_CRAWL_RATE=1000                   # target torrents/hour
MAGNETAR_CRAWL_WORKERS=4
MAGNETAR_CRAWL_PORT=6881                   # UDP listen port

# Metadata Matching
MAGNETAR_MATCH_ENABLED=true
MAGNETAR_MATCH_BATCH_SIZE=100
MAGNETAR_MATCH_INTERVAL=10s
MAGNETAR_MATCH_MAX_ATTEMPTS=4
MAGNETAR_TMDB_API_KEY=
MAGNETAR_TVDB_API_KEY=

# Backup
MAGNETAR_BACKUP_ENABLED=false
MAGNETAR_BACKUP_PATH=/data/backups
MAGNETAR_BACKUP_SCHEDULE=0 3 * * *        # cron: 3am daily
MAGNETAR_BACKUP_RETAIN=7                  # keep last N

# Maintenance
MAGNETAR_ANALYZE_INTERVAL=50000           # run ANALYZE every N inserts
MAGNETAR_INTEGRITY_CHECK_DAILY=true       # daily quick_check
```

---

## Deployment

### SQLite Mode (Recommended)

```yaml
services:
  magnetar:
    image: ghcr.io/you/magnetar:latest
    ports:
      - "3333:3333"
      - "6881:6881/udp"
    volumes:
      - ./data:/data
    environment:
      - MAGNETAR_DB_PATH=/data/magnetar.db
      - MAGNETAR_TMDB_API_KEY=${TMDB_API_KEY}
    restart: unless-stopped
```

### MariaDB Mode

```yaml
services:
  magnetar:
    image: ghcr.io/you/magnetar:latest
    ports:
      - "3333:3333"
      - "6881:6881/udp"
    environment:
      - MAGNETAR_DB_BACKEND=mariadb
      - MAGNETAR_DB_DSN=magnetar:${DB_PASS}@tcp(mariadb:3306)/magnetar
      - MAGNETAR_TMDB_API_KEY=${TMDB_API_KEY}
    depends_on:
      mariadb:
        condition: service_healthy
    restart: unless-stopped

  mariadb:
    image: mariadb:11
    volumes:
      - ./mysql:/var/lib/mysql
    environment:
      - MARIADB_DATABASE=magnetar
      - MARIADB_USER=magnetar
      - MARIADB_PASSWORD=${DB_PASS}
      - MARIADB_ROOT_PASSWORD=${DB_ROOT_PASS}
    healthcheck:
      test: ["CMD", "healthcheck.sh", "--connect", "--innodb_initialized"]
      interval: 10s
      timeout: 5s
      retries: 5
    restart: unless-stopped
```

---

## Project Structure

```
magnetar/
├── cmd/
│   └── magnetar/
│       └── main.go              # CLI: serve, migrate, import, backup
├── internal/
│   ├── config/
│   │   └── config.go            # env parsing, validation
│   ├── store/
│   │   ├── store.go             # Store interface + types
│   │   ├── sqlite.go            # SQLite implementation
│   │   ├── mariadb.go           # MariaDB implementation
│   │   ├── migrate.go           # cross-backend migration tool
│   │   └── store_test.go        # shared test suite (runs against both)
│   ├── crawler/
│   │   ├── dht.go               # BEP 5 DHT protocol
│   │   ├── metadata.go          # BEP 9 metadata fetch
│   │   └── crawler.go           # orchestration, rate limiting
│   ├── classify/
│   │   ├── filter.go            # media filter / categorizer
│   │   ├── parser.go            # torrent name parser (RTN-style)
│   │   ├── quality.go           # quality detection
│   │   └── patterns.go          # regex patterns, fansub groups
│   ├── matcher/
│   │   ├── matcher.go           # background worker loop
│   │   ├── tmdb.go              # TMDB API client
│   │   ├── tvdb.go              # TVDB API client
│   │   ├── anilist.go           # AniList API client
│   │   └── kitsu.go             # Kitsu API client
│   ├── api/
│   │   ├── server.go            # HTTP server setup, middleware
│   │   ├── torznab.go           # Torznab endpoint + XML generation
│   │   ├── torznab_caps.go      # caps response
│   │   ├── torznab_categories.go # Newznab category mapping
│   │   ├── hash.go              # hash lookup endpoints
│   │   └── stats.go             # health + stats endpoints
│   ├── importer/
│   │   └── bitmagnet.go         # Bitmagnet DB export importer
│   └── compress/
│       └── zstd.go              # zstd compress/decompress for files blob
├── frontend/                    # SvelteKit app (built + embedded)
│   ├── src/
│   └── build/                   # static output, embedded via go:embed
├── embed.go                     # go:embed for frontend static files
├── Dockerfile
├── docker-compose.yml
├── docker-compose.mariadb.yml
└── go.mod
```

---

## Phases

### Phase 1: Foundation
- Project scaffolding, config, CLI
- Store interface + SQLite implementation
- Schema, migrations, PRAGMAs, two-pool connections
- FTS5 setup with sync triggers
- Basic integrity check on startup
- Torrent name parser + quality detection

### Phase 2: Torznab + Hash API
- Torznab API: search, tvsearch, movie, caps
- Proper Newznab category mapping (2000/2030/2040/2045/2050/2070/5000/5010/5030/5040/5045/5070)
- Hash lookup API (single + bulk)
- Stats endpoint
- Test with Prowlarr → Sonarr/Radarr

### Phase 3: DHT Crawler
- Extract DHT crawler from Bitmagnet source (MIT licensed)
- Strip PostgreSQL/job queue dependencies, wire to Store interface
- Media filter / categorizer (two-pass)
- Crawl rate control + metrics
- Discard non-media at ingest

### Phase 4: Metadata Matching
- Background matcher with exponential backoff
- TMDB/TVDB API clients
- AniList/Kitsu matching for anime
- IMDB ID extraction from torrent names
- Rate limiting per API

### Phase 5: MariaDB Backend + Migration
- MariaDB Store implementation
- Cross-backend migration tool
- Shared integration test suite for both backends
- MariaDB docker-compose variant

### Phase 6: Frontend + Polish
- SvelteKit search UI (embedded in binary)
- Seeder/leecher updates (periodic DHT scrape)
- Bitmagnet DB import tool (one-time migration from existing Bitmagnet instance)
- Backup automation
- Prowlarr integration guide

---

## Success Metrics

- 2M+ media torrents in under 2GB (SQLite)
- Torznab queries < 20ms at 20M rows
- DHT crawl rate 1,000+ new media torrents/hour
- 90%+ match rate for movies/TV against TMDB
- < 1GB RAM total (Go binary + SQLite)
- Seamless migration between SQLite and MariaDB with zero data loss
- Full Prowlarr/Sonarr/Radarr compatibility with correct category mapping
- Daily integrity checks pass with zero corruption

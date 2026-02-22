package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/magnetar/magnetar/internal/config"
	"github.com/magnetar/magnetar/internal/tasklog"

	_ "github.com/mattn/go-sqlite3"
)

type SQLiteStore struct {
	writer *sql.DB
	reader *sql.DB
	cfg    *config.Config

	insertCount  atomic.Int64
	closed       atomic.Bool
	closeMu      sync.Mutex
	taskRegistry *tasklog.Registry
}

// SetTaskRegistry sets the task registry for reporting maintenance task status.
func (s *SQLiteStore) SetTaskRegistry(r *tasklog.Registry) {
	s.taskRegistry = r
}

func NewSQLiteStore(ctx context.Context, cfg *config.Config) (*SQLiteStore, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}

	dataDir := filepath.Dir(cfg.DBPath)
	if err := os.MkdirAll(dataDir, 0750); err != nil { //nolint:gosec // 0750 is intentional
		return nil, fmt.Errorf("creating data directory: %w", err)
	}

	s := &SQLiteStore{cfg: cfg}

	writerDSN := fmt.Sprintf("%s?_journal_mode=WAL&_busy_timeout=5000", cfg.DBPath)
	var err error
	s.writer, err = sql.Open("sqlite3", writerDSN)
	if err != nil {
		return nil, fmt.Errorf("opening writer connection: %w", err)
	}
	s.writer.SetMaxOpenConns(1)

	if err := s.runPragmas(ctx, s.writer); err != nil {
		_ = s.writer.Close()
		return nil, fmt.Errorf("setting writer pragmas: %w", err)
	}

	readerDSN := fmt.Sprintf("%s?mode=ro&_journal_mode=WAL", cfg.DBPath)
	s.reader, err = sql.Open("sqlite3", readerDSN)
	if err != nil {
		_ = s.writer.Close()
		return nil, fmt.Errorf("opening reader connection: %w", err)
	}
	s.reader.SetMaxOpenConns(runtime.NumCPU())

	if err := s.runPragmas(ctx, s.reader); err != nil {
		_ = s.writer.Close()
		_ = s.reader.Close()
		return nil, fmt.Errorf("setting reader pragmas: %w", err)
	}

	if err := s.IntegrityCheck(ctx); err != nil {
		_ = s.writer.Close()
		_ = s.reader.Close()
		return nil, fmt.Errorf("integrity check failed: %w", err)
	}

	if err := s.Migrate(ctx); err != nil {
		_ = s.writer.Close()
		_ = s.reader.Close()
		return nil, fmt.Errorf("migration failed: %w", err)
	}

	go s.maintenanceLoop()

	return s, nil
}

func (s *SQLiteStore) runPragmas(ctx context.Context, db *sql.DB) error {
	syncMode := "NORMAL"
	if s.cfg.DBSyncFull {
		syncMode = "FULL"
	}

	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		fmt.Sprintf("PRAGMA synchronous = %s", syncMode),
		"PRAGMA wal_autocheckpoint = 1000",
		fmt.Sprintf("PRAGMA cache_size = %d", s.cfg.DBCacheSize),
		fmt.Sprintf("PRAGMA mmap_size = %d", s.cfg.DBMmapSize),
		"PRAGMA temp_store = MEMORY",
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
	}

	for _, p := range pragmas {
		if _, err := db.ExecContext(ctx, p); err != nil {
			return fmt.Errorf("pragma %s: %w", p, err)
		}
	}

	return nil
}

func (s *SQLiteStore) Migrate(ctx context.Context) error {
	schema := `
	CREATE TABLE IF NOT EXISTS torrents (
		info_hash      BLOB PRIMARY KEY,
		name           TEXT NOT NULL,
		size           INTEGER,
		category       INTEGER NOT NULL,
		quality        INTEGER DEFAULT 0,
		files          BLOB,
		imdb_id        TEXT,
		tmdb_id        INTEGER,
		tvdb_id        INTEGER,
		anilist_id     INTEGER,
		kitsu_id       INTEGER,
		media_year     INTEGER,
		match_status   INTEGER DEFAULT 0,
		match_attempts INTEGER DEFAULT 0,
		match_after    INTEGER DEFAULT 0,
		seeders        INTEGER DEFAULT 0,
		leechers       INTEGER DEFAULT 0,
		source         INTEGER DEFAULT 0,
		discovered_at  INTEGER NOT NULL,
		updated_at     INTEGER
	) STRICT;

	CREATE VIRTUAL TABLE IF NOT EXISTS torrents_fts USING fts5(
		name,
		content='torrents',
		content_rowid='rowid',
		tokenize='unicode61 remove_diacritics 2'
	);

	CREATE TRIGGER IF NOT EXISTS torrents_fts_insert AFTER INSERT ON torrents BEGIN
		INSERT INTO torrents_fts(rowid, name) VALUES (new.rowid, new.name);
	END;

	CREATE TRIGGER IF NOT EXISTS torrents_fts_delete AFTER DELETE ON torrents BEGIN
		INSERT INTO torrents_fts(torrents_fts, rowid, name) VALUES ('delete', old.rowid, old.name);
	END;

	CREATE TRIGGER IF NOT EXISTS torrents_fts_update AFTER UPDATE OF name ON torrents BEGIN
		INSERT INTO torrents_fts(torrents_fts, rowid, name) VALUES ('delete', old.rowid, old.name);
		INSERT INTO torrents_fts(rowid, name) VALUES (new.rowid, new.name);
	END;

	CREATE INDEX IF NOT EXISTS idx_imdb       ON torrents(imdb_id)       WHERE imdb_id IS NOT NULL;
	CREATE INDEX IF NOT EXISTS idx_tmdb       ON torrents(tmdb_id)       WHERE tmdb_id IS NOT NULL;
	CREATE INDEX IF NOT EXISTS idx_tvdb       ON torrents(tvdb_id)       WHERE tvdb_id IS NOT NULL;
	CREATE INDEX IF NOT EXISTS idx_anilist    ON torrents(anilist_id)    WHERE anilist_id IS NOT NULL;
	CREATE INDEX IF NOT EXISTS idx_kitsu      ON torrents(kitsu_id)      WHERE kitsu_id IS NOT NULL;
	CREATE INDEX IF NOT EXISTS idx_category   ON torrents(category, quality, media_year);
	CREATE INDEX IF NOT EXISTS idx_discovered ON torrents(discovered_at DESC);
	CREATE INDEX IF NOT EXISTS idx_match_queue ON torrents(match_status, match_after) WHERE match_status != 1;
	CREATE INDEX IF NOT EXISTS idx_scrape_stale ON torrents(match_status, updated_at) WHERE match_status = 1;

	CREATE TABLE IF NOT EXISTS rejected_hashes (
		info_hash    BLOB PRIMARY KEY,
		rejected_at  INTEGER NOT NULL
	) STRICT;

	CREATE TABLE IF NOT EXISTS settings (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	) STRICT;
	`

	_, err := s.writer.ExecContext(ctx, schema)
	if err != nil {
		return fmt.Errorf("executing schema: %w", err)
	}

	return nil
}

func (s *SQLiteStore) UpsertTorrent(ctx context.Context, t *Torrent) error {
	if t == nil || len(t.InfoHash) != 20 {
		return ErrInvalidHash
	}

	filesBlob, err := compressFiles(t.Files)
	if err != nil {
		return fmt.Errorf("compressing files: %w", err)
	}

	query := `
	INSERT OR REPLACE INTO torrents (
		info_hash, name, size, category, quality, files,
		imdb_id, tmdb_id, tvdb_id, anilist_id, kitsu_id, media_year,
		match_status, match_attempts, match_after,
		seeders, leechers, source, discovered_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = s.writer.ExecContext(ctx, query,
		t.InfoHash, t.Name, t.Size, t.Category, t.Quality, filesBlob,
		nullString(t.IMDBID), nullInt(t.TMDBID), nullInt(t.TVDBID), nullInt(t.AniListID), nullInt(t.KitsuID), nullInt(t.MediaYear),
		t.MatchStatus, t.MatchAttempts, t.MatchAfter,
		t.Seeders, t.Leechers, t.Source, t.DiscoveredAt, t.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("upserting torrent: %w", err)
	}

	s.incrementInsertCount()
	return nil
}

func (s *SQLiteStore) UpsertTorrents(ctx context.Context, ts []*Torrent) error {
	if len(ts) == 0 {
		return nil
	}

	batchSize := 5000
	for i := 0; i < len(ts); i += batchSize {
		end := i + batchSize
		if end > len(ts) {
			end = len(ts)
		}
		if err := s.upsertBatch(ctx, ts[i:end]); err != nil {
			return err
		}
		s.insertCount.Add(int64(end - i))
	}

	return nil
}

func (s *SQLiteStore) upsertBatch(ctx context.Context, batch []*Torrent) error {
	tx, err := s.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, `
	INSERT OR REPLACE INTO torrents (
		info_hash, name, size, category, quality, files,
		imdb_id, tmdb_id, tvdb_id, anilist_id, kitsu_id, media_year,
		match_status, match_attempts, match_after,
		seeders, leechers, source, discovered_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("preparing statement: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, t := range batch {
		if t == nil || len(t.InfoHash) != 20 {
			_ = tx.Rollback()
			return ErrInvalidHash
		}

		filesBlob, err := compressFiles(t.Files)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("compressing files: %w", err)
		}

		_, err = stmt.ExecContext(ctx,
			t.InfoHash, t.Name, t.Size, t.Category, t.Quality, filesBlob,
			nullString(t.IMDBID), nullInt(t.TMDBID), nullInt(t.TVDBID), nullInt(t.AniListID), nullInt(t.KitsuID), nullInt(t.MediaYear),
			t.MatchStatus, t.MatchAttempts, t.MatchAfter,
			t.Seeders, t.Leechers, t.Source, t.DiscoveredAt, t.UpdatedAt,
		)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("upserting torrent: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetTorrent(ctx context.Context, infoHash []byte) (*Torrent, error) {
	if len(infoHash) != 20 {
		return nil, ErrInvalidHash
	}

	query := `
	SELECT info_hash, name, size, category, quality, files,
		imdb_id, tmdb_id, tvdb_id, anilist_id, kitsu_id, media_year,
		match_status, match_attempts, match_after,
		seeders, leechers, source, discovered_at, updated_at
	FROM torrents WHERE info_hash = ?`

	row := s.reader.QueryRowContext(ctx, query, infoHash)
	t, err := scanRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("querying torrent: %w", err)
	}
	return t, nil
}

func (s *SQLiteStore) BulkLookup(ctx context.Context, hashes [][]byte) ([]*Torrent, error) {
	if len(hashes) == 0 {
		return nil, nil
	}

	for _, h := range hashes {
		if len(h) != 20 {
			return nil, ErrInvalidHash
		}
	}

	placeholders := ""
	args := make([]interface{}, len(hashes))
	for i, h := range hashes {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args[i] = h
	}

	//nolint:gosec // placeholders are parameterized, column list is static
	query := fmt.Sprintf(`
	SELECT info_hash, name, size, category, quality, files,
		imdb_id, tmdb_id, tvdb_id, anilist_id, kitsu_id, media_year,
		match_status, match_attempts, match_after,
		seeders, leechers, source, discovered_at, updated_at
	FROM torrents WHERE info_hash IN (%s)`, placeholders)

	rows, err := s.reader.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("bulk querying torrents: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var torrents []*Torrent
	for rows.Next() {
		t, err := scanRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning torrent: %w", err)
		}
		torrents = append(torrents, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rows: %w", err)
	}

	return torrents, nil
}

func (s *SQLiteStore) DeleteTorrent(ctx context.Context, infoHash []byte) error {
	if len(infoHash) != 20 {
		return ErrInvalidHash
	}

	result, err := s.writer.ExecContext(ctx, "DELETE FROM torrents WHERE info_hash = ?", infoHash)
	if err != nil {
		return fmt.Errorf("deleting torrent: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}

	return nil
}

const sqlOrderByDiscoveredAtDesc = " ORDER BY discovered_at DESC LIMIT ? OFFSET ?"

func (s *SQLiteStore) ListRecent(ctx context.Context, opts SearchOpts) (*SearchResult, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}

	countQuery := `SELECT COUNT(*) FROM torrents`
	searchQuery := `SELECT ` + torrentSelectColumns + ` FROM torrents`

	var args []interface{}
	var whereClauses []string

	if len(opts.Categories) > 0 {
		placeholders := ""
		for i, cat := range opts.Categories {
			if i > 0 {
				placeholders += ","
			}
			placeholders += "?"
			args = append(args, cat)
		}
		whereClauses = append(whereClauses, fmt.Sprintf("category IN (%s)", placeholders))
	}
	if len(opts.Quality) > 0 {
		placeholders := ""
		for i, q := range opts.Quality {
			if i > 0 {
				placeholders += ","
			}
			placeholders += "?"
			args = append(args, q)
		}
		whereClauses = append(whereClauses, fmt.Sprintf("quality IN (%s)", placeholders))
	}

	if len(whereClauses) > 0 {
		where := " WHERE " + strings.Join(whereClauses, " AND ")
		countQuery += where
		searchQuery += where
	}

	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)

	var total int
	if err := s.reader.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, fmt.Errorf("counting results: %w", err)
	}

	searchQuery += sqlOrderByDiscoveredAtDesc
	args = append(args, limit, opts.Offset)

	rows, err := s.reader.QueryContext(ctx, searchQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("querying recent torrents: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var torrents []*Torrent
	for rows.Next() {
		t, err := scanRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning torrent: %w", err)
		}
		torrents = append(torrents, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rows: %w", err)
	}

	return &SearchResult{
		Torrents: torrents,
		Total:    total,
	}, nil
}

func (s *SQLiteStore) SearchByName(ctx context.Context, query string, opts SearchOpts) (*SearchResult, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	ftsQuery := sanitizeFTSQuery(query)
	if ftsQuery == "" {
		return &SearchResult{Total: 0}, nil
	}
	ftsQuery += "*"

	countQuery := `SELECT COUNT(*) FROM torrents_fts WHERE torrents_fts MATCH ?`
	args := []interface{}{ftsQuery}

	searchQuery := `
	SELECT t.info_hash, t.name, t.size, t.category, t.quality, t.files,
		t.imdb_id, t.tmdb_id, t.tvdb_id, t.anilist_id, t.kitsu_id, t.media_year,
		t.match_status, t.match_attempts, t.match_after,
		t.seeders, t.leechers, t.source, t.discovered_at, t.updated_at
	FROM torrents t
	JOIN torrents_fts fts ON t.rowid = fts.rowid
	WHERE torrents_fts MATCH ?`

	var whereClauses []string
	if len(opts.Categories) > 0 {
		placeholders := ""
		for i, cat := range opts.Categories {
			if i > 0 {
				placeholders += ","
			}
			placeholders += "?"
			args = append(args, cat)
		}
		whereClauses = append(whereClauses, fmt.Sprintf("t.category IN (%s)", placeholders))
	}
	if len(opts.Quality) > 0 {
		placeholders := ""
		for i, q := range opts.Quality {
			if i > 0 {
				placeholders += ","
			}
			placeholders += "?"
			args = append(args, q)
		}
		whereClauses = append(whereClauses, fmt.Sprintf("t.quality IN (%s)", placeholders))
	}
	if opts.MinYear > 0 {
		whereClauses = append(whereClauses, "t.media_year >= ?")
		args = append(args, opts.MinYear)
	}
	if opts.MaxYear > 0 {
		whereClauses = append(whereClauses, "t.media_year <= ?")
		args = append(args, opts.MaxYear)
	}

	for _, clause := range whereClauses {
		searchQuery += " AND " + clause
		countQuery += " AND EXISTS (SELECT 1 FROM torrents t WHERE t.rowid = torrents_fts.rowid AND " + clause[len("t."):] + ")"
	}

	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)

	var total int
	if err := s.reader.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, fmt.Errorf("counting results: %w", err)
	}

	searchQuery += " ORDER BY t.discovered_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, opts.Offset)

	rows, err := s.reader.QueryContext(ctx, searchQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("searching torrents: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var torrents []*Torrent
	for rows.Next() {
		t, err := scanRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning torrent: %w", err)
		}
		torrents = append(torrents, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rows: %w", err)
	}

	return &SearchResult{
		Torrents: torrents,
		Total:    total,
	}, nil
}

func (s *SQLiteStore) SearchByExternalID(ctx context.Context, id ExternalID) ([]*Torrent, error) {
	var column string
	switch id.Type {
	case "imdb":
		column = "imdb_id"
	case "tmdb":
		column = "tmdb_id"
	case "tvdb":
		column = "tvdb_id"
	case "anilist":
		column = "anilist_id"
	case "kitsu":
		column = "kitsu_id"
	default:
		return nil, fmt.Errorf("unknown external ID type: %s", id.Type)
	}

	//nolint:gosec // column is validated against a fixed allowlist above
	query := fmt.Sprintf(`
	SELECT info_hash, name, size, category, quality, files,
		imdb_id, tmdb_id, tvdb_id, anilist_id, kitsu_id, media_year,
		match_status, match_attempts, match_after,
		seeders, leechers, source, discovered_at, updated_at
	FROM torrents WHERE %s = ?`, column)

	rows, err := s.reader.QueryContext(ctx, query, id.Value)
	if err != nil {
		return nil, fmt.Errorf("searching by external ID: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var torrents []*Torrent
	for rows.Next() {
		t, err := scanRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning torrent: %w", err)
		}
		torrents = append(torrents, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rows: %w", err)
	}

	return torrents, nil
}

func (s *SQLiteStore) FetchUnmatched(ctx context.Context, limit int) ([]*Torrent, error) {
	if limit <= 0 {
		limit = 100
	}

	query := `
	SELECT info_hash, name, size, category, quality, files,
		imdb_id, tmdb_id, tvdb_id, anilist_id, kitsu_id, media_year,
		match_status, match_attempts, match_after,
		seeders, leechers, source, discovered_at, updated_at
	FROM torrents
	WHERE match_status IN (0, 2)
		AND match_after < unixepoch()
		AND match_attempts < 4
	ORDER BY discovered_at DESC
	LIMIT ?`

	rows, err := s.reader.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("fetching unmatched: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var torrents []*Torrent
	for rows.Next() {
		t, err := scanRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning torrent: %w", err)
		}
		torrents = append(torrents, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rows: %w", err)
	}

	return torrents, nil
}

func (s *SQLiteStore) UpdateMatchResult(ctx context.Context, infoHash []byte, m MatchResult) error {
	if len(infoHash) != 20 {
		return ErrInvalidHash
	}

	query := `
	UPDATE torrents SET
		match_status = ?,
		imdb_id = ?,
		tmdb_id = ?,
		tvdb_id = ?,
		anilist_id = ?,
		kitsu_id = ?,
		media_year = ?,
		match_attempts = match_attempts + 1,
		match_after = ?,
		updated_at = ?
	WHERE info_hash = ?`

	result, err := s.writer.ExecContext(ctx, query,
		m.Status,
		nullString(m.IMDBID),
		nullInt(m.TMDBID),
		nullInt(m.TVDBID),
		nullInt(m.AniListID),
		nullInt(m.KitsuID),
		nullInt(m.Year),
		m.MatchAfter,
		time.Now().Unix(),
		infoHash,
	)
	if err != nil {
		return fmt.Errorf("updating match result: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}

	return nil
}

func (s *SQLiteStore) UpdateCategory(ctx context.Context, infoHash []byte, category Category) error {
	if len(infoHash) != 20 {
		return ErrInvalidHash
	}

	result, err := s.writer.ExecContext(ctx,
		`UPDATE torrents SET category = ?, updated_at = ? WHERE info_hash = ?`,
		category, time.Now().Unix(), infoHash,
	)
	if err != nil {
		return fmt.Errorf("updating category: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}

	return nil
}

func (s *SQLiteStore) ListByMatchStatus(ctx context.Context, status MatchStatus, limit, offset int) (*SearchResult, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 1000 {
		limit = 1000
	}
	if offset < 0 {
		offset = 0
	}

	var total int
	if err := s.reader.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM torrents WHERE match_status = ?", status,
	).Scan(&total); err != nil {
		return nil, fmt.Errorf("counting by match status: %w", err)
	}

	query := `SELECT ` + torrentSelectColumns + ` FROM torrents WHERE match_status = ? ORDER BY updated_at DESC LIMIT ? OFFSET ?`

	rows, err := s.reader.QueryContext(ctx, query, status, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing by match status: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var torrents []*Torrent
	for rows.Next() {
		t, err := scanRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning torrent: %w", err)
		}
		torrents = append(torrents, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rows: %w", err)
	}

	return &SearchResult{
		Torrents: torrents,
		Total:    total,
	}, nil
}

func (s *SQLiteStore) ResetFailedMatches(ctx context.Context) (int64, error) {
	result, err := s.writer.ExecContext(ctx,
		`UPDATE torrents SET match_status = 0, match_attempts = 0, match_after = 0, updated_at = ? WHERE match_status = 2`,
		time.Now().Unix(),
	)
	if err != nil {
		return 0, fmt.Errorf("resetting failed matches: %w", err)
	}
	return result.RowsAffected()
}

func (s *SQLiteStore) Stats(ctx context.Context) (*DBStats, error) {
	stats := &DBStats{}

	if err := s.reader.QueryRowContext(ctx, "SELECT COUNT(*) FROM torrents").Scan(&stats.TotalTorrents); err != nil {
		return nil, fmt.Errorf("getting total count: %w", err)
	}

	if err := s.reader.QueryRowContext(ctx, "SELECT COUNT(*) FROM torrents WHERE match_status = 0").Scan(&stats.Unmatched); err != nil {
		return nil, fmt.Errorf("getting unmatched count: %w", err)
	}

	if err := s.reader.QueryRowContext(ctx, "SELECT COUNT(*) FROM torrents WHERE match_status = 1").Scan(&stats.Matched); err != nil {
		return nil, fmt.Errorf("getting matched count: %w", err)
	}

	if err := s.reader.QueryRowContext(ctx, "SELECT COUNT(*) FROM torrents WHERE match_status = 2").Scan(&stats.Failed); err != nil {
		return nil, fmt.Errorf("getting failed count: %w", err)
	}

	if info, err := os.Stat(s.cfg.DBPath); err == nil {
		stats.DBSize = info.Size()
	}

	if err := s.reader.QueryRowContext(ctx, "SELECT COALESCE(MAX(discovered_at), 0) FROM torrents").Scan(&stats.LastCrawl); err != nil {
		return nil, fmt.Errorf("getting last crawl: %w", err)
	}

	return stats, nil
}

func (s *SQLiteStore) GetSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := s.reader.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrSettingNotFound
		}
		return "", fmt.Errorf("getting setting %q: %w", key, err)
	}
	return value, nil
}

func (s *SQLiteStore) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.writer.ExecContext(ctx,
		"INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)", key, value)
	if err != nil {
		return fmt.Errorf("setting %q: %w", key, err)
	}
	return nil
}

func (s *SQLiteStore) GetAllSettings(ctx context.Context) (map[string]string, error) {
	rows, err := s.reader.QueryContext(ctx, "SELECT key, value FROM settings")
	if err != nil {
		return nil, fmt.Errorf("querying settings: %w", err)
	}
	defer func() { _ = rows.Close() }()

	settings := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("scanning setting: %w", err)
		}
		settings[k] = v
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating settings: %w", err)
	}
	return settings, nil
}

func (s *SQLiteStore) RejectHashes(ctx context.Context, hashes [][]byte) error {
	if len(hashes) == 0 {
		return nil
	}

	tx, err := s.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO rejected_hashes (info_hash, rejected_at) VALUES (?, ?)`)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("preparing statement: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	now := time.Now().Unix()
	for _, h := range hashes {
		if _, err := stmt.ExecContext(ctx, h, now); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("inserting rejected hash: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	return nil
}

func (s *SQLiteStore) AreRejected(ctx context.Context, hashes [][]byte) (map[[20]byte]bool, error) {
	result := make(map[[20]byte]bool)
	if len(hashes) == 0 {
		return result, nil
	}

	placeholders := ""
	args := make([]interface{}, len(hashes))
	for i, h := range hashes {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args[i] = h
	}

	//nolint:gosec // placeholders are parameterized
	query := fmt.Sprintf(`SELECT info_hash FROM rejected_hashes WHERE info_hash IN (%s)`, placeholders)

	rows, err := s.reader.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying rejected hashes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var h []byte
		if err := rows.Scan(&h); err != nil {
			return nil, fmt.Errorf("scanning rejected hash: %w", err)
		}
		if len(h) == 20 {
			var key [20]byte
			copy(key[:], h)
			result[key] = true
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rows: %w", err)
	}

	return result, nil
}

func (s *SQLiteStore) RejectedHashCount(ctx context.Context) (int64, error) {
	var count int64
	if err := s.reader.QueryRowContext(ctx, "SELECT COUNT(*) FROM rejected_hashes").Scan(&count); err != nil {
		return 0, fmt.Errorf("counting rejected hashes: %w", err)
	}
	return count, nil
}

func (s *SQLiteStore) PurgeOldRejected(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan).Unix()
	result, err := s.writer.ExecContext(ctx, `DELETE FROM rejected_hashes WHERE rejected_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("purging old rejected hashes: %w", err)
	}
	return result.RowsAffected()
}

func (s *SQLiteStore) PurgeJunkTorrents(ctx context.Context) (int64, error) {
	// Delete torrents with match_after set far in the future (100-year junk marker).
	// These are legacy junk torrents from before the junk filter existed.
	cutoff := time.Now().Unix() + 50*365*24*60*60 // now + 50 years
	result, err := s.writer.ExecContext(ctx, `DELETE FROM torrents WHERE match_after > ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("purging junk torrents: %w", err)
	}
	return result.RowsAffected()
}

func (s *SQLiteStore) Close() error {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()

	if s.closed.Load() {
		return nil
	}

	s.closed.Store(true)

	var errs []error
	if err := s.writer.Close(); err != nil {
		errs = append(errs, fmt.Errorf("closing writer: %w", err))
	}
	if err := s.reader.Close(); err != nil {
		errs = append(errs, fmt.Errorf("closing reader: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing store: %v", errs)
	}
	return nil
}

func (s *SQLiteStore) Checkpoint(ctx context.Context) error {
	var result string
	if err := s.writer.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&result, new(int), new(int)); err != nil {
		return fmt.Errorf("checkpoint: %w", err)
	}
	return nil
}

func (s *SQLiteStore) Analyze(ctx context.Context) error {
	if _, err := s.writer.ExecContext(ctx, "PRAGMA analysis_limit = 0"); err != nil {
		return fmt.Errorf("setting analysis limit: %w", err)
	}
	if _, err := s.writer.ExecContext(ctx, "ANALYZE"); err != nil {
		return fmt.Errorf("analyze: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ListRecentlyUpdated(ctx context.Context, limit int) ([]*Torrent, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}

	query := `SELECT ` + torrentSelectColumns + ` FROM torrents WHERE updated_at > 0 AND match_status = ? ORDER BY updated_at DESC LIMIT ?`
	rows, err := s.reader.QueryContext(ctx, query, MatchMatched, limit)
	if err != nil {
		return nil, fmt.Errorf("list recently updated: %w", err)
	}
	defer rows.Close()

	var torrents []*Torrent
	for rows.Next() {
		t, err := scanRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning row: %w", err)
		}
		torrents = append(torrents, t)
	}
	return torrents, rows.Err()
}

func (s *SQLiteStore) ListAllMatched(ctx context.Context) ([]*Torrent, error) {
	query := `SELECT ` + torrentSelectColumns + ` FROM torrents WHERE match_status = ? ORDER BY updated_at ASC`
	rows, err := s.reader.QueryContext(ctx, query, MatchMatched)
	if err != nil {
		return nil, fmt.Errorf("list stale for scrape: %w", err)
	}
	defer rows.Close()

	var torrents []*Torrent
	for rows.Next() {
		t, err := scanRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning row: %w", err)
		}
		torrents = append(torrents, t)
	}
	return torrents, rows.Err()
}

func (s *SQLiteStore) BulkUpdateSeedersLeechers(ctx context.Context, updates []SeedersLeechersUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	tx, err := s.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx,
		`UPDATE torrents SET seeders = ?, leechers = ?, updated_at = ? WHERE info_hash = ?`)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("preparing statement: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	now := time.Now().Unix()
	for _, u := range updates {
		if _, err := stmt.ExecContext(ctx, u.Seeders, u.Leechers, now, u.InfoHash); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("updating seeders/leechers: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	return nil
}

func (s *SQLiteStore) IntegrityCheck(ctx context.Context) error {
	var result string
	if err := s.reader.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); err != nil {
		return fmt.Errorf("integrity check query: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("database corruption detected: %s", result)
	}
	return nil
}

func (s *SQLiteStore) QuickCheck(ctx context.Context) error {
	var result string
	if err := s.reader.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&result); err != nil {
		return fmt.Errorf("quick check query: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("database corruption detected: %s", result)
	}
	return nil
}

func (s *SQLiteStore) Backup(ctx context.Context, destPath string) error {
	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0750); err != nil { //nolint:gosec // 0750 is intentional
		return fmt.Errorf("creating backup directory: %w", err)
	}

	_, err := s.reader.ExecContext(ctx, "VACUUM INTO ?", destPath)
	if err != nil {
		return fmt.Errorf("backup vacuum: %w", err)
	}
	return nil
}

// fetchPage is used by the migration tool to stream rows via keyset pagination.
func (s *SQLiteStore) fetchPage(ctx context.Context, afterDiscoveredAt int64, afterInfoHash []byte, limit int) ([]*Torrent, error) {
	query := `SELECT ` + torrentSelectColumns + `
	FROM torrents
	WHERE (discovered_at, info_hash) > (?, ?)
	ORDER BY discovered_at ASC, info_hash ASC
	LIMIT ?`

	if afterInfoHash == nil {
		afterInfoHash = make([]byte, 20)
	}

	rows, err := s.reader.QueryContext(ctx, query, afterDiscoveredAt, afterInfoHash, limit)
	if err != nil {
		return nil, fmt.Errorf("fetching page: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var torrents []*Torrent
	for rows.Next() {
		t, err := scanRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning torrent: %w", err)
		}
		torrents = append(torrents, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rows: %w", err)
	}

	return torrents, nil
}

func (s *SQLiteStore) incrementInsertCount() {
	count := s.insertCount.Add(1)
	if s.cfg.AnalyzeInterval > 0 && count%int64(s.cfg.AnalyzeInterval) == 0 {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			err := s.Analyze(ctx)
			if s.taskRegistry != nil {
				result := "OK"
				if err != nil {
					result = err.Error()
				}
				s.taskRegistry.Record("ANALYZE", result, err)
			}
		}()
	}
}

func (s *SQLiteStore) maintenanceLoop() {
	checkpointTicker := time.NewTicker(5 * time.Minute)
	defer checkpointTicker.Stop()

	var integrityTicker *time.Ticker
	var integrityC <-chan time.Time
	if s.cfg.IntegrityCheckDaily {
		integrityTicker = time.NewTicker(24 * time.Hour)
		integrityC = integrityTicker.C
		defer integrityTicker.Stop()
	}

	for {
		select {
		case <-checkpointTicker.C:
			if s.closed.Load() {
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			err := s.Checkpoint(ctx)
			if s.taskRegistry != nil {
				result := "OK"
				if err != nil {
					result = err.Error()
				}
				s.taskRegistry.Record("WAL Checkpoint", result, err)
			}
			cancel()

		case <-integrityC:
			if s.closed.Load() {
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			err := s.QuickCheck(ctx)
			if s.taskRegistry != nil {
				result := "OK"
				if err != nil {
					result = err.Error()
				}
				s.taskRegistry.Record("Integrity Check", result, err)
			}
			cancel()
		}
	}
}

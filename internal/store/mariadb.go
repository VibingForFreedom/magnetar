package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/magnetar/magnetar/internal/config"

	_ "github.com/go-sql-driver/mysql"
)

// MariaDBStore implements Store using MariaDB as the backend.
type MariaDBStore struct {
	db          *sql.DB
	cfg         *config.Config
	insertCount atomic.Int64
	closed      atomic.Bool
}

// NewMariaDBStore creates a new MariaDB-backed store.
func NewMariaDBStore(ctx context.Context, cfg *config.Config) (*MariaDBStore, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	if cfg.DBDSN == "" {
		return nil, fmt.Errorf("DB_DSN is required for MariaDB backend")
	}

	db, err := sql.Open("mysql", cfg.DBDSN)
	if err != nil {
		return nil, fmt.Errorf("opening mariadb connection: %w", err)
	}

	db.SetMaxOpenConns(cfg.DBMaxOpenConns)
	db.SetMaxIdleConns(cfg.DBMaxIdleConns)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("pinging mariadb: %w", err)
	}

	s := &MariaDBStore{db: db, cfg: cfg}

	if err := s.Migrate(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migration failed: %w", err)
	}

	go s.maintenanceLoop()

	return s, nil
}

func (s *MariaDBStore) Migrate(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS torrents (
			info_hash      BINARY(20)    NOT NULL PRIMARY KEY,
			name           VARCHAR(1000) NOT NULL,
			size           BIGINT,
			category       TINYINT       NOT NULL,
			quality        TINYINT       NOT NULL DEFAULT 0,
			files          MEDIUMBLOB,
			imdb_id        VARCHAR(16),
			tmdb_id        INT,
			tvdb_id        INT,
			anilist_id     INT,
			kitsu_id       INT,
			media_year     SMALLINT,
			match_status   TINYINT       NOT NULL DEFAULT 0,
			match_attempts TINYINT       NOT NULL DEFAULT 0,
			match_after    BIGINT        NOT NULL DEFAULT 0,
			seeders        INT           NOT NULL DEFAULT 0,
			leechers       INT           NOT NULL DEFAULT 0,
			source         TINYINT       NOT NULL DEFAULT 0,
			discovered_at  BIGINT        NOT NULL,
			updated_at     BIGINT,
			FULLTEXT INDEX idx_name_fts (name)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 ROW_FORMAT=COMPRESSED KEY_BLOCK_SIZE=8`,
		`CREATE INDEX idx_imdb ON torrents(imdb_id)`,
		`CREATE INDEX idx_tmdb ON torrents(tmdb_id)`,
		`CREATE INDEX idx_tvdb ON torrents(tvdb_id)`,
		`CREATE INDEX idx_anilist ON torrents(anilist_id)`,
		`CREATE INDEX idx_kitsu ON torrents(kitsu_id)`,
		`CREATE INDEX idx_category ON torrents(category, quality, media_year)`,
		`CREATE INDEX idx_discovered ON torrents(discovered_at)`,
		`CREATE INDEX idx_match_queue ON torrents(match_status, match_after)`,
		`CREATE TABLE IF NOT EXISTS rejected_hashes (
			info_hash    BINARY(20) NOT NULL PRIMARY KEY,
			rejected_at  BIGINT     NOT NULL
		) ENGINE=InnoDB`,
		`CREATE TABLE IF NOT EXISTS settings (
			` + "`key`" + `   VARCHAR(255) NOT NULL PRIMARY KEY,
			value VARCHAR(4000) NOT NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	}

	for _, stmt := range statements {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			// Ignore "duplicate key name" (index already exists) and "table already exists"
			errStr := err.Error()
			if strings.Contains(errStr, "Duplicate key name") || strings.Contains(errStr, "already exists") {
				continue
			}
			return fmt.Errorf("executing schema statement: %w", err)
		}
	}

	return nil
}

func (s *MariaDBStore) UpsertTorrent(ctx context.Context, t *Torrent) error {
	if t == nil || len(t.InfoHash) != 20 {
		return ErrInvalidHash
	}

	filesBlob, err := compressFiles(t.Files)
	if err != nil {
		return fmt.Errorf("compressing files: %w", err)
	}

	query := `
	INSERT INTO torrents (
		info_hash, name, size, category, quality, files,
		imdb_id, tmdb_id, tvdb_id, anilist_id, kitsu_id, media_year,
		match_status, match_attempts, match_after,
		seeders, leechers, source, discovered_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON DUPLICATE KEY UPDATE
		name=VALUES(name), size=VALUES(size), category=VALUES(category),
		quality=VALUES(quality), files=VALUES(files),
		imdb_id=VALUES(imdb_id), tmdb_id=VALUES(tmdb_id), tvdb_id=VALUES(tvdb_id),
		anilist_id=VALUES(anilist_id), kitsu_id=VALUES(kitsu_id), media_year=VALUES(media_year),
		match_status=VALUES(match_status), match_attempts=VALUES(match_attempts),
		match_after=VALUES(match_after),
		seeders=VALUES(seeders), leechers=VALUES(leechers), source=VALUES(source),
		updated_at=VALUES(updated_at)`

	_, err = s.db.ExecContext(ctx, query,
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

func (s *MariaDBStore) UpsertTorrents(ctx context.Context, ts []*Torrent) error {
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

func (s *MariaDBStore) upsertBatch(ctx context.Context, batch []*Torrent) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, `
	INSERT INTO torrents (
		info_hash, name, size, category, quality, files,
		imdb_id, tmdb_id, tvdb_id, anilist_id, kitsu_id, media_year,
		match_status, match_attempts, match_after,
		seeders, leechers, source, discovered_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON DUPLICATE KEY UPDATE
		name=VALUES(name), size=VALUES(size), category=VALUES(category),
		quality=VALUES(quality), files=VALUES(files),
		imdb_id=VALUES(imdb_id), tmdb_id=VALUES(tmdb_id), tvdb_id=VALUES(tvdb_id),
		anilist_id=VALUES(anilist_id), kitsu_id=VALUES(kitsu_id), media_year=VALUES(media_year),
		match_status=VALUES(match_status), match_attempts=VALUES(match_attempts),
		match_after=VALUES(match_after),
		seeders=VALUES(seeders), leechers=VALUES(leechers), source=VALUES(source),
		updated_at=VALUES(updated_at)`)
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

func (s *MariaDBStore) GetTorrent(ctx context.Context, infoHash []byte) (*Torrent, error) {
	if len(infoHash) != 20 {
		return nil, ErrInvalidHash
	}

	query := `SELECT ` + torrentSelectColumns + ` FROM torrents WHERE info_hash = ?`

	row := s.db.QueryRowContext(ctx, query, infoHash)
	t, err := scanRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("querying torrent: %w", err)
	}
	return t, nil
}

func (s *MariaDBStore) BulkLookup(ctx context.Context, hashes [][]byte) ([]*Torrent, error) {
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
	query := fmt.Sprintf(`SELECT `+torrentSelectColumns+` FROM torrents WHERE info_hash IN (%s)`, placeholders)

	rows, err := s.db.QueryContext(ctx, query, args...)
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

func (s *MariaDBStore) DeleteTorrent(ctx context.Context, infoHash []byte) error {
	if len(infoHash) != 20 {
		return ErrInvalidHash
	}

	result, err := s.db.ExecContext(ctx, "DELETE FROM torrents WHERE info_hash = ?", infoHash)
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

func (s *MariaDBStore) ListRecent(ctx context.Context, opts SearchOpts) (*SearchResult, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}

	countSQL := `SELECT COUNT(*) FROM torrents`
	searchSQL := `SELECT ` + torrentSelectColumns + ` FROM torrents`

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
		countSQL += where
		searchSQL += where
	}

	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)

	var total int
	if err := s.db.QueryRowContext(ctx, countSQL, countArgs...).Scan(&total); err != nil {
		return nil, fmt.Errorf("counting results: %w", err)
	}

	searchSQL += sqlOrderByDiscoveredAtDesc
	args = append(args, limit, opts.Offset)

	rows, err := s.db.QueryContext(ctx, searchSQL, args...)
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

func (s *MariaDBStore) SearchByName(ctx context.Context, query string, opts SearchOpts) (*SearchResult, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	ftsQuery := query + "*"

	countSQL := `SELECT COUNT(*) FROM torrents WHERE MATCH(name) AGAINST(? IN BOOLEAN MODE)`
	searchSQL := `SELECT ` + torrentSelectColumns + ` FROM torrents WHERE MATCH(name) AGAINST(? IN BOOLEAN MODE)`
	args := []interface{}{ftsQuery}

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
	if opts.MinYear > 0 {
		whereClauses = append(whereClauses, "media_year >= ?")
		args = append(args, opts.MinYear)
	}
	if opts.MaxYear > 0 {
		whereClauses = append(whereClauses, "media_year <= ?")
		args = append(args, opts.MaxYear)
	}

	for _, clause := range whereClauses {
		searchSQL += " AND " + clause
		countSQL += " AND " + clause
	}

	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)

	var total int
	if err := s.db.QueryRowContext(ctx, countSQL, countArgs...).Scan(&total); err != nil {
		return nil, fmt.Errorf("counting results: %w", err)
	}

	searchSQL += sqlOrderByDiscoveredAtDesc
	args = append(args, limit, opts.Offset)

	rows, err := s.db.QueryContext(ctx, searchSQL, args...)
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

func (s *MariaDBStore) SearchByExternalID(ctx context.Context, id ExternalID) ([]*Torrent, error) {
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
	query := fmt.Sprintf(`SELECT `+torrentSelectColumns+` FROM torrents WHERE %s = ?`, column)

	rows, err := s.db.QueryContext(ctx, query, id.Value)
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

func (s *MariaDBStore) FetchUnmatched(ctx context.Context, limit int) ([]*Torrent, error) {
	if limit <= 0 {
		limit = 100
	}

	query := `SELECT ` + torrentSelectColumns + `
	FROM torrents
	WHERE match_status IN (0, 2)
		AND match_after < UNIX_TIMESTAMP()
		AND match_attempts < 4
	ORDER BY discovered_at DESC
	LIMIT ?`

	rows, err := s.db.QueryContext(ctx, query, limit)
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

func (s *MariaDBStore) UpdateMatchResult(ctx context.Context, infoHash []byte, m MatchResult) error {
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

	result, err := s.db.ExecContext(ctx, query,
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

func (s *MariaDBStore) UpdateCategory(ctx context.Context, infoHash []byte, category Category) error {
	if len(infoHash) != 20 {
		return ErrInvalidHash
	}

	result, err := s.db.ExecContext(ctx,
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

func (s *MariaDBStore) ListByMatchStatus(ctx context.Context, status MatchStatus, limit, offset int) (*SearchResult, error) {
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
	if err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM torrents WHERE match_status = ?", status,
	).Scan(&total); err != nil {
		return nil, fmt.Errorf("counting by match status: %w", err)
	}

	query := `SELECT ` + torrentSelectColumns + ` FROM torrents WHERE match_status = ? ORDER BY updated_at DESC LIMIT ? OFFSET ?`

	rows, err := s.db.QueryContext(ctx, query, status, limit, offset)
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

func (s *MariaDBStore) ResetFailedMatches(ctx context.Context) (int64, error) {
	result, err := s.db.ExecContext(ctx,
		`UPDATE torrents SET match_status = 0, match_attempts = 0, match_after = 0, updated_at = ? WHERE match_status = 2`,
		time.Now().Unix(),
	)
	if err != nil {
		return 0, fmt.Errorf("resetting failed matches: %w", err)
	}
	return result.RowsAffected()
}

func (s *MariaDBStore) Stats(ctx context.Context) (*DBStats, error) {
	stats := &DBStats{}

	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM torrents").Scan(&stats.TotalTorrents); err != nil {
		return nil, fmt.Errorf("getting total count: %w", err)
	}

	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM torrents WHERE match_status = 0").Scan(&stats.Unmatched); err != nil {
		return nil, fmt.Errorf("getting unmatched count: %w", err)
	}

	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM torrents WHERE match_status = 1").Scan(&stats.Matched); err != nil {
		return nil, fmt.Errorf("getting matched count: %w", err)
	}

	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM torrents WHERE match_status = 2").Scan(&stats.Failed); err != nil {
		return nil, fmt.Errorf("getting failed count: %w", err)
	}

	var dbSize sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT SUM(data_length + index_length) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'torrents'`,
	).Scan(&dbSize)
	if err == nil {
		stats.DBSize = dbSize.Int64
	}

	if err := s.db.QueryRowContext(ctx, "SELECT COALESCE(MAX(discovered_at), 0) FROM torrents").Scan(&stats.LastCrawl); err != nil {
		return nil, fmt.Errorf("getting last crawl: %w", err)
	}

	return stats, nil
}

func (s *MariaDBStore) GetSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE `key` = ?", key).Scan(&value)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrSettingNotFound
		}
		return "", fmt.Errorf("getting setting %q: %w", key, err)
	}
	return value, nil
}

func (s *MariaDBStore) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO settings (`key`, value) VALUES (?, ?) ON DUPLICATE KEY UPDATE value = VALUES(value)", key, value)
	if err != nil {
		return fmt.Errorf("setting %q: %w", key, err)
	}
	return nil
}

func (s *MariaDBStore) GetAllSettings(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT `key`, value FROM settings")
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

func (s *MariaDBStore) RejectHashes(ctx context.Context, hashes [][]byte) error {
	if len(hashes) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, `INSERT IGNORE INTO rejected_hashes (info_hash, rejected_at) VALUES (?, ?)`)
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

func (s *MariaDBStore) AreRejected(ctx context.Context, hashes [][]byte) (map[[20]byte]bool, error) {
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

	rows, err := s.db.QueryContext(ctx, query, args...)
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

func (s *MariaDBStore) PurgeOldRejected(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan).Unix()
	result, err := s.db.ExecContext(ctx, `DELETE FROM rejected_hashes WHERE rejected_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("purging old rejected hashes: %w", err)
	}
	return result.RowsAffected()
}

func (s *MariaDBStore) PurgeJunkTorrents(ctx context.Context) (int64, error) {
	cutoff := time.Now().Unix() + 50*365*24*60*60
	result, err := s.db.ExecContext(ctx, `DELETE FROM torrents WHERE match_after > ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("purging junk torrents: %w", err)
	}
	return result.RowsAffected()
}

func (s *MariaDBStore) Close() error {
	if s.closed.Load() {
		return nil
	}
	s.closed.Store(true)

	if err := s.db.Close(); err != nil {
		return fmt.Errorf("closing mariadb connection: %w", err)
	}
	return nil
}

func (s *MariaDBStore) Checkpoint(_ context.Context) error {
	return nil // No-op for MariaDB
}

func (s *MariaDBStore) Analyze(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "ANALYZE TABLE torrents")
	if err != nil {
		return fmt.Errorf("analyze table: %w", err)
	}
	return nil
}

func (s *MariaDBStore) incrementInsertCount() {
	count := s.insertCount.Add(1)
	if s.cfg.AnalyzeInterval > 0 && count%int64(s.cfg.AnalyzeInterval) == 0 {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			if err := s.Analyze(ctx); err != nil {
				_ = err // best-effort background analyze
			}
		}()
	}
}

func (s *MariaDBStore) maintenanceLoop() {
	analyzeTicker := time.NewTicker(1 * time.Hour)
	defer analyzeTicker.Stop()

	for range analyzeTicker.C { // S1000: use for range instead of for { select {} }
		if s.closed.Load() {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		if err := s.Analyze(ctx); err != nil {
			_ = err // best-effort background analyze
		}
		cancel()
	}
}

// fetchPage is used by the migration tool to stream rows via keyset pagination.
func (s *MariaDBStore) fetchPage(ctx context.Context, afterDiscoveredAt int64, afterInfoHash []byte, limit int) ([]*Torrent, error) {
	query := `SELECT ` + torrentSelectColumns + `
	FROM torrents
	WHERE (discovered_at, info_hash) > (?, ?)
	ORDER BY discovered_at ASC, info_hash ASC
	LIMIT ?`

	if afterInfoHash == nil {
		afterInfoHash = make([]byte, 20)
	}

	rows, err := s.db.QueryContext(ctx, query, afterDiscoveredAt, afterInfoHash, limit)
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

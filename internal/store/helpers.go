package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/magnetar/magnetar/internal/compress"
)

// sanitizeFTSQuery strips FTS5/boolean-mode special characters and quotes each
// token so that user input like `m3gan\` or `foo"bar` cannot break the query.
// Returns an empty string if the input contains no searchable terms.
func sanitizeFTSQuery(raw string) string {
	// Strip everything that isn't a letter, digit, or whitespace.
	cleaned := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
			return r
		}
		return ' '
	}, raw)

	tokens := strings.Fields(cleaned)
	if len(tokens) == 0 {
		return ""
	}

	// Quote each token to prevent FTS operator interpretation.
	quoted := make([]string, 0, len(tokens))
	for _, t := range tokens {
		quoted = append(quoted, `"`+t+`"`)
	}
	return strings.Join(quoted, " ")
}

func nullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nullInt(i int) interface{} {
	if i == 0 {
		return nil
	}
	return i
}

func encodeFiles(files []File) ([]byte, error) {
	type fileJSON struct {
		Path string `json:"path"`
		Size int64  `json:"size"`
	}
	fjs := make([]fileJSON, 0, len(files)) // prealloc
	for _, f := range files {
		fjs = append(fjs, fileJSON(f)) // S1016: direct type conversion
	}
	return json.Marshal(fjs)
}

func decodeFiles(data []byte) ([]File, error) {
	type fileJSON struct {
		Path string `json:"path"`
		Size int64  `json:"size"`
	}
	var fjs []fileJSON
	if err := json.Unmarshal(data, &fjs); err != nil {
		return nil, err
	}
	files := make([]File, 0, len(fjs)) // prealloc
	for _, fj := range fjs {
		files = append(files, File(fj)) // S1016: direct type conversion
	}
	return files, nil
}

func compressFiles(files []File) ([]byte, error) {
	if len(files) == 0 {
		return nil, nil
	}
	data, err := encodeFiles(files)
	if err != nil {
		return nil, err
	}
	return compress.Compress(data)
}

func decompressFiles(data []byte) ([]File, error) {
	if len(data) == 0 {
		return nil, nil
	}
	decompressed, err := compress.Decompress(data)
	if err != nil {
		return nil, fmt.Errorf("decompressing files: %w", err)
	}
	return decodeFiles(decompressed)
}

// scanner is a common interface satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...interface{}) error
}

// scanRow scans a torrent row from any scanner (Row or Rows).
func scanRow(s scanner) (*Torrent, error) {
	t := &Torrent{}
	var filesBlob []byte
	var imdbID sql.NullString
	var tmdbID, tvdbID, anilistID, kitsuID, mediaYear sql.NullInt64
	var size sql.NullInt64
	var updatedAt sql.NullInt64

	err := s.Scan(
		&t.InfoHash, &t.Name, &size, &t.Category, &t.Quality, &filesBlob,
		&imdbID, &tmdbID, &tvdbID, &anilistID, &kitsuID, &mediaYear,
		&t.MatchStatus, &t.MatchAttempts, &t.MatchAfter,
		&t.Seeders, &t.Leechers, &t.Source, &t.DiscoveredAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}

	t.Size = size.Int64
	t.UpdatedAt = updatedAt.Int64
	if filesBlob != nil {
		t.Files, _ = decompressFiles(filesBlob)
	}

	t.IMDBID = imdbID.String
	t.TMDBID = int(tmdbID.Int64)
	t.TVDBID = int(tvdbID.Int64)
	t.AniListID = int(anilistID.Int64)
	t.KitsuID = int(kitsuID.Int64)
	t.MediaYear = int(mediaYear.Int64)

	return t, nil
}

// torrentSelectColumns is the standard SELECT column list for torrents.
const torrentSelectColumns = `info_hash, name, size, category, quality, files,
	imdb_id, tmdb_id, tvdb_id, anilist_id, kitsu_id, media_year,
	match_status, match_attempts, match_after,
	seeders, leechers, source, discovered_at, updated_at`

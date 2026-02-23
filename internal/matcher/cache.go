package matcher

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/valkey-io/valkey-go"
)

const (
	defaultHitTTL      = 24 * time.Hour
	defaultNegativeTTL = 6 * time.Hour

	prefixSearchMovie  = "magnetar:tmdb:search:movie:"
	prefixSearchTV     = "magnetar:tmdb:search:tv:"
	prefixFindIMDB     = "magnetar:tmdb:imdb:"
	prefixExtIDsMovie  = "magnetar:tmdb:extids:movie:"
	prefixExtIDsTV     = "magnetar:tmdb:extids:tv:"
	prefixSearchSeries = "magnetar:tvdb:series:"
	prefixAltMovie     = "magnetar:tmdb:alt:movie:"
	prefixAltTV        = "magnetar:tmdb:alt:tv:"
)

var nonAlphanumericASCII = regexp.MustCompile(`[^a-z0-9\p{L}\p{N}]+`)

// normalizeTitle lowercases and strips all non-alphanumeric/non-letter characters.
// Preserves Unicode letters (CJK, Cyrillic, etc.) for international title matching.
func normalizeTitle(title string) string {
	return nonAlphanumericASCII.ReplaceAllString(strings.ToLower(title), "")
}

// MatcherCache wraps TMDB/TVDB API calls with Valkey-backed caching.
// A nil *MatcherCache is safe — all methods pass through to the real client.
type MatcherCache struct {
	client      valkey.Client
	logger      *slog.Logger
	hitTTL      time.Duration
	negativeTTL time.Duration
}

// NewMatcherCache connects to Valkey at the given URL and returns a cache.
// Returns (nil, nil) if url is empty (cache disabled).
func NewMatcherCache(url string, logger *slog.Logger) (*MatcherCache, error) {
	if url == "" {
		return nil, nil
	}

	opt, err := valkey.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parsing valkey URL: %w", err)
	}

	client, err := valkey.NewClient(opt)
	if err != nil {
		return nil, fmt.Errorf("connecting to valkey: %w", err)
	}

	return &MatcherCache{
		client:      client,
		logger:      logger.With("component", "cache"),
		hitTTL:      defaultHitTTL,
		negativeTTL: defaultNegativeTTL,
	}, nil
}

// Close shuts down the Valkey connection.
func (c *MatcherCache) Close() {
	if c != nil {
		c.client.Close()
	}
}

// --- cached search result types ---

type searchResult struct {
	ID   int `json:"id"`
	Year int `json:"year"`
}

type findResult struct {
	TMDBID    int    `json:"tmdb_id"`
	MediaType string `json:"media_type"`
	Title     string `json:"title"`
	Year      int    `json:"year"`
}

type movieExtIDs struct {
	IMDBID string `json:"imdb_id"`
}

type tvExtIDs struct {
	IMDBID string `json:"imdb_id"`
	TVDBID int    `json:"tvdb_id"`
}

// --- sentinel for negative cache ---

const negativeSentinel = `{"_neg":true}`

func isNegative(data []byte) bool {
	return string(data) == negativeSentinel
}

// --- cache helpers ---

func (c *MatcherCache) get(ctx context.Context, key string) ([]byte, bool) {
	resp := c.client.Do(ctx, c.client.B().Get().Key(key).Build())
	b, err := resp.AsBytes()
	if err != nil {
		if !valkey.IsValkeyNil(err) {
			c.logger.Warn("cache get error", "key", key, "error", err)
		}
		return nil, false
	}
	return b, true
}

func (c *MatcherCache) setPositive(ctx context.Context, key string, data []byte) {
	err := c.client.Do(ctx,
		c.client.B().Set().Key(key).Value(string(data)).Ex(c.hitTTL).Build(),
	).Error()
	if err != nil {
		c.logger.Warn("cache set error", "key", key, "error", err)
	}
}

func (c *MatcherCache) setNegative(ctx context.Context, key string) {
	err := c.client.Do(ctx,
		c.client.B().Set().Key(key).Value(negativeSentinel).Ex(c.negativeTTL).Build(),
	).Error()
	if err != nil {
		c.logger.Warn("cache set negative error", "key", key, "error", err)
	}
}

func (c *MatcherCache) refreshTTL(ctx context.Context, key string) {
	err := c.client.Do(ctx,
		c.client.B().Expire().Key(key).Seconds(int64(c.hitTTL.Seconds())).Build(),
	).Error()
	if err != nil {
		c.logger.Warn("cache refresh ttl error", "key", key, "error", err)
	}
}

func searchKey(prefix, title string, year int) string {
	norm := normalizeTitle(title)
	return fmt.Sprintf("%s%s:%d", prefix, norm, year)
}

// --- SearchMovie ---

func (c *MatcherCache) SearchMovie(ctx context.Context, tmdb *TMDBClient, title string, year int) (int, int, error) {
	if c == nil {
		return tmdb.SearchMovie(ctx, title, year)
	}

	key := searchKey(prefixSearchMovie, title, year)

	// Check primary cache
	if data, ok := c.get(ctx, key); ok {
		if isNegative(data) {
			c.logger.Debug("cache hit (negative)", "key", key)
			return 0, 0, nil
		}
		var sr searchResult
		if err := json.Unmarshal(data, &sr); err == nil {
			c.logger.Debug("cache hit", "key", key, "tmdb_id", sr.ID)
			c.refreshTTL(ctx, key)
			return sr.ID, sr.Year, nil
		}
	}

	// Check alt-title index
	altKey := prefixAltMovie + normalizeTitle(title)
	if data, ok := c.get(ctx, altKey); ok && !isNegative(data) {
		var sr searchResult
		if err := json.Unmarshal(data, &sr); err == nil {
			c.logger.Debug("cache hit (alt title)", "key", altKey, "tmdb_id", sr.ID)
			c.refreshTTL(ctx, altKey)
			return sr.ID, sr.Year, nil
		}
	}

	// Cache miss — call TMDB
	tmdbID, releaseYear, err := tmdb.SearchMovie(ctx, title, year)
	if err != nil {
		return 0, 0, err
	}

	if tmdbID == 0 {
		c.setNegative(ctx, key)
		return 0, 0, nil
	}

	data, _ := json.Marshal(searchResult{ID: tmdbID, Year: releaseYear})
	c.setPositive(ctx, key, data)

	// Background: index alternative titles
	go c.indexAltTitles(context.Background(), tmdb, "movie", tmdbID, releaseYear)

	return tmdbID, releaseYear, nil
}

// --- SearchTV ---

func (c *MatcherCache) SearchTV(ctx context.Context, tmdb *TMDBClient, title string, year int) (int, int, error) {
	if c == nil {
		return tmdb.SearchTV(ctx, title, year)
	}

	key := searchKey(prefixSearchTV, title, year)

	if data, ok := c.get(ctx, key); ok {
		if isNegative(data) {
			c.logger.Debug("cache hit (negative)", "key", key)
			return 0, 0, nil
		}
		var sr searchResult
		if err := json.Unmarshal(data, &sr); err == nil {
			c.logger.Debug("cache hit", "key", key, "tmdb_id", sr.ID)
			c.refreshTTL(ctx, key)
			return sr.ID, sr.Year, nil
		}
	}

	// Check alt-title index
	altKey := prefixAltTV + normalizeTitle(title)
	if data, ok := c.get(ctx, altKey); ok && !isNegative(data) {
		var sr searchResult
		if err := json.Unmarshal(data, &sr); err == nil {
			c.logger.Debug("cache hit (alt title)", "key", altKey, "tmdb_id", sr.ID)
			c.refreshTTL(ctx, altKey)
			return sr.ID, sr.Year, nil
		}
	}

	tmdbID, firstAirYear, err := tmdb.SearchTV(ctx, title, year)
	if err != nil {
		return 0, 0, err
	}

	if tmdbID == 0 {
		c.setNegative(ctx, key)
		return 0, 0, nil
	}

	data, _ := json.Marshal(searchResult{ID: tmdbID, Year: firstAirYear})
	c.setPositive(ctx, key, data)

	go c.indexAltTitles(context.Background(), tmdb, "tv", tmdbID, firstAirYear)

	return tmdbID, firstAirYear, nil
}

// --- FindByIMDB ---

func (c *MatcherCache) FindByIMDB(ctx context.Context, tmdb *TMDBClient, imdbID string) (*TMDBFindResult, error) {
	if c == nil {
		return tmdb.FindByIMDB(ctx, imdbID)
	}

	key := prefixFindIMDB + imdbID

	if data, ok := c.get(ctx, key); ok {
		if isNegative(data) {
			c.logger.Debug("cache hit (negative)", "key", key)
			return nil, nil
		}
		var fr findResult
		if err := json.Unmarshal(data, &fr); err == nil {
			c.logger.Debug("cache hit", "key", key, "tmdb_id", fr.TMDBID)
			c.refreshTTL(ctx, key)
			return &TMDBFindResult{
				TMDBID:    fr.TMDBID,
				MediaType: fr.MediaType,
				Title:     fr.Title,
				Year:      fr.Year,
			}, nil
		}
	}

	result, err := tmdb.FindByIMDB(ctx, imdbID)
	if err != nil {
		return nil, err
	}

	if result == nil {
		c.setNegative(ctx, key)
		return nil, nil
	}

	data, _ := json.Marshal(findResult{
		TMDBID:    result.TMDBID,
		MediaType: result.MediaType,
		Title:     result.Title,
		Year:      result.Year,
	})
	c.setPositive(ctx, key, data)

	return result, nil
}

// --- GetMovieExternalIDs ---

func (c *MatcherCache) GetMovieExternalIDs(ctx context.Context, tmdb *TMDBClient, tmdbID int) (string, error) {
	if c == nil {
		return tmdb.GetMovieExternalIDs(ctx, tmdbID)
	}

	key := fmt.Sprintf("%s%d", prefixExtIDsMovie, tmdbID)

	if data, ok := c.get(ctx, key); ok {
		if isNegative(data) {
			return "", nil
		}
		var ext movieExtIDs
		if err := json.Unmarshal(data, &ext); err == nil {
			c.logger.Debug("cache hit", "key", key)
			c.refreshTTL(ctx, key)
			return ext.IMDBID, nil
		}
	}

	imdbID, err := tmdb.GetMovieExternalIDs(ctx, tmdbID)
	if err != nil {
		return "", err
	}

	if imdbID == "" {
		c.setNegative(ctx, key)
		return "", nil
	}

	data, _ := json.Marshal(movieExtIDs{IMDBID: imdbID})
	c.setPositive(ctx, key, data)

	return imdbID, nil
}

// --- GetTVExternalIDs ---

func (c *MatcherCache) GetTVExternalIDs(ctx context.Context, tmdb *TMDBClient, tmdbID int) (string, int, error) {
	if c == nil {
		return tmdb.GetTVExternalIDs(ctx, tmdbID)
	}

	key := fmt.Sprintf("%s%d", prefixExtIDsTV, tmdbID)

	if data, ok := c.get(ctx, key); ok {
		if isNegative(data) {
			return "", 0, nil
		}
		var ext tvExtIDs
		if err := json.Unmarshal(data, &ext); err == nil {
			c.logger.Debug("cache hit", "key", key)
			c.refreshTTL(ctx, key)
			return ext.IMDBID, ext.TVDBID, nil
		}
	}

	imdbID, tvdbID, err := tmdb.GetTVExternalIDs(ctx, tmdbID)
	if err != nil {
		return "", 0, err
	}

	if imdbID == "" && tvdbID == 0 {
		c.setNegative(ctx, key)
		return "", 0, nil
	}

	data, _ := json.Marshal(tvExtIDs{IMDBID: imdbID, TVDBID: tvdbID})
	c.setPositive(ctx, key, data)

	return imdbID, tvdbID, nil
}

// --- SearchSeries (TVDB) ---

func (c *MatcherCache) SearchSeries(ctx context.Context, tvdb *TVDBClient, title string, year int) (int, int, error) {
	if c == nil {
		return tvdb.SearchSeries(ctx, title, year)
	}

	key := searchKey(prefixSearchSeries, title, year)

	if data, ok := c.get(ctx, key); ok {
		if isNegative(data) {
			return 0, 0, nil
		}
		var sr searchResult
		if err := json.Unmarshal(data, &sr); err == nil {
			c.logger.Debug("cache hit", "key", key, "tvdb_id", sr.ID)
			c.refreshTTL(ctx, key)
			return sr.ID, sr.Year, nil
		}
	}

	tvdbID, firstAirYear, err := tvdb.SearchSeries(ctx, title, year)
	if err != nil {
		return 0, 0, err
	}

	if tvdbID == 0 {
		c.setNegative(ctx, key)
		return 0, 0, nil
	}

	data, _ := json.Marshal(searchResult{ID: tvdbID, Year: firstAirYear})
	c.setPositive(ctx, key, data)

	return tvdbID, firstAirYear, nil
}

// --- Alt-title indexing ---

// indexAltTitles fetches alternative titles AND translations from TMDB
// and stores reverse-index cache keys for each unique normalized title.
// This enables cache hits for non-English torrents (e.g. Japanese, Korean,
// French titles) without additional TMDB API calls.
func (c *MatcherCache) indexAltTitles(ctx context.Context, tmdb *TMDBClient, mediaType string, tmdbID int, year int) {
	// Collect titles from both sources — alt titles (market names, misspellings)
	// and translations (official localized titles in every language).
	var allTitles []string

	altTitles, err := tmdb.GetAlternativeTitles(ctx, mediaType, tmdbID)
	if err != nil {
		c.logger.Warn("failed to fetch alt titles", "media_type", mediaType, "tmdb_id", tmdbID, "error", err)
	} else {
		allTitles = append(allTitles, altTitles...)
	}

	translations, err := tmdb.GetTranslations(ctx, mediaType, tmdbID)
	if err != nil {
		c.logger.Warn("failed to fetch translations", "media_type", mediaType, "tmdb_id", tmdbID, "error", err)
	} else {
		allTitles = append(allTitles, translations...)
	}

	if len(allTitles) == 0 {
		return
	}

	prefix := prefixAltMovie
	if mediaType == "tv" {
		prefix = prefixAltTV
	}

	data, _ := json.Marshal(searchResult{ID: tmdbID, Year: year})

	// Deduplicate by normalized form to avoid redundant SET calls.
	seen := make(map[string]struct{}, len(allTitles))
	indexed := 0
	for _, title := range allTitles {
		norm := normalizeTitle(title)
		if norm == "" {
			continue
		}
		if _, ok := seen[norm]; ok {
			continue
		}
		seen[norm] = struct{}{}

		altKey := prefix + norm
		c.setPositive(ctx, altKey, data)
		indexed++
	}

	c.logger.Debug("indexed alt titles and translations",
		"media_type", mediaType,
		"tmdb_id", tmdbID,
		"alt_titles", len(altTitles),
		"translations", len(translations),
		"unique_indexed", indexed,
	)
}

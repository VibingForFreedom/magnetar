package matcher

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/magnetar/magnetar/internal/animedb"
	"github.com/magnetar/magnetar/internal/classify"
	"github.com/magnetar/magnetar/internal/metrics"
	"github.com/magnetar/magnetar/internal/store"
)

var backoffSchedule = []time.Duration{
	1 * time.Hour,
	6 * time.Hour,
	24 * time.Hour,
}

func backoffDuration(attempts int) time.Duration {
	if attempts < 0 {
		return 0
	}
	if attempts >= len(backoffSchedule) {
		return backoffSchedule[len(backoffSchedule)-1]
	}
	return backoffSchedule[attempts]
}

// Scaling tiers: backlog size → TMDB rate + worker count.
// TMDB allows ~40 req/s; each match uses 1-3 API calls,
// so workers * avg_calls <= rate limit.
type scaleTier struct {
	minBacklog int
	rps        float64
	burst      int
	workers    int
}

var scaleTiers = []scaleTier{
	{minBacklog: 5000, rps: 35, burst: 35, workers: 10},
	{minBacklog: 1000, rps: 20, burst: 20, workers: 6},
	{minBacklog: 100, rps: 10, burst: 10, workers: 4},
	{minBacklog: 0, rps: 4, burst: 4, workers: 1},
}

func tierForBacklog(backlog int) scaleTier {
	for _, t := range scaleTiers {
		if backlog >= t.minBacklog {
			return t
		}
	}
	return scaleTiers[len(scaleTiers)-1]
}

type Matcher struct {
	store   store.Store
	metrics *metrics.Metrics
	tmdb    *TMDBClient
	tvdb    *TVDBClient
	cache   *MatcherCache
	anilist *AniListClient
	kitsu   *KitsuClient
	animedb *animedb.AnimeDB
	cfg     Config
	paused  atomic.Bool
	logger  *slog.Logger
	stopped chan struct{}
}

// Pause stops the matcher from processing new batches.
func (m *Matcher) Pause() {
	m.paused.Store(true)
	m.logger.Info("matcher paused")
}

// Resume resumes matcher processing.
func (m *Matcher) Resume() {
	m.paused.Store(false)
	m.logger.Info("matcher resumed")
}

// IsPaused returns whether the matcher is paused.
func (m *Matcher) IsPaused() bool {
	return m.paused.Load()
}

func New(cfg Config, st store.Store, adb *animedb.AnimeDB, met *metrics.Metrics, logger *slog.Logger) *Matcher {
	m := &Matcher{
		store:   st,
		metrics: met,
		anilist: NewAniListClient(),
		kitsu:   NewKitsuClient(),
		animedb: adb,
		cfg:     cfg,
		logger:  logger.With("component", "matcher"),
		stopped: make(chan struct{}),
	}

	if cfg.TMDBAPIKey != "" {
		m.tmdb = NewTMDBClient(cfg.TMDBAPIKey)
	}
	if cfg.TVDBAPIKey != "" {
		m.tvdb = NewTVDBClient(cfg.TVDBAPIKey)
	}

	if cfg.ValkeyURL != "" {
		cache, err := NewMatcherCache(cfg.ValkeyURL, m.logger)
		if err != nil {
			m.logger.Error("failed to connect to valkey, caching disabled", "error", err)
		} else {
			m.cache = cache
			m.logger.Info("valkey cache enabled")
		}
	}

	return m
}

// CloseCache shuts down the Valkey cache connection.
func (m *Matcher) CloseCache() {
	if m.cache != nil {
		m.cache.Close()
	}
}

func (m *Matcher) Start(ctx context.Context) error {
	m.logger.Info("matcher started",
		"interval", m.cfg.Interval,
		"batch_size", m.cfg.BatchSize,
		"tmdb_enabled", m.tmdb != nil,
		"tvdb_enabled", m.tvdb != nil,
	)

	ticker := time.NewTicker(m.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-m.stopped:
			return nil
		case <-ticker.C:
			m.processBatch(ctx)
		}
	}
}

func (m *Matcher) Stop() {
	select {
	case <-m.stopped:
	default:
		close(m.stopped)
	}
}

// RunBatch runs one matching batch immediately, returning the number of torrents processed.
func (m *Matcher) RunBatch(ctx context.Context) (int, error) {
	torrents, err := m.store.FetchUnmatched(ctx, m.cfg.BatchSize)
	if err != nil {
		return 0, fmt.Errorf("fetching unmatched torrents: %w", err)
	}

	if len(torrents) == 0 {
		return 0, nil
	}

	m.scaleTMDB(len(torrents))
	m.processWithWorkers(ctx, torrents)
	return len(torrents), nil
}

func (m *Matcher) processBatch(ctx context.Context) {
	if m.paused.Load() {
		return
	}

	// Check total backlog to determine scaling tier
	backlog := m.cfg.BatchSize // fallback: assume batch-sized backlog
	if stats, err := m.store.Stats(ctx); err == nil {
		backlog = int(stats.Unmatched + stats.Failed)
	}

	tier := m.scaleTMDB(backlog)

	// Fetch a batch sized to what we can process this tick.
	// Each match uses ~2 TMDB API calls on average, so at R rps
	// over the tick interval we can match about R*interval/2 torrents.
	fetchSize := int(tier.rps*m.cfg.Interval.Seconds()) / 2
	if fetchSize > m.cfg.BatchSize {
		fetchSize = m.cfg.BatchSize
	}
	if fetchSize < 1 {
		fetchSize = 1
	}

	torrents, err := m.store.FetchUnmatched(ctx, fetchSize)
	if err != nil {
		m.logger.Error("fetching unmatched torrents", "error", err)
		return
	}

	if len(torrents) == 0 {
		return
	}

	m.processWithWorkersTier(ctx, torrents, tier)
}

// scaleTMDB adjusts the TMDB rate limiter and returns the worker count
// based on the current backlog size.
func (m *Matcher) scaleTMDB(backlog int) scaleTier {
	tier := tierForBacklog(backlog)
	if m.tmdb != nil {
		m.tmdb.SetRate(tier.rps, tier.burst)
	}
	m.logger.Info("matcher scaling",
		"backlog", backlog,
		"workers", tier.workers,
		"tmdb_rps", tier.rps,
	)
	return tier
}

// processWithWorkers fans out torrent matching across a dynamic worker pool.
// The TMDB rate limiter is shared across all workers, so they naturally throttle.
func (m *Matcher) processWithWorkers(ctx context.Context, torrents []*store.Torrent) {
	m.processWithWorkersTier(ctx, torrents, tierForBacklog(len(torrents)))
}

func (m *Matcher) processWithWorkersTier(ctx context.Context, torrents []*store.Torrent, tier scaleTier) {
	workers := tier.workers
	if workers > len(torrents) {
		workers = len(torrents)
	}

	work := make(chan *store.Torrent, len(torrents))
	for _, t := range torrents {
		work <- t
	}
	close(work)

	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for t := range work {
				select {
				case <-ctx.Done():
					return
				case <-m.stopped:
					return
				default:
				}

				// Skip adult/junk content — mark as failed with far-future backoff
				if classify.IsJunk(t.Name, nil) {
					result := store.MatchResult{
						Status:     store.MatchFailed,
						MatchAfter: time.Now().Add(876000 * time.Hour).Unix(), // ~100 years
					}
					if err := m.store.UpdateMatchResult(ctx, t.InfoHash, result); err != nil {
						m.logger.Error("updating junk match result",
							"info_hash", t.InfoHashHex(),
							"error", err,
						)
					}
					m.metrics.MatchJunk.Add(1)
					continue
				}

				m.metrics.MatchAttempts.Add(1)
				result := m.matchOne(ctx, t)
				if result.Status == store.MatchMatched {
					m.metrics.MatchSuccesses.Add(1)
					m.metrics.RecordMatch(1)
				} else {
					m.metrics.MatchFailures.Add(1)
					// Reset category to unknown on failure — the classifier
					// may have guessed wrong (e.g. adult content with 1080p tag → movie)
					if t.Category != store.CategoryUnknown {
						if err := m.store.UpdateCategory(ctx, t.InfoHash, store.CategoryUnknown); err != nil {
							m.logger.Error("resetting category on failure",
								"info_hash", t.InfoHashHex(),
								"error", err,
							)
						}
					}
				}
				if err := m.store.UpdateMatchResult(ctx, t.InfoHash, result); err != nil {
					m.logger.Error("updating match result",
						"info_hash", t.InfoHashHex(),
						"error", err,
					)
				}
			}
		}()
	}
	wg.Wait()
}

func (m *Matcher) matchOne(ctx context.Context, t *store.Torrent) store.MatchResult {
	parsed := classify.Parse(t.Name)

	imdbID := t.IMDBID
	if imdbID == "" {
		imdbID = parsed.IMDBID
	}

	// Path 1: IMDB ID available
	if imdbID != "" && m.tmdb != nil {
		result, ok := m.matchByIMDB(ctx, t, imdbID)
		if ok {
			return result
		}
	}

	// Path 2: Reclassify Unknown → Anime if offline DB matches
	if t.Category == store.CategoryUnknown && m.animedb != nil && m.animedb.IsLoaded() {
		if m.animedb.Contains(parsed.Title) {
			m.logger.Info("reclassifying unknown torrent as anime",
				"info_hash", t.InfoHashHex(),
				"title", parsed.Title,
			)
			if err := m.store.UpdateCategory(ctx, t.InfoHash, store.CategoryAnime); err != nil {
				m.logger.Error("failed to update category", "error", err)
			}
			return m.matchAnime(ctx, t, parsed)
		}
	}

	// Path 3: Dispatch by category
	switch t.Category {
	case store.CategoryMovie:
		return m.matchMovie(ctx, t, parsed)
	case store.CategoryTV:
		return m.matchTV(ctx, t, parsed)
	case store.CategoryAnime:
		return m.matchAnime(ctx, t, parsed)
	case store.CategoryUnknown:
		// Try movie match first, fall back to TV
		result := m.matchMovie(ctx, t, parsed)
		if result.Status == store.MatchMatched {
			return result
		}
		return m.matchTV(ctx, t, parsed)
	default:
		return m.matchMovie(ctx, t, parsed)
	}
}

func (m *Matcher) matchByIMDB(ctx context.Context, t *store.Torrent, imdbID string) (store.MatchResult, bool) {
	findRes, err := m.cache.FindByIMDB(ctx, m.tmdb, imdbID)
	if err != nil {
		m.logger.Warn("tmdb find by imdb failed", "imdb_id", imdbID, "error", err)
		return m.failResult(t), false
	}

	if findRes == nil {
		return store.MatchResult{}, false
	}

	result := store.MatchResult{
		Status: store.MatchMatched,
		IMDBID: imdbID,
		TMDBID: findRes.TMDBID,
		Year:   findRes.Year,
	}

	// Enrich with external IDs
	switch findRes.MediaType {
	case "movie":
		if extIMDB, err := m.cache.GetMovieExternalIDs(ctx, m.tmdb, findRes.TMDBID); err == nil && extIMDB != "" {
			result.IMDBID = extIMDB
		}
	case "tv":
		if extIMDB, tvdbID, err := m.cache.GetTVExternalIDs(ctx, m.tmdb, findRes.TMDBID); err == nil {
			if extIMDB != "" {
				result.IMDBID = extIMDB
			}
			result.TVDBID = tvdbID
		}
	}

	// If anime category, also query AniList + Kitsu
	if t.Category == store.CategoryAnime {
		parsed := classify.Parse(t.Name)
		title := parsed.Title
		if anilistID, err := m.anilist.SearchAnime(ctx, title); err == nil && anilistID > 0 {
			result.AniListID = anilistID
		}
		if kitsuID, err := m.kitsu.SearchAnime(ctx, title); err == nil && kitsuID > 0 {
			result.KitsuID = kitsuID
		}
	}

	m.logger.Info("matched by IMDB",
		"info_hash", t.InfoHashHex(),
		"imdb_id", result.IMDBID,
		"tmdb_id", result.TMDBID,
	)

	return result, true
}

func (m *Matcher) matchMovie(ctx context.Context, t *store.Torrent, parsed *classify.ParsedName) store.MatchResult {
	if m.tmdb == nil {
		return m.failResult(t)
	}

	title := parsed.Title
	year := parsed.Year

	tmdbID, releaseYear, err := m.cache.SearchMovie(ctx, m.tmdb, title, year)
	if err != nil {
		m.logger.Warn("tmdb search movie failed", "title", title, "error", err)
		return m.failResult(t)
	}

	// Retry without year if no results
	if tmdbID == 0 && year > 0 {
		tmdbID, releaseYear, err = m.cache.SearchMovie(ctx, m.tmdb, title, 0)
		if err != nil {
			m.logger.Warn("tmdb search movie retry failed", "title", title, "error", err)
			return m.failResult(t)
		}
	}

	if tmdbID == 0 {
		return m.failResult(t)
	}

	result := store.MatchResult{
		Status: store.MatchMatched,
		TMDBID: tmdbID,
		Year:   releaseYear,
	}

	// Get IMDB ID
	if imdbID, err := m.cache.GetMovieExternalIDs(ctx, m.tmdb, tmdbID); err == nil && imdbID != "" {
		result.IMDBID = imdbID
	}

	m.logger.Info("matched movie",
		"info_hash", t.InfoHashHex(),
		"title", title,
		"tmdb_id", tmdbID,
		"imdb_id", result.IMDBID,
	)

	return result
}

func (m *Matcher) matchTV(ctx context.Context, t *store.Torrent, parsed *classify.ParsedName) store.MatchResult {
	if m.tmdb == nil {
		return m.failResult(t)
	}

	title := parsed.Title
	year := parsed.Year

	tmdbID, firstAirYear, err := m.cache.SearchTV(ctx, m.tmdb, title, year)
	if err != nil {
		m.logger.Warn("tmdb search tv failed", "title", title, "error", err)
		return m.failResult(t)
	}

	// Retry without year if no results
	if tmdbID == 0 && year > 0 {
		tmdbID, firstAirYear, err = m.cache.SearchTV(ctx, m.tmdb, title, 0)
		if err != nil {
			m.logger.Warn("tmdb search tv retry failed", "title", title, "error", err)
			return m.failResult(t)
		}
	}

	if tmdbID == 0 {
		// TMDB found nothing — try TVDB as fallback
		if m.tvdb != nil {
			tvdbID, tvdbYear, tvdbErr := m.cache.SearchSeries(ctx, m.tvdb, title, year)
			if tvdbErr == nil && tvdbID == 0 && year > 0 {
				tvdbID, tvdbYear, tvdbErr = m.cache.SearchSeries(ctx, m.tvdb, title, 0)
			}
			if tvdbErr == nil && tvdbID > 0 {
				result := store.MatchResult{
					Status: store.MatchMatched,
					TVDBID: tvdbID,
					Year:   tvdbYear,
				}
				m.logger.Info("matched tv via tvdb fallback",
					"info_hash", t.InfoHashHex(),
					"title", title,
					"tvdb_id", tvdbID,
				)
				return result
			}
		}
		return m.failResult(t)
	}

	result := store.MatchResult{
		Status: store.MatchMatched,
		TMDBID: tmdbID,
		Year:   firstAirYear,
	}

	// Get external IDs (IMDB + TVDB)
	if imdbID, tvdbID, err := m.cache.GetTVExternalIDs(ctx, m.tmdb, tmdbID); err == nil {
		if imdbID != "" {
			result.IMDBID = imdbID
		}
		result.TVDBID = tvdbID
	}

	// If TVDB ID still missing, try TVDB directly
	if result.TVDBID == 0 && m.tvdb != nil {
		if tvdbID, _, err := m.cache.SearchSeries(ctx, m.tvdb, title, year); err == nil && tvdbID > 0 {
			result.TVDBID = tvdbID
		}
	}

	m.logger.Info("matched tv",
		"info_hash", t.InfoHashHex(),
		"title", title,
		"tmdb_id", tmdbID,
		"tvdb_id", result.TVDBID,
	)

	return result
}

func (m *Matcher) matchAnime(ctx context.Context, t *store.Torrent, parsed *classify.ParsedName) store.MatchResult {
	title := parsed.Title
	year := parsed.Year

	// Try offline DB first for instant matching
	if m.animedb != nil && m.animedb.IsLoaded() {
		if entry := m.animedb.Lookup(title); entry != nil {
			result := store.MatchResult{
				Status:    store.MatchMatched,
				AniListID: entry.AniListID,
				KitsuID:   entry.KitsuID,
				Year:      entry.Year,
			}

			m.logger.Info("matched anime via offline db",
				"info_hash", t.InfoHashHex(),
				"title", title,
				"anilist_id", result.AniListID,
				"kitsu_id", result.KitsuID,
			)

			return result
		}
	}

	result := store.MatchResult{
		Status: store.MatchFailed,
		Year:   year,
	}

	// AniList
	if anilistID, err := m.anilist.SearchAnime(ctx, title); err == nil && anilistID > 0 {
		result.AniListID = anilistID
		result.Status = store.MatchMatched
	}

	// Kitsu
	if kitsuID, err := m.kitsu.SearchAnime(ctx, title); err == nil && kitsuID > 0 {
		result.KitsuID = kitsuID
		result.Status = store.MatchMatched
	}

	// TMDB cross-reference
	if m.tmdb != nil {
		if tmdbID, _, err := m.cache.SearchTV(ctx, m.tmdb, title, year); err == nil && tmdbID > 0 {
			result.TMDBID = tmdbID
			result.Status = store.MatchMatched

			if imdbID, tvdbID, err := m.cache.GetTVExternalIDs(ctx, m.tmdb, tmdbID); err == nil {
				if imdbID != "" {
					result.IMDBID = imdbID
				}
				result.TVDBID = tvdbID
			}
		}
	}

	if result.Status == store.MatchMatched {
		m.logger.Info("matched anime",
			"info_hash", t.InfoHashHex(),
			"title", title,
			"anilist_id", result.AniListID,
			"kitsu_id", result.KitsuID,
		)
		return result
	}

	return m.failResult(t)
}

func (m *Matcher) failResult(t *store.Torrent) store.MatchResult {
	now := time.Now()
	backoff := backoffDuration(t.MatchAttempts)

	m.logger.Debug("match failed, scheduling retry",
		"info_hash", t.InfoHashHex(),
		"name", truncate(t.Name, 60),
		"attempts", t.MatchAttempts+1,
		"retry_after", backoff,
	)

	return store.MatchResult{
		Status:     store.MatchFailed,
		MatchAfter: now.Add(backoff).Unix(),
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return strings.TrimSpace(s[:maxLen]) + "..."
}

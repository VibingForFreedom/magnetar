package matcher

import (
	"context"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

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

type Matcher struct {
	store   store.Store
	metrics *metrics.Metrics
	tmdb    *TMDBClient
	tvdb    *TVDBClient
	anilist *AniListClient
	kitsu   *KitsuClient
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

func New(cfg Config, st store.Store, met *metrics.Metrics, logger *slog.Logger) *Matcher {
	m := &Matcher{
		store:   st,
		metrics: met,
		anilist: NewAniListClient(),
		kitsu:   NewKitsuClient(),
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

	return m
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

func (m *Matcher) processBatch(ctx context.Context) {
	if m.paused.Load() {
		return
	}
	torrents, err := m.store.FetchUnmatched(ctx, m.cfg.BatchSize)
	if err != nil {
		m.logger.Error("fetching unmatched torrents", "error", err)
		return
	}

	if len(torrents) == 0 {
		return
	}

	m.logger.Debug("processing batch", "count", len(torrents))

	for _, t := range torrents {
		select {
		case <-ctx.Done():
			return
		case <-m.stopped:
			return
		default:
		}

		m.metrics.MatchAttempts.Add(1)
		result := m.matchOne(ctx, t)
		if result.Status == store.MatchMatched {
			m.metrics.MatchSuccesses.Add(1)
			m.metrics.RecordMatch(1)
		} else {
			m.metrics.MatchFailures.Add(1)
		}
		if err := m.store.UpdateMatchResult(ctx, t.InfoHash, result); err != nil {
			m.logger.Error("updating match result",
				"info_hash", t.InfoHashHex(),
				"error", err,
			)
		}
	}
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

	// Path 2: Dispatch by category
	switch t.Category {
	case store.CategoryMovie:
		return m.matchMovie(ctx, t, parsed)
	case store.CategoryTV:
		return m.matchTV(ctx, t, parsed)
	case store.CategoryAnime:
		return m.matchAnime(ctx, t, parsed)
	default:
		return m.matchMovie(ctx, t, parsed)
	}
}

func (m *Matcher) matchByIMDB(ctx context.Context, t *store.Torrent, imdbID string) (store.MatchResult, bool) {
	findResult, err := m.tmdb.FindByIMDB(ctx, imdbID)
	if err != nil {
		m.logger.Warn("tmdb find by imdb failed", "imdb_id", imdbID, "error", err)
		return m.failResult(t), false
	}

	if findResult == nil {
		return store.MatchResult{}, false
	}

	result := store.MatchResult{
		Status: store.MatchMatched,
		IMDBID: imdbID,
		TMDBID: findResult.TMDBID,
		Year:   findResult.Year,
	}

	// Enrich with external IDs
	switch findResult.MediaType {
	case "movie":
		if extIMDB, err := m.tmdb.GetMovieExternalIDs(ctx, findResult.TMDBID); err == nil && extIMDB != "" {
			result.IMDBID = extIMDB
		}
	case "tv":
		if extIMDB, tvdbID, err := m.tmdb.GetTVExternalIDs(ctx, findResult.TMDBID); err == nil {
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

	tmdbID, releaseYear, err := m.tmdb.SearchMovie(ctx, title, year)
	if err != nil {
		m.logger.Warn("tmdb search movie failed", "title", title, "error", err)
		return m.failResult(t)
	}

	// Retry without year if no results
	if tmdbID == 0 && year > 0 {
		tmdbID, releaseYear, err = m.tmdb.SearchMovie(ctx, title, 0)
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
	if imdbID, err := m.tmdb.GetMovieExternalIDs(ctx, tmdbID); err == nil && imdbID != "" {
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

	tmdbID, firstAirYear, err := m.tmdb.SearchTV(ctx, title, year)
	if err != nil {
		m.logger.Warn("tmdb search tv failed", "title", title, "error", err)
		return m.failResult(t)
	}

	// Retry without year if no results
	if tmdbID == 0 && year > 0 {
		tmdbID, firstAirYear, err = m.tmdb.SearchTV(ctx, title, 0)
		if err != nil {
			m.logger.Warn("tmdb search tv retry failed", "title", title, "error", err)
			return m.failResult(t)
		}
	}

	if tmdbID == 0 {
		return m.failResult(t)
	}

	result := store.MatchResult{
		Status: store.MatchMatched,
		TMDBID: tmdbID,
		Year:   firstAirYear,
	}

	// Get external IDs (IMDB + TVDB)
	if imdbID, tvdbID, err := m.tmdb.GetTVExternalIDs(ctx, tmdbID); err == nil {
		if imdbID != "" {
			result.IMDBID = imdbID
		}
		result.TVDBID = tvdbID
	}

	// If TVDB ID still missing, try TVDB directly
	if result.TVDBID == 0 && m.tvdb != nil {
		if tvdbID, err := m.tvdb.SearchSeries(ctx, title, year); err == nil && tvdbID > 0 {
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
		if tmdbID, _, err := m.tmdb.SearchTV(ctx, title, year); err == nil && tmdbID > 0 {
			result.TMDBID = tmdbID
			result.Status = store.MatchMatched

			if imdbID, tvdbID, err := m.tmdb.GetTVExternalIDs(ctx, tmdbID); err == nil {
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

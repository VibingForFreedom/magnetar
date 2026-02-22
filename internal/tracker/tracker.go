package tracker

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/magnetar/magnetar/internal/config"
)

// ScrapeResult holds the best S/L counts found across all trackers.
type ScrapeResult struct {
	Seeders  int
	Leechers int
}

// trackerState tracks the adaptive batch limit for a single tracker.
type trackerState struct {
	batchLimit    atomic.Int32 // current max batch size
	successCount  atomic.Int64 // consecutive successful batches at current limit
	initialLimit  int32        // initial (max) limit for this protocol
}

// Scraper performs concurrent tracker scrapes for info hashes.
type Scraper struct {
	cfg    *config.Config
	logger *slog.Logger
	client *http.Client

	mu       sync.RWMutex
	trackers []trackerURL

	// Per-tracker adaptive batch state, keyed by host string.
	trackerStates sync.Map // map[string]*trackerState
}

// New creates a new tracker Scraper.
func New(cfg *config.Config, logger *slog.Logger) *Scraper {
	s := &Scraper{
		cfg:    cfg,
		logger: logger.With("component", "tracker_scraper"),
		client: &http.Client{Timeout: cfg.TrackerTimeout},
	}
	s.parseTrackers()
	return s
}

// Reconfigure re-parses the tracker list from config. Safe for concurrent use.
func (s *Scraper) Reconfigure() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.client.Timeout = s.cfg.TrackerTimeout
	s.parseTrackers()
	s.logger.Info("tracker scraper reconfigured",
		"enabled", s.cfg.TrackerEnabled,
		"trackers", len(s.trackers),
	)
}

func (s *Scraper) parseTrackers() {
	trackers := make([]trackerURL, 0, len(s.cfg.TrackerList))
	for _, raw := range s.cfg.TrackerList {
		t, err := parseTrackerURL(raw)
		if err != nil {
			s.logger.Warn("skipping invalid tracker URL", "url", raw, "error", err)
			continue
		}
		trackers = append(trackers, t)
	}
	s.trackers = trackers
}

// getTrackerState returns the adaptive state for a tracker, creating it if needed.
func (s *Scraper) getTrackerState(host string, proto trackerProto) *trackerState {
	if v, ok := s.trackerStates.Load(host); ok {
		return v.(*trackerState)
	}

	initial := int32(maxUDPBatchSize)
	if proto == protoHTTP {
		initial = int32(maxHTTPBatchSize)
	}

	st := &trackerState{initialLimit: initial}
	st.batchLimit.Store(initial)

	actual, _ := s.trackerStates.LoadOrStore(host, st)
	return actual.(*trackerState)
}

// halveBatchLimit halves the tracker's batch limit (min 1) and resets success count.
func (st *trackerState) halveBatchLimit() {
	current := st.batchLimit.Load()
	next := current / 2
	if next < 1 {
		next = 1
	}
	st.batchLimit.Store(next)
	st.successCount.Store(0)
}

// recordSuccess records a successful batch and possibly grows the limit.
func (st *trackerState) recordSuccess() {
	count := st.successCount.Add(1)
	// Every 100 successful batches, try growing by 25% (capped at initial max).
	if count%100 == 0 {
		current := st.batchLimit.Load()
		grown := current + current/4
		if grown > st.initialLimit {
			grown = st.initialLimit
		}
		if grown > current {
			st.batchLimit.Store(grown)
		}
	}
}

// ScrapeBatch performs concurrent batch scrapes across all configured trackers
// for multiple info hashes, returning the best S/L per hash.
func (s *Scraper) ScrapeBatch(ctx context.Context, hashes [][20]byte) map[[20]byte]ScrapeResult {
	if !s.cfg.TrackerEnabled || len(hashes) == 0 {
		return nil
	}

	s.mu.RLock()
	trackers := s.trackers
	s.mu.RUnlock()

	if len(trackers) == 0 {
		return nil
	}

	type trackerResult struct {
		entries map[[20]byte]ScrapeEntry
	}

	ch := make(chan trackerResult, len(trackers))

	for _, t := range trackers {
		go func(t trackerURL) {
			scrapeCtx, cancel := context.WithTimeout(ctx, s.cfg.TrackerTimeout)
			defer cancel()

			entries := s.scrapeTrackerBatch(scrapeCtx, t, hashes)
			ch <- trackerResult{entries: entries}
		}(t)
	}

	// Merge results: take max(seeders), max(leechers) per hash
	best := make(map[[20]byte]ScrapeResult, len(hashes))
	for range trackers {
		r := <-ch
		for hash, entry := range r.entries {
			existing := best[hash]
			if entry.Complete > existing.Seeders {
				existing.Seeders = entry.Complete
			}
			if entry.Incomplete > existing.Leechers {
				existing.Leechers = entry.Incomplete
			}
			best[hash] = existing
		}
	}

	return best
}

// scrapeTrackerBatch scrapes a single tracker for all hashes, using adaptive chunking.
func (s *Scraper) scrapeTrackerBatch(ctx context.Context, t trackerURL, hashes [][20]byte) map[[20]byte]ScrapeEntry {
	st := s.getTrackerState(t.host, t.proto)
	result := make(map[[20]byte]ScrapeEntry, len(hashes))

	remaining := hashes
	for len(remaining) > 0 {
		limit := int(st.batchLimit.Load())
		chunkSize := limit
		if chunkSize > len(remaining) {
			chunkSize = len(remaining)
		}
		chunk := remaining[:chunkSize]
		remaining = remaining[chunkSize:]

		var entries map[[20]byte]ScrapeEntry
		var err error

		switch t.proto {
		case protoUDP:
			entries, err = scrapeUDPBatch(ctx, t.host, chunk)
		case protoHTTP:
			entries, err = scrapeHTTPBatch(ctx, s.client, t.scrapeURL, chunk)
		}

		if err != nil {
			s.logger.Debug("batch scrape failed",
				"tracker", t.host,
				"batch_size", chunkSize,
				"error", err,
			)
			// Halve limit and retry remaining with smaller batches.
			// Don't retry this failed chunk — push it back to remaining.
			st.halveBatchLimit()
			remaining = append(chunk, remaining...)

			// If limit is already 1 and we still fail, skip this tracker.
			if limit <= 1 {
				s.logger.Debug("tracker batch limit exhausted, skipping",
					"tracker", t.host,
				)
				break
			}
			continue
		}

		st.recordSuccess()
		for h, e := range entries {
			result[h] = e
		}
	}

	return result
}

// Scrape performs concurrent scrapes across all configured trackers for a
// single info hash, returning the maximum S/L counts found.
func (s *Scraper) Scrape(ctx context.Context, infoHash [20]byte) ScrapeResult {
	results := s.ScrapeBatch(ctx, [][20]byte{infoHash})
	if r, ok := results[infoHash]; ok {
		return r
	}
	return ScrapeResult{}
}

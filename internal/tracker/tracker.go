package tracker

import (
	"context"
	"log/slog"
	"net/http"
	"sync"

	"github.com/magnetar/magnetar/internal/config"
)

// ScrapeResult holds the best S/L counts found across all trackers.
type ScrapeResult struct {
	Seeders  int
	Leechers int
}

// Scraper performs concurrent tracker scrapes for info hashes.
type Scraper struct {
	cfg    *config.Config
	logger *slog.Logger
	client *http.Client

	mu       sync.RWMutex
	trackers []trackerURL
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

// Scrape performs concurrent scrapes across all configured trackers,
// returning the maximum S/L counts found.
func (s *Scraper) Scrape(ctx context.Context, infoHash [20]byte) ScrapeResult {
	if !s.cfg.TrackerEnabled {
		return ScrapeResult{}
	}

	s.mu.RLock()
	trackers := s.trackers
	s.mu.RUnlock()

	if len(trackers) == 0 {
		return ScrapeResult{}
	}

	type result struct {
		entry ScrapeEntry
		err   error
	}

	ch := make(chan result, len(trackers))

	for _, t := range trackers {
		go func(t trackerURL) {
			scrapeCtx, cancel := context.WithTimeout(ctx, s.cfg.TrackerTimeout)
			defer cancel()

			var entry ScrapeEntry
			var err error

			switch t.proto {
			case protoUDP:
				entry, err = scrapeUDP(scrapeCtx, t.host, infoHash)
			case protoHTTP:
				entry, err = scrapeHTTP(scrapeCtx, s.client, t.scrapeURL, infoHash)
			}

			ch <- result{entry: entry, err: err}
		}(t)
	}

	best := ScrapeResult{}
	for range trackers {
		r := <-ch
		if r.err != nil {
			s.logger.Debug("tracker scrape failed", "error", r.err)
			continue
		}
		if r.entry.Complete > best.Seeders {
			best.Seeders = r.entry.Complete
		}
		if r.entry.Incomplete > best.Leechers {
			best.Leechers = r.entry.Incomplete
		}
	}

	return best
}

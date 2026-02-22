package api

import (
	"context"
	"net/http"
	"strconv"
	"sync/atomic"

	"github.com/magnetar/magnetar/internal/store"
	"github.com/magnetar/magnetar/internal/tracker"
)

type trackerStatsResponse struct {
	Trackers        []tracker.TrackerInfo `json:"trackers"`
	RecentlyUpdated []*store.Torrent     `json:"recently_updated"`
}

func (s *Server) handleTrackerStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}

	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > 200 {
		limit = 200
	}

	var trackers []tracker.TrackerInfo
	if s.trackerScraper != nil {
		trackers = s.trackerScraper.TrackerStats()
	}
	if trackers == nil {
		trackers = []tracker.TrackerInfo{}
	}

	updated, err := s.store.ListRecentlyUpdated(r.Context(), limit)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "failed to list recently updated")
		return
	}
	if updated == nil {
		updated = []*store.Torrent{}
	}

	s.writeJSON(w, http.StatusOK, trackerStatsResponse{
		Trackers:        trackers,
		RecentlyUpdated: updated,
	})
}

type scrapeResponse struct {
	Scraped int    `json:"scraped"`
	Updated int    `json:"updated"`
	Failed  int    `json:"failed"`
	Elapsed string `json:"elapsed"`
	Total   int    `json:"total"`
}

// scrapeRunning guards against concurrent full-DB scrapes.
var scrapeRunning atomic.Bool

const scrapeBatchSize = 500

func (s *Server) handleTrackerScrape(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}

	if s.trackerScraper == nil {
		s.writeError(w, http.StatusServiceUnavailable, "tracker scraper not configured")
		return
	}

	if !scrapeRunning.CompareAndSwap(false, true) {
		s.writeError(w, http.StatusConflict, "scrape already in progress")
		return
	}

	torrents, err := s.store.ListAllMatched(r.Context())
	if err != nil {
		scrapeRunning.Store(false)
		s.writeError(w, http.StatusInternalServerError, "failed to list matched torrents")
		return
	}

	total := len(torrents)
	if total == 0 {
		scrapeRunning.Store(false)
		s.writeJSON(w, http.StatusOK, scrapeResponse{})
		return
	}

	// Return immediately — scrape runs in background
	s.writeJSON(w, http.StatusAccepted, scrapeResponse{Total: total})

	go func() {
		defer scrapeRunning.Store(false)

		ctx := context.Background()
		for i := 0; i < len(torrents); i += scrapeBatchSize {
			end := i + scrapeBatchSize
			if end > len(torrents) {
				end = len(torrents)
			}
			s.scrapeAll(ctx, torrents[i:end])
		}
		s.logger.Printf("full tracker scrape complete: %d matched torrents", total)
	}()
}

func (s *Server) scrapeAll(ctx context.Context, torrents []*store.Torrent) scrapeResponse {
	// Collect all hashes
	hashes := make([][20]byte, len(torrents))
	hashToTorrent := make(map[[20]byte]*store.Torrent, len(torrents))
	for i, t := range torrents {
		var h [20]byte
		copy(h[:], t.InfoHash)
		hashes[i] = h
		hashToTorrent[h] = t
	}

	s.metrics.TrackerScrapeAttempts.Add(int64(len(hashes)))
	s.metrics.RecordTrackerScrape(int64(len(hashes)))

	// Single batch scrape call
	results := s.trackerScraper.ScrapeBatch(ctx, hashes)

	var resp scrapeResponse
	resp.Scraped = len(hashes)

	var updates []store.SeedersLeechersUpdate
	for _, h := range hashes {
		r, ok := results[h]
		if !ok || (r.Seeders == 0 && r.Leechers == 0) {
			resp.Failed++
			s.metrics.TrackerScrapeFailures.Add(1)
			continue
		}

		s.metrics.TrackerScrapeSuccesses.Add(1)

		t := hashToTorrent[h]
		updated := false
		seeders := t.Seeders
		if r.Seeders > seeders {
			seeders = r.Seeders
			updated = true
		}
		leechers := t.Leechers
		if r.Leechers > leechers {
			leechers = r.Leechers
			updated = true
		}

		if updated {
			updates = append(updates, store.SeedersLeechersUpdate{
				InfoHash: t.InfoHash,
				Seeders:  seeders,
				Leechers: leechers,
			})
		}
	}

	if len(updates) > 0 {
		if err := s.store.BulkUpdateSeedersLeechers(ctx, updates); err != nil {
			s.logger.Printf("tracker scrape bulk update failed: %v", err)
		} else {
			s.metrics.TrackerScrapeUpdated.Add(int64(len(updates)))
			resp.Updated = len(updates)
		}
	}

	return resp
}

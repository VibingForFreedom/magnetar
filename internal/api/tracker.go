package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/magnetar/magnetar/internal/store"
)

type scrapeResponse struct {
	Scraped int    `json:"scraped"`
	Updated int    `json:"updated"`
	Failed  int    `json:"failed"`
	Elapsed string `json:"elapsed"`
}

func (s *Server) handleTrackerScrape(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}

	if s.trackerScraper == nil {
		s.writeError(w, http.StatusServiceUnavailable, "tracker scraper not configured")
		return
	}

	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > 500 {
		limit = 500
	}

	result, err := s.store.ListRecent(r.Context(), store.SearchOpts{Limit: limit})
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "failed to list torrents")
		return
	}

	if len(result.Torrents) == 0 {
		s.writeJSON(w, http.StatusOK, scrapeResponse{})
		return
	}

	start := time.Now()
	resp := s.scrapeAll(r.Context(), result.Torrents)
	resp.Elapsed = time.Since(start).Truncate(time.Millisecond).String()

	s.writeJSON(w, http.StatusOK, resp)
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

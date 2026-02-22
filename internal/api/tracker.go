package api

import (
	"context"
	"encoding/hex"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/magnetar/magnetar/internal/store"
	"github.com/magnetar/magnetar/internal/tracker"
)

type scrapeResponse struct {
	Scraped  int `json:"scraped"`
	Updated  int `json:"updated"`
	Failed   int `json:"failed"`
	Elapsed  string `json:"elapsed"`
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
	const maxWorkers = 20

	type scrapeJob struct {
		torrent *store.Torrent
		result  tracker.ScrapeResult
		ok      bool
	}

	jobs := make(chan *store.Torrent, len(torrents))
	results := make(chan scrapeJob, len(torrents))

	workerCount := maxWorkers
	if len(torrents) < workerCount {
		workerCount = len(torrents)
	}

	var wg sync.WaitGroup
	for range workerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range jobs {
				s.metrics.TrackerScrapeAttempts.Add(1)
				s.metrics.RecordTrackerScrape(1)

				var hash [20]byte
				copy(hash[:], t.InfoHash)

				sr := s.trackerScraper.Scrape(ctx, hash)
				ok := sr.Seeders > 0 || sr.Leechers > 0

				if ok {
					s.metrics.TrackerScrapeSuccesses.Add(1)
				} else {
					s.metrics.TrackerScrapeFailures.Add(1)
				}

				results <- scrapeJob{torrent: t, result: sr, ok: ok}
			}
		}()
	}

	for _, t := range torrents {
		jobs <- t
	}
	close(jobs)

	go func() {
		wg.Wait()
		close(results)
	}()

	var resp scrapeResponse
	for j := range results {
		resp.Scraped++
		if !j.ok {
			resp.Failed++
			continue
		}

		updated := false
		if j.result.Seeders > j.torrent.Seeders {
			j.torrent.Seeders = j.result.Seeders
			updated = true
		}
		if j.result.Leechers > j.torrent.Leechers {
			j.torrent.Leechers = j.result.Leechers
			updated = true
		}

		if updated {
			j.torrent.UpdatedAt = time.Now().Unix()
			if err := s.store.UpsertTorrent(ctx, j.torrent); err != nil {
				s.logger.Printf("tracker scrape update failed for %s: %v",
					hex.EncodeToString(j.torrent.InfoHash), err)
				continue
			}
			s.metrics.TrackerScrapeUpdated.Add(1)
			resp.Updated++
		}
	}

	return resp
}

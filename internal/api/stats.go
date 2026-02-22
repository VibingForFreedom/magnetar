package api

import (
	"net/http"
	"time"

	"github.com/magnetar/magnetar/internal/metrics"
)

type dbStatsSummary struct {
	TotalTorrents int64 `json:"total_torrents"`
	Unmatched     int64 `json:"unmatched"`
	Matched       int64 `json:"matched"`
	Failed        int64 `json:"failed"`
	DBSize        int64 `json:"db_size"`
	LastCrawl     int64 `json:"last_crawl"`
}

type healthResponse struct {
	Status string          `json:"status"`
	DB     *dbStatsSummary `json:"db,omitempty"`
}

type statsResponse struct {
	dbStatsSummary
	Uptime        int64             `json:"uptime"`
	StartTime     string            `json:"start_time"`
	MatcherPaused bool              `json:"matcher_paused"`
	Metrics       *metrics.Snapshot `json:"metrics,omitempty"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := healthResponse{Status: "ok"}

	stats, err := s.store.Stats(r.Context())
	if err != nil {
		s.logger.Printf("health stats error: %v", err)
		s.writeJSON(w, http.StatusOK, resp)
		return
	}

	resp.DB = &dbStatsSummary{
		TotalTorrents: stats.TotalTorrents,
		Unmatched:     stats.Unmatched,
		Matched:       stats.Matched,
		Failed:        stats.Failed,
		DBSize:        stats.DBSize,
		LastCrawl:     stats.LastCrawl,
	}

	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.Stats(r.Context())
	if err != nil {
		s.logger.Printf("stats error: %v", err)
		s.writeError(w, http.StatusInternalServerError, "failed to fetch stats")
		return
	}

	uptime := int64(time.Since(s.start).Seconds())
	resp := statsResponse{
		dbStatsSummary: dbStatsSummary{
			TotalTorrents: stats.TotalTorrents,
			Unmatched:     stats.Unmatched,
			Matched:       stats.Matched,
			Failed:        stats.Failed,
			DBSize:        stats.DBSize,
			LastCrawl:     stats.LastCrawl,
		},
		Uptime:        uptime,
		StartTime:     s.start.UTC().Format(time.RFC3339),
		MatcherPaused: s.matcher != nil && s.matcher.IsPaused(),
	}

	if s.metrics != nil {
		snap := s.metrics.Snapshot()
		resp.Metrics = &snap
	}

	s.writeJSON(w, http.StatusOK, resp)
}

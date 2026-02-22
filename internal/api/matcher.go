package api

import (
	"net/http"
	"strconv"

	"github.com/magnetar/magnetar/internal/store"
)

type matcherListResponse struct {
	Results []*store.Torrent `json:"results"`
	Total   int              `json:"total"`
	Page    int              `json:"page"`
	Limit   int              `json:"limit"`
}

func (s *Server) handleMatcherRecent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	page, limit := s.parsePageLimit(r)
	offset := (page - 1) * limit

	result, err := s.store.ListByMatchStatus(r.Context(), store.MatchMatched, limit, offset)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "failed to list matched torrents")
		return
	}

	s.writeJSON(w, http.StatusOK, matcherListResponse{
		Results: result.Torrents,
		Total:   result.Total,
		Page:    page,
		Limit:   limit,
	})
}

func (s *Server) handleMatcherFailures(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	page, limit := s.parsePageLimit(r)
	offset := (page - 1) * limit

	result, err := s.store.ListByMatchStatus(r.Context(), store.MatchFailed, limit, offset)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "failed to list failed torrents")
		return
	}

	s.writeJSON(w, http.StatusOK, matcherListResponse{
		Results: result.Torrents,
		Total:   result.Total,
		Page:    page,
		Limit:   limit,
	})
}

func (s *Server) handleMatcherTrigger(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if s.matcherTrigger == nil {
		s.writeError(w, http.StatusServiceUnavailable, "matcher not available")
		return
	}

	count, err := s.matcherTrigger.RunBatch(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "failed to trigger matcher batch")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"triggered":  true,
		"batch_size": count,
	})
}

func (s *Server) parsePageLimit(r *http.Request) (page, limit int) {
	page = 1
	limit = 20

	if raw := r.URL.Query().Get("page"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			page = parsed
		}
	}

	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	if limit > 100 {
		limit = 100
	}

	return page, limit
}

package api

import (
	"net/http"
	"strconv"

	"github.com/magnetar/magnetar/internal/store"
)

func (s *Server) handleLatest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	q := r.URL.Query()
	limit, _ := s.parseLimitOffset(q, 50)
	page := 1
	if rawPage := q.Get("page"); rawPage != "" {
		if p, err := strconv.Atoi(rawPage); err == nil && p > 0 {
			page = p
		}
	}
	offset := (page - 1) * limit

	opts := store.SearchOpts{
		Limit:  limit,
		Offset: offset,
	}

	if rawCat := q.Get("category"); rawCat != "" {
		if catInt, err := strconv.Atoi(rawCat); err == nil {
			opts.Categories = []store.Category{store.Category(catInt)}
		}
	}

	if rawQuality := q.Get("quality"); rawQuality != "" {
		if qInt, err := strconv.Atoi(rawQuality); err == nil {
			opts.Quality = []store.Quality{store.Quality(qInt)}
		}
	}

	result, err := s.store.ListRecent(r.Context(), opts)
	if err != nil {
		s.logger.Printf("list recent error: %v", err)
		s.writeError(w, http.StatusInternalServerError, "failed to list torrents")
		return
	}

	results := make([]torrentResponse, 0, len(result.Torrents))
	for _, t := range result.Torrents {
		results = append(results, torrentResponseFromStore(t))
	}

	s.writeJSON(w, http.StatusOK, searchResponse{
		Results: results,
		Total:   result.Total,
		Page:    page,
		Limit:   limit,
	})
}

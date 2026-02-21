package api

import (
	"net/http"
	"strconv"

	"github.com/magnetar/magnetar/internal/store"
)

type searchResponse struct {
	Results []torrentResponse `json:"results"`
	Total   int               `json:"total"`
	Page    int               `json:"page"`
	Limit   int               `json:"limit"`
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	q := r.URL.Query()
	query := q.Get("q")
	if query == "" {
		s.writeError(w, http.StatusBadRequest, "query parameter 'q' is required")
		return
	}

	limit, _ := s.parseLimitOffset(q, 25)
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

	result, err := s.store.SearchByName(r.Context(), query, opts)
	if err != nil {
		s.logger.Printf("search error: %v", err)
		s.writeError(w, http.StatusInternalServerError, "search failed")
		return
	}

	results := make([]torrentResponse, 0, len(result.Torrents))
	for _, t := range result.Torrents {
		results = append(results, torrentResponseFromStore(t))
	}

	resp := searchResponse{
		Results: results,
		Total:   result.Total,
		Page:    page,
		Limit:   limit,
	}

	s.writeJSON(w, http.StatusOK, resp)
}

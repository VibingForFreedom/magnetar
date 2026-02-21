package api

import (
	"encoding/json"
	"net/http"
)

type toggleRequest struct {
	Paused bool `json:"paused"`
}

type toggleResponse struct {
	Component string `json:"component"`
	Paused    bool   `json:"paused"`
}

func (s *Server) handleCrawlerToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if s.crawler == nil {
		s.writeError(w, http.StatusServiceUnavailable, "crawler not available")
		return
	}

	var req toggleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Paused {
		s.crawler.Pause()
	} else {
		s.crawler.Resume()
	}

	s.writeJSON(w, http.StatusOK, toggleResponse{
		Component: "crawler",
		Paused:    s.crawler.IsPaused(),
	})
}

func (s *Server) handleMatcherToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if s.matcher == nil {
		s.writeError(w, http.StatusServiceUnavailable, "matcher not available")
		return
	}

	var req toggleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Paused {
		s.matcher.Pause()
	} else {
		s.matcher.Resume()
	}

	s.writeJSON(w, http.StatusOK, toggleResponse{
		Component: "matcher",
		Paused:    s.matcher.IsPaused(),
	})
}

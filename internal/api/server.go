package api

import (
	"encoding/json"
	"encoding/xml"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/magnetar/magnetar/internal/config"
	"github.com/magnetar/magnetar/internal/metrics"
	"github.com/magnetar/magnetar/internal/store"
	"github.com/magnetar/magnetar/internal/web"
)

type Server struct {
	store   store.Store
	cfg     *config.Config
	metrics *metrics.Metrics
	start   time.Time
	logger  *log.Logger
}

func NewServer(st store.Store, cfg *config.Config, m *metrics.Metrics) *Server {
	return &Server{
		store:   st,
		cfg:     cfg,
		metrics: m,
		start:   time.Now(),
		logger:  log.Default(),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/events", s.handleSSE)
	mux.HandleFunc("/api/search", s.handleSearch)
	mux.HandleFunc("/api/settings", s.handleSettings)
	mux.HandleFunc("/api/torznab", s.handleTorznab)
	mux.HandleFunc("/api/torrents/lookup", s.handleHashBulk)
	mux.HandleFunc("/api/torrents/", s.handleHashGet)
	mux.Handle("/", web.Handler())

	return s.withLogging(s.withAuth(mux))
}

func (s *Server) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		s.logger.Printf("%s %s %d %s", r.Method, r.URL.Path, sw.status, time.Since(start))
	})
}

func (s *Server) withAuth(next http.Handler) http.Handler {
	if s.cfg == nil || s.cfg.APIKey == "" {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		if r.URL.Path == "/api/torznab" && r.URL.Query().Get("t") == "caps" {
			next.ServeHTTP(w, r)
			return
		}

		apiKey := r.URL.Query().Get("apikey")
		if apiKey == "" {
			apiKey = r.Header.Get("X-Api-Key")
		}
		if apiKey != s.cfg.APIKey {
			s.writeError(w, http.StatusUnauthorized, "invalid api key")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func (s *Server) writeXML(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		_ = xml.NewEncoder(w).Encode(v)
	}
}

func (s *Server) writeError(w http.ResponseWriter, status int, message string) {
	s.writeJSON(w, status, map[string]string{"error": message})
}

func (s *Server) parseLimitOffset(values url.Values, defaultLimit int) (limit, offset int) {
	limit = defaultLimit
	if limit < 1 {
		limit = 1
	}
	if limit > 1000 {
		limit = 1000
	}

	if rawLimit := values.Get("limit"); rawLimit != "" {
		if parsed, err := strconv.Atoi(rawLimit); err == nil {
			limit = parsed
		}
	}

	if limit < 1 {
		limit = 1
	}
	if limit > 1000 {
		limit = 1000
	}

	if rawOffset := values.Get("offset"); rawOffset != "" {
		if parsed, err := strconv.Atoi(rawOffset); err == nil {
			offset = parsed
		}
	}

	if offset < 0 {
		offset = 0
	}

	return limit, offset
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

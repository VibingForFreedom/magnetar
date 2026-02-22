package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/magnetar/magnetar/internal/config"
)

type settingsResponse struct {
	Database settingsDB      `json:"database"`
	Crawler  settingsCrawler `json:"crawler"`
	Matcher  settingsMatcher `json:"matcher"`
	Tracker  settingsTracker `json:"tracker"`
	General  settingsGeneral `json:"general"`
	API      settingsAPI     `json:"api"`
}

type settingsDB struct {
	Backend string `json:"backend"`
	Path    string `json:"path,omitempty"`
}

type settingsCrawler struct {
	Enabled bool `json:"enabled"`
	Paused  bool `json:"paused"`
	Workers int  `json:"workers"`
	Port    int  `json:"port"`
	Rate    int  `json:"rate"`
}

type settingsMatcher struct {
	Enabled     bool   `json:"enabled"`
	Paused      bool   `json:"paused"`
	BatchSize   int    `json:"batch_size"`
	Interval    string `json:"interval"`
	MaxAttempts int    `json:"max_attempts"`
	TMDBEnabled bool   `json:"tmdb_enabled"`
	TVDBEnabled bool   `json:"tvdb_enabled"`
}

type settingsTracker struct {
	Enabled  bool     `json:"enabled"`
	Trackers []string `json:"trackers"`
	Timeout  string   `json:"timeout"`
}

type settingsGeneral struct {
	LogLevel       string `json:"log_level"`
	AnimeDBEnabled bool   `json:"animedb_enabled"`
}

type settingsAPI struct {
	Port        int  `json:"port"`
	AuthEnabled bool `json:"auth_enabled"`
}

type settingsPutRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleSettingsGet(w)
	case http.MethodPut:
		s.handleSettingsPut(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleSettingsGet(w http.ResponseWriter) {
	trackers := s.cfg.TrackerList
	if trackers == nil {
		trackers = []string{}
	}

	resp := settingsResponse{
		Database: settingsDB{
			Backend: s.cfg.DBBackend,
		},
		Crawler: settingsCrawler{
			Enabled: s.cfg.CrawlEnabled,
			Paused:  s.crawler != nil && s.crawler.IsPaused(),
			Workers: s.cfg.CrawlWorkers,
			Port:    s.cfg.CrawlPort,
			Rate:    s.cfg.CrawlRate,
		},
		Matcher: settingsMatcher{
			Enabled:     s.cfg.MatchEnabled,
			Paused:      s.matcher != nil && s.matcher.IsPaused(),
			BatchSize:   s.cfg.MatchBatchSize,
			Interval:    s.cfg.MatchInterval.String(),
			MaxAttempts: s.cfg.MatchMaxAttempts,
			TMDBEnabled: s.cfg.TMDBAPIKey != "",
			TVDBEnabled: s.cfg.TVDBAPIKey != "",
		},
		Tracker: settingsTracker{
			Enabled:  s.cfg.TrackerEnabled,
			Trackers: trackers,
			Timeout:  s.cfg.TrackerTimeout.String(),
		},
		General: settingsGeneral{
			LogLevel:       s.cfg.LogLevel,
			AnimeDBEnabled: s.cfg.AnimeDBEnabled,
		},
		API: settingsAPI{
			Port:        s.cfg.Port,
			AuthEnabled: s.cfg.APIKey != "",
		},
	}

	if s.cfg.IsSQLite() {
		resp.Database.Path = s.cfg.DBPath
	}

	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleSettingsPut(w http.ResponseWriter, r *http.Request) {
	var req settingsPutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	req.Key = strings.TrimSpace(req.Key)
	if req.Key == "" {
		s.writeError(w, http.StatusBadRequest, "key is required")
		return
	}

	if !config.IsEditableKey(req.Key) {
		s.writeError(w, http.StatusBadRequest, "key is not editable: "+req.Key)
		return
	}

	if err := s.store.SetSetting(r.Context(), req.Key, req.Value); err != nil {
		s.writeError(w, http.StatusInternalServerError, "failed to save setting")
		return
	}

	s.cfg.ApplySetting(req.Key, req.Value)

	// Notify tracker scraper to reconfigure if tracker key changed
	if strings.HasPrefix(req.Key, "tracker_") && s.trackerScraper != nil {
		s.trackerScraper.Reconfigure()
	}

	requiresRestart := req.Key == config.KeyCrawlWorkers
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":           "ok",
		"key":              req.Key,
		"requires_restart": requiresRestart,
	})
}

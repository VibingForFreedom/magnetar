package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/magnetar/magnetar/internal/classify"
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
	LogLevel            string `json:"log_level"`
	AnimeDBEnabled      bool   `json:"animedb_enabled"`
	FilterAdultPatterns bool   `json:"filter_adult_patterns"`
	FilterAdultNames    bool   `json:"filter_adult_names"`
	FilterJunkNames     bool   `json:"filter_junk_names"`
}

type settingsAPI struct {
	Port        int  `json:"port"`
	AuthEnabled bool `json:"auth_enabled"`
}

type settingsPutRequestRaw struct {
	Key   string          `json:"key"`
	Value json.RawMessage `json:"value"`
}

type settingsPutRequest struct {
	Key   string
	Value string
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
	s.cfg.RLock()
	trackers := make([]string, len(s.cfg.TrackerList))
	copy(trackers, s.cfg.TrackerList)
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
			LogLevel:            s.cfg.LogLevel,
			AnimeDBEnabled:      s.cfg.AnimeDBEnabled,
			FilterAdultPatterns: s.cfg.FilterAdultPatterns,
			FilterAdultNames:    s.cfg.FilterAdultNames,
			FilterJunkNames:     s.cfg.FilterJunkNames,
		},
		API: settingsAPI{
			Port:        s.cfg.Port,
			AuthEnabled: s.cfg.APIKey != "",
		},
	}
	isSQLite := s.cfg.IsSQLite()
	dbPath := s.cfg.DBPath
	s.cfg.RUnlock()

	if isSQLite {
		resp.Database.Path = dbPath
	}

	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleSettingsPut(w http.ResponseWriter, r *http.Request) {
	var raw settingsPutRequestRaw
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Coerce value to string — accept string, number, or boolean JSON values.
	var valueStr string
	if len(raw.Value) > 0 && raw.Value[0] == '"' {
		if err := json.Unmarshal(raw.Value, &valueStr); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid value")
			return
		}
	} else {
		valueStr = strings.Trim(string(raw.Value), " ")
	}

	req := settingsPutRequest{Key: raw.Key, Value: valueStr}
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

	// Sync classify filter config when filter keys change
	if strings.HasPrefix(req.Key, "filter_") {
		s.cfg.RLock()
		filterCfg := classify.FilterConfig{
			FilterAdultPatterns: s.cfg.FilterAdultPatterns,
			FilterAdultNames:    s.cfg.FilterAdultNames,
			FilterJunkNames:     s.cfg.FilterJunkNames,
		}
		s.cfg.RUnlock()
		classify.SetFilterConfig(filterCfg)
	}

	requiresRestart := req.Key == config.KeyCrawlWorkers
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":           "ok",
		"key":              req.Key,
		"requires_restart": requiresRestart,
	})
}

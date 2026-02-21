package api

import (
	"net/http"
)

type settingsResponse struct {
	Database settingsDB      `json:"database"`
	Crawler  settingsCrawler `json:"crawler"`
	Matcher  settingsMatcher `json:"matcher"`
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
}

type settingsMatcher struct {
	Enabled     bool   `json:"enabled"`
	Paused      bool   `json:"paused"`
	BatchSize   int    `json:"batch_size"`
	Interval    string `json:"interval"`
	TMDBEnabled bool   `json:"tmdb_enabled"`
	TVDBEnabled bool   `json:"tvdb_enabled"`
}

type settingsAPI struct {
	Port       int  `json:"port"`
	AuthEnabled bool `json:"auth_enabled"`
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
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
		},
		Matcher: settingsMatcher{
			Enabled:     s.cfg.MatchEnabled,
			Paused:      s.matcher != nil && s.matcher.IsPaused(),
			BatchSize:   s.cfg.MatchBatchSize,
			Interval:    s.cfg.MatchInterval.String(),
			TMDBEnabled: s.cfg.TMDBAPIKey != "",
			TVDBEnabled: s.cfg.TVDBAPIKey != "",
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

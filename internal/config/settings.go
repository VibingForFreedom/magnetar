package config

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	KeyTrackerEnabled   = "tracker_enabled"
	KeyTrackerList      = "tracker_list"
	KeyTrackerTimeout   = "tracker_timeout"
	KeyMatchBatchSize   = "match_batch_size"
	KeyMatchInterval    = "match_interval"
	KeyMatchMaxAttempts = "match_max_attempts"
	KeyCrawlRate        = "crawl_rate"
	KeyCrawlWorkers     = "crawl_workers"
	KeyLogLevel         = "log_level"
	KeyAnimeDBEnabled        = "animedb_enabled"
	KeyFilterAdultPatterns   = "filter_adult_patterns"
	KeyFilterAdultNames      = "filter_adult_names"
	KeyFilterJunkNames       = "filter_junk_names"
)

// EditableKeys lists all setting keys that can be modified at runtime via the API.
var EditableKeys = []string{
	KeyTrackerEnabled,
	KeyTrackerList,
	KeyTrackerTimeout,
	KeyMatchBatchSize,
	KeyMatchInterval,
	KeyMatchMaxAttempts,
	KeyCrawlRate,
	KeyCrawlWorkers,
	KeyLogLevel,
	KeyAnimeDBEnabled,
	KeyFilterAdultPatterns,
	KeyFilterAdultNames,
	KeyFilterJunkNames,
}

// IsEditableKey returns true if the key is in the editable keys list.
func IsEditableKey(key string) bool {
	for _, k := range EditableKeys {
		if k == key {
			return true
		}
	}
	return false
}

// SettingsStore is the subset of Store needed for settings operations.
type SettingsStore interface {
	GetAllSettings(ctx context.Context) (map[string]string, error)
}

// ApplyOverrides loads all persisted settings from the store and applies them to the config.
func (c *Config) ApplyOverrides(ctx context.Context, st SettingsStore) error {
	settings, err := st.GetAllSettings(ctx)
	if err != nil {
		return fmt.Errorf("loading settings: %w", err)
	}
	for k, v := range settings {
		c.ApplySetting(k, v)
	}
	return nil
}

// ApplySetting applies a single setting value to the live config fields.
// Safe for concurrent use.
func (c *Config) ApplySetting(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch key {
	case KeyTrackerEnabled:
		if b, err := strconv.ParseBool(value); err == nil {
			c.TrackerEnabled = b
		}
	case KeyTrackerList:
		parts := strings.Split(value, "\n")
		list := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				list = append(list, p)
			}
		}
		c.TrackerList = list
	case KeyTrackerTimeout:
		if d, err := time.ParseDuration(value); err == nil {
			c.TrackerTimeout = d
		}
	case KeyMatchBatchSize:
		if n, err := strconv.Atoi(value); err == nil && n >= 1 {
			c.MatchBatchSize = n
		}
	case KeyMatchInterval:
		if d, err := time.ParseDuration(value); err == nil {
			c.MatchInterval = d
		}
	case KeyMatchMaxAttempts:
		if n, err := strconv.Atoi(value); err == nil && n >= 1 && n <= 10 {
			c.MatchMaxAttempts = n
		}
	case KeyCrawlRate:
		if n, err := strconv.Atoi(value); err == nil && n >= 0 {
			c.CrawlRate = n
		}
	case KeyCrawlWorkers:
		if n, err := strconv.Atoi(value); err == nil && n >= 1 && n <= 10 {
			c.CrawlWorkers = n
		}
	case KeyLogLevel:
		valid := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
		if valid[strings.ToLower(value)] {
			c.LogLevel = strings.ToLower(value)
		}
	case KeyAnimeDBEnabled:
		if b, err := strconv.ParseBool(value); err == nil {
			c.AnimeDBEnabled = b
		}
	case KeyFilterAdultPatterns:
		if b, err := strconv.ParseBool(value); err == nil {
			c.FilterAdultPatterns = b
		}
	case KeyFilterAdultNames:
		if b, err := strconv.ParseBool(value); err == nil {
			c.FilterAdultNames = b
		}
	case KeyFilterJunkNames:
		if b, err := strconv.ParseBool(value); err == nil {
			c.FilterJunkNames = b
		}
	}
}

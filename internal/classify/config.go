package classify

import "sync/atomic"

// FilterConfig controls which content filters are active.
// All filters are enabled by default.
type FilterConfig struct {
	// FilterAdultPatterns enables JAV code, porn studio regex, and keyword matching.
	FilterAdultPatterns bool
	// FilterAdultNames enables StashDB performer/studio name lookup for bare titles.
	FilterAdultNames bool
	// FilterJunkNames enables software/game name pattern rejection.
	FilterJunkNames bool
}

// DefaultFilterConfig returns a FilterConfig with all filters enabled.
func DefaultFilterConfig() FilterConfig {
	return FilterConfig{
		FilterAdultPatterns: true,
		FilterAdultNames:    true,
		FilterJunkNames:     true,
	}
}

var filterCfgPtr atomic.Pointer[FilterConfig]

func init() {
	cfg := DefaultFilterConfig()
	filterCfgPtr.Store(&cfg)
}

// SetFilterConfig updates the package-level filter configuration.
// Safe for concurrent use.
func SetFilterConfig(cfg FilterConfig) {
	filterCfgPtr.Store(&cfg)
}

// GetFilterConfig returns the current filter configuration.
// Safe for concurrent use.
func GetFilterConfig() FilterConfig {
	return *filterCfgPtr.Load()
}

// loadFilterCfg returns the current config for internal use.
func loadFilterCfg() FilterConfig {
	return *filterCfgPtr.Load()
}

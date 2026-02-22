package classify

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

var filterCfg = DefaultFilterConfig()

// SetFilterConfig updates the package-level filter configuration.
// Call once at startup after loading settings, and again when settings change.
func SetFilterConfig(cfg FilterConfig) {
	filterCfg = cfg
}

// GetFilterConfig returns the current filter configuration.
func GetFilterConfig() FilterConfig {
	return filterCfg
}

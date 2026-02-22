package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Port     int
	APIKey   string
	LogLevel string

	DBBackend      string
	DBPath         string
	DBCacheSize    int
	DBMmapSize     int64
	DBSyncFull     bool
	DBDSN          string
	DBMaxOpenConns int
	DBMaxIdleConns int

	CrawlEnabled bool
	CrawlRate    int
	CrawlWorkers int
	CrawlPort    int

	MatchEnabled     bool
	MatchBatchSize   int
	MatchInterval    time.Duration
	MatchMaxAttempts int
	TMDBAPIKey       string
	TVDBAPIKey       string

	TrackerEnabled bool
	TrackerList    []string
	TrackerTimeout time.Duration

	ValkeyURL string

	AnimeDBEnabled bool

	FilterAdultPatterns bool
	FilterAdultNames    bool
	FilterJunkNames     bool

	BackupEnabled bool
	BackupPath     string
	BackupSchedule string
	BackupRetain   int

	AnalyzeInterval     int
	IntegrityCheckDaily bool
}

func Load() (*Config, error) {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("loading .env file: %w", err)
	}

	cfg := &Config{
		Port:     getEnvInt("MAGNETAR_PORT", 3333),
		APIKey:   getEnvString("MAGNETAR_API_KEY", ""),
		LogLevel: getEnvString("MAGNETAR_LOG_LEVEL", "info"),

		DBBackend:      getEnvString("MAGNETAR_DB_BACKEND", "sqlite"),
		DBPath:         getEnvString("MAGNETAR_DB_PATH", "/data/magnetar.db"),
		DBCacheSize:    getEnvInt("MAGNETAR_DB_CACHE_SIZE", 65536),
		DBMmapSize:     getEnvInt64("MAGNETAR_DB_MMAP_SIZE", 268435456),
		DBSyncFull:     getEnvBool("MAGNETAR_DB_SYNC_FULL", false),
		DBDSN:          getEnvString("MAGNETAR_DB_DSN", ""),
		DBMaxOpenConns: getEnvInt("MAGNETAR_DB_MAX_OPEN_CONNS", 25),
		DBMaxIdleConns: getEnvInt("MAGNETAR_DB_MAX_IDLE_CONNS", 10),

		CrawlEnabled: getEnvBool("MAGNETAR_CRAWL_ENABLED", true),
		CrawlRate:    getEnvInt("MAGNETAR_CRAWL_RATE", 1000),
		CrawlWorkers: getEnvInt("MAGNETAR_CRAWL_WORKERS", 8),
		CrawlPort:    getEnvInt("MAGNETAR_CRAWL_PORT", 6881),

		MatchEnabled:     getEnvBool("MAGNETAR_MATCH_ENABLED", true),
		MatchBatchSize:   getEnvInt("MAGNETAR_MATCH_BATCH_SIZE", 100),
		MatchInterval:    getEnvDuration("MAGNETAR_MATCH_INTERVAL", 10*time.Second),
		MatchMaxAttempts: getEnvInt("MAGNETAR_MATCH_MAX_ATTEMPTS", 4),
		TMDBAPIKey:       getEnvString("MAGNETAR_TMDB_API_KEY", ""),
		TVDBAPIKey:       getEnvString("MAGNETAR_TVDB_API_KEY", ""),

		TrackerEnabled: getEnvBool("MAGNETAR_TRACKER_ENABLED", false),
		TrackerList:    getEnvStringSlice("MAGNETAR_TRACKER_LIST", nil),
		TrackerTimeout: getEnvDuration("MAGNETAR_TRACKER_TIMEOUT", 5*time.Second),

		ValkeyURL: getEnvString("MAGNETAR_VALKEY_URL", ""),

		AnimeDBEnabled: getEnvBool("MAGNETAR_ANIMEDB_ENABLED", true),

		FilterAdultPatterns: getEnvBool("MAGNETAR_FILTER_ADULT_PATTERNS", true),
		FilterAdultNames:    getEnvBool("MAGNETAR_FILTER_ADULT_NAMES", true),
		FilterJunkNames:     getEnvBool("MAGNETAR_FILTER_JUNK_NAMES", true),

		BackupEnabled:  getEnvBool("MAGNETAR_BACKUP_ENABLED", false),
		BackupPath:     getEnvString("MAGNETAR_BACKUP_PATH", "/data/backups"),
		BackupSchedule: getEnvString("MAGNETAR_BACKUP_SCHEDULE", "0 3 * * *"),
		BackupRetain:   getEnvInt("MAGNETAR_BACKUP_RETAIN", 7),

		AnalyzeInterval:     getEnvInt("MAGNETAR_ANALYZE_INTERVAL", 50000),
		IntegrityCheckDaily: getEnvBool("MAGNETAR_INTEGRITY_CHECK_DAILY", true),
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return cfg, nil
}

func (c *Config) IsSQLite() bool {
	return strings.ToLower(c.DBBackend) == "sqlite"
}

func (c *Config) IsMariaDB() bool {
	return strings.ToLower(c.DBBackend) == "mariadb"
}

func (c *Config) Validate() error {
	if !c.IsSQLite() && !c.IsMariaDB() {
		return fmt.Errorf("invalid DB_BACKEND: %q (must be 'sqlite' or 'mariadb')", c.DBBackend)
	}

	if c.IsMariaDB() && c.DBDSN == "" {
		return fmt.Errorf("DB_DSN is required when DB_BACKEND=mariadb")
	}

	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("invalid PORT: %d (must be 1-65535)", c.Port)
	}

	if c.CrawlPort < 1 || c.CrawlPort > 65535 {
		return fmt.Errorf("invalid CRAWL_PORT: %d (must be 1-65535)", c.CrawlPort)
	}

	if c.CrawlRate < 0 {
		return fmt.Errorf("invalid CRAWL_RATE: %d (must be >= 0)", c.CrawlRate)
	}

	if c.CrawlWorkers < 1 {
		return fmt.Errorf("invalid CRAWL_WORKERS: %d (must be >= 1)", c.CrawlWorkers)
	}

	if c.DBCacheSize < 0 {
		return fmt.Errorf("invalid DB_CACHE_SIZE: %d (must be >= 0)", c.DBCacheSize)
	}

	if c.DBMmapSize < 0 {
		return fmt.Errorf("invalid DB_MMAP_SIZE: %d (must be >= 0)", c.DBMmapSize)
	}

	if c.DBMaxOpenConns < 1 {
		return fmt.Errorf("invalid DB_MAX_OPEN_CONNS: %d (must be >= 1)", c.DBMaxOpenConns)
	}

	if c.DBMaxIdleConns < 0 {
		return fmt.Errorf("invalid DB_MAX_IDLE_CONNS: %d (must be >= 0)", c.DBMaxIdleConns)
	}

	if c.DBMaxIdleConns > c.DBMaxOpenConns {
		return fmt.Errorf("DB_MAX_IDLE_CONNS (%d) cannot exceed DB_MAX_OPEN_CONNS (%d)", c.DBMaxIdleConns, c.DBMaxOpenConns)
	}

	if c.MatchBatchSize < 1 {
		return fmt.Errorf("invalid MATCH_BATCH_SIZE: %d (must be >= 1)", c.MatchBatchSize)
	}

	if c.MatchInterval < 0 {
		return fmt.Errorf("invalid MATCH_INTERVAL: %v (must be >= 0)", c.MatchInterval)
	}

	if c.MatchMaxAttempts < 1 || c.MatchMaxAttempts > 10 {
		return fmt.Errorf("invalid MATCH_MAX_ATTEMPTS: %d (must be 1-10)", c.MatchMaxAttempts)
	}

	if c.BackupRetain < 1 {
		return fmt.Errorf("invalid BACKUP_RETAIN: %d (must be >= 1)", c.BackupRetain)
	}

	if c.AnalyzeInterval < 0 {
		return fmt.Errorf("invalid ANALYZE_INTERVAL: %d (must be >= 0)", c.AnalyzeInterval)
	}

	validLogLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLogLevels[strings.ToLower(c.LogLevel)] {
		return fmt.Errorf("invalid LOG_LEVEL: %q (must be debug, info, warn, or error)", c.LogLevel)
	}

	return nil
}

func getEnvString(key, def string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return def
}

func getEnvInt(key string, def int) int {
	if val := os.Getenv(key); val != "" {
		i, err := strconv.Atoi(val)
		if err != nil {
			return def
		}
		return i
	}
	return def
}

func getEnvInt64(key string, def int64) int64 {
	if val := os.Getenv(key); val != "" {
		i, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return def
		}
		return i
	}
	return def
}

func getEnvBool(key string, def bool) bool {
	if val := os.Getenv(key); val != "" {
		b, err := strconv.ParseBool(val)
		if err != nil {
			return def
		}
		return b
	}
	return def
}

func getEnvStringSlice(key string, def []string) []string {
	if val := os.Getenv(key); val != "" {
		parts := strings.Split(val, ",")
		result := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				result = append(result, p)
			}
		}
		if len(result) > 0 {
			return result
		}
	}
	return def
}

func getEnvDuration(key string, def time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		d, err := time.ParseDuration(val)
		if err != nil {
			return def
		}
		return d
	}
	return def
}

func SetupLogging(cfg *Config) {
	logLevel := strings.ToLower(cfg.LogLevel)
	switch logLevel {
	case "debug":
		_ = os.Setenv("LOG_LEVEL", "debug")
	case "warn":
		_ = os.Setenv("LOG_LEVEL", "warn")
	case "error":
		_ = os.Setenv("LOG_LEVEL", "error")
	default:
		_ = os.Setenv("LOG_LEVEL", "info")
	}
}

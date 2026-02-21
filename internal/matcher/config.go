package matcher

import (
	"time"

	"github.com/magnetar/magnetar/internal/config"
)

type Config struct {
	Enabled     bool
	BatchSize   int
	Interval    time.Duration
	MaxAttempts int
	TMDBAPIKey  string
	TVDBAPIKey  string
}

func NewConfig(cfg *config.Config) Config {
	return Config{
		Enabled:     cfg.MatchEnabled,
		BatchSize:   cfg.MatchBatchSize,
		Interval:    cfg.MatchInterval,
		MaxAttempts: cfg.MatchMaxAttempts,
		TMDBAPIKey:  cfg.TMDBAPIKey,
		TVDBAPIKey:  cfg.TVDBAPIKey,
	}
}

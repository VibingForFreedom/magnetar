package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	_ "net/http/pprof" //nolint:gosec // intentional: bound to localhost:6060 only
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/magnetar/magnetar/internal/animedb"
	"github.com/magnetar/magnetar/internal/api"
	"github.com/magnetar/magnetar/internal/classify"
	"github.com/magnetar/magnetar/internal/config"
	"github.com/magnetar/magnetar/internal/crawler"
	"github.com/magnetar/magnetar/internal/matcher"
	"github.com/magnetar/magnetar/internal/metrics"
	"github.com/magnetar/magnetar/internal/store"
	"github.com/magnetar/magnetar/internal/tasklog"
	"github.com/magnetar/magnetar/internal/tracker"
)

const (
	Version   = "0.1.0-dev"
	BuildDate = "unknown"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return runServe(nil)
	}

	switch os.Args[1] {
	case "serve":
		fs := flag.NewFlagSet("serve", flag.ExitOnError)
		if err := fs.Parse(os.Args[2:]); err != nil {
			return fmt.Errorf("parsing flags: %w", err)
		}
		return runServe(fs)
	case "migrate":
		return runMigrate(os.Args[2:])
	case "backup":
		return runBackup(os.Args[2:])
	case "version":
		printVersion()
		return nil
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		printUsage()
		return errors.New("unknown command")
	}
}

func printUsage() {
	fmt.Println(`Magnetar - DHT Crawler & Torrent Classification Engine

Usage:
  magnetar [command] [flags]

Commands:
  serve     Start the Magnetar server (default)
  migrate   Migrate data between backends
  backup    Create a manual database backup
  version   Print version information

Flags:
  -h, --help
      Show help

Use "magnetar [command] --help" for more information about a command.`)
}

func printVersion() {
	fmt.Printf("Magnetar v%s (built %s)\n", Version, BuildDate)
}

func runServe(_ *flag.FlagSet) error { //nolint:unparam
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	config.SetupLogging(cfg)

	logger := slog.Default()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	logger.Info("starting Magnetar server",
		"version", Version,
		"port", cfg.Port,
		"db_backend", cfg.DBBackend,
		"db_path", cfg.DBPath,
		"log_level", cfg.LogLevel,
	)

	var st store.Store
	switch {
	case cfg.IsSQLite():
		st, err = store.NewSQLiteStore(ctx, cfg)
		if err != nil {
			return fmt.Errorf("initializing sqlite store: %w", err)
		}
	case cfg.IsMariaDB():
		st, err = store.NewMariaDBStore(ctx, cfg)
		if err != nil {
			return fmt.Errorf("initializing mariadb store: %w", err)
		}
	default:
		return fmt.Errorf("unsupported database backend: %s", cfg.DBBackend)
	}
	defer func() {
		logger.Info("closing database store")
		if err := st.Close(); err != nil {
			logger.Error("error closing store", "error", err)
		}
	}()

	// Apply persisted settings overrides from database
	if err := cfg.ApplyOverrides(ctx, st); err != nil {
		logger.Warn("could not apply settings overrides", "error", err)
	}

	// Sync classify filter config from loaded settings
	syncFilterConfig(cfg)

	stats, err := st.Stats(ctx)
	if err != nil {
		logger.Warn("could not fetch initial stats", "error", err)
	} else {
		logger.Info("database initialized",
			"total_torrents", stats.TotalTorrents,
			"matched", stats.Matched,
			"unmatched", stats.Unmatched,
			"failed", stats.Failed,
			"db_size_mb", stats.DBSize/1024/1024,
		)
	}

	// Create task registry for scheduled task tracking
	registry := tasklog.New()

	// Register known tasks based on backend
	if cfg.IsSQLite() {
		registry.Register("WAL Checkpoint", "5m")
		if cfg.IntegrityCheckDaily {
			registry.Register("Integrity Check", "24h")
		}
	}
	if cfg.AnalyzeInterval > 0 {
		registry.Register("ANALYZE", "on insert threshold")
	} else if cfg.IsMariaDB() {
		registry.Register("ANALYZE", "1h")
	}
	registry.Register("Rejected Hash Purge", "24h")
	registry.Register("Junk Torrent Purge", "24h")

	// Wire registry to store and enable persistence
	switch s := st.(type) {
	case *store.SQLiteStore:
		s.SetTaskRegistry(registry)
	case *store.MariaDBStore:
		s.SetTaskRegistry(registry)
	}
	registry.SetPersister(ctx, st)

	m := metrics.New()

	apiServer := api.NewServer(st, cfg, m)
	apiServer.SetTaskRegistry(registry)

	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      apiServer.Handler(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // Disabled for SSE support
		IdleTimeout:  120 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("HTTP server listening", "addr", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// Start pprof debug server on localhost only
	runtime.SetMutexProfileFraction(5)
	runtime.SetBlockProfileRate(1)
	go func() {
		pprofServer := &http.Server{
			Addr:         "127.0.0.1:6060",
			Handler:      nil, // DefaultServeMux (net/http/pprof registers there)
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 120 * time.Second,
		}
		logger.Info("pprof server listening", "addr", pprofServer.Addr)
		if err := pprofServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Warn("pprof server error", "error", err)
		}
	}()

	// Start DHT crawler if enabled
	var dhtCrawler *crawler.Crawler
	if cfg.CrawlEnabled {
		crawlCfg := crawler.NewDefaultConfig()
		crawlCfg.Port = uint16(cfg.CrawlPort)           //nolint:gosec // validated in config
		crawlCfg.ScalingFactor = uint(cfg.CrawlWorkers) //nolint:gosec // validated in config

		var crawlErr error
		dhtCrawler, crawlErr = crawler.New(crawlCfg, st, m, logger)
		if crawlErr != nil {
			return fmt.Errorf("initializing DHT crawler: %w", crawlErr)
		}

		go func() {
			if err := dhtCrawler.Start(ctx); err != nil {
				logger.Error("DHT crawler error", "error", err)
			}
		}()

		logger.Info("DHT crawler started", "port", cfg.CrawlPort, "workers", cfg.CrawlWorkers)
		apiServer.SetCrawler(dhtCrawler)
	}

	// Create tracker scraper
	trackerScraper := tracker.New(cfg, logger)
	if dhtCrawler != nil {
		dhtCrawler.SetTrackerScraper(trackerScraper)
	}
	apiServer.SetTrackerScraper(trackerScraper)

	if cfg.TrackerEnabled {
		logger.Info("tracker scraper enabled", "trackers", len(cfg.TrackerList))
	}

	// Start anime offline database if enabled
	var adb *animedb.AnimeDB
	if cfg.AnimeDBEnabled {
		registry.Register("Anime DB Refresh", "24h")
		adb = animedb.New(logger)
		adb.SetTaskRegistry(registry)
		go adb.Start(ctx)
	}

	// Start daily purge goroutine for rejected hashes and legacy junk
	go runDailyPurge(ctx, st, registry, logger)

	// Start metadata matcher if enabled
	var metaMatcher *matcher.Matcher
	if cfg.MatchEnabled {
		matchCfg := matcher.NewConfig(cfg)
		metaMatcher = matcher.New(matchCfg, st, adb, m, logger)

		go func() {
			if err := metaMatcher.Start(ctx); err != nil {
				logger.Error("matcher error", "error", err)
			}
		}()

		logger.Info("metadata matcher started",
			"interval", cfg.MatchInterval,
			"batch_size", cfg.MatchBatchSize,
		)
		apiServer.SetMatcher(metaMatcher)
		apiServer.SetMatcherTrigger(metaMatcher)
	}

	select {
	case sig := <-sigChan:
		logger.Info("received shutdown signal", "signal", sig)
	case err := <-serverErr:
		logger.Error("HTTP server error", "error", err)
		return err
	}

	cancel()

	// Stop metadata matcher
	if metaMatcher != nil {
		logger.Info("stopping metadata matcher")
		metaMatcher.Stop()
		metaMatcher.CloseCache()
	}

	// Stop DHT crawler
	if dhtCrawler != nil {
		logger.Info("stopping DHT crawler")
		dhtCrawler.Stop()
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	logger.Info("shutting down HTTP server")
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown error", "error", err)
	}

	logger.Info("Magnetar server stopped")
	return nil
}

func runMigrate(args []string) error {
	fs := flag.NewFlagSet("migrate", flag.ExitOnError)
	from := fs.String("from", "", "Source backend type (sqlite, mariadb)")
	fromPath := fs.String("from-path", "", "Source database path or DSN")
	to := fs.String("to", "", "Destination backend type (sqlite, mariadb)")
	toDSN := fs.String("to-dsn", "", "Destination database path or DSN")
	batchSize := fs.Int("batch-size", 5000, "Rows per batch")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	if *from == "" || *fromPath == "" || *to == "" || *toDSN == "" {
		return errors.New("all flags required: --from, --from-path, --to, --to-dsn")
	}

	validBackends := map[string]bool{"sqlite": true, "mariadb": true}
	if !validBackends[*from] {
		return fmt.Errorf("invalid --from backend: %q (must be sqlite or mariadb)", *from)
	}
	if !validBackends[*to] {
		return fmt.Errorf("invalid --to backend: %q (must be sqlite or mariadb)", *to)
	}

	config.SetupLogging(&config.Config{LogLevel: "info"})

	ctx := context.Background()
	return store.RunMigration(ctx, store.MigrationConfig{
		FromBackend: *from,
		FromPath:    *fromPath,
		ToBackend:   *to,
		ToDSN:       *toDSN,
		BatchSize:   *batchSize,
	})
}

func runBackup(args []string) error {
	fs := flag.NewFlagSet("backup", flag.ExitOnError)
	output := fs.String("output", "", "Output path for backup file")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	if *output == "" {
		return errors.New("--output flag is required")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	config.SetupLogging(cfg)
	logger := slog.Default()

	if !cfg.IsSQLite() {
		return errors.New("backup is only supported for SQLite backend")
	}

	ctx := context.Background()

	st, err := store.NewSQLiteStore(ctx, cfg)
	if err != nil {
		return fmt.Errorf("initializing store: %w", err)
	}
	defer func() { _ = st.Close() }()

	logger.Info("creating backup", "output", *output)

	if err := st.Backup(ctx, *output); err != nil {
		return fmt.Errorf("creating backup: %w", err)
	}

	logger.Info("backup completed successfully", "path", *output)
	return nil
}

// syncFilterConfig pushes the config filter toggles into the classify package.
func syncFilterConfig(cfg *config.Config) {
	classify.SetFilterConfig(classify.FilterConfig{
		FilterAdultPatterns: cfg.FilterAdultPatterns,
		FilterAdultNames:    cfg.FilterAdultNames,
		FilterJunkNames:     cfg.FilterJunkNames,
	})
}

func runDailyPurge(ctx context.Context, st store.Store, registry *tasklog.Registry, logger *slog.Logger) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			purgeCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)

			purged, err := st.PurgeOldRejected(purgeCtx, 90*24*time.Hour)
			if err != nil {
				logger.Error("failed to purge old rejected hashes", "error", err)
			} else if purged > 0 {
				logger.Info("purged expired rejected hashes", "count", purged)
			}
			registry.Record("Rejected Hash Purge", fmt.Sprintf("Purged %d", purged), err)

			junkPurged, err := st.PurgeJunkTorrents(purgeCtx)
			if err != nil {
				logger.Error("failed to purge junk torrents", "error", err)
			} else if junkPurged > 0 {
				logger.Info("purged legacy junk torrents", "count", junkPurged)
			}
			registry.Record("Junk Torrent Purge", fmt.Sprintf("Purged %d", junkPurged), err)

			cancel()
		}
	}
}

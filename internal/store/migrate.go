package store

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/magnetar/magnetar/internal/config"
)

// MigrationConfig holds the parameters for a cross-backend migration.
type MigrationConfig struct {
	FromBackend string // "sqlite" or "mariadb"
	FromPath    string // file path for sqlite, DSN for mariadb
	ToBackend   string // "sqlite" or "mariadb"
	ToDSN       string // file path for sqlite, DSN for mariadb
	BatchSize   int    // rows per batch (default 5000)
}

// pageFetcher is the interface for keyset-paginated reads used during migration.
type pageFetcher interface {
	fetchPage(ctx context.Context, afterDiscoveredAt int64, afterInfoHash []byte, limit int) ([]*Torrent, error)
}

// RunMigration streams all torrents from source to destination using keyset pagination.
func RunMigration(ctx context.Context, mcfg MigrationConfig) error {
	logger := slog.Default()

	if mcfg.BatchSize <= 0 {
		mcfg.BatchSize = 5000
	}

	src, err := openStoreForMigration(ctx, mcfg.FromBackend, mcfg.FromPath)
	if err != nil {
		return fmt.Errorf("opening source store: %w", err)
	}
	defer src.Close()

	dst, err := openStoreForMigration(ctx, mcfg.ToBackend, mcfg.ToDSN)
	if err != nil {
		return fmt.Errorf("opening destination store: %w", err)
	}
	defer dst.Close()

	srcStats, err := src.Stats(ctx)
	if err != nil {
		return fmt.Errorf("getting source stats: %w", err)
	}

	totalRows := srcStats.TotalTorrents
	logger.Info("starting migration",
		"from", mcfg.FromBackend,
		"to", mcfg.ToBackend,
		"total_rows", totalRows,
		"batch_size", mcfg.BatchSize,
	)

	if totalRows == 0 {
		logger.Info("source is empty, nothing to migrate")
		return nil
	}

	fetcher, ok := src.(pageFetcher)
	if !ok {
		return fmt.Errorf("source store does not support page fetching")
	}

	var (
		migrated         int64
		afterDiscoveredAt int64
		afterInfoHash    []byte
		startTime        = time.Now()
	)

	for {
		page, err := fetcher.fetchPage(ctx, afterDiscoveredAt, afterInfoHash, mcfg.BatchSize)
		if err != nil {
			return fmt.Errorf("fetching page: %w", err)
		}

		if len(page) == 0 {
			break
		}

		if err := dst.UpsertTorrents(ctx, page); err != nil {
			return fmt.Errorf("upserting batch: %w", err)
		}

		migrated += int64(len(page))

		last := page[len(page)-1]
		afterDiscoveredAt = last.DiscoveredAt
		afterInfoHash = last.InfoHash

		elapsed := time.Since(startTime).Seconds()
		rate := float64(migrated) / elapsed
		pct := float64(migrated) / float64(totalRows) * 100

		logger.Info("migration progress",
			"migrated", migrated,
			"total", totalRows,
			"pct", fmt.Sprintf("%.1f%%", pct),
			"rate", fmt.Sprintf("%.0f rows/sec", rate),
		)

		if len(page) < mcfg.BatchSize {
			break
		}
	}

	dstStats, err := dst.Stats(ctx)
	if err != nil {
		logger.Warn("could not verify destination count", "error", err)
	} else if dstStats.TotalTorrents != totalRows {
		logger.Warn("row count mismatch",
			"source", totalRows,
			"destination", dstStats.TotalTorrents,
		)
	}

	logger.Info("migration completed",
		"migrated", migrated,
		"duration", time.Since(startTime).Round(time.Second),
	)

	return nil
}

// openStoreForMigration opens a Store for the given backend and path/DSN.
func openStoreForMigration(ctx context.Context, backend, pathOrDSN string) (Store, error) {
	switch backend {
	case "sqlite":
		cfg := &config.Config{
			DBBackend:   "sqlite",
			DBPath:      pathOrDSN,
			DBCacheSize: 65536,
			DBMmapSize:  268435456,
		}
		return NewSQLiteStore(ctx, cfg)
	case "mariadb":
		cfg := &config.Config{
			DBBackend:      "mariadb",
			DBDSN:          pathOrDSN,
			DBMaxOpenConns: 25,
			DBMaxIdleConns: 10,
		}
		return NewMariaDBStore(ctx, cfg)
	default:
		return nil, fmt.Errorf("unsupported backend: %s", backend)
	}
}

//go:build !nosqlite

package store

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/magnetar/magnetar/internal/config"
)

func TestRunMigration_SQLiteToSQLite(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "source.db")
	dstPath := filepath.Join(dstDir, "dest.db")

	ctx := context.Background()

	// Seed source
	src := newTestStoreAt(t, srcPath)
	const count = 150
	for i := range count {
		torrent := makeTestTorrent(makeInfoHash(), fmt.Sprintf("Migration Test %d", i))
		torrent.DiscoveredAt = int64(1000 + i) // deterministic ordering
		if err := src.UpsertTorrent(ctx, torrent); err != nil {
			t.Fatalf("UpsertTorrent failed: %v", err)
		}
	}
	_ = src.Close()

	err := RunMigration(ctx, MigrationConfig{
		FromBackend: "sqlite",
		FromPath:    srcPath,
		ToBackend:   "sqlite",
		ToDSN:       dstPath,
		BatchSize:   50,
	})
	if err != nil {
		t.Fatalf("RunMigration failed: %v", err)
	}

	// Verify destination
	dst := newTestStoreAt(t, dstPath)
	defer func() { _ = dst.Close() }()

	stats, err := dst.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}
	if stats.TotalTorrents != count {
		t.Errorf("expected %d torrents in destination, got %d", count, stats.TotalTorrents)
	}
}

func TestRunMigration_EmptySource(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "source.db")
	dstPath := filepath.Join(dstDir, "dest.db")

	ctx := context.Background()

	// Create empty source
	src := newTestStoreAt(t, srcPath)
	_ = src.Close()

	err := RunMigration(ctx, MigrationConfig{
		FromBackend: "sqlite",
		FromPath:    srcPath,
		ToBackend:   "sqlite",
		ToDSN:       dstPath,
		BatchSize:   50,
	})
	if err != nil {
		t.Fatalf("RunMigration with empty source failed: %v", err)
	}
}

func TestRunMigration_Idempotent(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "source.db")
	dstPath := filepath.Join(dstDir, "dest.db")

	ctx := context.Background()

	src := newTestStoreAt(t, srcPath)
	const count = 30
	for i := range count {
		torrent := makeTestTorrent(makeInfoHash(), fmt.Sprintf("Idempotent Test %d", i))
		torrent.DiscoveredAt = int64(2000 + i)
		if err := src.UpsertTorrent(ctx, torrent); err != nil {
			t.Fatalf("UpsertTorrent failed: %v", err)
		}
	}
	_ = src.Close()

	mcfg := MigrationConfig{
		FromBackend: "sqlite",
		FromPath:    srcPath,
		ToBackend:   "sqlite",
		ToDSN:       dstPath,
		BatchSize:   10,
	}

	// Run twice
	if err := RunMigration(ctx, mcfg); err != nil {
		t.Fatalf("first RunMigration failed: %v", err)
	}
	if err := RunMigration(ctx, mcfg); err != nil {
		t.Fatalf("second RunMigration failed: %v", err)
	}

	dst := newTestStoreAt(t, dstPath)
	defer func() { _ = dst.Close() }()

	stats, err := dst.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}
	if stats.TotalTorrents != count {
		t.Errorf("expected %d torrents after idempotent migration, got %d", count, stats.TotalTorrents)
	}
}

// newTestStoreAt creates a SQLiteStore at a specific path.
func newTestStoreAt(t *testing.T, dbPath string) *SQLiteStore {
	t.Helper()

	cfg := &config.Config{
		DBPath:          dbPath,
		DBCacheSize:     65536,
		DBMmapSize:      268435456,
		DBSyncFull:      false,
		AnalyzeInterval: 0,
	}

	st, err := NewSQLiteStore(context.Background(), cfg)
	if err != nil {
		t.Fatalf("failed to create store at %s: %v", dbPath, err)
	}

	return st
}

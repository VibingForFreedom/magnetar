//go:build !nosqlite

package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/magnetar/magnetar/internal/config"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := &config.Config{
		DBPath:          dbPath,
		DBCacheSize:     65536,
		DBMmapSize:      268435456,
		DBSyncFull:      false,
		AnalyzeInterval: 0,
	}

	store, err := NewSQLiteStore(context.Background(), cfg)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	t.Cleanup(func() {
		store.Close()
	})

	return store
}

func TestSQLiteStoreShared(t *testing.T) {
	storeTestSuite(t, func(t *testing.T) Store {
		return newTestStore(t)
	})
}

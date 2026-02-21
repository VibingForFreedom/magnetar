//go:build mariadb

package store

import (
	"context"
	"os"
	"testing"

	"github.com/magnetar/magnetar/internal/config"
)

func newTestMariaDBStore(t *testing.T) *MariaDBStore {
	t.Helper()

	dsn := os.Getenv("TEST_MARIADB_DSN")
	if dsn == "" {
		t.Skip("TEST_MARIADB_DSN not set, skipping MariaDB tests")
	}

	cfg := &config.Config{
		DBBackend:       "mariadb",
		DBDSN:           dsn,
		DBMaxOpenConns:  10,
		DBMaxIdleConns:  5,
		AnalyzeInterval: 0,
	}

	st, err := NewMariaDBStore(context.Background(), cfg)
	if err != nil {
		t.Fatalf("failed to create MariaDB store: %v", err)
	}

	// Clean table before each test
	if _, err := st.db.Exec("TRUNCATE TABLE torrents"); err != nil {
		t.Fatalf("failed to truncate torrents: %v", err)
	}

	t.Cleanup(func() {
		st.Close()
	})

	return st
}

func TestMariaDBStoreShared(t *testing.T) {
	storeTestSuite(t, func(t *testing.T) Store {
		return newTestMariaDBStore(t)
	})
}

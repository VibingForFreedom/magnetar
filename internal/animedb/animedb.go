package animedb

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/magnetar/magnetar/internal/tasklog"
)

const refreshInterval = 24 * time.Hour

// AnimeDB provides offline anime title lookup using data from
// manami-project/anime-offline-database and AniDB title dumps.
type AnimeDB struct {
	index        atomic.Pointer[TitleIndex]
	logger       *slog.Logger
	loaded       atomic.Bool
	taskRegistry *tasklog.Registry
}

// SetTaskRegistry sets the task registry for reporting refresh status.
func (db *AnimeDB) SetTaskRegistry(r *tasklog.Registry) {
	db.taskRegistry = r
}

// New creates a new AnimeDB instance.
func New(logger *slog.Logger) *AnimeDB {
	return &AnimeDB{
		logger: logger.With("component", "animedb"),
	}
}

// Load downloads both data sources, merges them, and builds the title index.
func (db *AnimeDB) Load(ctx context.Context) error {
	db.logger.Info("loading anime offline database")

	entries, allTitles, err := downloadManami(ctx)
	if err != nil {
		return err
	}
	db.logger.Info("downloaded manami database", "entries", len(entries))

	// Build AniDBID -> entry index for merging
	anidbMap := make(map[int]int, len(entries))
	for i, e := range entries {
		if e.AniDBID > 0 {
			anidbMap[e.AniDBID] = i
		}
	}

	// Download AniDB titles and merge extra titles
	anidbTitles, err := downloadAniDBTitles(ctx)
	if err != nil {
		db.logger.Warn("anidb title download failed, continuing without it", "error", err)
	} else {
		db.logger.Info("downloaded anidb titles", "anime_count", len(anidbTitles))
		merged := 0
		for anidbID, titles := range anidbTitles {
			if idx, ok := anidbMap[anidbID]; ok {
				allTitles[idx] = append(allTitles[idx], titles...)
				merged++
			}
		}
		db.logger.Info("merged anidb titles", "merged_entries", merged)
	}

	// Build the new index
	idx := newTitleIndex()
	for i, entry := range entries {
		idx.addEntry(entry, allTitles[i])
	}

	db.index.Store(idx)
	db.loaded.Store(true)

	db.logger.Info("anime offline database loaded",
		"entries", idx.Len(),
		"index_size", len(idx.exact),
	)

	return nil
}

// Start begins a background goroutine that refreshes the database periodically.
// It checks persisted timestamps to avoid re-downloading if recently loaded.
func (db *AnimeDB) Start(ctx context.Context) {
	sinceLastRun := db.timeSinceLastRun()

	if sinceLastRun < refreshInterval {
		remaining := refreshInterval - sinceLastRun
		db.logger.Info("anime db loaded recently, skipping initial download",
			"last_run_ago", sinceLastRun.Round(time.Minute).String(),
			"next_refresh_in", remaining.Round(time.Minute).String(),
		)

		select {
		case <-ctx.Done():
			return
		case <-time.After(remaining):
			err := db.Load(ctx)
			if err != nil {
				db.logger.Error("anime db refresh failed", "error", err)
			}
			db.recordTaskResult(err)
		}
	} else {
		err := db.Load(ctx)
		if err != nil {
			db.logger.Error("initial anime db load failed", "error", err)
		}
		db.recordTaskResult(err)
	}

	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := db.Load(ctx)
			if err != nil {
				db.logger.Error("anime db refresh failed", "error", err)
			}
			db.recordTaskResult(err)
		}
	}
}

func (db *AnimeDB) timeSinceLastRun() time.Duration {
	if db.taskRegistry == nil {
		return time.Duration(1<<63 - 1) // max duration — always load
	}
	return db.taskRegistry.TimeSinceLastRun("Anime DB Refresh")
}

func (db *AnimeDB) recordTaskResult(err error) {
	if db.taskRegistry == nil {
		return
	}
	if err != nil {
		db.taskRegistry.Record("Anime DB Refresh", err.Error(), err)
		return
	}
	result := fmt.Sprintf("Loaded %d titles", db.EntryCount())
	db.taskRegistry.Record("Anime DB Refresh", result, nil)
}

// Lookup searches for an anime by title and returns the matching entry or nil.
func (db *AnimeDB) Lookup(title string) *AnimeEntry {
	idx := db.index.Load()
	if idx == nil {
		return nil
	}
	return idx.Lookup(title)
}

// Contains returns true if the title matches any anime in the database.
func (db *AnimeDB) Contains(title string) bool {
	idx := db.index.Load()
	if idx == nil {
		return false
	}
	return idx.Contains(title)
}

// IsLoaded returns whether the database has been loaded at least once.
func (db *AnimeDB) IsLoaded() bool {
	return db.loaded.Load()
}

// EntryCount returns the number of anime entries in the database.
func (db *AnimeDB) EntryCount() int {
	idx := db.index.Load()
	if idx == nil {
		return 0
	}
	return idx.Len()
}

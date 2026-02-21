//go:build !nosqlite

package store

import (
	"context"
	"crypto/rand"
	"fmt"
	"path/filepath"
	"testing"
	"time"

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

func makeInfoHash() []byte {
	b := make([]byte, 20)
	rand.Read(b)
	return b
}

func makeTestTorrent(infoHash []byte, name string) *Torrent {
	return &Torrent{
		InfoHash:     infoHash,
		Name:         name,
		Size:         1024 * 1024 * 1024,
		Category:     CategoryMovie,
		Quality:      QualityFHD,
		Files:        []File{{Path: "movie.mkv", Size: 1024 * 1024 * 1024}},
		MatchStatus:  MatchUnmatched,
		Seeders:      10,
		Leechers:     5,
		Source:       SourceDHT,
		DiscoveredAt: time.Now().Unix(),
		UpdatedAt:    time.Now().Unix(),
	}
}

func TestSQLiteStore_CRUD(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	infoHash := makeInfoHash()
	original := makeTestTorrent(infoHash, "Test Movie 2024 1080p BluRay")
	original.IMDBID = "tt1234567"
	original.TMDBID = 12345
	original.MediaYear = 2024

	if err := store.UpsertTorrent(ctx, original); err != nil {
		t.Fatalf("UpsertTorrent failed: %v", err)
	}

	got, err := store.GetTorrent(ctx, infoHash)
	if err != nil {
		t.Fatalf("GetTorrent failed: %v", err)
	}

	if string(got.InfoHash) != string(original.InfoHash) {
		t.Errorf("InfoHash mismatch: got %x, want %x", got.InfoHash, original.InfoHash)
	}
	if got.Name != original.Name {
		t.Errorf("Name mismatch: got %q, want %q", got.Name, original.Name)
	}
	if got.Size != original.Size {
		t.Errorf("Size mismatch: got %d, want %d", got.Size, original.Size)
	}
	if got.Category != original.Category {
		t.Errorf("Category mismatch: got %d, want %d", got.Category, original.Category)
	}
	if got.Quality != original.Quality {
		t.Errorf("Quality mismatch: got %d, want %d", got.Quality, original.Quality)
	}
	if got.IMDBID != original.IMDBID {
		t.Errorf("IMDBID mismatch: got %q, want %q", got.IMDBID, original.IMDBID)
	}
	if got.TMDBID != original.TMDBID {
		t.Errorf("TMDBID mismatch: got %d, want %d", got.TMDBID, original.TMDBID)
	}
	if got.MediaYear != original.MediaYear {
		t.Errorf("MediaYear mismatch: got %d, want %d", got.MediaYear, original.MediaYear)
	}
	if got.Seeders != original.Seeders {
		t.Errorf("Seeders mismatch: got %d, want %d", got.Seeders, original.Seeders)
	}
	if got.Leechers != original.Leechers {
		t.Errorf("Leechers mismatch: got %d, want %d", got.Leechers, original.Leechers)
	}
	if got.Source != original.Source {
		t.Errorf("Source mismatch: got %d, want %d", got.Source, original.Source)
	}
	if len(got.Files) != len(original.Files) {
		t.Errorf("Files count mismatch: got %d, want %d", len(got.Files), len(original.Files))
	} else if got.Files[0].Path != original.Files[0].Path {
		t.Errorf("Files[0].Path mismatch: got %q, want %q", got.Files[0].Path, original.Files[0].Path)
	}

	updated := makeTestTorrent(infoHash, "Test Movie 2024 1080p BluRay REMUX")
	updated.Size = 2 * 1024 * 1024 * 1024
	updated.Seeders = 20
	updated.Leechers = 10
	updated.UpdatedAt = time.Now().Unix()

	if err := store.UpsertTorrent(ctx, updated); err != nil {
		t.Fatalf("UpsertTorrent (update) failed: %v", err)
	}

	got, err = store.GetTorrent(ctx, infoHash)
	if err != nil {
		t.Fatalf("GetTorrent after update failed: %v", err)
	}

	if got.Size != updated.Size {
		t.Errorf("Size after update mismatch: got %d, want %d", got.Size, updated.Size)
	}
	if got.Seeders != updated.Seeders {
		t.Errorf("Seeders after update mismatch: got %d, want %d", got.Seeders, updated.Seeders)
	}
	if got.Name != updated.Name {
		t.Errorf("Name after update mismatch: got %q, want %q", got.Name, updated.Name)
	}

	if err := store.DeleteTorrent(ctx, infoHash); err != nil {
		t.Fatalf("DeleteTorrent failed: %v", err)
	}

	_, err = store.GetTorrent(ctx, infoHash)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got: %v", err)
	}

	if err := store.DeleteTorrent(ctx, infoHash); err != ErrNotFound {
		t.Errorf("expected ErrNotFound when deleting non-existent torrent, got: %v", err)
	}
}

func TestSQLiteStore_BulkInsert(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	const count = 100
	torrents := make([]*Torrent, count)
	for i := 0; i < count; i++ {
		infoHash := makeInfoHash()
		torrents[i] = makeTestTorrent(infoHash, fmt.Sprintf("Bulk Movie %d 2024 1080p", i))
		torrents[i].Category = CategoryTV
		torrents[i].Quality = QualityHD
	}

	if err := store.UpsertTorrents(ctx, torrents); err != nil {
		t.Fatalf("UpsertTorrents failed: %v", err)
	}

	stats, err := store.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}

	if stats.TotalTorrents != count {
		t.Errorf("TotalTorrents mismatch: got %d, want %d", stats.TotalTorrents, count)
	}

	for i := 0; i < 10; i++ {
		idx := i * 10
		got, err := store.GetTorrent(ctx, torrents[idx].InfoHash)
		if err != nil {
			t.Errorf("GetTorrent[%d] failed: %v", idx, err)
			continue
		}
		if got.Name != torrents[idx].Name {
			t.Errorf("Name[%d] mismatch: got %q, want %q", idx, got.Name, torrents[idx].Name)
		}
	}
}

func TestSQLiteStore_Search(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	testTorrents := []*struct {
		name     string
		category Category
	}{
		{"The Matrix 1999 1080p BluRay", CategoryMovie},
		{"The Matrix Reloaded 2003 1080p BluRay", CategoryMovie},
		{"The Matrix Revolutions 2003 1080p BluRay", CategoryMovie},
		{"Breaking Bad S01 720p WEB-DL", CategoryTV},
		{"Game of Thrones S01 1080p BluRay", CategoryTV},
		{"Attack on Titan S01 1080p WEB-DL", CategoryAnime},
		{"Interstellar 2014 4K UHD BluRay", CategoryMovie},
	}

	for _, tt := range testTorrents {
		torrent := makeTestTorrent(makeInfoHash(), tt.name)
		torrent.Category = tt.category
		if err := store.UpsertTorrent(ctx, torrent); err != nil {
			t.Fatalf("UpsertTorrent failed for %q: %v", tt.name, err)
		}
	}

	result, err := store.SearchByName(ctx, "Matrix", SearchOpts{})
	if err != nil {
		t.Fatalf("SearchByName failed: %v", err)
	}

	if len(result.Torrents) < 3 {
		t.Errorf("expected at least 3 Matrix results, got %d", len(result.Torrents))
	}
	for _, t2 := range result.Torrents {
		if !contains(t2.Name, "Matrix") {
			t.Errorf("unexpected search result: %q", t2.Name)
		}
	}

	result, err = store.SearchByName(ctx, "Matrix", SearchOpts{Categories: []Category{CategoryMovie}})
	if err != nil {
		t.Fatalf("SearchByName with category filter failed: %v", err)
	}

	for _, t2 := range result.Torrents {
		if t2.Category != CategoryMovie {
			t.Errorf("expected only movies, got category %d for %q", t2.Category, t2.Name)
		}
	}

	result, err = store.SearchByName(ctx, "Breaking", SearchOpts{Limit: 10})
	if err != nil {
		t.Fatalf("SearchByName with limit failed: %v", err)
	}

	if len(result.Torrents) != 1 {
		t.Errorf("expected 1 Breaking Bad result, got %d", len(result.Torrents))
	}
	if len(result.Torrents) > 0 && result.Torrents[0].Name != "Breaking Bad S01 720p WEB-DL" {
		t.Errorf("unexpected result: %q", result.Torrents[0].Name)
	}
}

func TestSQLiteStore_MatchQueue(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	const unmatchedCount = 5
	for i := 0; i < unmatchedCount; i++ {
		torrent := makeTestTorrent(makeInfoHash(), fmt.Sprintf("Unmatched Movie %d 2024", i))
		torrent.MatchStatus = MatchUnmatched
		torrent.MatchAttempts = 0
		torrent.MatchAfter = 0
		if err := store.UpsertTorrent(ctx, torrent); err != nil {
			t.Fatalf("UpsertTorrent failed: %v", err)
		}
	}

	matchedHash := makeInfoHash()
	matchedTorrent := makeTestTorrent(matchedHash, "Already Matched Movie 2024")
	matchedTorrent.MatchStatus = MatchMatched
	if err := store.UpsertTorrent(ctx, matchedTorrent); err != nil {
		t.Fatalf("UpsertTorrent failed for matched: %v", err)
	}

	unmatched, err := store.FetchUnmatched(ctx, 10)
	if err != nil {
		t.Fatalf("FetchUnmatched failed: %v", err)
	}

	if len(unmatched) != unmatchedCount {
		t.Errorf("expected %d unmatched, got %d", unmatchedCount, len(unmatched))
	}

	testHash := unmatched[0].InfoHash
	matchResult := MatchResult{
		Status: MatchMatched,
		IMDBID: "tt9876543",
		TMDBID: 98765,
		Year:   2024,
	}

	if err := store.UpdateMatchResult(ctx, testHash, matchResult); err != nil {
		t.Fatalf("UpdateMatchResult failed: %v", err)
	}

	got, err := store.GetTorrent(ctx, testHash)
	if err != nil {
		t.Fatalf("GetTorrent failed: %v", err)
	}

	if got.MatchStatus != MatchMatched {
		t.Errorf("MatchStatus mismatch: got %d, want %d", got.MatchStatus, MatchMatched)
	}
	if got.IMDBID != matchResult.IMDBID {
		t.Errorf("IMDBID mismatch: got %q, want %q", got.IMDBID, matchResult.IMDBID)
	}
	if got.TMDBID != matchResult.TMDBID {
		t.Errorf("TMDBID mismatch: got %d, want %d", got.TMDBID, matchResult.TMDBID)
	}
	if got.MediaYear != matchResult.Year {
		t.Errorf("MediaYear mismatch: got %d, want %d", got.MediaYear, matchResult.Year)
	}
	if got.MatchAttempts != 1 {
		t.Errorf("MatchAttempts mismatch: got %d, want 1", got.MatchAttempts)
	}

	unmatched, err = store.FetchUnmatched(ctx, 10)
	if err != nil {
		t.Fatalf("FetchUnmatched after update failed: %v", err)
	}

	if len(unmatched) != unmatchedCount-1 {
		t.Errorf("expected %d unmatched after update, got %d", unmatchedCount-1, len(unmatched))
	}

	stats, err := store.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}

	if stats.Unmatched != unmatchedCount-1 {
		t.Errorf("Unmatched count mismatch: got %d, want %d", stats.Unmatched, unmatchedCount-1)
	}
	if stats.Matched != 2 {
		t.Errorf("Matched count mismatch: got %d, want 2", stats.Matched)
	}
}

func TestSQLiteStore_ExternalID(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	imdbHash := makeInfoHash()
	imdbTorrent := makeTestTorrent(imdbHash, "IMDB Test Movie 2024")
	imdbTorrent.IMDBID = "tt1234567"
	if err := store.UpsertTorrent(ctx, imdbTorrent); err != nil {
		t.Fatalf("UpsertTorrent failed: %v", err)
	}

	tmdbHash := makeInfoHash()
	tmdbTorrent := makeTestTorrent(tmdbHash, "TMDB Test Movie 2024")
	tmdbTorrent.TMDBID = 12345
	if err := store.UpsertTorrent(ctx, tmdbTorrent); err != nil {
		t.Fatalf("UpsertTorrent failed: %v", err)
	}

	anilistHash := makeInfoHash()
	anilistTorrent := makeTestTorrent(anilistHash, "AniList Test Anime 2024")
	anilistTorrent.AniListID = 12345
	anilistTorrent.Category = CategoryAnime
	if err := store.UpsertTorrent(ctx, anilistTorrent); err != nil {
		t.Fatalf("UpsertTorrent failed: %v", err)
	}

	results, err := store.SearchByExternalID(ctx, ExternalID{Type: "imdb", Value: "tt1234567"})
	if err != nil {
		t.Fatalf("SearchByExternalID (IMDB) failed: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 IMDB result, got %d", len(results))
	}
	if len(results) > 0 {
		if string(results[0].InfoHash) != string(imdbHash) {
			t.Errorf("IMDB result InfoHash mismatch")
		}
		if results[0].IMDBID != "tt1234567" {
			t.Errorf("IMDB result IMDBID mismatch: got %q", results[0].IMDBID)
		}
	}

	results, err = store.SearchByExternalID(ctx, ExternalID{Type: "tmdb", Value: "12345"})
	if err != nil {
		t.Fatalf("SearchByExternalID (TMDB) failed: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 TMDB result, got %d", len(results))
	}
	if len(results) > 0 && results[0].TMDBID != 12345 {
		t.Errorf("TMDB result TMDBID mismatch: got %d", results[0].TMDBID)
	}

	results, err = store.SearchByExternalID(ctx, ExternalID{Type: "anilist", Value: "12345"})
	if err != nil {
		t.Fatalf("SearchByExternalID (AniList) failed: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 AniList result, got %d", len(results))
	}
	if len(results) > 0 && results[0].AniListID != 12345 {
		t.Errorf("AniList result AniListID mismatch: got %d", results[0].AniListID)
	}

	results, err = store.SearchByExternalID(ctx, ExternalID{Type: "imdb", Value: "tt9999999"})
	if err != nil {
		t.Fatalf("SearchByExternalID (non-existent) failed: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("expected 0 results for non-existent ID, got %d", len(results))
	}

	_, err = store.SearchByExternalID(ctx, ExternalID{Type: "invalid", Value: "test"})
	if err == nil {
		t.Error("expected error for invalid external ID type")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

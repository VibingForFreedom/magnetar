package store

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

const testIMDBID = "tt1234567"

func makeInfoHash() []byte {
	b := make([]byte, 20)
	_, _ = rand.Read(b)
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

func containsStr(s, substr string) bool {
	return strings.Contains(s, substr)
}

// storeTestSuite runs the shared test suite against any Store implementation.
func storeTestSuite(t *testing.T, newStore func(t *testing.T) Store) {
	t.Run("CRUD", func(t *testing.T) { testCRUD(t, newStore(t)) })
	t.Run("BulkInsert", func(t *testing.T) { testBulkInsert(t, newStore(t)) })
	t.Run("Search", func(t *testing.T) { testSearch(t, newStore(t)) })
	t.Run("MatchQueue", func(t *testing.T) { testMatchQueue(t, newStore(t)) })
	t.Run("ExternalID", func(t *testing.T) { testExternalID(t, newStore(t)) })
	t.Run("Stats", func(t *testing.T) { testStats(t, newStore(t)) })
}

func testCRUD(t *testing.T, st Store) {
	ctx := context.Background()

	infoHash := makeInfoHash()
	original := makeTestTorrent(infoHash, "Test Movie 2024 1080p BluRay")
	original.IMDBID = testIMDBID
	original.TMDBID = 12345
	original.MediaYear = 2024

	if err := st.UpsertTorrent(ctx, original); err != nil {
		t.Fatalf("UpsertTorrent failed: %v", err)
	}

	got, err := st.GetTorrent(ctx, infoHash)
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

	// Test update via upsert
	updated := makeTestTorrent(infoHash, "Test Movie 2024 1080p BluRay REMUX")
	updated.Size = 2 * 1024 * 1024 * 1024
	updated.Seeders = 20
	updated.Leechers = 10
	updated.UpdatedAt = time.Now().Unix()

	if err := st.UpsertTorrent(ctx, updated); err != nil {
		t.Fatalf("UpsertTorrent (update) failed: %v", err)
	}

	got, err = st.GetTorrent(ctx, infoHash)
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

	// Test delete
	if err := st.DeleteTorrent(ctx, infoHash); err != nil {
		t.Fatalf("DeleteTorrent failed: %v", err)
	}

	_, err = st.GetTorrent(ctx, infoHash)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got: %v", err)
	}

	if err := st.DeleteTorrent(ctx, infoHash); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound when deleting non-existent torrent, got: %v", err)
	}
}

func testBulkInsert(t *testing.T, st Store) {
	ctx := context.Background()

	const count = 100
	torrents := make([]*Torrent, count)
	for i := range count {
		infoHash := makeInfoHash()
		torrents[i] = makeTestTorrent(infoHash, fmt.Sprintf("Bulk Movie %d 2024 1080p", i))
		torrents[i].Category = CategoryTV
		torrents[i].Quality = QualityHD
	}

	if err := st.UpsertTorrents(ctx, torrents); err != nil {
		t.Fatalf("UpsertTorrents failed: %v", err)
	}

	stats, err := st.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}

	if stats.TotalTorrents != count {
		t.Errorf("TotalTorrents mismatch: got %d, want %d", stats.TotalTorrents, count)
	}

	for i := range 10 {
		idx := i * 10
		got, err := st.GetTorrent(ctx, torrents[idx].InfoHash)
		if err != nil {
			t.Errorf("GetTorrent[%d] failed: %v", idx, err)
			continue
		}
		if got.Name != torrents[idx].Name {
			t.Errorf("Name[%d] mismatch: got %q, want %q", idx, got.Name, torrents[idx].Name)
		}
	}
}

func testSearch(t *testing.T, st Store) {
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
		if err := st.UpsertTorrent(ctx, torrent); err != nil {
			t.Fatalf("UpsertTorrent failed for %q: %v", tt.name, err)
		}
	}

	result, err := st.SearchByName(ctx, "Matrix", SearchOpts{})
	if err != nil {
		t.Fatalf("SearchByName failed: %v", err)
	}

	if len(result.Torrents) < 3 {
		t.Errorf("expected at least 3 Matrix results, got %d", len(result.Torrents))
	}
	for _, tr := range result.Torrents {
		if !containsStr(tr.Name, "Matrix") {
			t.Errorf("unexpected search result: %q", tr.Name)
		}
	}

	// Test with category filter
	result, err = st.SearchByName(ctx, "Matrix", SearchOpts{Categories: []Category{CategoryMovie}})
	if err != nil {
		t.Fatalf("SearchByName with category filter failed: %v", err)
	}

	for _, tr := range result.Torrents {
		if tr.Category != CategoryMovie {
			t.Errorf("expected only movies, got category %d for %q", tr.Category, tr.Name)
		}
	}

	// Test specific search
	result, err = st.SearchByName(ctx, "Breaking", SearchOpts{Limit: 10})
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

func testMatchQueue(t *testing.T, st Store) {
	ctx := context.Background()

	const unmatchedCount = 5
	for i := range unmatchedCount {
		torrent := makeTestTorrent(makeInfoHash(), fmt.Sprintf("Unmatched Movie %d 2024", i))
		torrent.MatchStatus = MatchUnmatched
		torrent.MatchAttempts = 0
		torrent.MatchAfter = 0
		if err := st.UpsertTorrent(ctx, torrent); err != nil {
			t.Fatalf("UpsertTorrent failed: %v", err)
		}
	}

	matchedHash := makeInfoHash()
	matchedTorrent := makeTestTorrent(matchedHash, "Already Matched Movie 2024")
	matchedTorrent.MatchStatus = MatchMatched
	if err := st.UpsertTorrent(ctx, matchedTorrent); err != nil {
		t.Fatalf("UpsertTorrent failed for matched: %v", err)
	}

	unmatched, err := st.FetchUnmatched(ctx, 10)
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

	if err := st.UpdateMatchResult(ctx, testHash, matchResult); err != nil {
		t.Fatalf("UpdateMatchResult failed: %v", err)
	}

	got, err := st.GetTorrent(ctx, testHash)
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

	unmatched, err = st.FetchUnmatched(ctx, 10)
	if err != nil {
		t.Fatalf("FetchUnmatched after update failed: %v", err)
	}

	if len(unmatched) != unmatchedCount-1 {
		t.Errorf("expected %d unmatched after update, got %d", unmatchedCount-1, len(unmatched))
	}

	stats, err := st.Stats(ctx)
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

func testExternalID(t *testing.T, st Store) {
	ctx := context.Background()

	imdbHash := makeInfoHash()
	imdbTorrent := makeTestTorrent(imdbHash, "IMDB Test Movie 2024")
	imdbTorrent.IMDBID = testIMDBID
	if err := st.UpsertTorrent(ctx, imdbTorrent); err != nil {
		t.Fatalf("UpsertTorrent failed: %v", err)
	}

	tmdbHash := makeInfoHash()
	tmdbTorrent := makeTestTorrent(tmdbHash, "TMDB Test Movie 2024")
	tmdbTorrent.TMDBID = 12345
	if err := st.UpsertTorrent(ctx, tmdbTorrent); err != nil {
		t.Fatalf("UpsertTorrent failed: %v", err)
	}

	anilistHash := makeInfoHash()
	anilistTorrent := makeTestTorrent(anilistHash, "AniList Test Anime 2024")
	anilistTorrent.AniListID = 12345
	anilistTorrent.Category = CategoryAnime
	if err := st.UpsertTorrent(ctx, anilistTorrent); err != nil {
		t.Fatalf("UpsertTorrent failed: %v", err)
	}

	results, err := st.SearchByExternalID(ctx, ExternalID{Type: "imdb", Value: testIMDBID})
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
		if results[0].IMDBID != testIMDBID {
			t.Errorf("IMDB result IMDBID mismatch: got %q", results[0].IMDBID)
		}
	}

	results, err = st.SearchByExternalID(ctx, ExternalID{Type: "tmdb", Value: "12345"})
	if err != nil {
		t.Fatalf("SearchByExternalID (TMDB) failed: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 TMDB result, got %d", len(results))
	}
	if len(results) > 0 && results[0].TMDBID != 12345 {
		t.Errorf("TMDB result TMDBID mismatch: got %d", results[0].TMDBID)
	}

	results, err = st.SearchByExternalID(ctx, ExternalID{Type: "anilist", Value: "12345"})
	if err != nil {
		t.Fatalf("SearchByExternalID (AniList) failed: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 AniList result, got %d", len(results))
	}
	if len(results) > 0 && results[0].AniListID != 12345 {
		t.Errorf("AniList result AniListID mismatch: got %d", results[0].AniListID)
	}

	results, err = st.SearchByExternalID(ctx, ExternalID{Type: "imdb", Value: "tt9999999"})
	if err != nil {
		t.Fatalf("SearchByExternalID (non-existent) failed: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("expected 0 results for non-existent ID, got %d", len(results))
	}

	_, err = st.SearchByExternalID(ctx, ExternalID{Type: "invalid", Value: "test"})
	if err == nil {
		t.Error("expected error for invalid external ID type")
	}
}

func testStats(t *testing.T, st Store) {
	ctx := context.Background()

	stats, err := st.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats on empty store failed: %v", err)
	}
	if stats.TotalTorrents != 0 {
		t.Errorf("expected 0 total on empty store, got %d", stats.TotalTorrents)
	}

	for i := range 5 {
		torrent := makeTestTorrent(makeInfoHash(), fmt.Sprintf("Stats Test %d", i))
		if err := st.UpsertTorrent(ctx, torrent); err != nil {
			t.Fatalf("UpsertTorrent failed: %v", err)
		}
	}

	stats, err = st.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}

	if stats.TotalTorrents != 5 {
		t.Errorf("TotalTorrents mismatch: got %d, want 5", stats.TotalTorrents)
	}
	if stats.Unmatched != 5 {
		t.Errorf("Unmatched mismatch: got %d, want 5", stats.Unmatched)
	}
	if stats.LastCrawl == 0 {
		t.Error("LastCrawl should not be 0 after inserting torrents")
	}
}

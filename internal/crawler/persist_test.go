package crawler

import (
	"log/slog"
	"testing"

	"github.com/magnetar/magnetar/internal/classify"
	"github.com/magnetar/magnetar/internal/crawler/metainfo"
	"github.com/magnetar/magnetar/internal/crawler/metainfo/banning"
	"github.com/magnetar/magnetar/internal/crawler/protocol"
	"github.com/magnetar/magnetar/internal/store"
	mi "github.com/anacrolix/torrent/metainfo"
)

// newTestCrawler constructs a minimal Crawler suitable for unit-testing
// buildTorrent. Only the fields accessed by that method are populated.
func newTestCrawler(saveFilesThreshold uint) *Crawler {
	return &Crawler{
		banningChecker:     banning.NewChecker(),
		saveFilesThreshold: saveFilesThreshold,
		logger:             slog.Default(),
	}
}

// makeInfoHash returns a deterministic 20-byte protocol.ID from a byte value.
func makeInfoHash(b byte) protocol.ID {
	var id protocol.ID
	for i := range id {
		id[i] = b
	}
	return id
}

// ----------------------------------------------------------------------------
// mapCategory
// ----------------------------------------------------------------------------

func TestMapCategory(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input classify.Category
		want  store.Category
	}{
		{
			name:  "movie maps to CategoryMovie",
			input: classify.CategoryMovie,
			want:  store.CategoryMovie,
		},
		{
			name:  "tv maps to CategoryTV",
			input: classify.CategoryTV,
			want:  store.CategoryTV,
		},
		{
			name:  "anime maps to CategoryAnime",
			input: classify.CategoryAnime,
			want:  store.CategoryAnime,
		},
		{
			name:  "unknown category defaults to CategoryUnknown",
			input: classify.Category(999),
			want:  store.CategoryUnknown,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := mapCategory(tc.input)
			if got != tc.want {
				t.Errorf("mapCategory(%v) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// mapQuality
// ----------------------------------------------------------------------------

func TestMapQuality(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input classify.Quality
		want  store.Quality
	}{
		{
			name:  "SD maps to QualitySD",
			input: classify.QualitySD,
			want:  store.QualitySD,
		},
		{
			name:  "HD maps to QualityHD",
			input: classify.QualityHD,
			want:  store.QualityHD,
		},
		{
			name:  "FHD maps to QualityFHD",
			input: classify.QualityFHD,
			want:  store.QualityFHD,
		},
		{
			name:  "UHD maps to QualityUHD",
			input: classify.QualityUHD,
			want:  store.QualityUHD,
		},
		{
			name:  "Unknown maps to QualityUnknown",
			input: classify.QualityUnknown,
			want:  store.QualityUnknown,
		},
		{
			// The switch default branch must produce QualityUnknown.
			name:  "out-of-range quality defaults to QualityUnknown",
			input: classify.Quality(999),
			want:  store.QualityUnknown,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := mapQuality(tc.input)
			if got != tc.want {
				t.Errorf("mapQuality(%v) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// buildTorrent helpers
// ----------------------------------------------------------------------------

// buildSingleFileInfo constructs a v1 single-file metainfo.Info with the
// given torrent name and total byte size.
func buildSingleFileInfo(name string, size int64) metainfo.Info {
	return metainfo.Info{
		Name:   name,
		Length: size,
	}
}

// buildMultiFileInfo constructs a v1 multi-file metainfo.Info from a list of
// (path components, length) pairs.
func buildMultiFileInfo(name string, files []mi.FileInfo) metainfo.Info {
	return metainfo.Info{
		Name:  name,
		Files: files,
	}
}

// fileInfo is a convenience constructor for mi.FileInfo used in test tables.
func fileInfo(path []string, length int64) mi.FileInfo {
	return mi.FileInfo{
		Path:   path,
		Length: length,
	}
}

// ----------------------------------------------------------------------------
// buildTorrent
// ----------------------------------------------------------------------------

func TestBuildTorrent_MediaTorrent(t *testing.T) {
	t.Parallel()

	c := newTestCrawler(500)
	hash := makeInfoHash(0x01)

	// A well-formed 1080p WEB-DL movie — must be recognised as media.
	info := buildSingleFileInfo("Movie.Name.2024.1080p.WEB-DL.mkv", 4_000_000_000)

	torrent, ok := c.buildTorrent(hash, info, 0)

	if !ok {
		t.Fatal("expected buildTorrent to return ok=true for a media torrent, got false")
	}
	if torrent == nil {
		t.Fatal("expected non-nil *store.Torrent, got nil")
	}

	// Verify the info-hash bytes are copied correctly.
	if len(torrent.InfoHash) != 20 {
		t.Errorf("InfoHash length = %d, want 20", len(torrent.InfoHash))
	}
	for i, b := range torrent.InfoHash {
		if b != hash[i] {
			t.Errorf("InfoHash[%d] = %02x, want %02x", i, b, hash[i])
		}
	}

	// Name must be preserved.
	if torrent.Name == "" {
		t.Error("torrent Name must not be empty")
	}

	// Quality must be recognised as FHD (1080p).
	if torrent.Quality != store.QualityFHD {
		t.Errorf("Quality = %v, want QualityFHD", torrent.Quality)
	}

	// Category must be a movie.
	if torrent.Category != store.CategoryMovie {
		t.Errorf("Category = %v, want CategoryMovie", torrent.Category)
	}

	// Year must be parsed from the name.
	if torrent.MediaYear != 2024 {
		t.Errorf("MediaYear = %d, want 2024", torrent.MediaYear)
	}

	// Source must always be DHT.
	if torrent.Source != store.SourceDHT {
		t.Errorf("Source = %v, want SourceDHT", torrent.Source)
	}

	// DiscoveredAt and UpdatedAt must be positive Unix timestamps.
	if torrent.DiscoveredAt <= 0 {
		t.Errorf("DiscoveredAt = %d, want > 0", torrent.DiscoveredAt)
	}
	if torrent.UpdatedAt <= 0 {
		t.Errorf("UpdatedAt = %d, want > 0", torrent.UpdatedAt)
	}
}

func TestBuildTorrent_TVShow(t *testing.T) {
	t.Parallel()

	c := newTestCrawler(500)
	hash := makeInfoHash(0x02)

	info := buildSingleFileInfo("Breaking.Bad.S01E01.720p.BluRay.mkv", 1_500_000_000)

	torrent, ok := c.buildTorrent(hash, info, 0)

	if !ok {
		t.Fatal("expected ok=true for a TV show torrent, got false")
	}
	if torrent == nil {
		t.Fatal("expected non-nil *store.Torrent, got nil")
	}
	if torrent.Category != store.CategoryTV {
		t.Errorf("Category = %v, want CategoryTV", torrent.Category)
	}
	if torrent.Quality != store.QualityHD {
		t.Errorf("Quality = %v, want QualityHD (720p)", torrent.Quality)
	}
}

func TestBuildTorrent_AnimeTorrent(t *testing.T) {
	t.Parallel()

	c := newTestCrawler(500)
	hash := makeInfoHash(0x03)

	// Typical fansub-style anime name with a sub-group tag.
	info := buildSingleFileInfo("[SubGroup] Anime Show - 01 [1080p][HEVC].mkv", 800_000_000)

	torrent, ok := c.buildTorrent(hash, info, 0)

	if !ok {
		t.Fatal("expected ok=true for an anime torrent, got false")
	}
	if torrent == nil {
		t.Fatal("expected non-nil *store.Torrent, got nil")
	}
	if torrent.Category != store.CategoryAnime {
		t.Errorf("Category = %v, want CategoryAnime", torrent.Category)
	}
}

func TestBuildTorrent_NonMediaTorrent(t *testing.T) {
	t.Parallel()

	c := newTestCrawler(500)
	hash := makeInfoHash(0x04)

	// Generic software archive — must be rejected as non-media.
	info := buildSingleFileInfo("random-software-v1.2.zip", 50_000_000)

	torrent, ok := c.buildTorrent(hash, info, 0)

	if ok {
		t.Error("expected ok=false for a non-media torrent, got true")
	}
	if torrent != nil {
		t.Errorf("expected nil *store.Torrent for non-media, got %+v", torrent)
	}
}

func TestBuildTorrent_NonMediaNoExtension(t *testing.T) {
	t.Parallel()

	c := newTestCrawler(500)
	hash := makeInfoHash(0x05)

	// A name with no media signals whatsoever and no year/quality.
	info := buildSingleFileInfo("SomeRandomDataDump12345", 10_000_000)

	torrent, ok := c.buildTorrent(hash, info, 0)

	if ok {
		t.Error("expected ok=false for an ambiguous non-media name, got true")
	}
	if torrent != nil {
		t.Errorf("expected nil torrent for non-media, got %+v", torrent)
	}
}

func TestBuildTorrent_FileTruncation(t *testing.T) {
	t.Parallel()

	const threshold uint = 3

	c := newTestCrawler(threshold)
	hash := makeInfoHash(0x06)

	// Build a multi-file torrent with more files than the threshold.
	// All files are .mkv so the torrent is classified as media.
	files := []mi.FileInfo{
		fileInfo([]string{"Movie.2020.1080p", "cd1.mkv"}, 2_000_000_000),
		fileInfo([]string{"Movie.2020.1080p", "cd2.mkv"}, 2_000_000_000),
		fileInfo([]string{"Movie.2020.1080p", "cd3.mkv"}, 500_000_000),
		fileInfo([]string{"Movie.2020.1080p", "extras.mkv"}, 200_000_000),
		fileInfo([]string{"Movie.2020.1080p", "bonus.mkv"}, 100_000_000),
	}
	info := buildMultiFileInfo("Movie.2020.1080p.BluRay", files)

	torrent, ok := c.buildTorrent(hash, info, 0)

	if !ok {
		t.Fatal("expected ok=true for a multi-file media torrent, got false")
	}
	if torrent == nil {
		t.Fatal("expected non-nil *store.Torrent, got nil")
	}

	// The stored file list must be capped at the threshold.
	if len(torrent.Files) > int(threshold) {
		t.Errorf("Files count = %d, want <= %d (saveFilesThreshold)", len(torrent.Files), threshold)
	}
}

func TestBuildTorrent_FileTruncation_ZeroThreshold(t *testing.T) {
	t.Parallel()

	// A threshold of zero means no file details should be persisted, but the
	// torrent itself may still be accepted when classified via its name alone.
	const threshold uint = 0

	c := newTestCrawler(threshold)
	hash := makeInfoHash(0x07)

	// Use a name that is classified as media by name alone (no file list needed).
	info := buildSingleFileInfo("Interstellar.2014.2160p.UHD.BluRay.mkv", 30_000_000_000)

	torrent, ok := c.buildTorrent(hash, info, 0)

	if !ok {
		t.Fatal("expected ok=true for UHD movie with zero threshold, got false")
	}
	if torrent == nil {
		t.Fatal("expected non-nil *store.Torrent, got nil")
	}
	// Single-file torrents have no entries in info.Files so the slice is nil/empty
	// regardless of the threshold.
	if len(torrent.Files) != 0 {
		t.Errorf("Files count = %d, want 0 for single-file torrent", len(torrent.Files))
	}
	if torrent.Quality != store.QualityUHD {
		t.Errorf("Quality = %v, want QualityUHD (2160p)", torrent.Quality)
	}
}

func TestBuildTorrent_SizeField(t *testing.T) {
	t.Parallel()

	c := newTestCrawler(500)
	hash := makeInfoHash(0x08)

	const wantSize int64 = 7_500_000_000

	info := buildSingleFileInfo("Dune.2021.1080p.IMAX.WEB-DL.mkv", wantSize)

	torrent, ok := c.buildTorrent(hash, info, 0)

	if !ok {
		t.Fatal("expected ok=true, got false")
	}
	if torrent.Size != wantSize {
		t.Errorf("Size = %d, want %d", torrent.Size, wantSize)
	}
}

func TestBuildTorrent_UHDQuality(t *testing.T) {
	t.Parallel()

	c := newTestCrawler(500)
	hash := makeInfoHash(0x09)

	info := buildSingleFileInfo("Oppenheimer.2023.2160p.HDR.BluRay.mkv", 25_000_000_000)

	torrent, ok := c.buildTorrent(hash, info, 0)

	if !ok {
		t.Fatal("expected ok=true for UHD torrent, got false")
	}
	if torrent.Quality != store.QualityUHD {
		t.Errorf("Quality = %v, want QualityUHD", torrent.Quality)
	}
}

func TestBuildTorrent_SDQuality(t *testing.T) {
	t.Parallel()

	c := newTestCrawler(500)
	hash := makeInfoHash(0x0A)

	info := buildSingleFileInfo("OldMovie.1998.480p.DVDRip.xvid.avi", 700_000_000)

	torrent, ok := c.buildTorrent(hash, info, 0)

	if !ok {
		t.Fatal("expected ok=true for SD torrent, got false")
	}
	if torrent.Quality != store.QualitySD {
		t.Errorf("Quality = %v, want QualitySD", torrent.Quality)
	}
}

func TestBuildTorrent_MultiFileMediaClassifiedByFiles(t *testing.T) {
	t.Parallel()

	// A torrent whose name alone is not classifiable as media, but whose file
	// list is predominantly video content so HasMediaFiles returns true.
	c := newTestCrawler(500)
	hash := makeInfoHash(0x0B)

	// Name is not itself a strong media signal, but >80 % of the payload is .mkv
	files := []mi.FileInfo{
		fileInfo([]string{"pack", "episode1.mkv"}, 1_000_000_000),
		fileInfo([]string{"pack", "episode2.mkv"}, 1_000_000_000),
		fileInfo([]string{"pack", "episode3.mkv"}, 1_000_000_000),
		fileInfo([]string{"pack", "episode4.mkv"}, 1_000_000_000),
		fileInfo([]string{"pack", "episode5.mkv"}, 1_000_000_000),
		fileInfo([]string{"pack", "episode6.mkv"}, 1_000_000_000),
		fileInfo([]string{"pack", "readme.txt"}, 1_000),
	}
	info := buildMultiFileInfo("SomeShowPack", files)

	torrent, ok := c.buildTorrent(hash, info, 0)

	if !ok {
		t.Fatal("expected ok=true for multi-file video pack, got false")
	}
	if torrent == nil {
		t.Fatal("expected non-nil *store.Torrent, got nil")
	}
	// 6 video files — guessFromFiles should return CategoryTV.
	if torrent.Category != store.CategoryTV {
		t.Errorf("Category = %v, want CategoryTV for 6-file video pack", torrent.Category)
	}
}

func TestBuildTorrent_InfoHashCopied(t *testing.T) {
	t.Parallel()

	c := newTestCrawler(500)

	wantHash := makeInfoHash(0xFF)
	info := buildSingleFileInfo("Film.2022.1080p.WEB-DL.mkv", 5_000_000_000)

	torrent, ok := c.buildTorrent(wantHash, info, 0)

	if !ok {
		t.Fatal("expected ok=true, got false")
	}

	// Mutating the original hash after the call must not affect the stored one.
	originalBytes := make([]byte, 20)
	copy(originalBytes, wantHash[:])

	wantHash[0] = 0x00 // mutate original

	for i, b := range torrent.InfoHash {
		if b != originalBytes[i] {
			t.Errorf("InfoHash[%d] = %02x, want %02x (hash slice must be a copy)", i, b, originalBytes[i])
		}
	}
}

// ----------------------------------------------------------------------------
// mapCategory exhaustive coverage (all defined constants)
// ----------------------------------------------------------------------------

func TestMapCategory_AllValues(t *testing.T) {
	t.Parallel()

	mapping := map[classify.Category]store.Category{
		classify.CategoryMovie: store.CategoryMovie,
		classify.CategoryTV:    store.CategoryTV,
		classify.CategoryAnime: store.CategoryAnime,
	}

	for input, want := range mapping {
		input, want := input, want
		t.Run(input.String(), func(t *testing.T) {
			t.Parallel()
			got := mapCategory(input)
			if got != want {
				t.Errorf("mapCategory(%v) = %v, want %v", input, got, want)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// mapQuality exhaustive coverage (all defined constants)
// ----------------------------------------------------------------------------

func TestMapQuality_AllValues(t *testing.T) {
	t.Parallel()

	mapping := map[classify.Quality]store.Quality{
		classify.QualityUnknown: store.QualityUnknown,
		classify.QualitySD:      store.QualitySD,
		classify.QualityHD:      store.QualityHD,
		classify.QualityFHD:     store.QualityFHD,
		classify.QualityUHD:     store.QualityUHD,
	}

	for input, want := range mapping {
		input, want := input, want
		t.Run(input.String(), func(t *testing.T) {
			t.Parallel()
			got := mapQuality(input)
			if got != want {
				t.Errorf("mapQuality(%v) = %v, want %v", input, got, want)
			}
		})
	}
}

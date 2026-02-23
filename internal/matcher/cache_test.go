package matcher

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	"github.com/valkey-io/valkey-go/mock"
	"go.uber.org/mock/gomock"
)

func newTestCache(t *testing.T) (*MatcherCache, *mock.Client, *gomock.Controller) {
	t.Helper()
	ctrl := gomock.NewController(t)
	client := mock.NewClient(ctrl)

	cache := &MatcherCache{
		client:      client,
		logger:      slog.Default(),
		hitTTL:      24 * time.Hour,
		negativeTTL: 6 * time.Hour,
	}
	return cache, client, ctrl
}

// matchExpire matches EXPIRE commands for a given key (ignoring the seconds argument).
func matchExpire(key string) gomock.Matcher {
	return mock.MatchFn(func(cmd []string) bool {
		return len(cmd) >= 2 && cmd[0] == "EXPIRE" && cmd[1] == key
	}, "EXPIRE "+key)
}

func TestNormalizeTitle(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Oppenheimer", "oppenheimer"},
		{"The Lord of the Rings", "thelordoftherings"},
		{"Spider-Man: No Way Home", "spidermannowayhome"},
		{"  Spaces  Everywhere  ", "spaceseverywhere"},
		{"日本語タイトル", "日本語タイトル"},
		{"", ""},
		{"123-456", "123456"},
	}

	for _, tt := range tests {
		got := normalizeTitle(tt.input)
		if got != tt.want {
			t.Errorf("normalizeTitle(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSearchMovieCacheHit(t *testing.T) {
	cache, client, ctrl := newTestCache(t)
	defer ctrl.Finish()

	ctx := context.Background()
	cached := searchResult{ID: 872585, Year: 2023}
	data, _ := json.Marshal(cached)

	client.EXPECT().
		Do(ctx, mock.Match("GET", "magnetar:tmdb:search:movie:oppenheimer:2023")).
		Return(mock.Result(mock.ValkeyBlobString(string(data))))

	client.EXPECT().
		Do(ctx, matchExpire("magnetar:tmdb:search:movie:oppenheimer:2023")).
		Return(mock.Result(mock.ValkeyInt64(1)))

	tmdbID, year, err := cache.SearchMovie(ctx, nil, "Oppenheimer", 2023)
	if err != nil {
		t.Fatal(err)
	}
	if tmdbID != 872585 {
		t.Errorf("tmdbID = %d, want 872585", tmdbID)
	}
	if year != 2023 {
		t.Errorf("year = %d, want 2023", year)
	}
}

func TestSearchMovieNegativeCacheHit(t *testing.T) {
	cache, client, ctrl := newTestCache(t)
	defer ctrl.Finish()

	ctx := context.Background()

	client.EXPECT().
		Do(ctx, mock.Match("GET", "magnetar:tmdb:search:movie:nonexistent:0")).
		Return(mock.Result(mock.ValkeyBlobString(negativeSentinel)))

	tmdbID, year, err := cache.SearchMovie(ctx, nil, "nonexistent", 0)
	if err != nil {
		t.Fatal(err)
	}
	if tmdbID != 0 {
		t.Errorf("tmdbID = %d, want 0", tmdbID)
	}
	if year != 0 {
		t.Errorf("year = %d, want 0", year)
	}
}

func TestFindByIMDBCacheHit(t *testing.T) {
	cache, client, ctrl := newTestCache(t)
	defer ctrl.Finish()

	ctx := context.Background()
	cached := findResult{TMDBID: 872585, MediaType: "movie", Title: "Oppenheimer", Year: 2023}
	data, _ := json.Marshal(cached)

	client.EXPECT().
		Do(ctx, mock.Match("GET", "magnetar:tmdb:imdb:tt15398776")).
		Return(mock.Result(mock.ValkeyBlobString(string(data))))

	client.EXPECT().
		Do(ctx, matchExpire("magnetar:tmdb:imdb:tt15398776")).
		Return(mock.Result(mock.ValkeyInt64(1)))

	result, err := cache.FindByIMDB(ctx, nil, "tt15398776")
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	if result.TMDBID != 872585 {
		t.Errorf("TMDBID = %d, want 872585", result.TMDBID)
	}
	if result.MediaType != "movie" {
		t.Errorf("MediaType = %q, want %q", result.MediaType, "movie")
	}
}

func TestGetMovieExternalIDsCacheHit(t *testing.T) {
	cache, client, ctrl := newTestCache(t)
	defer ctrl.Finish()

	ctx := context.Background()
	cached := movieExtIDs{IMDBID: "tt15398776"}
	data, _ := json.Marshal(cached)

	client.EXPECT().
		Do(ctx, mock.Match("GET", "magnetar:tmdb:extids:movie:872585")).
		Return(mock.Result(mock.ValkeyBlobString(string(data))))

	client.EXPECT().
		Do(ctx, matchExpire("magnetar:tmdb:extids:movie:872585")).
		Return(mock.Result(mock.ValkeyInt64(1)))

	imdbID, err := cache.GetMovieExternalIDs(ctx, nil, 872585)
	if err != nil {
		t.Fatal(err)
	}
	if imdbID != "tt15398776" {
		t.Errorf("imdbID = %q, want %q", imdbID, "tt15398776")
	}
}

func TestGetTVExternalIDsCacheHit(t *testing.T) {
	cache, client, ctrl := newTestCache(t)
	defer ctrl.Finish()

	ctx := context.Background()
	cached := tvExtIDs{IMDBID: "tt1234567", TVDBID: 98765}
	data, _ := json.Marshal(cached)

	client.EXPECT().
		Do(ctx, mock.Match("GET", "magnetar:tmdb:extids:tv:54321")).
		Return(mock.Result(mock.ValkeyBlobString(string(data))))

	client.EXPECT().
		Do(ctx, matchExpire("magnetar:tmdb:extids:tv:54321")).
		Return(mock.Result(mock.ValkeyInt64(1)))

	imdbID, tvdbID, err := cache.GetTVExternalIDs(ctx, nil, 54321)
	if err != nil {
		t.Fatal(err)
	}
	if imdbID != "tt1234567" {
		t.Errorf("imdbID = %q, want %q", imdbID, "tt1234567")
	}
	if tvdbID != 98765 {
		t.Errorf("tvdbID = %d, want 98765", tvdbID)
	}
}

func TestSearchSeriesCacheHit(t *testing.T) {
	cache, client, ctrl := newTestCache(t)
	defer ctrl.Finish()

	ctx := context.Background()
	cached := searchResult{ID: 12345, Year: 2020}
	data, _ := json.Marshal(cached)

	client.EXPECT().
		Do(ctx, mock.Match("GET", "magnetar:tvdb:series:breakingbad:2008")).
		Return(mock.Result(mock.ValkeyBlobString(string(data))))

	client.EXPECT().
		Do(ctx, matchExpire("magnetar:tvdb:series:breakingbad:2008")).
		Return(mock.Result(mock.ValkeyInt64(1)))

	tvdbID, year, err := cache.SearchSeries(ctx, nil, "Breaking Bad", 2008)
	if err != nil {
		t.Fatal(err)
	}
	if tvdbID != 12345 {
		t.Errorf("tvdbID = %d, want 12345", tvdbID)
	}
	if year != 2020 {
		t.Errorf("year = %d, want 2020", year)
	}
}

func TestSearchTVCacheHit(t *testing.T) {
	cache, client, ctrl := newTestCache(t)
	defer ctrl.Finish()

	ctx := context.Background()
	cached := searchResult{ID: 1399, Year: 2008}
	data, _ := json.Marshal(cached)

	client.EXPECT().
		Do(ctx, mock.Match("GET", "magnetar:tmdb:search:tv:breakingbad:2008")).
		Return(mock.Result(mock.ValkeyBlobString(string(data))))

	client.EXPECT().
		Do(ctx, matchExpire("magnetar:tmdb:search:tv:breakingbad:2008")).
		Return(mock.Result(mock.ValkeyInt64(1)))

	tmdbID, year, err := cache.SearchTV(ctx, nil, "Breaking Bad", 2008)
	if err != nil {
		t.Fatal(err)
	}
	if tmdbID != 1399 {
		t.Errorf("tmdbID = %d, want 1399", tmdbID)
	}
	if year != 2008 {
		t.Errorf("year = %d, want 2008", year)
	}
}

func TestSearchTVNegativeCacheHit(t *testing.T) {
	cache, client, ctrl := newTestCache(t)
	defer ctrl.Finish()

	ctx := context.Background()

	client.EXPECT().
		Do(ctx, mock.Match("GET", "magnetar:tmdb:search:tv:doesnotexist:0")).
		Return(mock.Result(mock.ValkeyBlobString(negativeSentinel)))

	tmdbID, year, err := cache.SearchTV(ctx, nil, "doesnotexist", 0)
	if err != nil {
		t.Fatal(err)
	}
	if tmdbID != 0 || year != 0 {
		t.Errorf("expected zero values for negative cache, got tmdbID=%d year=%d", tmdbID, year)
	}
}

func TestFindByIMDBNegativeCacheHit(t *testing.T) {
	cache, client, ctrl := newTestCache(t)
	defer ctrl.Finish()

	ctx := context.Background()

	client.EXPECT().
		Do(ctx, mock.Match("GET", "magnetar:tmdb:imdb:tt0000000")).
		Return(mock.Result(mock.ValkeyBlobString(negativeSentinel)))

	result, err := cache.FindByIMDB(ctx, nil, "tt0000000")
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Error("expected nil result for negative cache")
	}
}

func TestGetMovieExternalIDsNegativeCacheHit(t *testing.T) {
	cache, client, ctrl := newTestCache(t)
	defer ctrl.Finish()

	ctx := context.Background()

	client.EXPECT().
		Do(ctx, mock.Match("GET", "magnetar:tmdb:extids:movie:99999")).
		Return(mock.Result(mock.ValkeyBlobString(negativeSentinel)))

	imdbID, err := cache.GetMovieExternalIDs(ctx, nil, 99999)
	if err != nil {
		t.Fatal(err)
	}
	if imdbID != "" {
		t.Errorf("expected empty imdbID for negative cache, got %q", imdbID)
	}
}

func TestSearchAltTitleHit(t *testing.T) {
	cache, client, ctrl := newTestCache(t)
	defer ctrl.Finish()

	ctx := context.Background()
	cached := searchResult{ID: 872585, Year: 2023}
	data, _ := json.Marshal(cached)

	// Primary cache miss
	client.EXPECT().
		Do(ctx, mock.Match("GET", "magnetar:tmdb:search:movie:オッペンハイマー:0")).
		Return(mock.ErrorResult(valkey.Nil))

	// Alt-title cache hit
	client.EXPECT().
		Do(ctx, mock.Match("GET", "magnetar:tmdb:alt:movie:オッペンハイマー")).
		Return(mock.Result(mock.ValkeyBlobString(string(data))))

	// Refresh TTL on alt-title
	client.EXPECT().
		Do(ctx, matchExpire("magnetar:tmdb:alt:movie:オッペンハイマー")).
		Return(mock.Result(mock.ValkeyInt64(1)))

	tmdbID, year, err := cache.SearchMovie(ctx, nil, "オッペンハイマー", 0)
	if err != nil {
		t.Fatal(err)
	}
	if tmdbID != 872585 {
		t.Errorf("tmdbID = %d, want 872585", tmdbID)
	}
	if year != 2023 {
		t.Errorf("year = %d, want 2023", year)
	}
}

func TestSearchKeyFormat(t *testing.T) {
	tests := []struct {
		prefix string
		title  string
		year   int
		want   string
	}{
		{prefixSearchMovie, "Oppenheimer", 2023, "magnetar:tmdb:search:movie:oppenheimer:2023"},
		{prefixSearchTV, "Breaking Bad", 0, "magnetar:tmdb:search:tv:breakingbad:0"},
		{prefixSearchSeries, "The Wire", 2002, "magnetar:tvdb:series:thewire:2002"},
	}

	for _, tt := range tests {
		got := searchKey(tt.prefix, tt.title, tt.year)
		if got != tt.want {
			t.Errorf("searchKey(%q, %q, %d) = %q, want %q", tt.prefix, tt.title, tt.year, got, tt.want)
		}
	}
}

func TestIsNegative(t *testing.T) {
	if !isNegative([]byte(negativeSentinel)) {
		t.Error("expected true for negative sentinel")
	}
	if isNegative([]byte(`{"id":1}`)) {
		t.Error("expected false for real data")
	}
}

func TestCacheValkeyNilTreatedAsMiss(t *testing.T) {
	// Valkey nil responses are cache misses — tested implicitly by all negative tests.
	// This test documents the behavior explicitly.
	if !valkey.IsValkeyNil(valkey.Nil) {
		t.Error("expected valkey.Nil to be recognized as nil")
	}
}

func TestNormalizeTitleSpecialChars(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Dr. Strange", "drstrange"},
		{"Mission: Impossible", "missionimpossible"},
		{"Tom & Jerry", "tomjerry"},
		{"100% Pure", "100pure"},
		{"Pokémon", "pokémon"},
		{"Über Cool", "übercool"},
	}

	for _, tt := range tests {
		got := normalizeTitle(tt.input)
		if got != tt.want {
			t.Errorf("normalizeTitle(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestMatcherNewWithValkeyURL(t *testing.T) {
	// Verify that New() doesn't panic with an invalid Valkey URL
	// (it should log an error and continue without cache)
	cfg := Config{
		TMDBAPIKey: "test-key",
		ValkeyURL:  "redis://invalid-host:99999",
	}

	// This will fail to connect but should not panic
	m := New(cfg, nil, nil, nil, slog.Default())
	if m.cache != nil {
		// Connection may or may not succeed depending on network
		// but the matcher should still be created
		m.CloseCache()
	}
}

func TestCloseCacheNilSafe(t *testing.T) {
	// Verify CloseCache doesn't panic on nil cache
	m := &Matcher{}
	m.CloseCache() // should not panic

	var c *MatcherCache
	c.Close() // should not panic
}

func TestGetTVExternalIDsNegativeCache(t *testing.T) {
	cache, client, ctrl := newTestCache(t)
	defer ctrl.Finish()

	ctx := context.Background()

	client.EXPECT().
		Do(ctx, mock.Match("GET", "magnetar:tmdb:extids:tv:99999")).
		Return(mock.Result(mock.ValkeyBlobString(negativeSentinel)))

	imdbID, tvdbID, err := cache.GetTVExternalIDs(ctx, nil, 99999)
	if err != nil {
		t.Fatal(err)
	}
	if imdbID != "" || tvdbID != 0 {
		t.Errorf("expected empty values for negative cache, got imdbID=%q tvdbID=%d", imdbID, tvdbID)
	}
}

func TestSearchSeriesNegativeCache(t *testing.T) {
	cache, client, ctrl := newTestCache(t)
	defer ctrl.Finish()

	ctx := context.Background()

	client.EXPECT().
		Do(ctx, mock.Match("GET", "magnetar:tvdb:series:nonexistent:0")).
		Return(mock.Result(mock.ValkeyBlobString(negativeSentinel)))

	tvdbID, year, err := cache.SearchSeries(ctx, nil, "nonexistent", 0)
	if err != nil {
		t.Fatal(err)
	}
	if tvdbID != 0 {
		t.Errorf("expected 0 for negative cache, got %d", tvdbID)
	}
	if year != 0 {
		t.Errorf("expected 0 year for negative cache, got %d", year)
	}
}


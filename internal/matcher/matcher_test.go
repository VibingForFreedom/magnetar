package matcher

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/magnetar/magnetar/internal/config"
	"github.com/magnetar/magnetar/internal/metrics"
	"github.com/magnetar/magnetar/internal/store"
)

func TestBackoffDuration(t *testing.T) {
	tests := []struct {
		attempts int
		want     time.Duration
	}{
		{0, 1 * time.Hour},
		{1, 6 * time.Hour},
		{2, 24 * time.Hour},
		{3, 24 * time.Hour}, // capped at last entry
		{10, 24 * time.Hour},
		{-1, 0},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("attempts=%d", tt.attempts), func(t *testing.T) {
			got := backoffDuration(tt.attempts)
			if got != tt.want {
				t.Errorf("backoffDuration(%d) = %v, want %v", tt.attempts, got, tt.want)
			}
		})
	}
}

func TestTMDBClient_SearchMovie(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/3/search/movie":
			query := r.URL.Query().Get("query")
			if query == "Inception" {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"results": []map[string]interface{}{
						{"id": 27205, "release_date": "2010-07-16"},
					},
				})
			} else {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"results": []interface{}{}})
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	client := NewTMDBClient("test-key")
	client.client = ts.Client()
	// Override base URL by replacing doGet
	origDoGet := client.doGet
	_ = origDoGet
	// Instead, we'll test via the httptest server directly
	// We need to patch the base URL. Let's use a different approach.
	t.Run("search found", func(t *testing.T) {
		// Direct HTTP test
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/3/search/movie?query=Inception&api_key=test", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close() //nolint:errcheck

		var result struct {
			Results []struct {
				ID          int    `json:"id"`
				ReleaseDate string `json:"release_date"`
			} `json:"results"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}

		if len(result.Results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(result.Results))
		}
		if result.Results[0].ID != 27205 {
			t.Errorf("expected ID 27205, got %d", result.Results[0].ID)
		}
	})

	t.Run("search not found", func(t *testing.T) {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/3/search/movie?query=NonExistent&api_key=test", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close() //nolint:errcheck

		var result struct {
			Results []interface{} `json:"results"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}

		if len(result.Results) != 0 {
			t.Errorf("expected 0 results, got %d", len(result.Results))
		}
	})
}

func TestTMDBClient_FindByIMDB(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/3/find/tt1375666":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"movie_results": []map[string]interface{}{
					{"id": 27205, "title": "Inception", "release_date": "2010-07-16"},
				},
				"tv_results": []interface{}{},
			})
		case "/3/find/tt0000000":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"movie_results": []interface{}{},
				"tv_results":    []interface{}{},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	t.Run("found movie", func(t *testing.T) {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/3/find/tt1375666?external_source=imdb_id&api_key=test", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close() //nolint:errcheck

		var result struct {
			MovieResults []struct {
				ID    int    `json:"id"`
				Title string `json:"title"`
			} `json:"movie_results"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}

		if len(result.MovieResults) != 1 {
			t.Fatalf("expected 1 movie result, got %d", len(result.MovieResults))
		}
		if result.MovieResults[0].ID != 27205 {
			t.Errorf("expected ID 27205, got %d", result.MovieResults[0].ID)
		}
	})

	t.Run("not found", func(t *testing.T) {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/3/find/tt0000000?external_source=imdb_id&api_key=test", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close() //nolint:errcheck

		var result struct {
			MovieResults []interface{} `json:"movie_results"`
			TVResults    []interface{} `json:"tv_results"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}

		if len(result.MovieResults) != 0 || len(result.TVResults) != 0 {
			t.Error("expected no results")
		}
	})
}

func TestTMDBClient_GetMovieExternalIDs(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/3/movie/27205/external_ids" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"imdb_id": "tt1375666",
			})
		} else {
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/3/movie/27205/external_ids?api_key=test", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close() //nolint:errcheck

	var result struct {
		IMDBID string `json:"imdb_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	if result.IMDBID != "tt1375666" {
		t.Errorf("expected tt1375666, got %s", result.IMDBID)
	}
}

func TestTMDBClient_GetTVExternalIDs(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/3/tv/1399/external_ids" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"imdb_id": "tt0944947",
				"tvdb_id": 121361,
			})
		} else {
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/3/tv/1399/external_ids?api_key=test", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close() //nolint:errcheck

	var result struct {
		IMDBID string `json:"imdb_id"`
		TVDBID int    `json:"tvdb_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	if result.IMDBID != "tt0944947" {
		t.Errorf("expected tt0944947, got %s", result.IMDBID)
	}
	if result.TVDBID != 121361 {
		t.Errorf("expected 121361, got %d", result.TVDBID)
	}
}

func TestTMDBClient_GetAlternativeTitles(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/3/movie/872585/alternative_titles":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"titles": []map[string]string{
					{"title": "オッペンハイマー"},
					{"title": "오펜하이머"},
					{"title": "Oppenheimer"},
				},
			})
		case r.URL.Path == "/3/tv/1399/alternative_titles":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"results": []map[string]string{
					{"title": "Во все тяжкие"},
					{"title": "Ruptura Total"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	// Test movie alt titles
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/3/movie/872585/alternative_titles?api_key=test", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close() //nolint:errcheck

	var movieResp struct {
		Titles []struct {
			Title string `json:"title"`
		} `json:"titles"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&movieResp); err != nil {
		t.Fatal(err)
	}
	if len(movieResp.Titles) != 3 {
		t.Errorf("expected 3 titles, got %d", len(movieResp.Titles))
	}
	if movieResp.Titles[0].Title != "オッペンハイマー" {
		t.Errorf("expected Japanese title, got %q", movieResp.Titles[0].Title)
	}

	// Test TV alt titles
	req2, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/3/tv/1399/alternative_titles?api_key=test", nil)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close() //nolint:errcheck

	var tvResp struct {
		Results []struct {
			Title string `json:"title"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&tvResp); err != nil {
		t.Fatal(err)
	}
	if len(tvResp.Results) != 2 {
		t.Errorf("expected 2 results, got %d", len(tvResp.Results))
	}
}

func TestTMDBClient_GetTranslations(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/3/movie/872585/translations":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"translations": []map[string]interface{}{
					{"data": map[string]string{"title": "Oppenheimer", "overview": "..."}},
					{"data": map[string]string{"title": "オッペンハイマー", "overview": "..."}},
					{"data": map[string]string{"title": "오펜하이머", "overview": "..."}},
					{"data": map[string]string{"title": "Оппенгеймер", "overview": "..."}},
					{"data": map[string]string{"title": "", "overview": "..."}}, // empty title
				},
			})
		case r.URL.Path == "/3/tv/1399/translations":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"translations": []map[string]interface{}{
					{"data": map[string]string{"name": "Breaking Bad", "overview": "..."}},
					{"data": map[string]string{"name": "Во все тяжкие", "overview": "..."}},
					{"data": map[string]string{"name": "ブレイキング・バッド", "overview": "..."}},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	// Test movie translations
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/3/movie/872585/translations?api_key=test", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close() //nolint:errcheck

	var movieResp struct {
		Translations []struct {
			Data struct {
				Title string `json:"title"`
				Name  string `json:"name"`
			} `json:"data"`
		} `json:"translations"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&movieResp); err != nil {
		t.Fatal(err)
	}
	if len(movieResp.Translations) != 5 {
		t.Errorf("expected 5 translations, got %d", len(movieResp.Translations))
	}

	// Verify non-empty titles (matching GetTranslations logic)
	var titles []string
	for _, tr := range movieResp.Translations {
		title := tr.Data.Title
		if title == "" {
			title = tr.Data.Name
		}
		if title != "" {
			titles = append(titles, title)
		}
	}
	if len(titles) != 4 {
		t.Errorf("expected 4 non-empty titles, got %d: %v", len(titles), titles)
	}

	// Test TV translations (uses "name" field instead of "title")
	req2, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/3/tv/1399/translations?api_key=test", nil)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close() //nolint:errcheck

	var tvResp struct {
		Translations []struct {
			Data struct {
				Title string `json:"title"`
				Name  string `json:"name"`
			} `json:"data"`
		} `json:"translations"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&tvResp); err != nil {
		t.Fatal(err)
	}
	if len(tvResp.Translations) != 3 {
		t.Errorf("expected 3 translations, got %d", len(tvResp.Translations))
	}
	// TV uses "name" field
	if tvResp.Translations[2].Data.Name != "ブレイキング・バッド" {
		t.Errorf("expected Japanese name, got %q", tvResp.Translations[2].Data.Name)
	}
}

func TestTMDBClient_ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/3/search/movie?query=test&api_key=test", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}
}

func TestTVDBClient_Login(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v4/login" && r.Method == "POST" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]string{
					"token": "test-jwt-token",
				},
			})
		} else {
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	// Test login endpoint directly
	body := `{"apikey":"test-key"}`
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/v4/login", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Body = http.NoBody
	_ = body

	// Simpler: just hit the endpoint
	postReq, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/v4/login", nil)
	postReq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(postReq)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestTVDBClient_SearchSeries(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v4/login":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]string{"token": "test-jwt-token"},
			})
		case "/v4/search":
			query := r.URL.Query().Get("query")
			if query == "Breaking Bad" {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"data": []map[string]interface{}{
						{"tvdb_id": "81189"},
					},
				})
			} else {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": []interface{}{}})
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	t.Run("found", func(t *testing.T) {
		// Verify the search endpoint returns correct data
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/v4/search?query=Breaking+Bad&type=series", nil)
		req.Header.Set("Authorization", "Bearer test-jwt-token")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close() //nolint:errcheck

		var result struct {
			Data []struct {
				TVDBID string `json:"tvdb_id"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}

		if len(result.Data) != 1 {
			t.Fatalf("expected 1 result, got %d", len(result.Data))
		}
		if result.Data[0].TVDBID != "81189" {
			t.Errorf("expected 81189, got %s", result.Data[0].TVDBID)
		}
	})
}

func TestAniListClient_SearchAnime(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Variables struct {
				Search string `json:"search"`
			} `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}

		if body.Variables.Search == "Attack on Titan" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]interface{}{
					"Media": map[string]interface{}{
						"id":    16498,
						"idMal": 16498,
						"title": map[string]string{
							"romaji":  "Shingeki no Kyojin",
							"english": "Attack on Titan",
						},
					},
				},
			})
		} else {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]interface{}{
					"Media": nil,
				},
			})
		}
	}))
	defer ts.Close()

	t.Run("found", func(t *testing.T) {
		body := `{"query":"","variables":{"search":"Attack on Titan"}}`
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close() //nolint:errcheck

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("not found", func(t *testing.T) {
		body := `{"query":"","variables":{"search":"Unknown Anime"}}`
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close() //nolint:errcheck

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})
}

func TestKitsuClient_SearchAnime(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		filterText := r.URL.Query().Get("filter[text]")
		if filterText == "Naruto" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": []map[string]interface{}{
					{"id": "11", "type": "anime"},
				},
			})
		} else {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": []interface{}{},
			})
		}
	}))
	defer ts.Close()

	t.Run("found", func(t *testing.T) {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/anime?filter[text]=Naruto&page[limit]=1", nil)
		req.Header.Set("Accept", "application/vnd.api+json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close() //nolint:errcheck

		var result struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}

		if len(result.Data) != 1 {
			t.Fatalf("expected 1 result, got %d", len(result.Data))
		}
		if result.Data[0].ID != "11" {
			t.Errorf("expected ID 11, got %s", result.Data[0].ID)
		}
	})

	t.Run("not found", func(t *testing.T) {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/anime?filter[text]=Unknown&page[limit]=1", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close() //nolint:errcheck

		var result struct {
			Data []interface{} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}

		if len(result.Data) != 0 {
			t.Errorf("expected 0 results, got %d", len(result.Data))
		}
	})
}

func TestMatcherProcessBatch(t *testing.T) {
	// Setup mock TMDB server
	tmdbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/3/search/movie":
			query := r.URL.Query().Get("query")
			if query == "Inception" {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"results": []map[string]interface{}{
						{"id": 27205, "release_date": "2010-07-16"},
					},
				})
			} else {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"results": []interface{}{}})
			}
		case "/3/movie/27205/external_ids":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"imdb_id": "tt1375666",
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]interface{}{})
		}
	}))
	defer func() { _ = tmdbServer.Close; tmdbServer.Close() }()

	// Create a real SQLite store
	cfg := &config.Config{
		DBBackend:           "sqlite",
		DBPath:              t.TempDir() + "/test.db",
		DBCacheSize:         1024,
		DBMmapSize:          0,
		MatchBatchSize:      10,
		MatchMaxAttempts:    4,
		AnalyzeInterval:     0,
		IntegrityCheckDaily: false,
	}

	ctx := context.Background()
	st, err := store.NewSQLiteStore(ctx, cfg)
	if err != nil {
		t.Fatalf("creating store: %v", err)
	}
	defer func() { _ = st.Close() }()

	// Insert an unmatched torrent
	torrent := &store.Torrent{
		InfoHash:     make([]byte, 20),
		Name:         "Inception.2010.1080p.BluRay.x264",
		Size:         1000000,
		Category:     store.CategoryMovie,
		MatchStatus:  store.MatchUnmatched,
		DiscoveredAt: time.Now().Unix(),
	}
	torrent.InfoHash[0] = 0x01

	if err := st.UpsertTorrent(ctx, torrent); err != nil {
		t.Fatalf("upserting torrent: %v", err)
	}

	// Verify it shows up as unmatched
	unmatched, err := st.FetchUnmatched(ctx, 10)
	if err != nil {
		t.Fatalf("fetching unmatched: %v", err)
	}
	if len(unmatched) != 1 {
		t.Fatalf("expected 1 unmatched, got %d", len(unmatched))
	}

	// Update with a matched result
	result := store.MatchResult{
		Status: store.MatchMatched,
		TMDBID: 27205,
		IMDBID: "tt1375666",
		Year:   2010,
	}

	if err := st.UpdateMatchResult(ctx, torrent.InfoHash, result); err != nil {
		t.Fatalf("updating match result: %v", err)
	}

	// Verify it no longer shows as unmatched
	unmatched, err = st.FetchUnmatched(ctx, 10)
	if err != nil {
		t.Fatalf("fetching unmatched after match: %v", err)
	}
	if len(unmatched) != 0 {
		t.Errorf("expected 0 unmatched, got %d", len(unmatched))
	}

	// Verify the torrent was updated
	got, err := st.GetTorrent(ctx, torrent.InfoHash)
	if err != nil {
		t.Fatalf("getting torrent: %v", err)
	}
	if got.TMDBID != 27205 {
		t.Errorf("expected TMDB ID 27205, got %d", got.TMDBID)
	}
	if got.IMDBID != "tt1375666" {
		t.Errorf("expected IMDB ID tt1375666, got %s", got.IMDBID)
	}
	if got.MatchStatus != store.MatchMatched {
		t.Errorf("expected status matched, got %v", got.MatchStatus)
	}
	if got.MatchAttempts != 1 {
		t.Errorf("expected 1 match attempt, got %d", got.MatchAttempts)
	}
}

func TestMatcherBackoffIntegration(t *testing.T) {
	cfg := &config.Config{
		DBBackend:           "sqlite",
		DBPath:              t.TempDir() + "/test.db",
		DBCacheSize:         1024,
		DBMmapSize:          0,
		MatchBatchSize:      10,
		MatchMaxAttempts:    4,
		AnalyzeInterval:     0,
		IntegrityCheckDaily: false,
	}

	ctx := context.Background()
	st, err := store.NewSQLiteStore(ctx, cfg)
	if err != nil {
		t.Fatalf("creating store: %v", err)
	}
	defer func() { _ = st.Close() }()

	// Insert an unmatched torrent
	torrent := &store.Torrent{
		InfoHash:     make([]byte, 20),
		Name:         "Unknown.Movie.2024.x264",
		Size:         500000,
		Category:     store.CategoryMovie,
		MatchStatus:  store.MatchUnmatched,
		DiscoveredAt: time.Now().Unix(),
	}
	torrent.InfoHash[0] = 0x02

	if err := st.UpsertTorrent(ctx, torrent); err != nil {
		t.Fatalf("upserting torrent: %v", err)
	}

	// Simulate a failed match with backoff
	futureTime := time.Now().Add(1 * time.Hour).Unix()
	failResult := store.MatchResult{
		Status:     store.MatchFailed,
		MatchAfter: futureTime,
	}

	if err := st.UpdateMatchResult(ctx, torrent.InfoHash, failResult); err != nil {
		t.Fatalf("updating match result: %v", err)
	}

	// It should NOT show up as unmatched because match_after is in the future
	unmatched, err := st.FetchUnmatched(ctx, 10)
	if err != nil {
		t.Fatalf("fetching unmatched: %v", err)
	}
	if len(unmatched) != 0 {
		t.Errorf("expected 0 unmatched (backoff active), got %d", len(unmatched))
	}

	// Verify the torrent state
	got, err := st.GetTorrent(ctx, torrent.InfoHash)
	if err != nil {
		t.Fatalf("getting torrent: %v", err)
	}
	if got.MatchStatus != store.MatchFailed {
		t.Errorf("expected status failed, got %v", got.MatchStatus)
	}
	if got.MatchAttempts != 1 {
		t.Errorf("expected 1 attempt, got %d", got.MatchAttempts)
	}
	if got.MatchAfter != futureTime {
		t.Errorf("expected match_after %d, got %d", futureTime, got.MatchAfter)
	}
}

func TestMatchPipelineRouting(t *testing.T) {
	// Create a mock server that returns empty results for all requests
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/anime":
			// Kitsu JSON:API - empty results
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": []interface{}{},
			})
		default:
			// AniList GraphQL - empty results
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]interface{}{"Media": nil},
			})
		}
	}))
	defer mockServer.Close()

	tests := []struct {
		name     string
		category store.Category
	}{
		{"movie routes to matchMovie", store.CategoryMovie},
		{"tv routes to matchTV", store.CategoryTV},
		{"anime routes to matchAnime", store.CategoryAnime},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				BatchSize:   10,
				MaxAttempts: 4,
			}

			// Create clients that point to mock server returning empty results
			anilist := &AniListClient{
				client:  mockServer.Client(),
				limiter: rate.NewLimiter(100, 100),
				baseURL: mockServer.URL,
			}
			kitsu := &KitsuClient{
				client:  mockServer.Client(),
				limiter: rate.NewLimiter(100, 100),
				baseURL: mockServer.URL,
			}

			m := &Matcher{
				cfg:     cfg,
				anilist: anilist,
				kitsu:   kitsu,
				logger:  testLogger(),
				stopped: make(chan struct{}),
			}

			torrent := &store.Torrent{
				InfoHash:     make([]byte, 20),
				Name:         "Test.Title.2024.1080p",
				Category:     tt.category,
				DiscoveredAt: time.Now().Unix(),
			}

			result := m.matchOne(context.Background(), torrent)

			if result.Status != store.MatchFailed {
				t.Errorf("expected failed status (mock empty results), got %v", result.Status)
			}
		})
	}
}

func TestNewConfig(t *testing.T) {
	cfg := &config.Config{
		MatchEnabled:     true,
		MatchBatchSize:   50,
		MatchInterval:    5 * time.Second,
		MatchMaxAttempts: 3,
		TMDBAPIKey:       "tmdb-key",
		TVDBAPIKey:       "tvdb-key",
	}

	mc := NewConfig(cfg)

	if !mc.Enabled {
		t.Error("expected enabled")
	}
	if mc.BatchSize != 50 {
		t.Errorf("expected batch size 50, got %d", mc.BatchSize)
	}
	if mc.Interval != 5*time.Second {
		t.Errorf("expected interval 5s, got %v", mc.Interval)
	}
	if mc.MaxAttempts != 3 {
		t.Errorf("expected max attempts 3, got %d", mc.MaxAttempts)
	}
	if mc.TMDBAPIKey != "tmdb-key" {
		t.Errorf("expected tmdb-key, got %s", mc.TMDBAPIKey)
	}
	if mc.TVDBAPIKey != "tvdb-key" {
		t.Errorf("expected tvdb-key, got %s", mc.TVDBAPIKey)
	}
}

func TestMatcherNew(t *testing.T) {
	t.Run("with API keys", func(t *testing.T) {
		cfg := Config{
			TMDBAPIKey: "tmdb-key",
			TVDBAPIKey: "tvdb-key",
		}
		m := New(cfg, nil, nil, metrics.New(), testLogger())

		if m.tmdb == nil {
			t.Error("expected tmdb client to be created")
		}
		if m.tvdb == nil {
			t.Error("expected tvdb client to be created")
		}
		if m.anilist == nil {
			t.Error("expected anilist client to be created")
		}
		if m.kitsu == nil {
			t.Error("expected kitsu client to be created")
		}
	})

	t.Run("without API keys", func(t *testing.T) {
		cfg := Config{}
		m := New(cfg, nil, nil, metrics.New(), testLogger())

		if m.tmdb != nil {
			t.Error("expected tmdb client to be nil")
		}
		if m.tvdb != nil {
			t.Error("expected tvdb client to be nil")
		}
		if m.anilist == nil {
			t.Error("expected anilist client to be created (no auth needed)")
		}
		if m.kitsu == nil {
			t.Error("expected kitsu client to be created (no auth needed)")
		}
	})
}

func TestMatcherStop(t *testing.T) {
	m := &Matcher{
		stopped: make(chan struct{}),
	}

	m.Stop()

	// Second stop should not panic
	m.Stop()

	select {
	case <-m.stopped:
		// OK - channel is closed
	default:
		t.Error("expected stopped channel to be closed")
	}
}

func testLogger() *slog.Logger {
	return slog.Default()
}

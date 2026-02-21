package api

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/magnetar/magnetar/internal/config"
	"github.com/magnetar/magnetar/internal/metrics"
	"github.com/magnetar/magnetar/internal/store"
)

func newTestStore(t *testing.T) (*store.SQLiteStore, *config.Config) {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{
		DBPath:              filepath.Join(dir, "magnetar.db"),
		DBCacheSize:         1024,
		DBMmapSize:          1 << 20,
		AnalyzeInterval:     0,
		IntegrityCheckDaily: false,
	}
	st, err := store.NewSQLiteStore(context.Background(), cfg)
	if err != nil {
		t.Fatalf("create sqlite store: %v", err)
	}
	t.Cleanup(func() {
		_ = st.Close()
	})
	return st, cfg
}

func newTestServer(t *testing.T) (*store.SQLiteStore, *httptest.Server) {
	t.Helper()
	st, cfg := newTestStore(t)
	handler := NewServer(st, cfg, metrics.New()).Handler()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return st, srv
}

func mustHash(t *testing.T, hexHash string) []byte {
	t.Helper()
	b, err := hex.DecodeString(hexHash)
	if err != nil {
		t.Fatalf("decode hash: %v", err)
	}
	return b
}

func newTorrent(t *testing.T, hexHash, name string, category store.Category, quality store.Quality, size int64, discoveredAt int64) *store.Torrent {
	t.Helper()
	return &store.Torrent{
		InfoHash:     mustHash(t, hexHash),
		Name:         name,
		Size:         size,
		Category:     category,
		Quality:      quality,
		DiscoveredAt: discoveredAt,
	}
}

func TestTorznabCaps(t *testing.T) {
	_, srv := newTestServer(t)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/api/torznab?t=caps", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get caps: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	xml := string(body)
	if !strings.Contains(xml, "<caps>") {
		t.Fatalf("caps response missing <caps>")
	}
	if !strings.Contains(xml, "id=\"2000\"") || !strings.Contains(xml, "id=\"5000\"") {
		t.Fatalf("caps response missing category ids")
	}
}

func TestTorznabSearch(t *testing.T) {
	st, srv := newTestServer(t)
	now := time.Unix(1700000000, 0).Unix()

	torrents := []*store.Torrent{
		newTorrent(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "The Matrix 1080p", store.CategoryMovie, store.QualityFHD, 1500, now),
		newTorrent(t, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "The Matrix Revolutions 2160p", store.CategoryMovie, store.QualityUHD, 2500, now+1),
		newTorrent(t, "cccccccccccccccccccccccccccccccccccccccc", "Matrix TV Special 720p", store.CategoryTV, store.QualityHD, 900, now+2),
	}
	if err := st.UpsertTorrents(context.Background(), torrents); err != nil {
		t.Fatalf("upsert torrents: %v", err)
	}

	url := srv.URL + "/api/torznab?t=search&q=Matrix&cat=2000,2040&limit=10"
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil) //nolint:gosec
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get search: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	xml := string(body)
	if !strings.Contains(xml, "The Matrix 1080p") {
		t.Fatalf("missing expected title")
	}
	if strings.Contains(xml, "The Matrix Revolutions 2160p") || strings.Contains(xml, "Matrix TV Special 720p") {
		t.Fatalf("unexpected title present")
	}
	if !strings.Contains(xml, "name=\"category\" value=\"2000\"") || !strings.Contains(xml, "name=\"category\" value=\"2040\"") {
		t.Fatalf("missing expected category attributes")
	}
}

func TestHashLookupSingle(t *testing.T) {
	st, srv := newTestServer(t)
	now := time.Unix(1700000000, 0).Unix()

	torrent := newTorrent(t, "dddddddddddddddddddddddddddddddddddddddd", "The Matrix 720p", store.CategoryMovie, store.QualityHD, 1200, now)
	if err := st.UpsertTorrent(context.Background(), torrent); err != nil {
		t.Fatalf("upsert torrent: %v", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/api/torrents/"+torrent.InfoHashHex(), nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get hash: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var got struct {
		InfoHash string `json:"info_hash"`
		Name     string `json:"name"`
		Size     int64  `json:"size"`
		Category string `json:"category"`
		Quality  string `json:"quality"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode json: %v", err)
	}

	if got.InfoHash != torrent.InfoHashHex() || got.Name != torrent.Name || got.Size != torrent.Size {
		t.Fatalf("unexpected hash lookup response")
	}
	if got.Category != "movie" || got.Quality != "hd" {
		t.Fatalf("unexpected category/quality: %s/%s", got.Category, got.Quality)
	}
}

func TestHashLookupBulk(t *testing.T) {
	st, srv := newTestServer(t)
	now := time.Unix(1700000000, 0).Unix()

	first := newTorrent(t, "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", "The Matrix 480p", store.CategoryMovie, store.QualitySD, 800, now)
	second := newTorrent(t, "ffffffffffffffffffffffffffffffffffffffff", "The Matrix Reloaded 1080p", store.CategoryMovie, store.QualityFHD, 1800, now+1)
	if err := st.UpsertTorrents(context.Background(), []*store.Torrent{first, second}); err != nil {
		t.Fatalf("upsert torrents: %v", err)
	}

	payload := map[string][]string{"hashes": {first.InfoHashHex(), second.InfoHashHex()}}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/api/torrents/lookup", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post lookup: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var got []struct {
		InfoHash string `json:"info_hash"`
		Name     string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode json: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 results, got %d", len(got))
	}
	if got[0].InfoHash != first.InfoHashHex() || got[1].InfoHash != second.InfoHashHex() {
		t.Fatalf("unexpected order in bulk lookup response")
	}
}

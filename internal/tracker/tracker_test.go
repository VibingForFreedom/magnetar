package tracker

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/magnetar/magnetar/internal/config"
)

func TestAnnounceToScrape(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"basic", "http://tracker.example.com/announce", "http://tracker.example.com/scrape"},
		{"with path", "http://tracker.example.com/path/announce", "http://tracker.example.com/path/scrape"},
		{"with params", "http://tracker.example.com/announce?passkey=abc", "http://tracker.example.com/scrape?passkey=abc"},
		{"no announce", "http://tracker.example.com/other", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := announcToScrape(tt.input)
			if got != tt.expected {
				t.Errorf("announcToScrape(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseTrackerURL(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantProto trackerProto
		wantHost  string
		wantErr   bool
	}{
		{"udp with port", "udp://tracker.opentrackr.org:1337/announce", protoUDP, "tracker.opentrackr.org:1337", false},
		{"udp without port", "udp://tracker.example.com/announce", protoUDP, "tracker.example.com:80", false},
		{"http", "http://tracker.example.com/announce", protoHTTP, "tracker.example.com", false},
		{"https", "https://tracker.example.com/announce", protoHTTP, "tracker.example.com", false},
		{"unsupported", "wss://tracker.example.com/announce", protoUDP, "", true},
		{"no announce for http", "http://tracker.example.com/other", protoHTTP, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTrackerURL(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for %q", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error for %q: %v", tt.input, err)
				return
			}
			if got.proto != tt.wantProto {
				t.Errorf("proto = %d, want %d", got.proto, tt.wantProto)
			}
			if got.host != tt.wantHost {
				t.Errorf("host = %q, want %q", got.host, tt.wantHost)
			}
		})
	}
}

func TestDecodeScrapeResponse(t *testing.T) {
	hash := [20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
	body := fmt.Sprintf("d5:filesd20:%s%see", string(hash[:]), "d8:completei42e10:incompletei7ee")

	entries, err := decodeScrapeResponse([]byte(body))
	if err != nil {
		t.Fatalf("decodeScrapeResponse: %v", err)
	}

	entry, ok := entries[hash]
	if !ok {
		t.Fatal("hash not found in response")
	}
	if entry.Complete != 42 {
		t.Errorf("Complete = %d, want 42", entry.Complete)
	}
	if entry.Incomplete != 7 {
		t.Errorf("Incomplete = %d, want 7", entry.Incomplete)
	}
}

func TestDecodeScrapeResponseEmpty(t *testing.T) {
	body := []byte("d5:filesdee")
	entries, err := decodeScrapeResponse(body)
	if err != nil {
		t.Fatalf("decodeScrapeResponse: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestScrapeHTTP(t *testing.T) {
	hash := [20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		body := fmt.Sprintf("d5:filesd20:%s%see", string(hash[:]), "d8:completei100e10:incompletei25ee")
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	entry, err := scrapeHTTP(context.Background(), client, ts.URL, hash)
	if err != nil {
		t.Fatalf("scrapeHTTP: %v", err)
	}
	if entry.Complete != 100 {
		t.Errorf("Complete = %d, want 100", entry.Complete)
	}
	if entry.Incomplete != 25 {
		t.Errorf("Incomplete = %d, want 25", entry.Incomplete)
	}
}

func TestScrapeUDP(t *testing.T) {
	lc := net.ListenConfig{}
	pc, err := lc.ListenPacket(context.Background(), "udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket: %v", err)
	}
	defer func() { _ = pc.Close() }()

	addr := pc.LocalAddr().String()

	go func() {
		buf := make([]byte, 256)

		n, clientAddr, err := pc.ReadFrom(buf)
		if err != nil || n < 16 {
			return
		}
		txnID := binary.BigEndian.Uint32(buf[12:16])
		connectionID := uint64(0xDEADBEEF)

		resp := make([]byte, 16)
		binary.BigEndian.PutUint32(resp[0:4], udpActionConnect)
		binary.BigEndian.PutUint32(resp[4:8], txnID)
		binary.BigEndian.PutUint64(resp[8:16], connectionID)
		_, _ = pc.WriteTo(resp, clientAddr)

		n, clientAddr, err = pc.ReadFrom(buf)
		if err != nil || n < 36 {
			return
		}
		txnID = binary.BigEndian.Uint32(buf[12:16])

		scrapeResp := make([]byte, 20)
		binary.BigEndian.PutUint32(scrapeResp[0:4], udpActionScrape)
		binary.BigEndian.PutUint32(scrapeResp[4:8], txnID)
		binary.BigEndian.PutUint32(scrapeResp[8:12], 55)  // seeders
		binary.BigEndian.PutUint32(scrapeResp[12:16], 10) // completed
		binary.BigEndian.PutUint32(scrapeResp[16:20], 12) // leechers
		_, _ = pc.WriteTo(scrapeResp, clientAddr)
	}()

	hash := [20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	entry, err := scrapeUDP(ctx, addr, hash)
	if err != nil {
		t.Fatalf("scrapeUDP: %v", err)
	}
	if entry.Complete != 55 {
		t.Errorf("Complete = %d, want 55", entry.Complete)
	}
	if entry.Incomplete != 12 {
		t.Errorf("Incomplete = %d, want 12", entry.Incomplete)
	}
}

// TestScrapeUDPBatch verifies batch UDP scraping with multiple hashes.
func TestScrapeUDPBatch(t *testing.T) {
	hash1 := [20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
	hash2 := [20]byte{21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40}
	hash3 := [20]byte{41, 42, 43, 44, 45, 46, 47, 48, 49, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59, 60}

	lc := net.ListenConfig{}
	pc, err := lc.ListenPacket(context.Background(), "udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket: %v", err)
	}
	defer func() { _ = pc.Close() }()

	addr := pc.LocalAddr().String()

	go func() {
		buf := make([]byte, 1024)

		// Connect
		n, clientAddr, err := pc.ReadFrom(buf)
		if err != nil || n < 16 {
			return
		}
		txnID := binary.BigEndian.Uint32(buf[12:16])

		resp := make([]byte, 16)
		binary.BigEndian.PutUint32(resp[0:4], udpActionConnect)
		binary.BigEndian.PutUint32(resp[4:8], txnID)
		binary.BigEndian.PutUint64(resp[8:16], 0xDEADBEEF)
		_, _ = pc.WriteTo(resp, clientAddr)

		// Scrape — expect 3 hashes (header 16 + 3*20 = 76 bytes)
		n, clientAddr, err = pc.ReadFrom(buf)
		if err != nil || n < 76 {
			return
		}
		txnID = binary.BigEndian.Uint32(buf[12:16])

		// Response: header(8) + 3 entries * 12 bytes = 44 bytes
		scrapeResp := make([]byte, 44)
		binary.BigEndian.PutUint32(scrapeResp[0:4], udpActionScrape)
		binary.BigEndian.PutUint32(scrapeResp[4:8], txnID)
		// Entry 1: seeders=10, completed=5, leechers=3
		binary.BigEndian.PutUint32(scrapeResp[8:12], 10)
		binary.BigEndian.PutUint32(scrapeResp[12:16], 5)
		binary.BigEndian.PutUint32(scrapeResp[16:20], 3)
		// Entry 2: seeders=20, completed=8, leechers=7
		binary.BigEndian.PutUint32(scrapeResp[20:24], 20)
		binary.BigEndian.PutUint32(scrapeResp[24:28], 8)
		binary.BigEndian.PutUint32(scrapeResp[28:32], 7)
		// Entry 3: seeders=30, completed=15, leechers=11
		binary.BigEndian.PutUint32(scrapeResp[32:36], 30)
		binary.BigEndian.PutUint32(scrapeResp[36:40], 15)
		binary.BigEndian.PutUint32(scrapeResp[40:44], 11)

		_, _ = pc.WriteTo(scrapeResp, clientAddr)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	results, err := scrapeUDPBatch(ctx, addr, [][20]byte{hash1, hash2, hash3})
	if err != nil {
		t.Fatalf("scrapeUDPBatch: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	tests := []struct {
		hash     [20]byte
		seeders  int
		leechers int
	}{
		{hash1, 10, 3},
		{hash2, 20, 7},
		{hash3, 30, 11},
	}
	for _, tt := range tests {
		e, ok := results[tt.hash]
		if !ok {
			t.Errorf("missing result for hash %x", tt.hash[:4])
			continue
		}
		if e.Complete != tt.seeders {
			t.Errorf("hash %x: Complete = %d, want %d", tt.hash[:4], e.Complete, tt.seeders)
		}
		if e.Incomplete != tt.leechers {
			t.Errorf("hash %x: Incomplete = %d, want %d", tt.hash[:4], e.Incomplete, tt.leechers)
		}
	}
}

// TestScrapeHTTPBatch verifies batch HTTP scraping with multiple hashes.
func TestScrapeHTTPBatch(t *testing.T) {
	hash1 := [20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
	hash2 := [20]byte{21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify we received multiple info_hash params
		rawQuery := r.URL.RawQuery
		params, _ := url.ParseQuery(rawQuery)
		hashes := params["info_hash"]
		if len(hashes) != 2 {
			t.Errorf("expected 2 info_hash params, got %d", len(hashes))
		}

		// Build response with both hashes
		body := fmt.Sprintf("d5:filesd20:%s%s20:%s%see",
			string(hash1[:]), "d8:completei50e10:incompletei15ee",
			string(hash2[:]), "d8:completei80e10:incompletei22ee",
		)
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	results, err := scrapeHTTPBatch(context.Background(), client, ts.URL, [][20]byte{hash1, hash2})
	if err != nil {
		t.Fatalf("scrapeHTTPBatch: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	e1 := results[hash1]
	if e1.Complete != 50 || e1.Incomplete != 15 {
		t.Errorf("hash1: got S=%d L=%d, want S=50 L=15", e1.Complete, e1.Incomplete)
	}
	e2 := results[hash2]
	if e2.Complete != 80 || e2.Incomplete != 22 {
		t.Errorf("hash2: got S=%d L=%d, want S=80 L=22", e2.Complete, e2.Incomplete)
	}
}

// TestScrapeBatchMergesBestAcrossTrackers verifies that ScrapeBatch takes max S/L.
func TestScrapeBatchMergesBestAcrossTrackers(t *testing.T) {
	hash := [20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}

	// Tracker 1: returns seeders=10, leechers=5
	ts1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		body := fmt.Sprintf("d5:filesd20:%s%see", string(hash[:]), "d8:completei10e10:incompletei5ee")
		_, _ = w.Write([]byte(body))
	}))
	defer ts1.Close()

	// Tracker 2: returns seeders=8, leechers=12
	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		body := fmt.Sprintf("d5:filesd20:%s%see", string(hash[:]), "d8:completei8e10:incompletei12ee")
		_, _ = w.Write([]byte(body))
	}))
	defer ts2.Close()

	cfg := &config.Config{
		TrackerEnabled: true,
		TrackerList: []string{
			ts1.URL + "/announce",
			ts2.URL + "/announce",
		},
		TrackerTimeout: 5 * time.Second,
	}

	s := New(cfg, newTestLogger())

	results := s.ScrapeBatch(context.Background(), [][20]byte{hash})
	r, ok := results[hash]
	if !ok {
		t.Fatal("hash not found in results")
	}
	// Best: seeders=10 (from ts1), leechers=12 (from ts2)
	if r.Seeders != 10 {
		t.Errorf("Seeders = %d, want 10", r.Seeders)
	}
	if r.Leechers != 12 {
		t.Errorf("Leechers = %d, want 12", r.Leechers)
	}
}

// TestAdaptiveBatchBackoff verifies that batch limit halves on failure.
func TestAdaptiveBatchBackoff(t *testing.T) {
	st := &trackerState{initialLimit: 50}
	st.batchLimit.Store(50)

	st.halveBatchLimit()
	if got := st.batchLimit.Load(); got != 25 {
		t.Errorf("after first halve: batchLimit = %d, want 25", got)
	}

	st.halveBatchLimit()
	if got := st.batchLimit.Load(); got != 12 {
		t.Errorf("after second halve: batchLimit = %d, want 12", got)
	}

	// Halve down to minimum
	for range 10 {
		st.halveBatchLimit()
	}
	if got := st.batchLimit.Load(); got != 1 {
		t.Errorf("after many halves: batchLimit = %d, want 1", got)
	}
}

// TestAdaptiveBatchGrowth verifies batch limit growth after successful batches.
func TestAdaptiveBatchGrowth(t *testing.T) {
	st := &trackerState{initialLimit: 50}
	st.batchLimit.Store(20)

	// 99 successes should not trigger growth
	for range 99 {
		st.recordSuccess()
	}
	if got := st.batchLimit.Load(); got != 20 {
		t.Errorf("after 99 successes: batchLimit = %d, want 20", got)
	}

	// 100th success triggers growth: 20 + 20/4 = 25
	st.recordSuccess()
	if got := st.batchLimit.Load(); got != 25 {
		t.Errorf("after 100 successes: batchLimit = %d, want 25", got)
	}

	// Growth is capped at initialLimit
	st.batchLimit.Store(45)
	st.successCount.Store(99)
	st.recordSuccess()
	if got := st.batchLimit.Load(); got != 50 {
		t.Errorf("growth should cap at initial: batchLimit = %d, want 50", got)
	}

	// At max, should not grow further
	st.successCount.Store(99)
	st.recordSuccess()
	if got := st.batchLimit.Load(); got != 50 {
		t.Errorf("at max should stay: batchLimit = %d, want 50", got)
	}
}

func TestScraperScrapeDisabled(t *testing.T) {
	cfg := &config.Config{
		TrackerEnabled: false,
		TrackerTimeout: 5 * time.Second,
	}

	s := New(cfg, newTestLogger())
	result := s.Scrape(context.Background(), [20]byte{})
	if result.Seeders != 0 || result.Leechers != 0 {
		t.Errorf("expected zero result when disabled, got S=%d L=%d", result.Seeders, result.Leechers)
	}
}

func TestScraperReconfigure(t *testing.T) {
	cfg := &config.Config{
		TrackerEnabled: true,
		TrackerList:    []string{"udp://tracker.example.com:1337/announce"},
		TrackerTimeout: 5 * time.Second,
	}

	s := New(cfg, newTestLogger())

	if len(s.trackers) != 1 {
		t.Fatalf("expected 1 tracker, got %d", len(s.trackers))
	}

	cfg.TrackerList = []string{
		"udp://a.example.com:1337/announce",
		"http://b.example.com/announce",
	}
	s.Reconfigure()

	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.trackers) != 2 {
		t.Errorf("expected 2 trackers after reconfigure, got %d", len(s.trackers))
	}
}

// TestScrapeBatchDisabled verifies ScrapeBatch returns nil when disabled.
func TestScrapeBatchDisabled(t *testing.T) {
	cfg := &config.Config{
		TrackerEnabled: false,
		TrackerTimeout: 5 * time.Second,
	}

	s := New(cfg, newTestLogger())
	results := s.ScrapeBatch(context.Background(), [][20]byte{{1}})
	if results != nil {
		t.Errorf("expected nil results when disabled, got %v", results)
	}
}

// TestScrapeBatchEmpty verifies ScrapeBatch handles empty input.
func TestScrapeBatchEmpty(t *testing.T) {
	cfg := &config.Config{
		TrackerEnabled: true,
		TrackerTimeout: 5 * time.Second,
	}

	s := New(cfg, newTestLogger())
	results := s.ScrapeBatch(context.Background(), nil)
	if results != nil {
		t.Errorf("expected nil results for empty hashes, got %v", results)
	}
}

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

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
	// Build a bencoded scrape response:
	// d5:filesd20:<hash>d8:completei42e10:incompletei7eeee
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
	// Start a mock UDP tracker
	lc := net.ListenConfig{}
	pc, err := lc.ListenPacket(context.Background(), "udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket: %v", err)
	}
	defer func() { _ = pc.Close() }()

	addr := pc.LocalAddr().String()

	go func() {
		buf := make([]byte, 256)

		// Handle connect request
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

		// Handle scrape request
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

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

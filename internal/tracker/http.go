package tracker

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
)

const (
	// maxHTTPBatchSize is the default max hashes per HTTP scrape request.
	maxHTTPBatchSize = 50

	// maxHTTPResponseSize limits how much of the response body we read.
	// With batch scrapes the response can be larger than single-hash scrapes.
	maxHTTPResponseSize = 256 * 1024 // 256 KiB
)

// scrapeHTTPBatch performs an HTTP tracker scrape for multiple info hashes.
// Returns a map of hash -> ScrapeEntry for all successfully scraped hashes.
func scrapeHTTPBatch(ctx context.Context, client *http.Client, scrapeURL string, hashes [][20]byte) (map[[20]byte]ScrapeEntry, error) {
	if len(hashes) == 0 {
		return nil, nil
	}

	u, err := url.Parse(scrapeURL)
	if err != nil {
		return nil, fmt.Errorf("parsing scrape URL: %w", err)
	}

	// Build raw query string manually — url.Values.Encode() percent-encodes
	// binary hash bytes which many trackers don't accept for info_hash params.
	// We need each 20-byte hash as a separate info_hash parameter.
	var qb strings.Builder
	if u.RawQuery != "" {
		qb.WriteString(u.RawQuery)
	}
	for _, h := range hashes {
		if qb.Len() > 0 {
			qb.WriteByte('&')
		}
		qb.WriteString("info_hash=")
		qb.WriteString(url.QueryEscape(string(h[:])))
	}
	u.RawQuery = qb.String()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP scrape request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP scrape status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxHTTPResponseSize))
	if err != nil {
		return nil, fmt.Errorf("reading scrape response: %w", err)
	}

	entries, err := decodeScrapeResponse(body)
	if err != nil {
		return nil, fmt.Errorf("decoding scrape response: %w", err)
	}

	return entries, nil
}

// announceHTTP performs an HTTP tracker announce to get peer addresses for an info hash.
func announceHTTP(ctx context.Context, client *http.Client, announceURL string, infoHash [20]byte) ([]netip.AddrPort, error) {
	// Generate a random peer ID
	var peerID [20]byte
	peerID[0] = '-'
	peerID[1] = 'M'
	peerID[2] = 'G'
	for i := 3; i < 20; i++ {
		peerID[i] = byte(rand.IntN(256)) //nolint:gosec
	}

	u, err := url.Parse(announceURL)
	if err != nil {
		return nil, fmt.Errorf("parsing announce URL: %w", err)
	}

	q := u.Query()
	q.Set("info_hash", string(infoHash[:]))
	q.Set("peer_id", string(peerID[:]))
	q.Set("port", "6881")
	q.Set("uploaded", "0")
	q.Set("downloaded", "0")
	q.Set("left", "999999")
	q.Set("compact", "1")
	q.Set("numwant", "50")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating announce request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP announce request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP announce status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxHTTPResponseSize))
	if err != nil {
		return nil, fmt.Errorf("reading announce response: %w", err)
	}

	return decodeAnnounceResponse(body)
}

// decodeAnnounceResponse parses a bencoded announce response and extracts
// the compact peer list from the "peers" key.
func decodeAnnounceResponse(data []byte) ([]netip.AddrPort, error) {
	d, _, err := decodeDict(data)
	if err != nil {
		return nil, fmt.Errorf("decoding announce response: %w", err)
	}

	peersRaw, ok := d["peers"]
	if !ok {
		return nil, fmt.Errorf("no peers key in announce response")
	}

	peersBytes, ok := peersRaw.([]byte)
	if !ok {
		return nil, fmt.Errorf("peers value is not raw bytes")
	}

	// The peers value is a bencoded string containing compact 6-byte peers.
	// Since decodeDict stores raw bytes, we need to decode the string.
	peerData, _, err := decodeString(peersBytes, 0)
	if err != nil {
		return nil, fmt.Errorf("decoding peers string: %w", err)
	}

	numPeers := len(peerData) / 6
	peers := make([]netip.AddrPort, 0, numPeers)
	for i := range numPeers {
		off := i * 6
		if off+6 > len(peerData) {
			break
		}
		ip := netip.AddrFrom4([4]byte{peerData[off], peerData[off+1], peerData[off+2], peerData[off+3]})
		port := binary.BigEndian.Uint16(peerData[off+4 : off+6])
		peers = append(peers, netip.AddrPortFrom(ip, port))
	}

	return peers, nil
}

// scrapeHTTP performs an HTTP tracker scrape for a single info hash.
func scrapeHTTP(ctx context.Context, client *http.Client, scrapeURL string, infoHash [20]byte) (ScrapeEntry, error) {
	results, err := scrapeHTTPBatch(ctx, client, scrapeURL, [][20]byte{infoHash})
	if err != nil {
		return ScrapeEntry{}, err
	}
	if entry, ok := results[infoHash]; ok {
		return entry, nil
	}
	return ScrapeEntry{}, fmt.Errorf("info hash not found in scrape response")
}

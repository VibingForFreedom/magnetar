package tracker

import (
	"context"
	"fmt"
	"io"
	"net/http"
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

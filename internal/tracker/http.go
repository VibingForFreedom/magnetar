package tracker

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// scrapeHTTP performs an HTTP tracker scrape for the given info hash.
func scrapeHTTP(ctx context.Context, client *http.Client, scrapeURL string, infoHash [20]byte) (ScrapeEntry, error) {
	u, err := url.Parse(scrapeURL)
	if err != nil {
		return ScrapeEntry{}, fmt.Errorf("parsing scrape URL: %w", err)
	}

	q := u.Query()
	q.Set("info_hash", string(infoHash[:]))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return ScrapeEntry{}, fmt.Errorf("creating request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return ScrapeEntry{}, fmt.Errorf("HTTP scrape request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return ScrapeEntry{}, fmt.Errorf("HTTP scrape status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return ScrapeEntry{}, fmt.Errorf("reading scrape response: %w", err)
	}

	entries, err := decodeScrapeResponse(body)
	if err != nil {
		return ScrapeEntry{}, fmt.Errorf("decoding scrape response: %w", err)
	}

	if entry, ok := entries[infoHash]; ok {
		return entry, nil
	}

	return ScrapeEntry{}, fmt.Errorf("info hash not found in scrape response")
}

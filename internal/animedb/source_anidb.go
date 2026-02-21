package animedb

import (
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const anidbTitlesURL = "http://anidb.net/api/anime-titles.dat.gz"

// downloadAniDBTitles fetches the AniDB title dump and returns a map of
// AniDB ID to all title variants for that anime.
func downloadAniDBTitles(ctx context.Context) (map[int][]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, anidbTitlesURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "magnetar/1.0")

	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading anidb titles: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anidb download returned status %d", resp.StatusCode)
	}

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("decompressing anidb titles: %w", err)
	}
	defer gz.Close()

	return parseAniDBTitles(gz)
}

// parseAniDBTitles parses the AniDB title dump format.
// Lines starting with # are comments. Data lines: anidbID|type|language|title
func parseAniDBTitles(r io.Reader) (map[int][]string, error) {
	titles := make(map[int][]string, 15000)
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "|", 4)
		if len(parts) < 4 {
			continue
		}

		id, err := strconv.Atoi(parts[0])
		if err != nil || id <= 0 {
			continue
		}

		title := strings.TrimSpace(parts[3])
		if title == "" {
			continue
		}

		titles[id] = append(titles[id], title)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning anidb titles: %w", err)
	}

	return titles, nil
}

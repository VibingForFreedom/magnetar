package matcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

const anilistBaseURL = "https://graphql.anilist.co"

type AniListClient struct {
	client  *http.Client
	limiter *rate.Limiter
	baseURL string
}

func NewAniListClient() *AniListClient {
	return &AniListClient{
		client:  &http.Client{Timeout: 10 * time.Second},
		limiter: rate.NewLimiter(1.5, 2), // 90 req/min
		baseURL: anilistBaseURL,
	}
}

const anilistSearchQuery = `
query ($search: String) {
  Media(search: $search, type: ANIME) {
    id
    idMal
    title {
      romaji
      english
    }
  }
}
`

func (c *AniListClient) SearchAnime(ctx context.Context, title string) (anilistID int, err error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return 0, fmt.Errorf("rate limiter: %w", err)
	}

	body, err := json.Marshal(map[string]interface{}{
		"query": anilistSearchQuery,
		"variables": map[string]string{
			"search": title,
		},
	})
	if err != nil {
		return 0, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("anilist returned status %d", resp.StatusCode)
	}

	var result struct {
		Data struct {
			Media *struct {
				ID int `json:"id"`
			} `json:"Media"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("decoding response: %w", err)
	}

	if result.Data.Media == nil {
		return 0, nil
	}

	return result.Data.Media.ID, nil
}

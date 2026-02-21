package matcher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/time/rate"
)

const kitsuBaseURL = "https://kitsu.app/api/edge"

type KitsuClient struct {
	client  *http.Client
	limiter *rate.Limiter
	baseURL string
}

func NewKitsuClient() *KitsuClient {
	return &KitsuClient{
		client:  &http.Client{Timeout: 10 * time.Second},
		limiter: rate.NewLimiter(30, 30), // ~30 req/s
		baseURL: kitsuBaseURL,
	}
}

func (c *KitsuClient) SearchAnime(ctx context.Context, title string) (kitsuID int, err error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return 0, fmt.Errorf("rate limiter: %w", err)
	}

	params := url.Values{
		"filter[text]": {title},
		"page[limit]":  {"1"},
	}

	reqURL := c.baseURL + "/anime?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return 0, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.api+json")

	resp, err := c.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("kitsu returned status %d", resp.StatusCode)
	}

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("decoding response: %w", err)
	}

	if len(result.Data) == 0 {
		return 0, nil
	}

	var id int
	_, _ = fmt.Sscanf(result.Data[0].ID, "%d", &id)
	return id, nil
}

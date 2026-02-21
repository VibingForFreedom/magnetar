package matcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const tvdbBaseURL = "https://api4.thetvdb.com/v4"

type TVDBClient struct {
	apiKey  string
	client  *http.Client
	limiter *rate.Limiter

	mu          sync.Mutex
	token       string
	tokenExpiry time.Time
}

func NewTVDBClient(apiKey string) *TVDBClient {
	return &TVDBClient{
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 10 * time.Second},
		limiter: rate.NewLimiter(20, 20), // 20 req/s
	}
}

func (c *TVDBClient) SearchSeries(ctx context.Context, title string, year int) (tvdbID int, err error) {
	if err := c.ensureToken(ctx); err != nil {
		return 0, err
	}

	params := url.Values{
		"query": {title},
		"type":  {"series"},
	}
	if year > 0 {
		params.Set("year", strconv.Itoa(year))
	}

	var resp struct {
		Data []struct {
			TVDBID string `json:"tvdb_id"`
		} `json:"data"`
	}

	if err := c.doGet(ctx, "/search", params, &resp); err != nil {
		return 0, fmt.Errorf("tvdb search series: %w", err)
	}

	if len(resp.Data) == 0 {
		return 0, nil
	}

	id, _ := strconv.Atoi(resp.Data[0].TVDBID)
	return id, nil
}

func (c *TVDBClient) SearchByIMDB(ctx context.Context, imdbID string) (tvdbID int, err error) {
	if err := c.ensureToken(ctx); err != nil {
		return 0, err
	}

	params := url.Values{
		"remoteId": {imdbID},
		"type":     {"series"},
	}

	var resp struct {
		Data []struct {
			TVDBID string `json:"tvdb_id"`
		} `json:"data"`
	}

	if err := c.doGet(ctx, "/search", params, &resp); err != nil {
		return 0, fmt.Errorf("tvdb search by imdb: %w", err)
	}

	if len(resp.Data) == 0 {
		return 0, nil
	}

	id, _ := strconv.Atoi(resp.Data[0].TVDBID)
	return id, nil
}

func (c *TVDBClient) ensureToken(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.token != "" && time.Now().Before(c.tokenExpiry) {
		return nil
	}

	return c.login(ctx)
}

func (c *TVDBClient) login(ctx context.Context) error {
	if err := c.limiter.Wait(ctx); err != nil {
		return fmt.Errorf("rate limiter: %w", err)
	}

	body, err := json.Marshal(map[string]string{
		"apikey": c.apiKey,
	})
	if err != nil {
		return fmt.Errorf("marshaling login body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tvdbBaseURL+"/login", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("executing login request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("tvdb login failed with status %d", resp.StatusCode)
	}

	var result struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decoding login response: %w", err)
	}

	c.token = result.Data.Token
	c.tokenExpiry = time.Now().Add(24 * time.Hour) // conservative 24h expiry

	return nil
}

func (c *TVDBClient) doGet(ctx context.Context, path string, params url.Values, dst interface{}) error {
	if err := c.limiter.Wait(ctx); err != nil {
		return fmt.Errorf("rate limiter: %w", err)
	}

	reqURL := tvdbBaseURL + path
	if params != nil {
		reqURL += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	c.mu.Lock()
	token := c.token
	c.mu.Unlock()

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d for %s", resp.StatusCode, path)
	}

	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}

	return nil
}

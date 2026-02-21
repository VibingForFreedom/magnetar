package matcher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"golang.org/x/time/rate"
)

const tmdbBaseURL = "https://api.themoviedb.org/3"

type TMDBClient struct {
	apiKey  string
	client  *http.Client
	limiter *rate.Limiter
}

type TMDBFindResult struct {
	TMDBID    int
	MediaType string // "movie" or "tv"
	Title     string
	Year      int
}

func NewTMDBClient(apiKey string) *TMDBClient {
	return &TMDBClient{
		apiKey: apiKey,
		client: &http.Client{Timeout: 10 * time.Second},
		limiter: rate.NewLimiter(4, 4), // 40 req/10s
	}
}

func (c *TMDBClient) SearchMovie(ctx context.Context, title string, year int) (tmdbID int, releaseYear int, err error) {
	params := url.Values{
		"query": {title},
	}
	if year > 0 {
		params.Set("year", strconv.Itoa(year))
	}

	var resp struct {
		Results []struct {
			ID          int    `json:"id"`
			ReleaseDate string `json:"release_date"`
		} `json:"results"`
	}

	if err := c.doGet(ctx, "/search/movie", params, &resp); err != nil {
		return 0, 0, fmt.Errorf("tmdb search movie: %w", err)
	}

	if len(resp.Results) == 0 {
		return 0, 0, nil
	}

	result := resp.Results[0]
	var ry int
	if len(result.ReleaseDate) >= 4 {
		ry, _ = strconv.Atoi(result.ReleaseDate[:4])
	}

	return result.ID, ry, nil
}

func (c *TMDBClient) SearchTV(ctx context.Context, title string, year int) (tmdbID int, firstAirYear int, err error) {
	params := url.Values{
		"query": {title},
	}
	if year > 0 {
		params.Set("first_air_date_year", strconv.Itoa(year))
	}

	var resp struct {
		Results []struct {
			ID           int    `json:"id"`
			FirstAirDate string `json:"first_air_date"`
		} `json:"results"`
	}

	if err := c.doGet(ctx, "/search/tv", params, &resp); err != nil {
		return 0, 0, fmt.Errorf("tmdb search tv: %w", err)
	}

	if len(resp.Results) == 0 {
		return 0, 0, nil
	}

	result := resp.Results[0]
	var fay int
	if len(result.FirstAirDate) >= 4 {
		fay, _ = strconv.Atoi(result.FirstAirDate[:4])
	}

	return result.ID, fay, nil
}

func (c *TMDBClient) FindByIMDB(ctx context.Context, imdbID string) (*TMDBFindResult, error) {
	params := url.Values{
		"external_source": {"imdb_id"},
	}

	var resp struct {
		MovieResults []struct {
			ID          int    `json:"id"`
			Title       string `json:"title"`
			ReleaseDate string `json:"release_date"`
		} `json:"movie_results"`
		TVResults []struct {
			ID           int    `json:"id"`
			Name         string `json:"name"`
			FirstAirDate string `json:"first_air_date"`
		} `json:"tv_results"`
	}

	path := "/find/" + imdbID
	if err := c.doGet(ctx, path, params, &resp); err != nil {
		return nil, fmt.Errorf("tmdb find by imdb: %w", err)
	}

	if len(resp.MovieResults) > 0 {
		r := resp.MovieResults[0]
		var year int
		if len(r.ReleaseDate) >= 4 {
			year, _ = strconv.Atoi(r.ReleaseDate[:4])
		}
		return &TMDBFindResult{
			TMDBID:    r.ID,
			MediaType: "movie",
			Title:     r.Title,
			Year:      year,
		}, nil
	}

	if len(resp.TVResults) > 0 {
		r := resp.TVResults[0]
		var year int
		if len(r.FirstAirDate) >= 4 {
			year, _ = strconv.Atoi(r.FirstAirDate[:4])
		}
		return &TMDBFindResult{
			TMDBID:    r.ID,
			MediaType: "tv",
			Title:     r.Name,
			Year:      year,
		}, nil
	}

	return nil, nil
}

func (c *TMDBClient) GetMovieExternalIDs(ctx context.Context, tmdbID int) (imdbID string, err error) {
	var resp struct {
		IMDBID string `json:"imdb_id"`
	}

	path := fmt.Sprintf("/movie/%d/external_ids", tmdbID)
	if err := c.doGet(ctx, path, nil, &resp); err != nil {
		return "", fmt.Errorf("tmdb get movie external ids: %w", err)
	}

	return resp.IMDBID, nil
}

func (c *TMDBClient) GetTVExternalIDs(ctx context.Context, tmdbID int) (imdbID string, tvdbID int, err error) {
	var resp struct {
		IMDBID string `json:"imdb_id"`
		TVDBID int    `json:"tvdb_id"`
	}

	path := fmt.Sprintf("/tv/%d/external_ids", tmdbID)
	if err := c.doGet(ctx, path, nil, &resp); err != nil {
		return "", 0, fmt.Errorf("tmdb get tv external ids: %w", err)
	}

	return resp.IMDBID, resp.TVDBID, nil
}

func (c *TMDBClient) doGet(ctx context.Context, path string, params url.Values, dst interface{}) error {
	if err := c.limiter.Wait(ctx); err != nil {
		return fmt.Errorf("rate limiter: %w", err)
	}

	if params == nil {
		params = url.Values{}
	}
	params.Set("api_key", c.apiKey)

	reqURL := tmdbBaseURL + path + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

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

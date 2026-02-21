package animedb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const manamiURL = "https://github.com/manami-project/anime-offline-database/releases/latest/download/anime-offline-database-minified.json"

type manamiDatabase struct {
	Data []manamiEntry `json:"data"`
}

type manamiEntry struct {
	Sources     []string    `json:"sources"`
	Title       string      `json:"title"`
	Synonyms    []string    `json:"synonyms"`
	AnimeSeason manamiSeason `json:"animeSeason"`
}

type manamiSeason struct {
	Year int `json:"year"`
}

// downloadManami fetches the manami anime-offline-database JSON and parses it
// into AnimeEntry slices along with their associated titles (primary + synonyms).
func downloadManami(ctx context.Context) ([]AnimeEntry, [][]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manamiURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("creating request: %w", err)
	}

	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("downloading manami database: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("manami download returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("reading manami response: %w", err)
	}

	var db manamiDatabase
	if err := json.Unmarshal(body, &db); err != nil {
		return nil, nil, fmt.Errorf("parsing manami JSON: %w", err)
	}

	entries := make([]AnimeEntry, 0, len(db.Data))
	allTitles := make([][]string, 0, len(db.Data))

	for _, me := range db.Data {
		entry := AnimeEntry{
			Title: me.Title,
			Year:  me.AnimeSeason.Year,
		}

		for _, src := range me.Sources {
			parseSourceURL(&entry, src)
		}

		titles := make([]string, 0, 1+len(me.Synonyms))
		titles = append(titles, me.Title)
		titles = append(titles, me.Synonyms...)

		entries = append(entries, entry)
		allTitles = append(allTitles, titles)
	}

	return entries, allTitles, nil
}

// parseManami parses manami JSON data directly (for testing).
func parseManami(data []byte) ([]AnimeEntry, [][]string, error) {
	var db manamiDatabase
	if err := json.Unmarshal(data, &db); err != nil {
		return nil, nil, fmt.Errorf("parsing manami JSON: %w", err)
	}

	entries := make([]AnimeEntry, 0, len(db.Data))
	allTitles := make([][]string, 0, len(db.Data))

	for _, me := range db.Data {
		entry := AnimeEntry{
			Title: me.Title,
			Year:  me.AnimeSeason.Year,
		}

		for _, src := range me.Sources {
			parseSourceURL(&entry, src)
		}

		titles := make([]string, 0, 1+len(me.Synonyms))
		titles = append(titles, me.Title)
		titles = append(titles, me.Synonyms...)

		entries = append(entries, entry)
		allTitles = append(allTitles, titles)
	}

	return entries, allTitles, nil
}

var sourcePatterns = []struct {
	prefix string
	setter func(entry *AnimeEntry, id int)
}{
	{"https://anilist.co/anime/", func(e *AnimeEntry, id int) { e.AniListID = id }},
	{"https://kitsu.app/anime/", func(e *AnimeEntry, id int) { e.KitsuID = id }},
	{"https://kitsu.io/anime/", func(e *AnimeEntry, id int) { e.KitsuID = id }},
	{"https://myanimelist.net/anime/", func(e *AnimeEntry, id int) { e.MALID = id }},
	{"https://anidb.net/anime/", func(e *AnimeEntry, id int) { e.AniDBID = id }},
}

func parseSourceURL(entry *AnimeEntry, url string) {
	for _, sp := range sourcePatterns {
		if strings.HasPrefix(url, sp.prefix) {
			idStr := url[len(sp.prefix):]
			if id, err := strconv.Atoi(idStr); err == nil && id > 0 {
				sp.setter(entry, id)
			}
			return
		}
	}
}

// Command fetch-adult-lists queries StashDB for performers and studios,
// fetches the LDNOOBW dirty-words list, and generates
// internal/classify/adult_data.go with compiled lookup maps.
//
// Usage:
//
//	go run ./scripts/fetch-adult-lists --api-key=YOUR_KEY
//	# or
//	STASHDB_API_KEY=YOUR_KEY go run ./scripts/fetch-adult-lists
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

const (
	stashDBURL  = "https://stashdb.org/graphql"
	ldnoobwURL  = "https://raw.githubusercontent.com/LDNOOBW/List-of-Dirty-Naughty-Obscene-and-Otherwise-Bad-Words/master/en"
	outputFile  = "internal/classify/adult_data.go"
	pageSize    = 1000
	httpTimeout = 30 * time.Second
)

// falsePositiveKeywords are dirty-word entries that collide with legit media titles.
// These are removed from the LDNOOBW list to avoid false positives.
var falsePositiveKeywords = map[string]bool{
	"ass":       true,
	"asses":     true,
	"balls":     true,
	"bang":      true,
	"beaver":    true,
	"blow":      true,
	"blown":     true,
	"bone":      true,
	"boned":     true,
	"boner":     true,
	"breast":    true,
	"breasts":   true,
	"cherry":    true,
	"cock":      true,
	"cocks":     true,
	"cox":       true,
	"crack":     true,
	"damn":      true,
	"dyke":      true,
	"erect":     true,
	"finger":    true,
	"fist":      true,
	"flash":     true,
	"fondle":    true,
	"hell":      true,
	"hole":      true,
	"hooker":    true,
	"horn":      true,
	"horny":     true,
	"hot":       true,
	"hump":      true,
	"jack":      true,
	"jerk":      true,
	"kinky":     true,
	"knob":      true,
	"laid":      true,
	"lick":      true,
	"lover":     true,
	"moan":      true,
	"mount":     true,
	"nail":      true,
	"naked":     true,
	"naughty":   true,
	"nut":       true,
	"nuts":      true,
	"pecker":    true,
	"peter":     true,
	"pink":      true,
	"piss":      true,
	"pissed":    true,
	"plug":      true,
	"pole":      true,
	"poo":       true,
	"prick":     true,
	"puff":      true,
	"pump":      true,
	"queer":     true,
	"ram":       true,
	"rubber":    true,
	"screw":     true,
	"sex":       true,
	"sexy":      true,
	"shaft":     true,
	"shoot":     true,
	"slap":      true,
	"slave":     true,
	"snatch":    true,
	"spank":     true,
	"spunk":     true,
	"strip":     true,
	"suck":      true,
	"thrust":    true,
	"tool":      true,
	"wank":      true,
	"wet":       true,
	"whip":      true,
	"whore":     true,
	"wild":      true,
	"wood":      true,
	"xxx":       true, // already in regex patterns
	"spit":      true,
	"tongue":    true,
	"ride":      true,
	"harder":    true,
	"come":      true,
	"stroke":    true,
	"swallow":   true,
	"tied":      true,
	"bondage":   true,
	"choke":     true,
	"gag":       true,
	"collar":    true,
	"cage":      true,
	"domination": true,
	"submission":  true,
}

type graphQLRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

type performerResponse struct {
	Data struct {
		QueryPerformers struct {
			Count      int `json:"count"`
			Performers []struct {
				Name string `json:"name"`
			} `json:"performers"`
		} `json:"queryPerformers"`
	} `json:"data"`
}

type studioResponse struct {
	Data struct {
		QueryStudios struct {
			Count   int `json:"count"`
			Studios []struct {
				Name string `json:"name"`
			} `json:"studios"`
		} `json:"queryStudios"`
	} `json:"data"`
}

func main() {
	apiKey := flag.String("api-key", os.Getenv("STASHDB_API_KEY"), "StashDB API key (or STASHDB_API_KEY env)")
	flag.Parse()

	if *apiKey == "" {
		fmt.Fprintln(os.Stderr, "error: StashDB API key required (--api-key or STASHDB_API_KEY env)")
		os.Exit(1)
	}

	client := &http.Client{Timeout: httpTimeout}

	fmt.Println("Fetching performers from StashDB...")
	performers := fetchPerformers(client, *apiKey)
	fmt.Printf("  → %d performers fetched\n", len(performers))

	fmt.Println("Fetching studios from StashDB...")
	studios := fetchStudios(client, *apiKey)
	fmt.Printf("  → %d studios fetched\n", len(studios))

	fmt.Println("Fetching LDNOOBW dirty words...")
	keywords := fetchDirtyWords(client)
	fmt.Printf("  → %d keywords (after filtering)\n", len(keywords))

	fmt.Println("Generating adult_data.go...")
	generateFile(performers, studios, keywords)
	fmt.Printf("  → Written to %s\n", outputFile)
}

func fetchPerformers(client *http.Client, apiKey string) map[string]bool {
	result := make(map[string]bool)
	page := 1

	for {
		query := `query ($input: PerformerQueryInput!) {
			queryPerformers(input: $input) {
				count
				performers { name }
			}
		}`

		variables := map[string]any{
			"input": map[string]any{
				"page":     page,
				"per_page": pageSize,
			},
		}

		body := doGraphQL(client, apiKey, query, variables)
		var resp performerResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			fmt.Fprintf(os.Stderr, "error parsing performers page %d: %v\n", page, err)
			os.Exit(1)
		}

		for _, p := range resp.Data.QueryPerformers.Performers {
			name := normalizeName(p.Name)
			if shouldIncludeName(name) {
				result[name] = true
			}
		}

		total := resp.Data.QueryPerformers.Count
		fetched := page * pageSize
		fmt.Printf("  page %d: %d/%d\n", page, min(fetched, total), total)

		if fetched >= total {
			break
		}
		page++
	}

	return result
}

func fetchStudios(client *http.Client, apiKey string) map[string]bool {
	result := make(map[string]bool)
	page := 1

	for {
		query := `query ($input: StudioQueryInput!) {
			queryStudios(input: $input) {
				count
				studios { name }
			}
		}`

		variables := map[string]any{
			"input": map[string]any{
				"page":     page,
				"per_page": pageSize,
			},
		}

		body := doGraphQL(client, apiKey, query, variables)
		var resp studioResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			fmt.Fprintf(os.Stderr, "error parsing studios page %d: %v\n", page, err)
			os.Exit(1)
		}

		for _, s := range resp.Data.QueryStudios.Studios {
			name := normalizeName(s.Name)
			if len(name) >= 3 {
				result[name] = true
			}
		}

		total := resp.Data.QueryStudios.Count
		fetched := page * pageSize
		fmt.Printf("  page %d: %d/%d\n", page, min(fetched, total), total)

		if fetched >= total {
			break
		}
		page++
	}

	return result
}

func fetchDirtyWords(client *http.Client) map[string]bool {
	resp, err := client.Get(ldnoobwURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error fetching LDNOOBW: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	result := make(map[string]bool)
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		word := strings.TrimSpace(strings.ToLower(scanner.Text()))
		if word == "" || len(word) < 4 {
			continue
		}
		if falsePositiveKeywords[word] {
			continue
		}
		result[word] = true
	}

	return result
}

func doGraphQL(client *http.Client, apiKey, query string, variables map[string]any) []byte {
	reqBody := graphQLRequest{
		Query:     query,
		Variables: variables,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error marshaling request: %v\n", err)
		os.Exit(1)
	}

	req, err := http.NewRequest("POST", stashDBURL, bytes.NewReader(data))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating request: %v\n", err)
		os.Exit(1)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("ApiKey", apiKey)

	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error executing request: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "StashDB returned %d: %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading response: %v\n", err)
		os.Exit(1)
	}

	return body
}

// normalizeName lowercases, collapses whitespace, and strips non-letter/space chars.
func normalizeName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	// Remove content in parentheses (disambiguation like "Jayden Lee (II)")
	name = regexp.MustCompile(`\s*\([^)]*\)\s*`).ReplaceAllString(name, " ")
	// Collapse whitespace
	name = regexp.MustCompile(`\s+`).ReplaceAllString(name, " ")
	return strings.TrimSpace(name)
}

// shouldIncludeName filters out names too short or single common words.
func shouldIncludeName(name string) bool {
	if len(name) < 4 {
		return false
	}
	// Must contain at least one space (first + last name) to reduce false positives
	if !strings.Contains(name, " ") {
		return false
	}
	// Skip if all parts are very short
	parts := strings.Fields(name)
	if len(parts) < 2 {
		return false
	}
	// Skip names where any part is a single character (initials like "A. B.")
	allShort := true
	for _, p := range parts {
		cleaned := strings.Trim(p, ".")
		if len(cleaned) > 1 {
			allShort = false
			break
		}
	}
	if allShort {
		return false
	}
	// Require at least one part with >= 2 alphabetic characters
	hasAlpha := false
	for _, p := range parts {
		alphaCount := 0
		for _, r := range p {
			if unicode.IsLetter(r) {
				alphaCount++
			}
		}
		if alphaCount >= 2 {
			hasAlpha = true
			break
		}
	}
	return hasAlpha
}

func generateFile(performers, studios, keywords map[string]bool) {
	var buf bytes.Buffer

	now := time.Now().UTC().Format("2006-01-02")

	fmt.Fprintf(&buf, "// Code generated by scripts/fetch-adult-lists. DO NOT EDIT.\n")
	fmt.Fprintf(&buf, "// Generated: %s\n", now)
	fmt.Fprintf(&buf, "// Source: StashDB (performers: %d, studios: %d), LDNOOBW (keywords: %d)\n\n", len(performers), len(studios), len(keywords))
	fmt.Fprintf(&buf, "package classify\n\n")

	writeMap(&buf, "adultPerformers", performers)
	writeMap(&buf, "adultStudios", studios)
	writeMap(&buf, "adultKeywords", keywords)

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error formatting generated code: %v\n", err)
		fmt.Fprintln(os.Stderr, "Writing unformatted output...")
		formatted = buf.Bytes()
	}

	if err := os.WriteFile(outputFile, formatted, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing %s: %v\n", outputFile, err)
		os.Exit(1)
	}
}

func writeMap(buf *bytes.Buffer, name string, m map[string]bool) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fmt.Fprintf(buf, "var %s = map[string]bool{\n", name)
	for _, k := range keys {
		fmt.Fprintf(buf, "\t%q: true,\n", k)
	}
	fmt.Fprintf(buf, "}\n\n")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

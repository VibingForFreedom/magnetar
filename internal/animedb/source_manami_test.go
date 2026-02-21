package animedb

import (
	"testing"
)

func TestParseManami(t *testing.T) {
	data := []byte(`{
		"data": [
			{
				"sources": [
					"https://anilist.co/anime/1535",
					"https://kitsu.app/anime/1376",
					"https://myanimelist.net/anime/1535",
					"https://anidb.net/anime/4563"
				],
				"title": "Death Note",
				"synonyms": ["DN", "DEATH NOTE"],
				"animeSeason": {"year": 2006}
			},
			{
				"sources": [
					"https://anilist.co/anime/16498",
					"https://kitsu.io/anime/7442"
				],
				"title": "Attack on Titan",
				"synonyms": ["Shingeki no Kyojin"],
				"animeSeason": {"year": 2013}
			}
		]
	}`)

	entries, allTitles, err := parseManami(data)
	if err != nil {
		t.Fatalf("parseManami error: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}

	// Death Note entry
	dn := entries[0]
	if dn.AniListID != 1535 {
		t.Errorf("Death Note AniListID = %d, want 1535", dn.AniListID)
	}
	if dn.KitsuID != 1376 {
		t.Errorf("Death Note KitsuID = %d, want 1376", dn.KitsuID)
	}
	if dn.MALID != 1535 {
		t.Errorf("Death Note MALID = %d, want 1535", dn.MALID)
	}
	if dn.AniDBID != 4563 {
		t.Errorf("Death Note AniDBID = %d, want 4563", dn.AniDBID)
	}
	if dn.Title != "Death Note" {
		t.Errorf("Death Note Title = %q, want %q", dn.Title, "Death Note")
	}
	if dn.Year != 2006 {
		t.Errorf("Death Note Year = %d, want 2006", dn.Year)
	}

	// Titles should include primary + synonyms
	if len(allTitles[0]) != 3 {
		t.Errorf("Death Note titles count = %d, want 3", len(allTitles[0]))
	}

	// Attack on Titan entry with kitsu.io URL
	aot := entries[1]
	if aot.AniListID != 16498 {
		t.Errorf("AoT AniListID = %d, want 16498", aot.AniListID)
	}
	if aot.KitsuID != 7442 {
		t.Errorf("AoT KitsuID = %d, want 7442", aot.KitsuID)
	}
}

func TestParseManami_EmptyData(t *testing.T) {
	data := []byte(`{"data": []}`)
	entries, _, err := parseManami(data)
	if err != nil {
		t.Fatalf("parseManami error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries, want 0", len(entries))
	}
}

func TestParseManami_InvalidJSON(t *testing.T) {
	data := []byte(`not json`)
	_, _, err := parseManami(data)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParseSourceURL(t *testing.T) {
	tests := []struct {
		url   string
		field string
		want  int
	}{
		{"https://anilist.co/anime/1535", "anilist", 1535},
		{"https://kitsu.app/anime/1376", "kitsu", 1376},
		{"https://kitsu.io/anime/1376", "kitsu", 1376},
		{"https://myanimelist.net/anime/1535", "mal", 1535},
		{"https://anidb.net/anime/4563", "anidb", 4563},
		{"https://unknown.com/anime/123", "none", 0},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			entry := AnimeEntry{}
			parseSourceURL(&entry, tt.url)

			var got int
			switch tt.field {
			case "anilist":
				got = entry.AniListID
			case "kitsu":
				got = entry.KitsuID
			case "mal":
				got = entry.MALID
			case "anidb":
				got = entry.AniDBID
			case "none":
				got = entry.AniListID + entry.KitsuID + entry.MALID + entry.AniDBID
			}

			if got != tt.want {
				t.Errorf("parseSourceURL(%q) %s = %d, want %d", tt.url, tt.field, got, tt.want)
			}
		})
	}
}

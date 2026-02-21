package animedb

import (
	"strings"
	"testing"
)

func TestParseAniDBTitles(t *testing.T) {
	input := `# AniDB Anime Titles Dump
# Created: 2024-01-01
1|1|en|Crest of the Stars
1|2|ja|星界の紋章
1|3|x-jat|Seikai no Monshou
2|1|en|Cowboy Bebop
2|4|fr|Cowboy Bebop
3|1|en|Trigun
`

	titles, err := parseAniDBTitles(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parseAniDBTitles error: %v", err)
	}

	if len(titles) != 3 {
		t.Fatalf("got %d anime, want 3", len(titles))
	}

	// ID 1 should have 3 titles
	if len(titles[1]) != 3 {
		t.Errorf("anime 1 titles count = %d, want 3", len(titles[1]))
	}
	if titles[1][0] != "Crest of the Stars" {
		t.Errorf("anime 1 first title = %q, want %q", titles[1][0], "Crest of the Stars")
	}

	// ID 2 should have 2 titles
	if len(titles[2]) != 2 {
		t.Errorf("anime 2 titles count = %d, want 2", len(titles[2]))
	}

	// ID 3 should have 1 title
	if len(titles[3]) != 1 {
		t.Errorf("anime 3 titles count = %d, want 1", len(titles[3]))
	}
}

func TestParseAniDBTitles_SkipsComments(t *testing.T) {
	input := `# Comment line
# Another comment
1|1|en|Test Anime
`

	titles, err := parseAniDBTitles(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parseAniDBTitles error: %v", err)
	}

	if len(titles) != 1 {
		t.Errorf("got %d anime, want 1", len(titles))
	}
}

func TestParseAniDBTitles_InvalidLines(t *testing.T) {
	input := `bad line
1|en|missing field
abc|1|en|non-numeric id
0|1|en|zero id
-1|1|en|negative id
1|1|en|
2|1|en|Valid Entry
`

	titles, err := parseAniDBTitles(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parseAniDBTitles error: %v", err)
	}

	// Only "Valid Entry" should be parsed (id=2)
	if len(titles) != 1 {
		t.Errorf("got %d anime, want 1", len(titles))
	}
	if titles[2] == nil || titles[2][0] != "Valid Entry" {
		t.Error("expected 'Valid Entry' for id 2")
	}
}

func TestParseAniDBTitles_Empty(t *testing.T) {
	titles, err := parseAniDBTitles(strings.NewReader(""))
	if err != nil {
		t.Fatalf("parseAniDBTitles error: %v", err)
	}
	if len(titles) != 0 {
		t.Errorf("got %d anime, want 0", len(titles))
	}
}

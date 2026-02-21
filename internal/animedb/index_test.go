package animedb

import (
	"testing"
)

func TestNormalizeTitle(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Death Note", "death note"},
		{"[SubGroup] Death Note", "death note"},
		{"Death.Note", "death note"},
		{"Death_Note", "death note"},
		{"Death-Note", "death note"},
		{"Attack on Titan Season 3", "attack on titan"},
		{"Attack on Titan 2nd Season", "attack on titan"},
		{"Naruto Part 2", "naruto"},
		{"Bleach Cour 2", "bleach"},
		{"Steins;Gate (TV)", "steins;gate"},
		{"[HorribleSubs] One Piece - 1000 [1080p]", "one piece 1000"},
		{"Sword Art Online II", "sword art online"},
		{"My Hero Academia III", "my hero academia"},
		{"Evangelion The Animation", "evangelion"},
		{"Some Anime Batch", "some anime"},
		{"Some Anime Complete", "some anime"},
		{"", ""},
		{"   ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeTitle(tt.input)
			if got != tt.want {
				t.Errorf("normalizeTitle(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestTitleIndex_Lookup(t *testing.T) {
	idx := newTitleIndex()

	idx.addEntry(AnimeEntry{
		AniListID: 1535,
		Title:     "Death Note",
		Year:      2006,
	}, []string{"Death Note", "DN", "DEATH NOTE"})

	idx.addEntry(AnimeEntry{
		AniListID: 16498,
		Title:     "Attack on Titan",
		Year:      2013,
	}, []string{"Attack on Titan", "Shingeki no Kyojin", "AoT"})

	idx.addEntry(AnimeEntry{
		AniListID: 20,
		Title:     "Naruto",
		Year:      2002,
	}, []string{"Naruto", "NARUTO"})

	t.Run("exact match", func(t *testing.T) {
		entry := idx.Lookup("Death Note")
		if entry == nil {
			t.Fatal("expected match for 'Death Note'")
		}
		if entry.AniListID != 1535 {
			t.Errorf("got AniListID %d, want 1535", entry.AniListID)
		}
	})

	t.Run("case insensitive", func(t *testing.T) {
		entry := idx.Lookup("death note")
		if entry == nil {
			t.Fatal("expected match for 'death note'")
		}
		if entry.AniListID != 1535 {
			t.Errorf("got AniListID %d, want 1535", entry.AniListID)
		}
	})

	t.Run("synonym match", func(t *testing.T) {
		entry := idx.Lookup("Shingeki no Kyojin")
		if entry == nil {
			t.Fatal("expected match for 'Shingeki no Kyojin'")
		}
		if entry.AniListID != 16498 {
			t.Errorf("got AniListID %d, want 16498", entry.AniListID)
		}
	})

	t.Run("tail trimming match", func(t *testing.T) {
		entry := idx.Lookup("Attack on Titan Final Season")
		if entry == nil {
			t.Fatal("expected match for 'Attack on Titan Final Season'")
		}
		if entry.AniListID != 16498 {
			t.Errorf("got AniListID %d, want 16498", entry.AniListID)
		}
	})

	t.Run("no match", func(t *testing.T) {
		entry := idx.Lookup("Nonexistent Anime Title")
		if entry != nil {
			t.Errorf("expected nil, got entry with AniListID %d", entry.AniListID)
		}
	})

	t.Run("empty string", func(t *testing.T) {
		entry := idx.Lookup("")
		if entry != nil {
			t.Error("expected nil for empty string")
		}
	})

	t.Run("contains", func(t *testing.T) {
		if !idx.Contains("Naruto") {
			t.Error("expected Contains to return true for 'Naruto'")
		}
		if idx.Contains("Dragon Ball") {
			t.Error("expected Contains to return false for 'Dragon Ball'")
		}
	})

	t.Run("len", func(t *testing.T) {
		if idx.Len() != 3 {
			t.Errorf("got Len %d, want 3", idx.Len())
		}
	})
}

func TestTitleIndex_DuplicateNormalized(t *testing.T) {
	idx := newTitleIndex()

	idx.addEntry(AnimeEntry{AniListID: 1, Title: "Test Anime"}, []string{"Test Anime"})
	idx.addEntry(AnimeEntry{AniListID: 2, Title: "test anime"}, []string{"test anime"})

	entry := idx.Lookup("test anime")
	if entry == nil {
		t.Fatal("expected match")
	}
	if entry.AniListID != 1 {
		t.Errorf("got AniListID %d, want 1 (first wins)", entry.AniListID)
	}
}

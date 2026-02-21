package animedb

import (
	"strings"
	"sync"
)

// TitleIndex provides exact-match lookup of anime titles.
type TitleIndex struct {
	mu      sync.RWMutex
	entries []AnimeEntry
	exact   map[string]*AnimeEntry
}

// newTitleIndex creates a new empty TitleIndex.
func newTitleIndex() *TitleIndex {
	return &TitleIndex{
		exact: make(map[string]*AnimeEntry),
	}
}

// addEntry adds an anime entry with its titles to the index.
// Duplicate normalized titles are ignored (first entry wins).
func (idx *TitleIndex) addEntry(entry AnimeEntry, titles []string) {
	idx.entries = append(idx.entries, entry)
	ptr := &idx.entries[len(idx.entries)-1]

	for _, t := range titles {
		norm := normalizeTitle(t)
		if norm == "" {
			continue
		}
		if _, exists := idx.exact[norm]; !exists {
			idx.exact[norm] = ptr
		}
	}
}

// Lookup searches for an anime by title. It first tries an exact match
// on the normalized title, then progressively removes trailing words
// (up to 3 removals) to handle titles with extra season/part qualifiers.
func (idx *TitleIndex) Lookup(title string) *AnimeEntry {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	norm := normalizeTitle(title)
	if norm == "" {
		return nil
	}

	if entry, ok := idx.exact[norm]; ok {
		return entry
	}

	// Progressive tail trimming: remove last word up to 3 times
	trimmed := norm
	for i := 0; i < 3; i++ {
		lastSpace := strings.LastIndex(trimmed, " ")
		if lastSpace <= 0 {
			break
		}
		trimmed = trimmed[:lastSpace]
		if entry, ok := idx.exact[trimmed]; ok {
			return entry
		}
	}

	return nil
}

// Contains returns true if the title matches any anime in the index.
func (idx *TitleIndex) Contains(title string) bool {
	return idx.Lookup(title) != nil
}

// Len returns the number of entries in the index.
func (idx *TitleIndex) Len() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.entries)
}

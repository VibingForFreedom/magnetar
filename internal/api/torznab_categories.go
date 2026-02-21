package api

import (
	"sort"

	"github.com/magnetar/magnetar/internal/classify"
	"github.com/magnetar/magnetar/internal/store"
)

// Newznab category IDs that Magnetar supports.
const (
	// Movies (2000-2099)
	CatMovies       = 2000
	CatMoviesSD     = 2030
	CatMoviesHD     = 2040
	CatMoviesUHD    = 2045
	CatMoviesBluRay = 2050
	CatMoviesWebDL  = 2070
	CatMoviesOther  = 2020

	// TV (5000-5099)
	CatTV      = 5000
	CatTVSD    = 5030
	CatTVHD    = 5040
	CatTVUHD   = 5045
	CatTVWebDL = 5010
	CatTVAnime = 5070
	CatTVOther = 5050
)

// MapToNewznab converts internal category+quality+source info to Newznab category IDs.
// Returns multiple IDs since a torrent can belong to a parent + sub-category.
func MapToNewznab(t *store.Torrent, parsed *classify.ParsedName) []int {
	if t == nil {
		return nil
	}

	var cats []int
	seen := map[int]bool{}
	add := func(id int) {
		if seen[id] {
			return
		}
		seen[id] = true
		cats = append(cats, id)
	}

	isBluRay := false
	isWebDL := false
	if parsed != nil {
		isBluRay = parsed.IsBluRay
		isWebDL = parsed.IsWebDL
	}

	switch t.Category {
	case store.CategoryMovie:
		add(CatMovies)
		switch {
		case isBluRay:
			add(CatMoviesBluRay)
		case isWebDL:
			add(CatMoviesWebDL)
		default:
			switch t.Quality {
			case store.QualitySD:
				add(CatMoviesSD)
			case store.QualityHD, store.QualityFHD:
				add(CatMoviesHD)
			case store.QualityUHD:
				add(CatMoviesUHD)
			default:
				add(CatMoviesOther)
			}
		}

	case store.CategoryTV:
		add(CatTV)
		switch {
		case isWebDL:
			add(CatTVWebDL)
		default:
			switch t.Quality {
			case store.QualitySD:
				add(CatTVSD)
			case store.QualityHD, store.QualityFHD:
				add(CatTVHD)
			case store.QualityUHD:
				add(CatTVUHD)
			default:
				add(CatTVOther)
			}
		}

	case store.CategoryAnime:
		add(CatTV)
		add(CatTVAnime)
	}

	return cats
}

// ParseNewznabCategories converts Newznab category IDs from a Torznab request
// into internal filter criteria for the store.
func ParseNewznabCategories(catIDs []int) store.SearchOpts {
	opts := store.SearchOpts{}
	if len(catIDs) == 0 {
		return opts
	}

	catSet := make(map[int]bool, len(catIDs))
	for _, id := range catIDs {
		catSet[id] = true
	}

	catSeen := map[store.Category]bool{}
	addCategory := func(cat store.Category) {
		if catSeen[cat] {
			return
		}
		catSeen[cat] = true
		opts.Categories = append(opts.Categories, cat)
	}

	qualitySeen := map[store.Quality]bool{}
	addQuality := func(q store.Quality) {
		if qualitySeen[q] {
			return
		}
		qualitySeen[q] = true
		opts.Quality = append(opts.Quality, q)
	}

	if catSet[CatMovies] {
		addCategory(store.CategoryMovie)
	}
	if catSet[CatTV] {
		addCategory(store.CategoryTV)
		addCategory(store.CategoryAnime)
	}

	if catSet[CatMoviesSD] {
		addCategory(store.CategoryMovie)
		addQuality(store.QualitySD)
	}
	if catSet[CatMoviesHD] {
		addCategory(store.CategoryMovie)
		addQuality(store.QualityHD)
		addQuality(store.QualityFHD)
	}
	if catSet[CatMoviesUHD] {
		addCategory(store.CategoryMovie)
		addQuality(store.QualityUHD)
	}
	if catSet[CatMoviesBluRay] || catSet[CatMoviesWebDL] || catSet[CatMoviesOther] {
		addCategory(store.CategoryMovie)
	}

	if catSet[CatTVSD] {
		addCategory(store.CategoryTV)
		addQuality(store.QualitySD)
	}
	if catSet[CatTVHD] {
		addCategory(store.CategoryTV)
		addQuality(store.QualityHD)
		addQuality(store.QualityFHD)
	}
	if catSet[CatTVUHD] {
		addCategory(store.CategoryTV)
		addQuality(store.QualityUHD)
	}
	if catSet[CatTVWebDL] || catSet[CatTVOther] {
		addCategory(store.CategoryTV)
	}

	if catSet[CatTVAnime] {
		addCategory(store.CategoryAnime)
	}

	if catSet[CatTVAnime] {
		hasNonAnime := catSet[CatMovies] || catSet[CatMoviesSD] || catSet[CatMoviesHD] || catSet[CatMoviesUHD] ||
			catSet[CatMoviesBluRay] || catSet[CatMoviesWebDL] || catSet[CatMoviesOther] ||
			catSet[CatTV] || catSet[CatTVSD] || catSet[CatTVHD] || catSet[CatTVUHD] || catSet[CatTVWebDL] || catSet[CatTVOther]
		if !hasNonAnime {
			opts.Categories = []store.Category{store.CategoryAnime}
			catSeen = map[store.Category]bool{store.CategoryAnime: true}
		}
	}

	if len(opts.Categories) > 1 {
		sort.Slice(opts.Categories, func(i, j int) bool {
			return opts.Categories[i] < opts.Categories[j]
		})
	}
	if len(opts.Quality) > 1 {
		sort.Slice(opts.Quality, func(i, j int) bool {
			return opts.Quality[i] < opts.Quality[j]
		})
	}

	return opts
}

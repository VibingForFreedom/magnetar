package animedb

// AnimeEntry represents a single anime with IDs from multiple sources.
type AnimeEntry struct {
	AniListID int
	KitsuID   int
	MALID     int
	AniDBID   int
	Title     string
	Year      int
}

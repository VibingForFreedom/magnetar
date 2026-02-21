package api

import "encoding/xml"

type torznabCaps struct {
	XMLName    xml.Name          `xml:"caps"`
	Server     torznabCapsServer `xml:"server"`
	Searching  torznabSearching  `xml:"searching"`
	Categories torznabCategories `xml:"categories"`
}

type torznabCapsServer struct {
	Version string `xml:"version,attr"`
	Title   string `xml:"title,attr"`
}

type torznabSearching struct {
	Search      torznabSearchType `xml:"search"`
	TVSearch    torznabSearchType `xml:"tv-search"`
	MovieSearch torznabSearchType `xml:"movie-search"`
}

type torznabSearchType struct {
	Available       string `xml:"available,attr"`
	SupportedParams string `xml:"supportedParams,attr"`
}

type torznabCategories struct {
	Categories []torznabCategory `xml:"category"`
}

type torznabCategory struct {
	ID      int             `xml:"id,attr"`
	Name    string          `xml:"name,attr"`
	Subcats []torznabSubcat `xml:"subcat"`
}

type torznabSubcat struct {
	ID   int    `xml:"id,attr"`
	Name string `xml:"name,attr"`
}

func torznabCapsResponse() torznabCaps {
	return torznabCaps{
		Server: torznabCapsServer{
			Version: "1.0",
			Title:   "Magnetar",
		},
		Searching: torznabSearching{
			Search: torznabSearchType{
				Available:       "yes",
				SupportedParams: "q",
			},
			TVSearch: torznabSearchType{
				Available:       "yes",
				SupportedParams: "q,season,ep,imdbid,tvdbid",
			},
			MovieSearch: torznabSearchType{
				Available:       "yes",
				SupportedParams: "q,imdbid,tmdbid",
			},
		},
		Categories: torznabCategories{
			Categories: []torznabCategory{
				{
					ID:   CatMovies,
					Name: "Movies",
					Subcats: []torznabSubcat{
						{ID: CatMoviesOther, Name: "Movies/Other"},
						{ID: CatMoviesSD, Name: "Movies/SD"},
						{ID: CatMoviesHD, Name: "Movies/HD"},
						{ID: CatMoviesUHD, Name: "Movies/UHD"},
						{ID: CatMoviesBluRay, Name: "Movies/BluRay"},
						{ID: CatMoviesWebDL, Name: "Movies/WEB-DL"},
					},
				},
				{
					ID:   CatTV,
					Name: "TV",
					Subcats: []torznabSubcat{
						{ID: CatTVWebDL, Name: "TV/WEB-DL"},
						{ID: CatTVSD, Name: "TV/SD"},
						{ID: CatTVHD, Name: "TV/HD"},
						{ID: CatTVUHD, Name: "TV/UHD"},
						{ID: CatTVOther, Name: "TV/Other"},
						{ID: CatTVAnime, Name: "TV/Anime"},
					},
				},
			},
		},
	}
}

package classify

import (
	"path/filepath"
	"regexp"
	"strings"
)

type Category int

const (
	CategoryMovie Category = iota
	CategoryTV
	CategoryAnime
)

func (c Category) String() string {
	switch c {
	case CategoryMovie:
		return "Movie"
	case CategoryTV:
		return "TV"
	case CategoryAnime:
		return "Anime"
	default:
		return "Unknown"
	}
}

type File struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

func Classify(name string, files []File) (Category, bool) {
	if IsAnime(name) {
		return CategoryAnime, true
	}

	if IsTV(name) {
		return CategoryTV, true
	}

	if IsMovie(name) {
		return CategoryMovie, true
	}

	if HasMediaFiles(files) {
		return guessFromFiles(files)
	}

	return CategoryMovie, false
}

func IsAnime(name string) bool {
	if animeSubGroupPattern.MatchString(name) {
		return true
	}

	n := strings.ToLower(name)

	animeKeywords := []string{
		"fansub",
		"subs",
		"anime",
		"hevc",
		"hi10p",
		"hi10",
		"10bit",
		"dual-audio",
		"dual audio",
		"uncensored",
		"unc",
		"bdrip",
		"dubbed",
	}

	for _, kw := range animeKeywords {
		if strings.Contains(n, kw) {
			if animeEpisodePattern.MatchString(name) {
				return true
			}
		}
	}

	for _, indicator := range animeIndicators {
		if indicator.MatchString(name) {
			return true
		}
	}

	return false
}

func IsTV(name string) bool {
	for _, indicator := range tvIndicators {
		if indicator.MatchString(name) {
			return true
		}
	}

	n := strings.ToLower(name)

	tvKeywords := []string{
		"s01e", "s02e", "s03e", "s04e", "s05e",
		"s06e", "s07e", "s08e", "s09e", "s10e",
		"complete series",
		"complete season",
		"season 1", "season 2", "season 3",
	}

	for _, kw := range tvKeywords {
		if strings.Contains(n, kw) {
			return true
		}
	}

	return false
}

func IsMovie(name string) bool {
	p := Parse(name)

	if p.Season != -1 || p.Episode != -1 {
		return false
	}

	for _, indicator := range movieIndicators {
		if indicator.MatchString(name) {
			return true
		}
	}

	n := strings.ToLower(name)

	movieKeywords := []string{
		"bluray",
		"blu-ray",
		"web-dl",
		"webdl",
		"remux",
		"imax",
	}

	for _, kw := range movieKeywords {
		if strings.Contains(n, kw) && p.Year > 0 {
			return true
		}
	}

	if p.Year > 0 && p.Quality != QualityUnknown {
		return true
	}

	return false
}

func HasMediaFiles(files []File) bool {
	if len(files) == 0 {
		return false
	}

	videoExts := map[string]bool{
		".mkv":  true,
		".mp4":  true,
		".avi":  true,
		".wmv":  true,
		".ts":   true,
		".m2ts": true,
		".mov":  true,
		".m4v":  true,
		".webm": true,
	}

	var videoSize, totalSize int64
	for _, f := range files {
		totalSize += f.Size
		ext := strings.ToLower(filepath.Ext(f.Path))
		if videoExts[ext] {
			videoSize += f.Size
		}
	}

	if totalSize == 0 {
		return false
	}

	return float64(videoSize)/float64(totalSize) > 0.80
}

func guessFromFiles(files []File) (Category, bool) {
	if len(files) == 0 {
		return CategoryMovie, false
	}

	videoExts := map[string]bool{
		".mkv":  true,
		".mp4":  true,
		".avi":  true,
		".wmv":  true,
		".ts":   true,
		".m2ts": true,
		".mov":  true,
		".m4v":  true,
		".webm": true,
	}

	videoCount := 0
	totalCount := len(files)
	var videoFiles []File

	for _, f := range files {
		ext := strings.ToLower(filepath.Ext(f.Path))
		if videoExts[ext] {
			videoCount++
			videoFiles = append(videoFiles, f)
		}
	}

	if videoCount == 0 {
		return CategoryMovie, false
	}

	if videoCount == 1 {
		return CategoryMovie, true
	}

	if videoCount >= 2 && videoCount <= 5 {
		episodePatterns := []*regexp.Regexp{
			regexp.MustCompile(`(?i)[\s._-](\d{1,3})(?:[\s._-]|$)`),
			regexp.MustCompile(`(?i)S\d{1,2}E\d{1,2}`),
			regexp.MustCompile(`(?i)Episode[\s._]?\d{1,3}`),
		}

		episodeMatches := 0
		for _, vf := range videoFiles {
			for _, ep := range episodePatterns {
				if ep.MatchString(vf.Path) {
					episodeMatches++
					break
				}
			}
		}

		if episodeMatches >= videoCount/2 {
			return CategoryTV, true
		}
	}

	if videoCount > 5 {
		return CategoryTV, true
	}

	confidence := float64(videoCount) / float64(totalCount)
	return CategoryMovie, confidence > 0.5
}

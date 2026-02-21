package classify

import (
	"regexp"
	"strings"
)

type ParsedName struct {
	Title      string
	Year       int
	Season     int
	Episode    int
	Quality    Quality
	Source     string
	Codec      string
	Audio      string
	HDR        string
	Group      string
	IsRemux    bool
	IsBluRay   bool
	IsWebDL    bool
	IsWebRip   bool
	IMDBID     string
	Resolution string
	SubGroup   string
}

func Parse(name string) *ParsedName {
	p := &ParsedName{
		Season:  -1,
		Episode: -1,
	}

	p.SubGroup = extractSubGroup(name)

	season, episode, found := extractSeasonEpisode(name)
	if found {
		p.Season = season
		p.Episode = episode
	} else if p.SubGroup != "" {
		p.Episode = extractAnimeEpisode(name)
	}

	p.Year = extractYear(name)
	p.Resolution = extractResolution(name)
	p.Source = extractSource(name)
	p.Codec = extractCodec(name)
	p.Audio = extractAudio(name)
	p.HDR = extractHDR(name)
	p.Group = extractGroup(name)
	p.IMDBID = extractIMDBID(name)

	if p.Resolution != "" {
		p.Quality = DetectQualityFromResolution(p.Resolution)
	} else {
		p.Quality = DetectQuality(name)
	}

	p.IsRemux = remuxPattern.MatchString(name)
	p.IsBluRay = p.Source == "bluray" || strings.Contains(strings.ToLower(name), "bluray") || strings.Contains(strings.ToLower(name), "blu-ray")
	p.IsWebDL = p.Source == "web-dl" || strings.Contains(strings.ToLower(name), "web-dl") || strings.Contains(strings.ToLower(name), "webdl")
	p.IsWebRip = p.Source == "webrip" || strings.Contains(strings.ToLower(name), "webrip")

	p.Title = extractTitle(name, p)

	return p
}

func extractTitle(name string, p *ParsedName) string {
	title := name

	extPattern := regexp.MustCompile(`\.(mkv|mp4|avi|wmv|ts|m2ts)$`)
	title = extPattern.ReplaceAllString(title, "")

	if p.SubGroup != "" {
		title = regexp.MustCompile(`(?i)^\[`+regexp.QuoteMeta(p.SubGroup)+`\]\s*`).ReplaceAllString(title, "")
	}

	title = regexp.MustCompile(`(?i)\s*\[[^\]]*\]\s*`).ReplaceAllString(title, " ")
	title = regexp.MustCompile(`(?i)\s*\([^\)]*\)\s*`).ReplaceAllString(title, " ")

	if p.Season != -1 || p.Episode != -1 {
		for _, pat := range seasonEpisodePatterns {
			title = pat.ReplaceAllString(title, " ")
		}
		for _, pat := range episodeOnlyPatterns {
			title = pat.ReplaceAllString(title, " ")
		}
	}

	if p.SubGroup != "" && p.Episode != -1 {
		title = regexp.MustCompile(`(?i)[\s._-]-[\s._]*\d+(?:v\d)?[\s._~-]*`).ReplaceAllString(title, " ")
		title = regexp.MustCompile(`(?i)[\s._~-]+\d+(?:v\d)?[\s._~-]+`).ReplaceAllString(title, " ")
	}

	if p.IMDBID != "" {
		title = regexp.MustCompile(`(?i)`+regexp.QuoteMeta(p.IMDBID)).ReplaceAllString(title, " ")
	}

	title = titleCleanPattern.ReplaceAllString(title, "")

	for {
		newTitle := titleCleanPattern.ReplaceAllString(title, "")
		if newTitle == title {
			break
		}
		title = newTitle
	}

	title = regexp.MustCompile(`(?i)[\s._-]mkv[\s._-]?$`).ReplaceAllString(title, "")

	if p.Year > 0 {
		yearStr := regexp.QuoteMeta(string(rune(p.Year/1000+'0')) + string(rune(p.Year%1000/100+'0')) + string(rune(p.Year%100/10+'0')) + string(rune(p.Year%10+'0')))
		title = regexp.MustCompile(`(?i)[\s._-]`+yearStr+`[\s._-]?$`).ReplaceAllString(title, "")
		title = regexp.MustCompile(`(?i)[\s._-]`+yearStr+`[\s._-]`).ReplaceAllString(title, " ")
	}

	title = strings.ReplaceAll(title, ".", " ")
	title = strings.ReplaceAll(title, "_", " ")

	spacePattern := regexp.MustCompile(`\s+`)
	title = spacePattern.ReplaceAllString(title, " ")

	title = strings.TrimSpace(title)

	for strings.HasPrefix(title, "-") || strings.HasPrefix(title, "_") || strings.HasPrefix(title, ".") || strings.HasPrefix(title, "~") {
		title = strings.TrimPrefix(title, "-")
		title = strings.TrimPrefix(title, "_")
		title = strings.TrimPrefix(title, ".")
		title = strings.TrimPrefix(title, "~")
		title = strings.TrimSpace(title)
	}
	for strings.HasSuffix(title, "-") || strings.HasSuffix(title, "_") || strings.HasSuffix(title, ".") || strings.HasSuffix(title, "~") {
		title = strings.TrimSuffix(title, "-")
		title = strings.TrimSuffix(title, "_")
		title = strings.TrimSuffix(title, ".")
		title = strings.TrimSuffix(title, "~")
		title = strings.TrimSpace(title)
	}

	return title
}

package classify

import (
	"regexp"
	"strings"
)

type pattern struct {
	regex *regexp.Regexp
}

func compile(s string) *regexp.Regexp {
	return regexp.MustCompile("(?i)" + s)
}

var (
	seasonEpisodePatterns = []*regexp.Regexp{
		compile(`S(\d{1,2})E(\d{1,2})(?:-E?(\d{1,2}))?`),
		compile(`S(\d{1,2})\s+E(\d{1,2})`),
		compile(`(\d{1,2})x(\d{2,3})(?:-(\d{2,3}))?`),
		compile(`Season[\s._]?(\d{1,2})[\s._-]+Episode[\s._]?(\d{1,2})`),
		compile(`Season[\s._]?(\d{1,2})(?:[\s._-]|$)`),
		compile(`S(\d{1,2})(?:[\s._-]Complete)?(?:[\s._-]|$)`),
	}

	episodeOnlyPatterns = []*regexp.Regexp{
		compile(`Episode[\s._]?(\d{1,3})`),
		compile(`[\s._-]E(\d{1,3})(?:[\s._-]|$)`),
	}

	yearInParensPattern = compile(`\((\d{4})\)`)

	yearPattern = compile(`[\s._-](\d{4})(?:[\s._-]|$)`)

	resolutionPatterns = map[string]*regexp.Regexp{
		"2160p": compile(`[\s._-](2160p|4k|uhd)(?:[\s._-]|$)`),
		"1080p": compile(`[\s._-](1080p|1080i|fhd)(?:[\s._-]|$)`),
		"720p":  compile(`[\s._-](720p|720i)(?:[\s._-]|$)`),
		"480p":  compile(`[\s._-](480p|480i|sd)(?:[\s._-]|$)`),
		"576p":  compile(`[\s._-](576p|576i)(?:[\s._-]|$)`),
	}

	resolutionOrder = []string{"2160p", "1080p", "720p", "576p", "480p"}

	sourcePatterns = map[string]*regexp.Regexp{
		"bluray":   compile(`[\s._-](BluRay|Blu-?Ray|BDRip|BDRemux|BRip|BRRip)(?:[\s._-]|$)`),
		"web-dl":   compile(`[\s._-](WEB-?DL|WEBDL)(?:[\s._-]|$)`),
		"webrip":   compile(`[\s._-](WEBRip|WEB-?Rip)(?:[\s._-]|$)`),
		"hdtv":     compile(`[\s._-](HDTV|HD-?TV)(?:[\s._-]|$)`),
		"dvdrip":   compile(`[\s._-](DVDRip|DVD-?Rip|DVDR)(?:[\s._-]|$)`),
		"dvd":      compile(`[\s._-]DVD(?:[\s._-]|$)`),
		"sdtv":     compile(`[\s._-](SDTV|DSR|PDTV)(?:[\s._-]|$)`),
		"dtheater": compile(`[\s._-]DTheater(?:[\s._-]|$)`),
	}

	codecPatterns = map[string]*regexp.Regexp{
		"av1":   compile(`[\s._-]AV1(?:[\s._-]|$)`),
		"hevc":  compile(`[\s._-](HEVC|H\.?265|x265)(?:[\s._-]|$)`),
		"h265":  compile(`[\s._-](H\.?265|x265|HEVC)(?:[\s._-]|$)`),
		"h264":  compile(`[\s._-](H\.?264|x264|AVC)(?:[\s._-]|$)`),
		"vp9":   compile(`[\s._-]VP9(?:[\s._-]|$)`),
		"xvid":  compile(`[\s._-]XviD(?:[\s._-]|$)`),
		"divx":  compile(`[\s._-]DivX(?:[\s._-]|$)`),
		"mpeg2": compile(`[\s._-]MPEG-?2(?:[\s._-]|$)`),
	}

	codecOrder = []string{"av1", "hevc", "h265", "h264", "vp9", "xvid", "divx", "mpeg2"}

	audioPatterns = map[string]*regexp.Regexp{
		"atmos":  compile(`[\s._-]Atmos(?:[\s._-]|$)`),
		"truehd": compile(`[\s._-]TrueHD(?:[\s._-]|$)`),
		"dts-ma": compile(`[\s._-](DTS-?HD[\s._-]?MA|DTSMA)(?:[\s._-]|$)`),
		"dts-hd": compile(`[\s._-](DTS-?HD|DTSHD)(?:[\s._-]|$)`),
		"dts-x":  compile(`[\s._-](DTS[\s._-]?X|DTSX)(?:[\s._-]|$)`),
		"dts":    compile(`[\s._-]DTS(?:[\s._-]|$)`),
		"ddp5.1": compile(`[\s._-](DDP5\.?1|DDP\.?5\.?1|DD\+5\.?1)(?:[\s._-]|$)`),
		"dd5.1":  compile(`[\s._-](DD5\.?1|DD5|DD\.?5\.?1|DolbyDigital5\.?1)(?:[\s._-]|$)`),
		"ac3":    compile(`[\s._-]AC-?3(?:[\s._-]|$)`),
		"aac":    compile(`[\s._-]AAC(?:[\s._-]|$)`),
		"flac":   compile(`[\s._-]FLAC(?:[\s._-]|$)`),
		"lpcm":   compile(`[\s._-]LPCM(?:[\s._-]|$)`),
	}

	audioOrder = []string{"atmos", "truehd", "dts-ma", "dts-hd", "dts-x", "dts", "ddp5.1", "dd5.1", "ac3", "aac", "flac", "lpcm"}

	hdrPatterns = map[string]*regexp.Regexp{
		"dolby-vision": compile(`[\s._-](Dolby[\s._-]?Vision|DoVi)(?:[\s._-]|$)`),
		"dv":           compile(`[\s._-]DV(?:[\s._-]|$)`),
		"hdr10+":       compile(`[\s._-]HDR10\+(?:[\s._-]|$)`),
		"hdr10":        compile(`[\s._-]HDR10(?:[\s._-]|$)`),
		"hdr":          compile(`[\s._-]HDR(?:[\s._-]|$)`),
		"bt2020":       compile(`[\s._-]BT\.?2020(?:[\s._-]|$)`),
		"hybrid":       compile(`[\s._-]Hybrid(?:[\s._-]|$)`),
	}

	hdrOrder = []string{"dolby-vision", "hdr10+", "hdr10", "dv", "hdr", "bt2020", "hybrid"}

	remuxPattern = compile(`[\s._-](REMUX|Remux)(?:[\s._-]|$)`)

	groupPattern = compile(`[\s._-]([A-Za-z][A-Za-z0-9]{1,19})$`)

	animeSubGroupPattern = compile(`^\[([^\]]+)\]`)

	animeEpisodePattern = compile(`[\s._-](\d{2,4})(?:v\d)?(?:[\s._\[(]|$)`)

	imdbIDPattern = compile(`(tt\d{7,9})`)

	animeIndicators = []*regexp.Regexp{
		compile(`^\[[^\]]+\]`),
		compile(`(?i)\[.*?(?:fansub|subs?|anime)\]`),
		compile(`(?i)[\s._-](10bit|Hi10P|Hi10|Hi444PP)[\s._-]`),
		compile(`(?i)[\s._-](Dual[\s._-]?Audio)[\s._-]`),
		compile(`(?i)[\s._-](UNCENSORED|Uncensored)[\s._-]`),
	}

	tvIndicators = []*regexp.Regexp{
		compile(`(?i)S\d{1,2}E\d{1,2}`),
		compile(`(?i)Season[\s._]?\d{1,2}`),
		compile(`(?i)Complete[\s._-]?Series`),
		compile(`(?i)\d{1,2}x\d{2,3}`),
		compile(`(?i)Episode[\s._]?\d{1,3}`),
		compile(`(?i)[\s._-]EP?(\d{1,3})[\s._-]`),
	}

	movieIndicators = []*regexp.Regexp{
		compile(`(?i)\(\d{4}\).*(?:480p|720p|1080p|2160p|4K|UHD)`),
		compile(`(?i)(?:480p|720p|1080p|2160p|4K|UHD).*tt\d{7,9}`),
	}

	crcPattern = compile(`\[([A-Fa-f0-9]{8})\]`)

	trailingCleanPattern = compile(`[\s._-]*(H\.?264|H\.?265|x264|x265|HEVC|AV1|XviD|DivX|MPEG-?2|AAC|AC-?3|DTS|DDP|DD|Atmos|TrueHD|HDR|DV|Dolby|REMUX|Remux|HDTV|WEB|DVD|BluRay|NF|AMZN|HMAX|DSNP|ATVP|iT|MPX)[\s._\w]*$`)

	endReleasePattern = compile(`[\s._-]+(?:H\.?264|H\.?265|x264|x265|HEVC|AV1|XviD|DivX|MPEG-?2|AAC|AC-?3|DTS|DDP5\.?1|DD5\.?1|DD|Atmos|TrueHD|HDR10\+|HDR10|HDR|DV|Dolby|REMUX|Remux|WEB-?DL|WEBRip|HDTV|BluRay|DVDRip|BDRip|BRRip|NF|AMZN|HMAX|DSNP|ATVP)[\s._\w\-]*$`)

	titleCleanPattern = compile(`(?i)[\s._-]+(?:480p|720p|1080p|1080i|2160p|4k|uhd|fhd|sd|bluray|blu-?ray|web-?dl|webdl|webrip|hdtv|dvdrip|bdrip|brrip|brip|bdremux|remux|h\.?264|h\.?265|x264|x265|hevc|av1|xvid|divx|mpeg-?2|aac|ac-?3|dts-?hd|dts-?ma|dts|ddp5\.?1|dd5\.?1|dd|atmos|truehd|hdr10\+|hdr10|hdr|dv|dolby|nf|amzn|hmax|dsnp|atvp|dts[\s._-]?x|dts[\s._-]?hd[\s._-]?ma|dolby[\s._-]?vision|dual[\s._-]?audio)[\s._\w\-\+\~]*$`)
)

func extractSeasonEpisode(name string) (season, episode int, found bool) {
	season, episode = -1, -1

	for _, p := range seasonEpisodePatterns {
		matches := p.FindStringSubmatch(name)
		if matches != nil {
			if len(matches) >= 2 && matches[1] != "" {
				season = parseInt(matches[1])
				found = true
			}
			if len(matches) >= 3 && matches[2] != "" {
				episode = parseInt(matches[2])
			}
			if season != -1 {
				return season, episode, found
			}
		}
	}

	return season, episode, found
}

func extractYear(name string) int {
	if m := yearInParensPattern.FindStringSubmatch(name); m != nil {
		year := parseInt(m[1])
		if isValidYear(year) {
			return year
		}
	}

	allYears := yearPattern.FindAllStringSubmatch(name, -1)
	for i := len(allYears) - 1; i >= 0; i-- {
		year := parseInt(allYears[i][1])
		if isValidYear(year) {
			return year
		}
	}

	return 0
}

func extractResolution(name string) string {
	for _, res := range resolutionOrder {
		if p, ok := resolutionPatterns[res]; ok && p.MatchString(name) {
			return res
		}
	}
	return ""
}

func extractSource(name string) string {
	for source, p := range sourcePatterns {
		if p.MatchString(name) {
			return source
		}
	}
	return ""
}

func extractCodec(name string) string {
	for _, codec := range codecOrder {
		if p, ok := codecPatterns[codec]; ok && p.MatchString(name) {
			return codec
		}
	}
	return ""
}

func extractAudio(name string) string {
	for _, audio := range audioOrder {
		if p, ok := audioPatterns[audio]; ok && p.MatchString(name) {
			return audio
		}
	}
	return ""
}

func extractHDR(name string) string {
	for _, hdr := range hdrOrder {
		if p, ok := hdrPatterns[hdr]; ok && p.MatchString(name) {
			if hdr == "dv" {
				return "dolby-vision"
			}
			return hdr
		}
	}
	return ""
}

func extractGroup(name string) string {
	cleaned := strings.TrimSuffix(name, ".mkv")
	cleaned = strings.TrimSuffix(cleaned, ".mp4")
	cleaned = strings.TrimSuffix(cleaned, ".avi")

	cleaned = titleCleanPattern.ReplaceAllString(cleaned, "")

	matches := groupPattern.FindStringSubmatch(cleaned)
	if matches != nil {
		return matches[1]
	}

	parts := strings.FieldsFunc(cleaned, func(r rune) bool {
		return r == '-' || r == '_'
	})
	if len(parts) >= 2 {
		group := parts[len(parts)-1]
		if len(group) >= 2 && len(group) <= 20 {
			return group
		}
	}

	return ""
}

func extractSubGroup(name string) string {
	matches := animeSubGroupPattern.FindStringSubmatch(name)
	if matches != nil {
		return matches[1]
	}
	return ""
}

func extractIMDBID(name string) string {
	matches := imdbIDPattern.FindStringSubmatch(name)
	if matches != nil {
		return matches[1]
	}
	return ""
}

func extractAnimeEpisode(name string) int {
	subGroupRemoved := animeSubGroupPattern.ReplaceAllString(name, "")
	matches := animeEpisodePattern.FindAllStringSubmatch(subGroupRemoved, -1)
	if len(matches) > 0 {
		for _, m := range matches {
			ep := parseInt(m[1])
			if ep > 0 && ep < 10000 {
				return ep
			}
		}
	}
	return -1
}

func parseInt(s string) int {
	var result int
	for _, c := range strings.TrimSpace(s) {
		if c >= '0' && c <= '9' {
			result = result*10 + int(c-'0')
		} else {
			break
		}
	}
	return result
}

func isValidYear(year int) bool {
	return year >= 1900 && year <= 2100
}

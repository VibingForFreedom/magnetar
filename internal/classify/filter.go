package classify

import (
	"path/filepath"
	"regexp"
	"strings"
)

type Category int

const (
	CategoryMovie   Category = 0
	CategoryTV      Category = 1
	CategoryAnime   Category = 2
	CategoryUnknown Category = 3
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

// Extension sets for file classification.
// Video and allowed extensions are kept intentionally separate so that
// a torrent with only subtitles (no video) is still rejected.
var (
	videoExts = map[string]bool{
		".mkv":  true,
		".mp4":  true,
		".avi":  true,
		".wmv":  true,
		".ts":   true,
		".m2ts": true,
		".mov":  true,
		".m4v":  true,
		".webm": true,
		".flv":  true,
		".mpg":  true,
		".mpeg": true,
		".vob":  true,
		".ogv":  true,
	}

	// Files that commonly accompany media and should NOT count as junk.
	// These are harmless metadata, subtitles, cover art, etc.
	allowedExts = map[string]bool{
		// Subtitles
		".srt": true, ".sub": true, ".ass": true, ".ssa": true,
		".idx": true, ".sup": true, ".vtt": true,
		// Metadata
		".nfo": true, ".txt": true, ".sfv": true,
		// Cover art / images
		".jpg": true, ".jpeg": true, ".png": true, ".bmp": true,
		// Audio (soundtrack, audio tracks)
		".mp3": true, ".flac": true, ".aac": true, ".ac3": true,
		".dts": true, ".ogg": true, ".wav": true, ".m4a": true,
		".wma": true, ".opus": true, ".eac3": true,
		// Disc structures
		".ifo": true, ".bup": true,
		// Samples
		".sample": true,
	}

	// Extensions that are strong signals the torrent is NOT media.
	// If these dominate by size, we reject the torrent.
	junkExts = map[string]bool{
		// Archives
		".rar": true, ".zip": true, ".7z": true, ".tar": true,
		".gz": true, ".bz2": true, ".xz": true, ".zst": true,
		".cab": true, ".ace": true,
		// Executables / installers
		".exe": true, ".msi": true, ".bat": true, ".cmd": true,
		".com": true, ".scr": true, ".pif": true, ".ps1": true,
		".sh": true,
		// Mobile
		".apk": true, ".ipa": true, ".xapk": true, ".aab": true,
		// Disk images
		".iso": true, ".img": true, ".dmg": true, ".vhd": true,
		".vmdk": true, ".qcow2": true, ".bin": true, ".cue": true,
		".nrg": true, ".mdf": true, ".mds": true,
		// Libraries / code
		".dll": true, ".so": true, ".dylib": true, ".sys": true,
		".drv": true, ".class": true, ".jar": true, ".war": true,
		".deb": true, ".rpm": true, ".pkg": true, ".snap": true,
		".appimage": true, ".flatpak": true,
		// Documents (not media)
		".pdf": true, ".doc": true, ".docx": true, ".epub": true,
		".mobi": true, ".azw3": true, ".cbr": true, ".cbz": true,
		".xls": true, ".xlsx": true, ".ppt": true, ".pptx": true,
		// Games
		".pak": true, ".vpk": true, ".wad": true, ".bsp": true,
		".unity3d": true, ".assets": true, ".dat": true,
		// Torrent / misc
		".torrent": true, ".lnk": true,
	}

	// Name patterns that strongly indicate non-media content.
	junkNamePatterns = []*regexp.Regexp{
		compile(`(?i)[\s._-](crack|keygen|patch|serial|activat|loader)[\s._-]`),
		compile(`(?i)[\s._-](portable|repack|setup|install)[\s._-]`),
		compile(`(?i)\b(v\d+\.\d+|x86|x64|x86_64|amd64|arm64)\b`),
		compile(`(?i)[\s._-](apk|android|ios)[\s._-]`),
		compile(`(?i)(windows|win7|win10|win11|macos|linux)[\s._-]`),
		compile(`(?i)[\s._-](nulled|cracked|warez|scene)[\s._-]`),
		compile(`(?i)(adobe|autodesk|microsoft|office)\s+(20\d{2}|cs\d|cc)`),
	}
)

func Classify(name string, files []File) Category {
	if IsAnime(name) {
		return CategoryAnime
	}

	if IsTV(name) {
		return CategoryTV
	}

	if IsMovie(name) {
		return CategoryMovie
	}

	if HasMediaFiles(files) {
		cat, confident := guessFromFiles(files)
		if confident {
			return cat
		}
	}

	return CategoryUnknown
}

// IsJunk returns true if the torrent is clearly not media content.
// It checks both the torrent name and file extensions.
// This should be called BEFORE Classify to avoid wasting work.
func IsJunk(name string, files []File) bool {
	// Name-based rejection: strong software/game signals
	for _, p := range junkNamePatterns {
		if p.MatchString(name) {
			// Don't reject if the name ALSO has strong media signals
			if hasMediaSignals(name) {
				return false
			}
			return true
		}
	}

	// No files to check — can't determine from extensions alone
	if len(files) == 0 {
		return false
	}

	// Extension-based rejection: if junk files dominate by size,
	// the torrent is not media even if it has a few video files.
	var junkSize, videoSize, totalSize int64
	for _, f := range files {
		totalSize += f.Size
		ext := strings.ToLower(filepath.Ext(f.Path))
		if junkExts[ext] {
			junkSize += f.Size
		} else if videoExts[ext] {
			videoSize += f.Size
		}
	}

	if totalSize == 0 {
		return false
	}

	// If junk extensions make up >50% of total size, reject
	if float64(junkSize)/float64(totalSize) > 0.50 {
		return true
	}

	// If there are files but zero video and zero audio, reject
	// (pure subtitle/nfo packs are useless without video)
	if videoSize == 0 && len(files) > 0 {
		var audioSize int64
		for _, f := range files {
			ext := strings.ToLower(filepath.Ext(f.Path))
			if ext == ".mp3" || ext == ".flac" || ext == ".ogg" || ext == ".wav" || ext == ".m4a" || ext == ".opus" || ext == ".wma" {
				audioSize += f.Size
			}
		}
		// No video and no significant audio — not media
		if audioSize == 0 {
			return true
		}
	}

	return false
}

// hasMediaSignals returns true if the torrent name has strong
// indicators of being a movie/TV release, which should override
// junk name pattern matches (e.g., "Repack" is used in both
// scene releases and software).
func hasMediaSignals(name string) bool {
	p := Parse(name)
	if p.Year > 0 && p.Quality != QualityUnknown {
		return true
	}
	if p.Season != -1 || p.Episode != -1 {
		return true
	}
	// Check for scene release patterns
	for _, indicator := range movieIndicators {
		if indicator.MatchString(name) {
			return true
		}
	}
	for _, indicator := range tvIndicators {
		if indicator.MatchString(name) {
			return true
		}
	}
	return false
}

func IsAnime(name string) bool {
	n := strings.ToLower(name)

	// Strong anime keywords — if present with episode numbering, it's anime
	animeKeywords := []string{
		"fansub",
		"anime",
		"hi10p",
		"hi10",
		"dual-audio",
		"dual audio",
	}

	for _, kw := range animeKeywords {
		if strings.Contains(n, kw) {
			if animeEpisodePattern.MatchString(name) {
				return true
			}
		}
	}

	// [SubGroup] Title - Episode pattern (must have episode number)
	if animeSubGroupPattern.MatchString(name) && animeEpisodePattern.MatchString(name) {
		return true
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

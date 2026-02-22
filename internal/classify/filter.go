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
		// Music audio formats / bitrates
		compile(`(?i)(?:^|[\s._\[-])(FLAC|MP3|320kbps|V0|CBR|VBR|WEB-FLAC|Lossless|Hi-?Res|24bit|16bit|44\.1kHz|48kHz|96kHz|192kHz)(?:[\s._\]-]|$)`),
		// Music release types
		compile(`(?i)(?:^|[\s._\[-])(Discography|Discografia|Mixtape|Greatest\.?Hits|Best\.?Of|Live\.?Album)(?:[\s._\]-]|$)`),
	}

	// Adult content patterns — JAV codes, porn studios, explicit keywords.
	adultPatterns = []*regexp.Regexp{
		// JAV codes: 2-10 alphanumeric chars followed by hyphen and 3-5 digits, optionally with suffix
		// Matches both traditional (CAWD-507) and numeric-prefix (529STCV-216) codes
		compile(`(?:^|[\s._\[@])([A-Z0-9]{2,10})-(\d{3,5})(?:-?[A-Z])?(?:[\s._\]@]|$)`),
		// JAV codes after domain watermarks: "domain.com@CODE-123"
		compile(`\.(?:com|net|org|cc|io|me|xyz|top)@([A-Z0-9]{2,10})-(\d{3,5})(?:-?[A-Z])?(?:[\s._\]@]|$)`),
		// Japanese adult-specific terms (uncensored excluded — used in anime)
		compile(`(?i)(?:^|[\s._-])(無修正|中出し|潮吹き|痴女|素人|熟女|巨乳|美乳|爆乳|淫乱|変態|近親相姦|人妻)(?:[\s._-]|$)`),
		// Porn studios / sites
		compile(`(?i)(?:^|[\s._\[-])(Brazzers|BangBros|RealityKings|Nubiles|MetArt|Hegre|FakeTaxi|Blacked|Tushy|Vixen|Deeper|AllGirlMassage|MomsBangTeens|BadDaddyPOV|PornFidelity|SexMex|NewSensations|LegalPorno|PornWorld|WhenGirlsPlay|DigitalPlayground|NaughtyAmerica|Babes\.com|MoFos|TeamSkeet|Perv|FTVGirls|Karups|ATKGirlfriends|GirlsWay|HardX|EvilAngel|JulesJordan|Milfy|FiLF|PublicPickups|OldGoesYoung|FacialAbuse|WhiteTeensBlackCocks|MuchaSexo|PrivateSociety|ALSScan|OnlyFans|DontBreakMe|Mofos|PropertySex|FakeHub|Nubile[s]?Films|DadCrush|SisLovesMe|MyFamilyPies|PureTaboo|Vixenplus|Slayed|Twistys|PassionHD|FantasyHD|CastingCouch|ExploitedCollegeGirls|GloryholeSwallow|SpankBang|Caribbeancom|1Pondo|Heyzo|TokyoHot|Pacopacomama|10Musume|Kin8tengoku|PornPros|DaneJones|SexArt|AllFineGirls|PureMature|Lubed|Holed|Tiny4K|GirlsDoPorn|Playboy|Penthouse|DigitalDesire|Scoreland|DDFNetwork|AnalVids|GonzoXXX|ManuelFerrara|Dorcel|Private\.com|Wicked|SweetSinner|MissaX|FamilyStrokes|StepSiblings|BrattySis)[\s._\]-]`),
		// FC2-PPV codes (Japanese amateur adult pay-per-view)
		compile(`(?i)(?:^|[\s._-])FC2[-_]?PPV[-_]?\d{5,}`),
		// JAV codes with quality prefix: "[FHD]CAWD-507", "[HD]ABW-123"
		// Placed outside anime-skip range since [FHD]+JAV code is unambiguous
		compile(`\[(?:FHD|HD|SD|4K|UHD)\]\s*([A-Z0-9]{2,10})-(\d{3,5})`),
		// Explicit content keywords
		compile(`(?i)(?:^|[\s._-])(XXX|porn|hentai|jav|erotic[ao]?|gangbang|creampie|blowjob|handjob|deepthroat|bukkake|BDSM|femdom|cuckold|futanari|ahegao|orgasm|masturbat|dildo|vibrator|anal\.?sex|oral\.?sex|squirt(?:ing)?|cumshot|threesome|foursome|double.?penetration|ass.?fuck|face.?fuck|tit.?fuck|rough.?sex|interracial|hotwife|swinger)(?:[\s._-]|$)`),
		// Chinese/Japanese adult indicators
		compile(`(?:骚|淫|肏|屄|鸡巴|阴茎|阴道|做爱|口交|肛交|中出|颜射|潮吹|痴女|素人|熟女|巨乳|爆乳|无码|有码|抽插|啪啪|约炮|嫩穴|荡妇|淫妻|绿帽|裸聊|自慰|大奶|美鲍|馒头逼|丝袜诱惑|失禁|大尺度|福利姬|操到|内射|白虎|粉穴|美臀|翘臀|骑乘|后入|口爆|吞精|足交|乳交|3P|多P|群P|调教|捆绑|露出|野战|车震|偷拍|走光|透视|鱼香肉丝|蜜桃|果冻|麻豆|探花)`),
		// Chinese adult leak / amateur patterns
		compile(`(?:推特.*粉丝.*福利|门槛福利|反差婊|极品.*母狗|阿黑颜|欺负操|狠狠.*操|全网.*裸聊|最新流出.*大尺度|宿舍.*诱惑)`),
		// Known hentai anime titles (adult content that looks like regular anime)
		compile(`(?:^|[\s._\[-])(Otomedori|Koutetsu no Majo Annerose|Kuroinu|Euphoria|Starless|Bible Black|La Blue Girl|Words Worth|Discipline|Stringendo|Resort Boin|Kanojo x Kanojo|Overflow|Rance|Mokkai Shiyo|Sei Yariman|Dropout|JK to Ero Konbini|Mankitsu Happening|Baku Ane|Shikkoku no Shaga|Tsumamigui|Kansen Ball Buster|Kyonyuu|Eroge|Itadaki Seieki|Shoujo Ramune|Maki-chan to Nau)(?:[\s._\]-]|$)`),
		// Adult site watermarks and prolific adult uploaders
		compile(`(?i)(?:sex8\.cc|91porn|pornhub|xvideos|xhamster|xnxx|javbus|javlib|avgle|18p2p|hjd2048|69av|theporn|selang|suu55|色狼网|性吧|swag\.live|麻豆传媒|蜜桃传媒|果冻传媒|星空传媒|天美传媒|madoubt\.com|caribbeancom|1pondo|heyzo|tokyohot|10musume|kin8tengoku|pacopacomama|383229\.xyz|1024core|t66y|thz\.la|sehuatang|onejav|javmost|javfinder|javguru|javhdporn|javbangers|javhd\.pro|supjav|missav|jable\.tv|XiuRen|MyGirl|Ugirls|Youmi|Xiaoyu|HuaYang|MFStar|MiStar|51vv|91porn\.com|caoliu|1024cl|t66y\.com|草榴|色中色|香蕉社区|Yurievij)`),
		// Online courses / tutorials (not media)
		compile(`(?i)(?:^|[\s._-])(Udemy|Coursera|Masterclass|Tutorial|Bootcamp|Course|Certification|Learn\s+\w+\s+in|Complete\s+Guide|from\s+Zero\s+to\s+Hero)(?:[\s._-]|$)`),
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

// IsAdult returns true if the torrent name matches adult content patterns.
// For anime-looking names (bracket groups), only JAV code patterns are skipped
// since they overlap with anime episode codes like [ABCD-123].
// Studios, explicit keywords, and site watermarks are always checked.
func IsAdult(name string) bool {
	isAnime := IsAnime(name)
	for i, p := range adultPatterns {
		// Skip JAV code patterns (indices 0-1) for anime-looking names
		// since they overlap with anime fansub group tags
		if isAnime && i <= 1 {
			continue
		}
		if p.MatchString(name) {
			return true
		}
	}
	return false
}

// IsJunk returns true if the torrent is clearly not media content.
// It checks both the torrent name and file extensions.
// This should be called BEFORE Classify to avoid wasting work.
func IsJunk(name string, files []File) bool {
	// Adult content detection (pattern-based — JAV codes, studios, keywords)
	if filterCfg.FilterAdultPatterns && IsAdult(name) {
		return true
	}

	// Name-based rejection: strong software/game signals
	if filterCfg.FilterJunkNames {
		for _, p := range junkNamePatterns {
			if p.MatchString(name) {
				// Don't reject if the name ALSO has strong media signals
				if hasMediaSignals(name) {
					break
				}
				return true
			}
		}
	}

	// Adult name detection: only check bare names without media signals.
	// Legit media like "Jayden.Lee.2024.1080p.WEB-DL" has signals and skips this.
	// A bare "Jayden Lee" has zero signals and gets checked against performer/studio lists.
	if filterCfg.FilterAdultNames && !hasMediaSignals(name) && isAdultName(name) {
		return true
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

	// If there are files but zero video, check what's left
	if videoSize == 0 && len(files) > 0 {
		var audioSize int64
		var hasSubtitles bool
		for _, f := range files {
			ext := strings.ToLower(filepath.Ext(f.Path))
			if ext == ".mp3" || ext == ".flac" || ext == ".ogg" || ext == ".wav" || ext == ".m4a" || ext == ".opus" || ext == ".wma" || ext == ".aac" || ext == ".ape" || ext == ".alac" {
				audioSize += f.Size
			}
			if ext == ".srt" || ext == ".ass" || ext == ".ssa" || ext == ".sub" || ext == ".idx" || ext == ".sup" {
				hasSubtitles = true
			}
		}
		// No video and no audio — not media (pure subtitle/nfo packs)
		if audioSize == 0 {
			return true
		}
		// Audio-only with no subtitles = music album (not media for this tracker)
		// Exception: keep if name has media signals (could be audio drama with SxxExx)
		if !hasSubtitles && !hasMediaSignals(name) {
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

// isAdultName checks if a normalized title matches known adult performer names,
// studio names, or dirty keywords from the generated adult_data.go maps.
// It should only be called for titles WITHOUT media signals to avoid false positives.
func isAdultName(name string) bool {
	normalized := normalizeForAdultCheck(name)
	if normalized == "" {
		return false
	}

	// Check full name against performer map
	if adultPerformers[normalized] {
		return true
	}

	// Check full name against studio map
	if adultStudios[normalized] {
		return true
	}

	// Check each word against keywords
	words := strings.Fields(normalized)
	for _, w := range words {
		if adultKeywords[w] {
			return true
		}
	}

	// Sliding window bigram check against performers
	// e.g., "Jayden Lee Hardcore" → check "jayden lee", "lee hardcore"
	if len(words) >= 2 {
		for i := range len(words) - 1 {
			bigram := words[i] + " " + words[i+1]
			if adultPerformers[bigram] {
				return true
			}
		}
	}

	// Check trigrams for performers with three-word names
	if len(words) >= 3 {
		for i := range len(words) - 2 {
			trigram := words[i] + " " + words[i+1] + " " + words[i+2]
			if adultPerformers[trigram] {
				return true
			}
		}
	}

	return false
}

// normalizeForAdultCheck prepares a torrent name for adult content lookup:
// lowercase, strip watermarks, collapse separators to spaces, trim.
func normalizeForAdultCheck(name string) string {
	// Strip domain watermark prefixes
	name = siteWatermarkPattern.ReplaceAllString(name, "")
	name = siteAtWatermarkPattern.ReplaceAllString(name, "")
	name = siteDomainAtWatermarkPattern.ReplaceAllString(name, "")

	name = strings.ToLower(name)

	// Replace common separators with spaces
	name = strings.NewReplacer(
		".", " ",
		"_", " ",
		"-", " ",
		"~", " ",
	).Replace(name)

	// Collapse whitespace
	fields := strings.Fields(name)
	return strings.Join(fields, " ")
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
		"temporada", "temp.", "cap.",
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

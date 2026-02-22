package classify

import (
	"testing"
)

func TestParseMovie(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantTitle   string
		wantYear    int
		wantSeason  int
		wantEpisode int
		wantQuality Quality
		wantSource  string
		wantCodec   string
		wantAudio   string
		wantHDR     string
		wantIMDBID  string
		wantRes     string
		wantRemux   bool
		wantBluRay  bool
		wantWebDL   bool
		wantWebRip  bool
	}{
		{
			name:        "Oppenheimer 4K WEB-DL",
			input:       "Oppenheimer.2023.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-GROUP",
			wantTitle:   "Oppenheimer",
			wantYear:    2023,
			wantSeason:  -1,
			wantEpisode: -1,
			wantQuality: QualityUHD,
			wantSource:  "web-dl",
			wantCodec:   "hevc",
			wantAudio:   "atmos",
			wantHDR:     "dolby-vision",
			wantIMDBID:  "",
			wantRes:     "2160p",
			wantRemux:   false,
			wantBluRay:  false,
			wantWebDL:   true,
			wantWebRip:  false,
		},
		{
			name:        "Movie 1080p BluRay",
			input:       "The.Matrix.1999.1080p.BluRay.REMUX.DTS-HD.MA.5.1.AVC-GROUP",
			wantTitle:   "The Matrix",
			wantYear:    1999,
			wantSeason:  -1,
			wantEpisode: -1,
			wantQuality: QualityFHD,
			wantSource:  "bluray",
			wantCodec:   "h264",
			wantAudio:   "dts-ma",
			wantRes:     "1080p",
			wantRemux:   true,
			wantBluRay:  true,
			wantWebDL:   false,
			wantWebRip:  false,
		},
		{
			name:        "Movie with IMDB ID",
			input:       "Dune.Part.Two.2024.1080p.WEB-DL.DDP5.1.Atmos.H.264-GROUP_tt15239678",
			wantTitle:   "Dune Part Two",
			wantYear:    2024,
			wantSeason:  -1,
			wantEpisode: -1,
			wantQuality: QualityFHD,
			wantSource:  "web-dl",
			wantCodec:   "h264",
			wantAudio:   "atmos",
			wantIMDBID:  "tt15239678",
			wantWebDL:   true,
		},
		{
			name:        "Movie with Dolby Vision",
			input:       "Movie.2023.1080p.WEB-DL.Dolby.Vision.H.265-GROUP",
			wantTitle:   "Movie",
			wantYear:    2023,
			wantSeason:  -1,
			wantEpisode: -1,
			wantQuality: QualityFHD,
			wantSource:  "web-dl",
			wantHDR:     "dolby-vision",
			wantWebDL:   true,
		},
		{
			name:        "Movie 4K with HDR10+",
			input:       "Avatar.The.Way.of.Water.2022.2160p.WEB-DL.DV.HDR10+.HEVC.DDP5.1.Atmos-GROUP",
			wantTitle:   "Avatar The Way of Water",
			wantYear:    2022,
			wantSeason:  -1,
			wantEpisode: -1,
			wantQuality: QualityUHD,
			wantSource:  "web-dl",
			wantCodec:   "hevc",
			wantAudio:   "atmos",
			wantHDR:     "hdr10+",
			wantRes:     "2160p",
			wantWebDL:   true,
		},
		{
			name:        "SD Movie",
			input:       "Old.Movie.2005.DVDRip.XviD-GROUP",
			wantTitle:   "Old Movie",
			wantYear:    2005,
			wantSeason:  -1,
			wantEpisode: -1,
			wantQuality: QualitySD,
			wantSource:  "dvdrip",
			wantCodec:   "xvid",
		},
		{
			name:        "720p HDTV",
			input:       "Some.TV.Show.2010.720p.HDTV.x264-GROUP",
			wantTitle:   "Some TV Show",
			wantYear:    2010,
			wantSeason:  -1,
			wantEpisode: -1,
			wantQuality: QualityHD,
			wantSource:  "hdtv",
			wantCodec:   "h264",
			wantRes:     "720p",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)

			if got.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", got.Title, tt.wantTitle)
			}
			if got.Year != tt.wantYear {
				t.Errorf("Year = %d, want %d", got.Year, tt.wantYear)
			}
			if got.Season != tt.wantSeason {
				t.Errorf("Season = %d, want %d", got.Season, tt.wantSeason)
			}
			if got.Episode != tt.wantEpisode {
				t.Errorf("Episode = %d, want %d", got.Episode, tt.wantEpisode)
			}
			if got.Quality != tt.wantQuality {
				t.Errorf("Quality = %v, want %v", got.Quality, tt.wantQuality)
			}
			if tt.wantSource != "" && got.Source != tt.wantSource {
				t.Errorf("Source = %q, want %q", got.Source, tt.wantSource)
			}
			if tt.wantCodec != "" && got.Codec != tt.wantCodec {
				t.Errorf("Codec = %q, want %q", got.Codec, tt.wantCodec)
			}
			if tt.wantAudio != "" && got.Audio != tt.wantAudio {
				t.Errorf("Audio = %q, want %q", got.Audio, tt.wantAudio)
			}
			if tt.wantHDR != "" && got.HDR != tt.wantHDR {
				t.Errorf("HDR = %q, want %q", got.HDR, tt.wantHDR)
			}
			if tt.wantIMDBID != "" && got.IMDBID != tt.wantIMDBID {
				t.Errorf("IMDBID = %q, want %q", got.IMDBID, tt.wantIMDBID)
			}
			if tt.wantRes != "" && got.Resolution != tt.wantRes {
				t.Errorf("Resolution = %q, want %q", got.Resolution, tt.wantRes)
			}
			if got.IsRemux != tt.wantRemux {
				t.Errorf("IsRemux = %v, want %v", got.IsRemux, tt.wantRemux)
			}
			if got.IsBluRay != tt.wantBluRay {
				t.Errorf("IsBluRay = %v, want %v", got.IsBluRay, tt.wantBluRay)
			}
			if got.IsWebDL != tt.wantWebDL {
				t.Errorf("IsWebDL = %v, want %v", got.IsWebDL, tt.wantWebDL)
			}
			if got.IsWebRip != tt.wantWebRip {
				t.Errorf("IsWebRip = %v, want %v", got.IsWebRip, tt.wantWebRip)
			}
		})
	}
}

func TestParseTV(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantTitle   string
		wantYear    int
		wantSeason  int
		wantEpisode int
		wantQuality Quality
		wantSource  string
	}{
		{
			name:        "TV Show S01E01",
			input:       "The.Last.of.Us.S01E01.1080p.WEB-DL.DDP5.1.H.264-GROUP",
			wantTitle:   "The Last of Us",
			wantYear:    0,
			wantSeason:  1,
			wantEpisode: 1,
			wantQuality: QualityFHD,
			wantSource:  "web-dl",
		},
		{
			name:        "TV Show with year",
			input:       "House.of.the.Dragon.S02E01.2024.1080p.WEB-DL.DDP5.1-GROUP",
			wantTitle:   "House of the Dragon",
			wantYear:    2024,
			wantSeason:  2,
			wantEpisode: 1,
			wantQuality: QualityFHD,
			wantSource:  "web-dl",
		},
		{
			name:        "TV Show episode range",
			input:       "Breaking.Bad.S05E01-E08.1080p.BluRay.x264-GROUP",
			wantTitle:   "Breaking Bad",
			wantSeason:  5,
			wantEpisode: 1,
			wantQuality: QualityFHD,
			wantSource:  "bluray",
		},
		{
			name:        "TV Show 1x01 format",
			input:       "Game.of.Thrones.1x01.720p.HDTV.x264-GROUP",
			wantTitle:   "Game of Thrones",
			wantSeason:  1,
			wantEpisode: 1,
			wantQuality: QualityHD,
			wantSource:  "hdtv",
		},
		{
			name:        "TV Show 4K",
			input:       "Stranger.Things.S04E01.2160p.NF.WEB-DL.DDP5.1.Atmos.DV.HEVC-GROUP",
			wantTitle:   "Stranger Things",
			wantSeason:  4,
			wantEpisode: 1,
			wantQuality: QualityUHD,
			wantSource:  "web-dl",
		},
		{
			name:        "TV Show Season only",
			input:       "The.Office.Season.1.1080p.WEB-DL-GROUP",
			wantTitle:   "The Office",
			wantSeason:  1,
			wantEpisode: -1,
			wantQuality: QualityFHD,
			wantSource:  "web-dl",
		},
		{
			name:        "TV Show multi-episode",
			input:       "Show.Name.S01E01E02.1080p.WEB-DL-GROUP",
			wantTitle:   "Show Name",
			wantSeason:  1,
			wantEpisode: 1,
			wantQuality: QualityFHD,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)

			if got.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", got.Title, tt.wantTitle)
			}
			if got.Year != tt.wantYear {
				t.Errorf("Year = %d, want %d", got.Year, tt.wantYear)
			}
			if got.Season != tt.wantSeason {
				t.Errorf("Season = %d, want %d", got.Season, tt.wantSeason)
			}
			if got.Episode != tt.wantEpisode {
				t.Errorf("Episode = %d, want %d", got.Episode, tt.wantEpisode)
			}
			if got.Quality != tt.wantQuality {
				t.Errorf("Quality = %v, want %v", got.Quality, tt.wantQuality)
			}
			if tt.wantSource != "" && got.Source != tt.wantSource {
				t.Errorf("Source = %q, want %q", got.Source, tt.wantSource)
			}
		})
	}
}

func TestParseAnime(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantTitle    string
		wantSeason   int
		wantEpisode  int
		wantQuality  Quality
		wantSubGroup string
	}{
		{
			name:         "Anime with fansub",
			input:        "[SubGroup] Anime Name - 01 [1080p].mkv",
			wantTitle:    "Anime Name",
			wantSeason:   -1,
			wantEpisode:  1,
			wantQuality:  QualityFHD,
			wantSubGroup: "SubGroup",
		},
		{
			name:         "Anime with episode number",
			input:        "[HorribleSubs] Jujutsu Kaisen - 24 [1080p].mkv",
			wantTitle:    "Jujutsu Kaisen",
			wantSeason:   -1,
			wantEpisode:  24,
			wantQuality:  QualityFHD,
			wantSubGroup: "HorribleSubs",
		},
		{
			name:         "Anime 4K HEVC",
			input:        "[GROUP] Demon Slayer - 01 [2160p][HEVC][10bit].mkv",
			wantTitle:    "Demon Slayer",
			wantSeason:   -1,
			wantEpisode:  1,
			wantQuality:  QualityUHD,
			wantSubGroup: "GROUP",
		},
		{
			name:         "Anime with version",
			input:        "[SubGroup] Anime Title - 01v2 [720p].mkv",
			wantTitle:    "Anime Title",
			wantSeason:   -1,
			wantEpisode:  1,
			wantQuality:  QualityHD,
			wantSubGroup: "SubGroup",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)

			if got.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", got.Title, tt.wantTitle)
			}
			if got.Season != tt.wantSeason {
				t.Errorf("Season = %d, want %d", got.Season, tt.wantSeason)
			}
			if got.Episode != tt.wantEpisode {
				t.Errorf("Episode = %d, want %d", got.Episode, tt.wantEpisode)
			}
			if got.Quality != tt.wantQuality {
				t.Errorf("Quality = %v, want %v", got.Quality, tt.wantQuality)
			}
			if got.SubGroup != tt.wantSubGroup {
				t.Errorf("SubGroup = %q, want %q", got.SubGroup, tt.wantSubGroup)
			}
		})
	}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		files        []File
		wantCategory Category
	}{
		{
			name:         "TV Show by name",
			input:        "The.Last.of.Us.S01E01.1080p.WEB-DL-GROUP",
			wantCategory: CategoryTV,
		},
		{
			name:         "Movie by name",
			input:        "Oppenheimer.2023.2160p.WEB-DL-GROUP",
			wantCategory: CategoryMovie,
		},
		{
			name:         "Anime by fansub pattern",
			input:        "[SubGroup] Anime Name - 01 [1080p].mkv",
			wantCategory: CategoryAnime,
		},
		{
			name:         "Anime with dual audio",
			input:        "Anime.Title.S01.Dual.Audio.1080p-GROUP",
			wantCategory: CategoryAnime,
		},
		{
			name:  "Single media file guessed as movie",
			input: "Some.Unknown.Release",
			files: []File{
				{Path: "video.mkv", Size: 1000000000},
			},
			wantCategory: CategoryMovie,
		},
		{
			name:  "TV by file count",
			input: "Some.Release.GROUP",
			files: []File{
				{Path: "Episode01.mkv", Size: 500000000},
				{Path: "Episode02.mkv", Size: 500000000},
				{Path: "Episode03.mkv", Size: 500000000},
				{Path: "Episode04.mkv", Size: 500000000},
				{Path: "Episode05.mkv", Size: 500000000},
				{Path: "Episode06.mkv", Size: 500000000},
			},
			wantCategory: CategoryTV,
		},
		{
			name:         "Unknown - JAV code",
			input:        "CAWD-062",
			wantCategory: CategoryUnknown,
		},
		{
			name:         "Unknown - random string",
			input:        "some-random-file-without-media-indicators",
			wantCategory: CategoryUnknown,
		},
		{
			name:         "Unknown - release group prefix not anime",
			input:        "[TGx] Some Random Release",
			wantCategory: CategoryUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCat := Classify(tt.input, tt.files)

			if gotCat != tt.wantCategory {
				t.Errorf("Category = %v, want %v", gotCat, tt.wantCategory)
			}
		})
	}
}

func TestDetectQuality(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  Quality
	}{
		{"4K", "Movie.4K.WEB-DL", QualityUHD},
		{"2160p", "Movie.2160p.WEB-DL", QualityUHD},
		{"UHD", "Movie.UHD.BluRay", QualityUHD},
		{"1080p", "Movie.1080p.BluRay", QualityFHD},
		{"1080i", "Movie.1080i.HDTV", QualityFHD},
		{"720p", "Movie.720p.HDTV", QualityHD},
		{"480p", "Movie.480p.DVDRip", QualitySD},
		{"DVDRip", "Movie.DVDRip.XviD", QualitySD},
		{"Unknown", "Movie.Title", QualityUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectQuality(tt.input)
			if got != tt.want {
				t.Errorf("DetectQuality(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsAnime(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"Fansub pattern", "[HorribleSubs] Anime - 01 [1080p].mkv", true},
		{"Dual audio", "Anime.Title.Dual.Audio.1080p", true},
		{"Hi10P", "Anime.Title.Hi10P.720p", true},
		{"Not anime", "The.Matrix.1999.1080p.BluRay", false},
		{"TV show", "The.Office.S01E01.1080p.WEB-DL", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsAnime(tt.input)
			if got != tt.want {
				t.Errorf("IsAnime(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsTV(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"S01E01", "Show.S01E01.1080p.WEB-DL", true},
		{"1x01", "Show.1x01.720p.HDTV", true},
		{"Season", "Show.Season.1.1080p.BluRay", true},
		{"Complete series", "Show.Complete.Series.1080p", true},
		{"Not TV", "Movie.2023.1080p.BluRay", false},
		{"Episode keyword", "Show.Episode.01.1080p", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsTV(tt.input)
			if got != tt.want {
				t.Errorf("IsTV(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsMovie(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"Year and quality", "Movie.2023.1080p.WEB-DL", true},
		{"BluRay with year", "Movie.1999.BluRay.1080p", true},
		{"TV show not movie", "Show.S01E01.1080p.WEB-DL", false},
		{"Just name", "Some.Random.Name", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsMovie(tt.input)
			if got != tt.want {
				t.Errorf("IsMovie(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsAdult(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"JAV code SNIS-070", "SNIS-070", true},
		{"JAV code CAWD-507", "CAWD-507", true},
		{"JAV code with suffix", "MIAB-575-C", true},
		{"JAV code numeric prefix 529STCV-216", "529STCV-216", true},
		{"JAV code with domain watermark", "dccdom.com@529STCV-216", true},
		{"Porn studio Brazzers", "Brazzers.21.05.15.Gabbie.Carter.XXX.1080p", true},
		{"Porn studio RealityKings", "RealityKings - MomsBangTeens - Lucy Tyler", true},
		{"XXX keyword", "PornWorld.25.09.20.Sladyen.Skaya.XXX.1080p.MP4", true},
		{"Chinese adult", "骚逼媳妇归来帮我泻火", true},
		{"Adult site watermark", "sex8.cc some content", true},
		{"OnlyFans", "OnlyFans_gina-carla-onlyfans-nude-tease", true},
		{"Online course", "Udemy Complete Python Bootcamp 2024", true},
		{"Not adult - normal movie", "The.Matrix.1999.1080p.BluRay", false},
		{"Not adult - TV show", "The.Last.of.Us.S01E01.1080p.WEB-DL", false},
		{"Not adult - anime uncensored", "[SubGroup] Prison School Uncensored - 01 [1080p].mkv", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsAdult(tt.input)
			if got != tt.want {
				t.Errorf("IsAdult(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsJunkAdultContent(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"JAV code filtered", "SDDE-623", true},
		{"Porn site filtered", "PornWorld.25.09.20.XXX.1080p.MP4", true},
		{"Normal movie passes", "Oppenheimer.2023.1080p.WEB-DL", false},
		{"Anime with uncensored passes", "[SubGroup] Anime Title Uncensored - 01 [1080p].mkv", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsJunk(tt.input, nil)
			if got != tt.want {
				t.Errorf("IsJunk(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestEpisodeTitleTruncation(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantTitle string
	}{
		{
			name:      "Episode title stripped",
			input:     "Solo.Leveling.S02E06.Dont.Look.Down.on.My.Guys.1080p.CR.WEB-DL",
			wantTitle: "Solo Leveling",
		},
		{
			name:      "No episode title pollution",
			input:     "The.Rookie.S07E08.Wildfire.1080p.AMZN.WEB-DL",
			wantTitle: "The Rookie",
		},
		{
			name:      "3-digit episode",
			input:     "The.Young.and.the.Restless.S52E112.1080p.WEB.h264",
			wantTitle: "The Young and the Restless",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			if got.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", got.Title, tt.wantTitle)
			}
		})
	}
}

func TestSiteWatermarkStripping(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantTitle string
	}{
		{
			name:      "www.UIndex.org prefix",
			input:     "www.UIndex.org    -    Predator.Badlands.2025.1080p.TELESYNC.V2.x264-SyncUP",
			wantTitle: "Predator Badlands",
		},
		{
			name:      "www.Torrenting.com prefix",
			input:     "www.Torrenting.com   -    In.and.Out.1997.720p.WEB.h264-NOMA",
			wantTitle: "In and Out",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			if got.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", got.Title, tt.wantTitle)
			}
		})
	}
}

func TestSpanishTVPatterns(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantTV      bool
		wantSeason  int
		wantEpisode int
	}{
		{
			name:        "Temporada pattern",
			input:       "The Resident - Temporada 1 [HDTV][Cap.101]",
			wantTV:      true,
			wantSeason:  1,
			wantEpisode: -1, // Cap is in brackets, parsed separately
		},
		{
			name:        "Cap pattern standalone",
			input:       "La lista final [HDTV][Cap.102]",
			wantTV:      true,
			wantSeason:  -1,
			wantEpisode: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTV := IsTV(tt.input)
			if gotTV != tt.wantTV {
				t.Errorf("IsTV(%q) = %v, want %v", tt.input, gotTV, tt.wantTV)
			}
			if tt.wantSeason != 0 || tt.wantEpisode != 0 {
				p := Parse(tt.input)
				if tt.wantSeason != 0 && p.Season != tt.wantSeason {
					t.Errorf("Season = %d, want %d", p.Season, tt.wantSeason)
				}
				if tt.wantEpisode != 0 && p.Episode != tt.wantEpisode {
					t.Errorf("Episode = %d, want %d", p.Episode, tt.wantEpisode)
				}
			}
		})
	}
}

func TestHasMediaFiles(t *testing.T) {
	tests := []struct {
		name  string
		files []File
		want  bool
	}{
		{
			name: "All video files",
			files: []File{
				{Path: "video.mkv", Size: 1000000000},
				{Path: "video.mp4", Size: 1000000000},
			},
			want: true,
		},
		{
			name: "Mixed with small non-video",
			files: []File{
				{Path: "video.mkv", Size: 1000000000},
				{Path: "sample.txt", Size: 1000},
			},
			want: true,
		},
		{
			name: "Mostly non-video",
			files: []File{
				{Path: "data.bin", Size: 1000000000},
				{Path: "video.mkv", Size: 100000},
			},
			want: false,
		},
		{
			name:  "Empty",
			files: []File{},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasMediaFiles(tt.files)
			if got != tt.want {
				t.Errorf("HasMediaFiles() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsAdultName(t *testing.T) {
	// Inject test data into the package-level maps
	origPerformers := adultPerformers
	origStudios := adultStudios
	origKeywords := adultKeywords
	defer func() {
		adultPerformers = origPerformers
		adultStudios = origStudios
		adultKeywords = origKeywords
	}()

	adultPerformers = map[string]bool{
		"jayden lee":     true,
		"mia khalifa":    true,
		"lana rhoades":   true,
		"riley reid":     true,
		"johnny sins":    true,
		"maria ozawa":    true,
		"asa akira":      true,
		"sasha grey":     true,
	}
	adultStudios = map[string]bool{
		"evil angel":     true,
		"bang bros":       true,
		"wicked pictures": true,
	}
	adultKeywords = map[string]bool{
		"gangbang":   true,
		"creampie":   true,
		"bukkake":    true,
	}

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// Should match — bare performer names (no media signals)
		{"bare performer name", "Jayden Lee", true},
		{"performer dotted", "Jayden.Lee", true},
		{"performer hyphenated", "Mia-Khalifa", true},
		{"performer in longer title", "Jayden Lee Hardcore Scene", true},

		// Should match — bare studio names
		{"bare studio name", "Evil Angel", true},
		{"studio dotted", "Evil.Angel", true},

		// Should match — keywords
		{"keyword in title", "Amateur Gangbang Collection", true},

		// Should NOT match — has media signals
		// (isAdultName is only called when !hasMediaSignals, but test the function itself)
		{"performer name function still matches", "Riley Reid", true},

		// Should NOT match — not in maps
		{"legit actor name", "John Smith", false},
		{"random title", "Basketball Championship 2024", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isAdultName(tt.input)
			if got != tt.want {
				t.Errorf("isAdultName(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsJunkWithAdultNames(t *testing.T) {
	// Inject test data
	origPerformers := adultPerformers
	origStudios := adultStudios
	origKeywords := adultKeywords
	origCfg := filterCfg
	defer func() {
		adultPerformers = origPerformers
		adultStudios = origStudios
		adultKeywords = origKeywords
		filterCfg = origCfg
	}()

	adultPerformers = map[string]bool{
		"jayden lee": true,
	}
	adultStudios = map[string]bool{}
	adultKeywords = map[string]bool{}

	// Ensure all filters enabled
	SetFilterConfig(DefaultFilterConfig())

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// Bare name → junk (no media signals, matches performer)
		{"bare performer name is junk", "Jayden Lee", true},
		// Name with media signals → NOT junk (skips name check)
		{"performer with media signals passes", "Jayden.Lee.2024.1080p.WEB-DL", false},
		// TV show with performer name → NOT junk
		{"TV show passes", "Breaking.Bad.S02E08.1080p", false},
		// Domain watermark + JAV → junk (caught by adult patterns)
		{"domain watermark JAV", "dccdom.com@529STCV-216", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsJunk(tt.input, nil)
			if got != tt.want {
				t.Errorf("IsJunk(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestFilterConfigToggles(t *testing.T) {
	origCfg := filterCfg
	defer func() { filterCfg = origCfg }()

	// With adult patterns disabled, JAV codes should pass
	SetFilterConfig(FilterConfig{
		FilterAdultPatterns: false,
		FilterAdultNames:    true,
		FilterJunkNames:     true,
	})
	if IsJunk("CAWD-507", nil) {
		t.Error("expected CAWD-507 to pass when FilterAdultPatterns=false")
	}

	// With junk names disabled, software patterns should pass
	SetFilterConfig(FilterConfig{
		FilterAdultPatterns: true,
		FilterAdultNames:    true,
		FilterJunkNames:     false,
	})
	if IsJunk("Adobe Photoshop 2024 crack keygen", nil) {
		t.Error("expected software title to pass when FilterJunkNames=false")
	}

	// With adult names disabled, bare performer names should pass
	origPerformers := adultPerformers
	defer func() { adultPerformers = origPerformers }()
	adultPerformers = map[string]bool{"jayden lee": true}

	SetFilterConfig(FilterConfig{
		FilterAdultPatterns: true,
		FilterAdultNames:    false,
		FilterJunkNames:     true,
	})
	if IsJunk("Jayden Lee", nil) {
		t.Error("expected bare performer name to pass when FilterAdultNames=false")
	}

	// Re-enable → should be caught
	SetFilterConfig(DefaultFilterConfig())
	if !IsJunk("Jayden Lee", nil) {
		t.Error("expected bare performer name to be junk when FilterAdultNames=true")
	}
}

func TestDomainWatermarkStripping(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantTitle string
	}{
		{
			name:      "domain.com@ watermark stripped",
			input:     "dccdom.com@Some.Movie.2024.1080p.WEB-DL",
			wantTitle: "Some Movie",
		},
		{
			name:      "domain.net@ watermark stripped",
			input:     "tracker.net@Another.Title.2023.720p",
			wantTitle: "Another Title",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			if got.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", got.Title, tt.wantTitle)
			}
		})
	}
}

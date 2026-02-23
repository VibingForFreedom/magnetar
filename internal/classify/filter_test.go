package classify

import "testing"

func TestIsAdultFilterImprovements(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// Step 1: JAV distribution site watermarks
		{"onejav watermark", "onejav.com@ABW-123", true},
		{"javmost watermark", "javmost.com CAWD-300", true},
		{"missav watermark", "missav SONE-001", true},
		{"jable.tv watermark", "jable.tv IPX-789", true},
		{"supjav watermark", "supjav@MIDE-456", true},
		{"sehuatang watermark", "sehuatang.net PRED-100", true},

		// Step 2: Porn studios
		{"TeamSkeet studio", "TeamSkeet - Jayden Lee 1080p", true},
		{"PureMature studio", "PureMature.Brandi.Love.1080p", true},
		{"SweetSinner studio", "SweetSinner - Some Title", true},
		{"BrattySis studio", "BrattySis.Step.Sister", true},
		{"FamilyStrokes studio", "FamilyStrokes.Some.Title", true},
		{"Dorcel studio", "Dorcel Some Scene", true},
		{"AnalVids studio", "AnalVids.Some.Title", true},
		{"Scoreland studio", "Scoreland Something", true},
		{"DDFNetwork studio", "DDFNetwork.SomeScene", true},
		{"Playboy studio", "Playboy Plus - Model Name", true},
		{"Penthouse studio", "Penthouse.SomeScene", true},
		{"MissaX studio", "MissaX Some Title", true},

		// Step 3: [FHD]/[HD] JAV prefix
		{"FHD prefix JAV", "[FHD]CAWD-507", true},
		{"HD prefix JAV", "[HD]ABW-123", true},
		{"4K prefix JAV", "[4K]SONE-001", true},
		{"UHD prefix JAV", "[UHD]MIDE-456", true},
		{"SD prefix JAV", "[SD]PRED-100", true},
		{"FHD prefix with space", "[FHD] IPX-789", true},

		// Step 4: Hentai anime titles
		{"Bible Black hentai", "Bible Black S01E01", true},
		{"Overflow hentai", "Overflow - 01 [1080p]", true},
		{"Itadaki Seieki", "[SubGroup] Itadaki Seieki - 01", true},
		{"Mankitsu Happening", "Mankitsu Happening 02", true},
		{"Euphoria hentai", "Euphoria - Episode 1", true},
		{"Kuroinu hentai", "Kuroinu 03 [720p]", true},
		{"Resort Boin", "Resort Boin Episode 3", true},
		{"Shoujo Ramune", "Shoujo Ramune 01", true},
		{"Rance hentai", "[Fansub] Rance - 01 [1080p]", true},

		// Step 5: Yurievij uploader
		{"Yurievij uploader", "Yurievij collection part 1", true},
		{"Yurievij in name", "Something Yurievij 2024", true},

		// Step 6: Chinese adult patterns
		{"XiuRen watermark", "XiuRen No.1234", true},
		{"MyGirl watermark", "MyGirl Vol.123", true},
		{"HuaYang watermark", "HuaYang No.456", true},
		{"chinese 麻豆", "某某 麻豆 视频", true},
		{"chinese 探花", "某某 探花 约会", true},
		{"chinese 果冻", "某某 果冻 传媒", true},
		{"chinese 草榴", "草榴社区 something", true},

		// Step 7: Bare explicit keywords
		{"cumshot keyword", "Amazing cumshot compilation", true},
		{"squirting keyword", "squirting compilation 2024", true},
		{"threesome keyword", "amateur threesome video", true},
		{"double penetration", "double penetration scene", true},
		{"gangbang keyword", "gangbang party", true},
		{"interracial keyword", "interracial scene 01", true},

		// Existing patterns still work
		{"standard JAV code", "CAWD-507", true},
		{"JAV with domain", "dccdom.com@529STCV-216", true},
		{"Brazzers studio", "Brazzers.Some.Scene.1080p", true},
		{"XXX keyword", "Some.XXX.Movie.2024", true},
		{"FC2-PPV code", "FC2-PPV-1234567", true},

		// Negative cases — should NOT match
		{"regular movie", "The.Matrix.1999.1080p.BluRay", false},
		{"regular TV", "Breaking.Bad.S01E01.720p", false},
		{"regular anime", "[SubGroup] Naruto - 001 [1080p]", false},
		{"clean title", "Inception 2010 2160p UHD", false},
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

func TestIsJunkMusicFiltering(t *testing.T) {
	tests := []struct {
		name  string
		input string
		files []File
		want  bool
	}{
		{
			name:  "FLAC in name is junk",
			input: "Artist - Album (2024) [FLAC]",
			want:  true,
		},
		{
			name:  "MP3 320kbps in name is junk",
			input: "Artist - Discography (320kbps) MP3",
			want:  true,
		},
		{
			name:  "Discography in name is junk",
			input: "Pink Floyd - Discography 1967-2014",
			want:  true,
		},
		{
			name:  "Greatest Hits in name is junk",
			input: "Queen - Greatest Hits (1981) V0",
			want:  true,
		},
		{
			name:  "FLAC with media signals passes (audio drama)",
			input: "Some.Show.S01E01.FLAC",
			want:  false,
		},
		{
			name:  "audio-only files = music = junk",
			input: "Unknown Artist Album",
			files: []File{
				{Path: "01 Track.mp3", Size: 5000000},
				{Path: "02 Track.mp3", Size: 5000000},
				{Path: "cover.jpg", Size: 100000},
			},
			want: true,
		},
		{
			name:  "audio with subtitles passes (might be drama)",
			input: "Some Drama Thing",
			files: []File{
				{Path: "audio.flac", Size: 50000000},
				{Path: "subs.srt", Size: 50000},
			},
			want: false,
		},
		{
			name:  "audio-only with media signals passes",
			input: "Drama.S01E01.Audio",
			files: []File{
				{Path: "episode.flac", Size: 50000000},
			},
			want: false,
		},
		{
			name:  "video files present = not music junk",
			input: "Movie Name",
			files: []File{
				{Path: "movie.mkv", Size: 1000000000},
				{Path: "soundtrack.mp3", Size: 5000000},
			},
			want: false,
		},
	}

	origCfg := GetFilterConfig()
	defer func() { SetFilterConfig(origCfg) }()
	SetFilterConfig(DefaultFilterConfig())

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsJunk(tt.input, tt.files)
			if got != tt.want {
				t.Errorf("IsJunk(%q, files) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsAdultAnimeExemption(t *testing.T) {
	// Anime-looking names should skip JAV code patterns but catch hentai titles
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "anime with JAV-like code passes",
			input: "[SubGroup] ABCD-123 - 01 [1080p]",
			want:  false,
		},
		{
			name:  "anime with hentai title caught",
			input: "[SubGroup] Bible Black - 01 [1080p]",
			want:  true,
		},
		{
			name:  "anime with explicit keyword caught",
			input: "[SubGroup] Some Hentai Show - 01 [1080p]",
			want:  true,
		},
		{
			name:  "anime with FHD prefix JAV-like passes",
			input: "[FHD] Some Anime - 01 [SubGroup]",
			want:  false,
		},
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

package store

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

type Category int

const (
	CategoryMovie   Category = 0
	CategoryTV      Category = 1
	CategoryAnime   Category = 2
	CategoryUnknown Category = 3
)

const categoryUnknownStr = "unknown"

func (c Category) String() string {
	switch c {
	case CategoryMovie:
		return "movie"
	case CategoryTV:
		return "tv"
	case CategoryAnime:
		return "anime"
	default:
		return categoryUnknownStr
	}
}

func ParseCategory(s string) Category {
	switch s {
	case "movie", "movies":
		return CategoryMovie
	case "tv", "television":
		return CategoryTV
	case "anime":
		return CategoryAnime
	case categoryUnknownStr, "other":
		return CategoryUnknown
	default:
		return CategoryUnknown
	}
}

func (c Category) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.String())
}

func (c *Category) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	*c = ParseCategory(s)
	return nil
}

type Quality int

const (
	QualityUnknown Quality = iota
	QualitySD
	QualityHD
	QualityFHD
	QualityUHD
)

func (q Quality) String() string {
	switch q {
	case QualitySD:
		return "sd"
	case QualityHD:
		return "hd"
	case QualityFHD:
		return "fhd"
	case QualityUHD:
		return "uhd"
	default:
		return categoryUnknownStr
	}
}

func ParseQuality(s string) Quality {
	switch s {
	case "sd", "480p", "480i":
		return QualitySD
	case "hd", "720p":
		return QualityHD
	case "fhd", "1080p", "1080i":
		return QualityFHD
	case "uhd", "4k", "2160p":
		return QualityUHD
	default:
		return QualityUnknown
	}
}

func (q Quality) MarshalJSON() ([]byte, error) {
	return json.Marshal(q.String())
}

func (q *Quality) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	*q = ParseQuality(s)
	return nil
}

type MatchStatus int

const (
	MatchUnmatched MatchStatus = iota
	MatchMatched
	MatchFailed
)

func (m MatchStatus) String() string {
	switch m {
	case MatchMatched:
		return "matched"
	case MatchFailed:
		return "failed"
	default:
		return "unmatched"
	}
}

func ParseMatchStatus(s string) MatchStatus {
	switch s {
	case "matched":
		return MatchMatched
	case "failed":
		return MatchFailed
	default:
		return MatchUnmatched
	}
}

func (m MatchStatus) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.String())
}

func (m *MatchStatus) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	*m = ParseMatchStatus(s)
	return nil
}

type Source int

const (
	SourceDHT Source = iota
	SourceBitmagnet
)

func (s Source) String() string {
	switch s {
	case SourceBitmagnet:
		return "bitmagnet"
	default:
		return "dht"
	}
}

func ParseSource(s string) Source {
	switch s {
	case "bitmagnet":
		return SourceBitmagnet
	default:
		return SourceDHT
	}
}

func (s Source) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

func (s *Source) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	*s = ParseSource(str)
	return nil
}

type File struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type Torrent struct {
	InfoHash      []byte      `json:"-"`
	Name          string      `json:"name"`
	Size          int64       `json:"size"`
	Category      Category    `json:"category"`
	Quality       Quality     `json:"quality"`
	Files         []File      `json:"files,omitempty"`
	IMDBID        string      `json:"imdb_id,omitempty"`
	TMDBID        int         `json:"tmdb_id,omitempty"`
	TVDBID        int         `json:"tvdb_id,omitempty"`
	AniListID     int         `json:"anilist_id,omitempty"`
	KitsuID       int         `json:"kitsu_id,omitempty"`
	MediaYear     int         `json:"media_year,omitempty"`
	MatchStatus   MatchStatus `json:"match_status"`
	MatchAttempts int         `json:"match_attempts"`
	MatchAfter    int64       `json:"match_after"`
	Seeders       int         `json:"seeders"`
	Leechers      int         `json:"leechers"`
	Source        Source      `json:"source"`
	DiscoveredAt  int64       `json:"discovered_at"`
	UpdatedAt     int64       `json:"updated_at"`
}

func (t *Torrent) InfoHashHex() string {
	if t.InfoHash == nil {
		return ""
	}
	return hex.EncodeToString(t.InfoHash)
}

func (t *Torrent) SetInfoHashFromHex(s string) error {
	b, err := hex.DecodeString(s)
	if err != nil {
		return fmt.Errorf("invalid info hash hex: %w", err)
	}
	if len(b) != 20 {
		return fmt.Errorf("info hash must be 20 bytes, got %d", len(b))
	}
	t.InfoHash = b
	return nil
}

type torrentJSON struct {
	InfoHash      string      `json:"info_hash"`
	Name          string      `json:"name"`
	Size          int64       `json:"size"`
	Category      Category    `json:"category"`
	Quality       Quality     `json:"quality"`
	Files         []File      `json:"files,omitempty"`
	IMDBID        string      `json:"imdb_id,omitempty"`
	TMDBID        int         `json:"tmdb_id,omitempty"`
	TVDBID        int         `json:"tvdb_id,omitempty"`
	AniListID     int         `json:"anilist_id,omitempty"`
	KitsuID       int         `json:"kitsu_id,omitempty"`
	MediaYear     int         `json:"media_year,omitempty"`
	MatchStatus   MatchStatus `json:"match_status"`
	MatchAttempts int         `json:"match_attempts"`
	MatchAfter    int64       `json:"match_after"`
	Seeders       int         `json:"seeders"`
	Leechers      int         `json:"leechers"`
	Source        Source      `json:"source"`
	DiscoveredAt  int64       `json:"discovered_at"`
	UpdatedAt     int64       `json:"updated_at"`
}

func (t Torrent) MarshalJSON() ([]byte, error) {
	tj := torrentJSON{
		InfoHash:      t.InfoHashHex(),
		Name:          t.Name,
		Size:          t.Size,
		Category:      t.Category,
		Quality:       t.Quality,
		Files:         t.Files,
		IMDBID:        t.IMDBID,
		TMDBID:        t.TMDBID,
		TVDBID:        t.TVDBID,
		AniListID:     t.AniListID,
		KitsuID:       t.KitsuID,
		MediaYear:     t.MediaYear,
		MatchStatus:   t.MatchStatus,
		MatchAttempts: t.MatchAttempts,
		MatchAfter:    t.MatchAfter,
		Seeders:       t.Seeders,
		Leechers:      t.Leechers,
		Source:        t.Source,
		DiscoveredAt:  t.DiscoveredAt,
		UpdatedAt:     t.UpdatedAt,
	}
	return json.Marshal(tj)
}

func (t *Torrent) UnmarshalJSON(data []byte) error {
	var tj torrentJSON
	if err := json.Unmarshal(data, &tj); err != nil {
		return err
	}
	t.InfoHash, _ = hex.DecodeString(tj.InfoHash)
	t.Name = tj.Name
	t.Size = tj.Size
	t.Category = tj.Category
	t.Quality = tj.Quality
	t.Files = tj.Files
	t.IMDBID = tj.IMDBID
	t.TMDBID = tj.TMDBID
	t.TVDBID = tj.TVDBID
	t.AniListID = tj.AniListID
	t.KitsuID = tj.KitsuID
	t.MediaYear = tj.MediaYear
	t.MatchStatus = tj.MatchStatus
	t.MatchAttempts = tj.MatchAttempts
	t.MatchAfter = tj.MatchAfter
	t.Seeders = tj.Seeders
	t.Leechers = tj.Leechers
	t.Source = tj.Source
	t.DiscoveredAt = tj.DiscoveredAt
	t.UpdatedAt = tj.UpdatedAt
	return nil
}

type SearchOpts struct {
	Categories []Category
	MinYear    int
	MaxYear    int
	Quality    []Quality
	Limit      int
	Offset     int
}

type SearchResult struct {
	Torrents []*Torrent
	Total    int
}

type ExternalID struct {
	Type  string
	Value string
}

func (e ExternalID) String() string {
	return fmt.Sprintf("%s:%s", e.Type, e.Value)
}

type MatchResult struct {
	Status     MatchStatus
	IMDBID     string
	TMDBID     int
	TVDBID     int
	AniListID  int
	KitsuID    int
	Year       int
	MatchAfter int64 // unix timestamp for next retry (0 = eligible immediately)
}

type DBStats struct {
	TotalTorrents int64
	Unmatched     int64
	Matched       int64
	Failed        int64
	DBSize        int64
	LastCrawl     int64
	Uptime        int64
}

type Store interface {
	UpsertTorrent(ctx context.Context, t *Torrent) error
	UpsertTorrents(ctx context.Context, ts []*Torrent) error
	GetTorrent(ctx context.Context, infoHash []byte) (*Torrent, error)
	BulkLookup(ctx context.Context, hashes [][]byte) ([]*Torrent, error)
	DeleteTorrent(ctx context.Context, infoHash []byte) error
	ListRecent(ctx context.Context, opts SearchOpts) (*SearchResult, error)
	SearchByName(ctx context.Context, query string, opts SearchOpts) (*SearchResult, error)
	SearchByExternalID(ctx context.Context, id ExternalID) ([]*Torrent, error)
	FetchUnmatched(ctx context.Context, limit int) ([]*Torrent, error)
	UpdateMatchResult(ctx context.Context, infoHash []byte, m MatchResult) error
	UpdateCategory(ctx context.Context, infoHash []byte, category Category) error
	ResetFailedMatches(ctx context.Context) (int64, error)
	ListByMatchStatus(ctx context.Context, status MatchStatus, limit, offset int) (*SearchResult, error)
	Stats(ctx context.Context) (*DBStats, error)
	GetSetting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error
	GetAllSettings(ctx context.Context) (map[string]string, error)
	RejectHashes(ctx context.Context, hashes [][]byte) error
	AreRejected(ctx context.Context, hashes [][]byte) (map[[20]byte]bool, error)
	RejectedHashCount(ctx context.Context) (int64, error)
	PurgeOldRejected(ctx context.Context, olderThan time.Duration) (int64, error)
	PurgeJunkTorrents(ctx context.Context) (int64, error)
	ListRecentlyUpdated(ctx context.Context, limit int) ([]*Torrent, error)
	ListAllMatched(ctx context.Context) ([]*Torrent, error)
	BulkUpdateSeedersLeechers(ctx context.Context, updates []SeedersLeechersUpdate) error
	Migrate(ctx context.Context) error
	Close() error
	Checkpoint(ctx context.Context) error
	Analyze(ctx context.Context) error
}

// SeedersLeechersUpdate holds a single seeders/leechers update for a torrent.
type SeedersLeechersUpdate struct {
	InfoHash []byte
	Seeders  int
	Leechers int
}

var (
	ErrNotFound        = fmt.Errorf("torrent not found")
	ErrInvalidHash     = fmt.Errorf("invalid info hash")
	ErrInvalidInput    = fmt.Errorf("invalid input")
	ErrSettingNotFound = fmt.Errorf("setting not found")
)

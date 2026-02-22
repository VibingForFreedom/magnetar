package api

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/magnetar/magnetar/internal/classify"
	"github.com/magnetar/magnetar/internal/store"
)

const (
	torznabXMLNSNewznab = "http://www.newznab.com/DTD/2010/feeds/attributes/"
	torznabXMLNSTorznab = "http://torznab.com/schemas/2015/feed"
)

type torznabRSS struct {
	XMLName xml.Name       `xml:"rss"`
	Version string         `xml:"version,attr"`
	Newznab string         `xml:"xmlns:newznab,attr"`
	Torznab string         `xml:"xmlns:torznab,attr"`
	Channel torznabChannel `xml:"channel"`
}

type torznabChannel struct {
	Title       string           `xml:"title"`
	Description string           `xml:"description,omitempty"`
	Link        string           `xml:"link,omitempty"`
	Response    *newznabResponse `xml:"newznab:response,omitempty"`
	Error       *torznabError    `xml:"torznab:error,omitempty"`
	Items       []torznabItem    `xml:"item,omitempty"`
}

type newznabResponse struct {
	Offset int `xml:"offset,attr"`
	Total  int `xml:"total,attr"`
}

type torznabError struct {
	Code        int    `xml:"code,attr"`
	Description string `xml:"description,attr"`
}

type torznabItem struct {
	Title     string           `xml:"title"`
	GUID      string           `xml:"guid"`
	Size      int64            `xml:"size"`
	Enclosure torznabEnclosure `xml:"enclosure"`
	PubDate   string           `xml:"pubDate"`
	Attrs     []newznabAttr    `xml:"newznab:attr"`
}

type torznabEnclosure struct {
	URL    string `xml:"url,attr"`
	Length int64  `xml:"length,attr"`
	Type   string `xml:"type,attr"`
}

type newznabAttr struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

func (s *Server) handleTorznab(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	switch values.Get("t") {
	case "caps":
		s.writeXML(w, http.StatusOK, torznabCapsResponse())
		return
	case "search", "tvsearch", "movie":
	default:
		s.writeTorznabError(w, http.StatusBadRequest, 200, "unknown torznab command")
		return
	}

	query := strings.TrimSpace(values.Get("q"))
	catIDs := parseTorznabCategoryIDs(values.Get("cat"))
	opts := ParseNewznabCategories(catIDs)
	limit, offset := s.parseLimitOffset(values, 100)
	opts.Limit = limit
	opts.Offset = offset

	imdbRaw := strings.TrimSpace(values.Get("imdbid"))
	tmdbRaw := strings.TrimSpace(values.Get("tmdbid"))
	tvdbRaw := strings.TrimSpace(values.Get("tvdbid"))

	season := parseOptionalInt(values.Get("season"))
	episode := parseOptionalInt(values.Get("ep"))
	query = augmentWithSeasonEpisode(query, season, episode)

	if imdbRaw != "" || tmdbRaw != "" || tvdbRaw != "" {
		id, err := buildExternalID(imdbRaw, tmdbRaw, tvdbRaw)
		if err != nil {
			s.writeTorznabError(w, http.StatusBadRequest, 200, err.Error())
			return
		}

		torrents, err := s.store.SearchByExternalID(r.Context(), id)
		if err != nil {
			s.writeTorznabError(w, http.StatusInternalServerError, 100, "search failed")
			return
		}
		total := len(torrents)
		torrents = paginateTorrents(torrents, limit, offset)
		items := buildTorznabItems(torrents)
		s.writeXML(w, http.StatusOK, torznabRSSResponse(items, total, offset))
		return
	}

	if query == "" {
		// RSS mode: return recent torrents filtered by category (no text search).
		result, err := s.store.ListRecent(r.Context(), opts)
		if err != nil {
			s.writeTorznabError(w, http.StatusInternalServerError, 100, "search failed")
			return
		}
		items := buildTorznabItems(result.Torrents)
		s.writeXML(w, http.StatusOK, torznabRSSResponse(items, result.Total, offset))
		return
	}

	result, err := s.store.SearchByName(r.Context(), query, opts)
	if err != nil {
		s.writeTorznabError(w, http.StatusInternalServerError, 100, "search failed")
		return
	}

	items := buildTorznabItems(result.Torrents)
	s.writeXML(w, http.StatusOK, torznabRSSResponse(items, result.Total, offset))
}

// Torznab endpoints return XML error payloads instead of JSON, using torznab:error.
func (s *Server) writeTorznabError(w http.ResponseWriter, status, code int, message string) {
	resp := torznabRSS{
		Version: "2.0",
		Newznab: torznabXMLNSNewznab,
		Torznab: torznabXMLNSTorznab,
		Channel: torznabChannel{
			Title:       "Magnetar",
			Description: "Magnetar Torznab Error",
			Error: &torznabError{
				Code:        code,
				Description: message,
			},
		},
	}
	s.writeXML(w, status, resp)
}

func torznabRSSResponse(items []torznabItem, total, offset int) torznabRSS {
	return torznabRSS{
		Version: "2.0",
		Newznab: torznabXMLNSNewznab,
		Torznab: torznabXMLNSTorznab,
		Channel: torznabChannel{
			Title:       "Magnetar",
			Description: "Magnetar Torznab Feed",
			Response: &newznabResponse{
				Offset: offset,
				Total:  total,
			},
			Items: items,
		},
	}
}

func parseTorznabCategoryIDs(raw string) []int {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	ids := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if id, err := strconv.Atoi(part); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

func parseOptionalInt(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return parsed
}

func augmentWithSeasonEpisode(query string, season, episode int) string {
	if season <= 0 && episode <= 0 {
		return query
	}
	suffix := ""
	switch {
	case season > 0 && episode > 0:
		suffix = "S" + pad2(season) + "E" + pad2(episode)
	case season > 0:
		suffix = "S" + pad2(season)
	case episode > 0:
		suffix = "E" + pad2(episode)
	}

	if query == "" {
		return suffix
	}
	return strings.TrimSpace(query + " " + suffix)
}

func pad2(v int) string {
	if v < 10 {
		return "0" + strconv.Itoa(v)
	}
	return strconv.Itoa(v)
}

func buildExternalID(imdbRaw, tmdbRaw, tvdbRaw string) (store.ExternalID, error) {
	if imdbRaw != "" {
		imdbNumeric := strings.TrimPrefix(strings.ToLower(imdbRaw), "tt")
		if imdbNumeric == "" {
			return store.ExternalID{}, fmt.Errorf("invalid imdbid")
		}
		if _, err := strconv.Atoi(imdbNumeric); err != nil {
			return store.ExternalID{}, fmt.Errorf("invalid imdbid")
		}
		return store.ExternalID{Type: "imdb", Value: "tt" + imdbNumeric}, nil
	}
	if tmdbRaw != "" {
		tmdbRaw = strings.TrimSpace(tmdbRaw)
		if _, err := strconv.Atoi(tmdbRaw); err != nil {
			return store.ExternalID{}, fmt.Errorf("invalid tmdbid")
		}
		return store.ExternalID{Type: "tmdb", Value: tmdbRaw}, nil
	}
	if tvdbRaw != "" {
		tvdbRaw = strings.TrimSpace(tvdbRaw)
		if _, err := strconv.Atoi(tvdbRaw); err != nil {
			return store.ExternalID{}, fmt.Errorf("invalid tvdbid")
		}
		return store.ExternalID{Type: "tvdb", Value: tvdbRaw}, nil
	}
	return store.ExternalID{}, store.ErrInvalidInput
}

func paginateTorrents(torrents []*store.Torrent, limit, offset int) []*store.Torrent {
	if offset >= len(torrents) {
		return nil
	}
	end := offset + limit
	if end > len(torrents) {
		end = len(torrents)
	}
	return torrents[offset:end]
}

func buildTorznabItems(torrents []*store.Torrent) []torznabItem {
	items := make([]torznabItem, 0, len(torrents))
	for _, t := range torrents {
		if t == nil {
			continue
		}
		parsed := classify.Parse(t.Name)
		cats := MapToNewznab(t, parsed)
		attrs := make([]newznabAttr, 0, len(cats)+4)
		for _, cat := range cats {
			attrs = append(attrs, newznabAttr{Name: "category", Value: strconv.Itoa(cat)})
		}
		imdb := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(t.IMDBID)), "tt")
		if imdb != "" {
			attrs = append(attrs, newznabAttr{Name: "imdb", Value: imdb})
		}
		if t.TMDBID != 0 {
			attrs = append(attrs, newznabAttr{Name: "tmdb", Value: strconv.Itoa(t.TMDBID)})
		}
		attrs = append(attrs, newznabAttr{Name: "seeders", Value: strconv.Itoa(t.Seeders)})
		attrs = append(attrs, newznabAttr{Name: "peers", Value: strconv.Itoa(t.Seeders + t.Leechers)})

		infoHash := t.InfoHashHex()
		items = append(items, torznabItem{
			Title:   t.Name,
			GUID:    infoHash,
			Size:    t.Size,
			PubDate: time.Unix(t.DiscoveredAt, 0).UTC().Format(time.RFC1123Z),
			Enclosure: torznabEnclosure{
				URL:    "magnet:?xt=urn:btih:" + infoHash,
				Length: t.Size,
				Type:   "application/x-bittorrent",
			},
			Attrs: attrs,
		})
	}
	return items
}

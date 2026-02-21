package api

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/magnetar/magnetar/internal/store"
)

type hashLookupRequest struct {
	Hashes []string `json:"hashes"`
}

type torrentResponse struct {
	InfoHash string         `json:"info_hash"`
	Name     string         `json:"name"`
	Size     int64          `json:"size"`
	Category store.Category `json:"category"`
	Quality  store.Quality  `json:"quality"`
	Files    []store.File   `json:"files"`
	IMDBID   string         `json:"imdb_id"`
	TMDBID   int            `json:"tmdb_id"`
	Seeders  int            `json:"seeders"`
	Leechers int            `json:"leechers"`
}

func (s *Server) handleHashGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	raw := strings.TrimPrefix(r.URL.Path, "/api/torrents/")
	if raw == "" || strings.Contains(raw, "/") {
		s.writeError(w, http.StatusBadRequest, "invalid info hash")
		return
	}

	infoHash, _, err := parseInfoHash(raw)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid info hash")
		return
	}

	torrent, err := s.store.GetTorrent(r.Context(), infoHash)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			s.writeError(w, http.StatusNotFound, "torrent not found")
		case errors.Is(err, store.ErrInvalidHash):
			s.writeError(w, http.StatusBadRequest, "invalid info hash")
		default:
			s.writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	s.writeJSON(w, http.StatusOK, torrentResponseFromStore(torrent))
}

func (s *Server) handleHashBulk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req hashLookupRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Hashes) == 0 {
		s.writeError(w, http.StatusBadRequest, "hashes required")
		return
	}

	hashes := make([][]byte, 0, len(req.Hashes))
	ordered := make([]string, 0, len(req.Hashes))
	for _, raw := range req.Hashes {
		infoHash, hexHash, err := parseInfoHash(raw)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid info hash")
			return
		}
		hashes = append(hashes, infoHash)
		ordered = append(ordered, hexHash)
	}

	torrents, err := s.store.BulkLookup(r.Context(), hashes)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrInvalidHash):
			s.writeError(w, http.StatusBadRequest, "invalid info hash")
		default:
			s.writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	if len(torrents) == 0 {
		s.writeError(w, http.StatusNotFound, "torrent not found")
		return
	}

	byHash := make(map[string]*store.Torrent, len(torrents))
	for _, t := range torrents {
		if t == nil {
			continue
		}
		byHash[t.InfoHashHex()] = t
	}

	responses := make([]torrentResponse, 0, len(byHash))
	for _, hexHash := range ordered {
		if t := byHash[hexHash]; t != nil {
			responses = append(responses, torrentResponseFromStore(t))
		}
	}
	if len(responses) == 0 {
		s.writeError(w, http.StatusNotFound, "torrent not found")
		return
	}

	s.writeJSON(w, http.StatusOK, responses)
}

func parseInfoHash(input string) ([]byte, string, error) {
	hash := strings.TrimSpace(input)
	lower := strings.ToLower(hash)
	switch {
	case strings.HasPrefix(lower, "urn:btih:"):
		hash = hash[len("urn:btih:"):]
	case strings.HasPrefix(lower, "btih:"):
		hash = hash[len("btih:"):]
	}

	if len(hash) != 40 {
		return nil, "", store.ErrInvalidHash
	}

	decoded, err := hex.DecodeString(hash)
	if err != nil || len(decoded) != 20 {
		return nil, "", store.ErrInvalidHash
	}

	return decoded, hex.EncodeToString(decoded), nil
}

func torrentResponseFromStore(t *store.Torrent) torrentResponse {
	if t == nil {
		return torrentResponse{}
	}
	return torrentResponse{
		InfoHash: t.InfoHashHex(),
		Name:     t.Name,
		Size:     t.Size,
		Category: t.Category,
		Quality:  t.Quality,
		Files:    t.Files,
		IMDBID:   t.IMDBID,
		TMDBID:   t.TMDBID,
		Seeders:  t.Seeders,
		Leechers: t.Leechers,
	}
}

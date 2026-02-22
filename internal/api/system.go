package api

import (
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/magnetar/magnetar/internal/tasklog"
)

type systemDatabase struct {
	Backend string `json:"backend"`
	Path    string `json:"path,omitempty"`
	Size    int64  `json:"size"`
}

type systemProcess struct {
	Uptime    int64  `json:"uptime"`
	StartTime string `json:"start_time"`
	GoVersion string `json:"go_version"`
	OSArch    string `json:"os_arch"`
}

type systemResponse struct {
	Database           systemDatabase `json:"database"`
	TotalTorrents      int64          `json:"total_torrents"`
	Unmatched          int64          `json:"unmatched"`
	Matched            int64          `json:"matched"`
	Failed             int64          `json:"failed"`
	RejectedHashCount  int64          `json:"rejected_hashes_count"`
	Process            systemProcess  `json:"process"`
	Tasks              []tasklog.Entry `json:"tasks"`
}

func (s *Server) handleSystem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx := r.Context()

	stats, err := s.store.Stats(ctx)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "failed to fetch stats")
		return
	}

	rejectedCount, err := s.store.RejectedHashCount(ctx)
	if err != nil {
		s.logger.Printf("rejected hash count error: %v", err)
		rejectedCount = 0
	}

	resp := systemResponse{
		Database: systemDatabase{
			Backend: s.cfg.DBBackend,
			Size:    stats.DBSize,
		},
		TotalTorrents:     stats.TotalTorrents,
		Unmatched:         stats.Unmatched,
		Matched:           stats.Matched,
		Failed:            stats.Failed,
		RejectedHashCount: rejectedCount,
		Process: systemProcess{
			Uptime:    int64(time.Since(s.start).Seconds()),
			StartTime: s.start.UTC().Format(time.RFC3339),
			GoVersion: runtime.Version(),
			OSArch:    fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		},
	}

	if s.cfg.IsSQLite() {
		resp.Database.Path = s.cfg.DBPath
	}

	if s.taskRegistry != nil {
		resp.Tasks = s.taskRegistry.Snapshot()
	}

	s.writeJSON(w, http.StatusOK, resp)
}

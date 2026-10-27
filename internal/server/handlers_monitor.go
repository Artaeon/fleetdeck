package server

import (
	"net/http"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/monitor"
)

func (s *Server) handleMonitorStatus(w http.ResponseWriter, r *http.Request) {
	// The path comes only from operator configuration, never an HTTP parameter.
	path := s.cfg.Monitoring.StateFile
	if path == "" {
		writeJSON(w, map[string]any{"configured": false, "targets": []monitor.CheckSummary{}})
		return
	}
	state, err := monitor.LoadState(path)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "Monitoring state is unavailable. Current health is unknown.")
		return
	}
	summaries, err := monitor.Summarize(state, time.Now())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "Monitoring state is invalid. Current health is unknown.")
		return
	}
	writeJSON(w, map[string]any{"configured": true, "targets": summaries})
}

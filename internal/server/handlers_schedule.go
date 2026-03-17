package server

import (
	"fmt"
	"net/http"

	"github.com/fleetdeck/fleetdeck/internal/audit"
	"github.com/fleetdeck/fleetdeck/internal/schedule"
)

func (s *Server) handleListSchedules(w http.ResponseWriter, r *http.Request) {
	timers, err := schedule.ListTimers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("listing timers: %v", err))
		return
	}
	if timers == nil {
		timers = []schedule.TimerStatus{}
	}
	writeJSON(w, timers)
}

func (s *Server) handleEnableSchedule(w http.ResponseWriter, r *http.Request) {
	projectName := r.PathValue("project")
	if !validProjectName.MatchString(projectName) {
		writeError(w, http.StatusBadRequest, "invalid project name")
		return
	}

	if _, err := s.db.GetProject(projectName); err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	sched := "daily"
	if q := r.URL.Query().Get("schedule"); q != "" {
		sched = q
	}

	if err := schedule.InstallTimer(projectName, sched); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("installing timer: %v", err))
		return
	}
	if err := schedule.EnableTimer(projectName); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("enabling timer: %v", err))
		return
	}

	audit.Log("schedule.enable", projectName, fmt.Sprintf("schedule=%s via=api", sched), true)
	writeJSON(w, map[string]string{"status": "enabled", "schedule": sched})
}

func (s *Server) handleDisableSchedule(w http.ResponseWriter, r *http.Request) {
	projectName := r.PathValue("project")
	if !validProjectName.MatchString(projectName) {
		writeError(w, http.StatusBadRequest, "invalid project name")
		return
	}

	if err := schedule.RemoveTimer(projectName); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("removing timer: %v", err))
		return
	}

	audit.Log("schedule.disable", projectName, "via=api", true)
	writeJSON(w, map[string]string{"status": "disabled"})
}

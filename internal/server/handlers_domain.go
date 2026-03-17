package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/fleetdeck/fleetdeck/internal/audit"
)

func (s *Server) handleUpdateProjectDomain(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !validProjectName.MatchString(name) {
		writeError(w, http.StatusBadRequest, "invalid project name")
		return
	}

	p, err := s.db.GetProject(name)
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	var req struct {
		Domain string `json:"domain"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Domain == "" {
		writeError(w, http.StatusBadRequest, "domain is required")
		return
	}

	p.Domain = req.Domain
	if err := s.db.UpdateProject(p); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update domain")
		return
	}

	audit.Log("project.domain", name, fmt.Sprintf("domain=%s via=api", req.Domain), true)
	writeJSON(w, map[string]string{"status": "updated", "domain": req.Domain})
}

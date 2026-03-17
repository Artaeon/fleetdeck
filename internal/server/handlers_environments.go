package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/fleetdeck/fleetdeck/internal/audit"
	"github.com/fleetdeck/fleetdeck/internal/environments"
)

func (s *Server) envManager() *environments.Manager {
	return environments.NewManager(s.cfg.Server.BasePath)
}

func (s *Server) handleListEnvironments(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !validProjectName.MatchString(name) {
		writeError(w, http.StatusBadRequest, "invalid project name")
		return
	}

	if _, err := s.db.GetProject(name); err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	envs, err := s.envManager().List(name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("listing environments: %v", err))
		return
	}
	if envs == nil {
		envs = []environments.Environment{}
	}

	writeJSON(w, envs)
}

func (s *Server) handleCreateEnvironment(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !validProjectName.MatchString(name) {
		writeError(w, http.StatusBadRequest, "invalid project name")
		return
	}

	if _, err := s.db.GetProject(name); err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	var req struct {
		Environment string `json:"environment"`
		Domain      string `json:"domain"`
		Branch      string `json:"branch"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Environment == "" || req.Domain == "" {
		writeError(w, http.StatusBadRequest, "environment and domain are required")
		return
	}

	env, err := s.envManager().Create(name, req.Environment, req.Domain, req.Branch)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("creating environment: %v", err))
		return
	}

	audit.Log("environment.create", name, fmt.Sprintf("env=%s domain=%s via=api", req.Environment, req.Domain), true)
	writeJSON(w, env)
}

func (s *Server) handleDeleteEnvironment(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	envName := r.PathValue("env")

	if !validProjectName.MatchString(name) {
		writeError(w, http.StatusBadRequest, "invalid project name")
		return
	}

	if _, err := s.db.GetProject(name); err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	if err := s.envManager().Delete(name, envName); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("deleting environment: %v", err))
		return
	}

	audit.Log("environment.delete", name, fmt.Sprintf("env=%s via=api", envName), true)
	writeJSON(w, map[string]string{"status": "deleted"})
}

func (s *Server) handlePromoteEnvironment(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !validProjectName.MatchString(name) {
		writeError(w, http.StatusBadRequest, "invalid project name")
		return
	}

	if _, err := s.db.GetProject(name); err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	var req struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.From == "" || req.To == "" {
		writeError(w, http.StatusBadRequest, "from and to are required")
		return
	}

	if err := s.envManager().Promote(name, req.From, req.To); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("promoting environment: %v", err))
		return
	}

	audit.Log("environment.promote", name, fmt.Sprintf("from=%s to=%s via=api", req.From, req.To), true)
	writeJSON(w, map[string]string{"status": "promoted"})
}

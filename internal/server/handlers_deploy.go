package server

import (
	"context"
	"encoding/json"
	"net/http"
)

func (s *Server) handleDeployProject(w http.ResponseWriter, r *http.Request) {
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

	// Accept optional JSON body with deploy options
	var req struct {
		Strategy string `json:"strategy"`
		NoCache  bool   `json:"no_cache"`
		Fresh    bool   `json:"fresh"`
	}
	// Ignore decode errors -- fields are all optional
	json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req)

	// Tracked so Shutdown drains before closing the DB handle.
	proj := p
	s.goAsyncJob(func(context.Context) { s.runDeployment(proj, "", "api-trigger") })

	writeJSON(w, map[string]string{"status": "deploying", "project": p.Name})
}

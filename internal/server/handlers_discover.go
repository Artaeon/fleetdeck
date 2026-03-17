package server

import (
	"encoding/json"
	"net/http"

	"github.com/fleetdeck/fleetdeck/internal/audit"
	"github.com/fleetdeck/fleetdeck/internal/db"
	"github.com/fleetdeck/fleetdeck/internal/discover"
	"github.com/fleetdeck/fleetdeck/internal/project"
)

func (s *Server) handleDiscover(w http.ResponseWriter, r *http.Request) {
	projects, err := discover.DiscoverAll(s.cfg, s.db)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "discovery failed")
		return
	}

	// Filter to unmanaged by default
	showAll := r.URL.Query().Get("all") == "true"
	if !showAll {
		var filtered []discover.DiscoveredProject
		for _, p := range projects {
			if !p.AlreadyManaged {
				filtered = append(filtered, p)
			}
		}
		projects = filtered
	}
	if projects == nil {
		projects = []discover.DiscoveredProject{}
	}

	writeJSON(w, projects)
}

func (s *Server) handleDiscoverImport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Projects []struct {
			Name      string `json:"name"`
			Dir       string `json:"dir"`
			Domain    string `json:"domain"`
			LinuxUser string `json:"linux_user"`
		} `json:"projects"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	var imported []string
	for _, p := range req.Projects {
		if p.Name == "" || p.Dir == "" {
			continue
		}
		linuxUser := p.LinuxUser
		if linuxUser == "" {
			linuxUser = project.LinuxUserName(p.Name)
		}
		domain := p.Domain
		if domain == "" {
			domain = p.Name + ".local"
		}

		proj := &db.Project{
			Name:        p.Name,
			Domain:      domain,
			LinuxUser:   linuxUser,
			ProjectPath: p.Dir,
			Template:    "custom",
			Status:      "created",
			Source:      "discovered",
		}
		if err := s.db.CreateProject(proj); err != nil {
			continue
		}
		imported = append(imported, p.Name)
		audit.Log("discover.import", p.Name, "via=api", true)
	}
	if imported == nil {
		imported = []string{}
	}

	writeJSON(w, map[string]interface{}{
		"imported": imported,
		"count":    len(imported),
	})
}

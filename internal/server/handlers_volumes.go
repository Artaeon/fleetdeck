package server

import (
	"fmt"
	"net/http"
	"os/exec"
	"strings"

	"github.com/fleetdeck/fleetdeck/internal/audit"
)

type apiVolume struct {
	Name       string `json:"name"`
	Driver     string `json:"driver"`
	Mountpoint string `json:"mountpoint"`
}

func (s *Server) handleListVolumes(w http.ResponseWriter, r *http.Request) {
	out, err := exec.Command("docker", "volume", "ls", "--format", "{{.Name}}\t{{.Driver}}\t{{.Mountpoint}}").CombinedOutput()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list volumes")
		return
	}

	var volumes []apiVolume
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) >= 2 {
			v := apiVolume{Name: parts[0], Driver: parts[1]}
			if len(parts) == 3 {
				v.Mountpoint = parts[2]
			}
			volumes = append(volumes, v)
		}
	}
	if volumes == nil {
		volumes = []apiVolume{}
	}

	writeJSON(w, volumes)
}

func (s *Server) handleDeleteVolume(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "volume name is required")
		return
	}

	// Basic validation: prevent shell injection
	if strings.ContainsAny(name, " \t\n;|&$`\"'\\") {
		writeError(w, http.StatusBadRequest, "invalid volume name")
		return
	}

	out, err := exec.Command("docker", "volume", "rm", name).CombinedOutput()
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to remove volume: %s", strings.TrimSpace(string(out))))
		return
	}

	audit.Log("volume.delete", name, "via=api", true)
	writeJSON(w, map[string]string{"status": "deleted"})
}

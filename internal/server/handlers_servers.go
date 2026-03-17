package server

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/audit"
	"github.com/fleetdeck/fleetdeck/internal/db"
)

var validServerName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*[a-z0-9]$`)

func (s *Server) handleCreateServer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		Host    string `json:"host"`
		Port    string `json:"port"`
		User    string `json:"user"`
		KeyPath string `json:"key_path"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Name == "" || req.Host == "" {
		writeError(w, http.StatusBadRequest, "name and host are required")
		return
	}

	if !validServerName.MatchString(req.Name) {
		writeError(w, http.StatusBadRequest, "invalid server name")
		return
	}

	if req.Port == "" {
		req.Port = "22"
	}
	if req.User == "" {
		req.User = "root"
	}

	srv := &db.Server{
		Name:    req.Name,
		Host:    req.Host,
		Port:    req.Port,
		User:    req.User,
		KeyPath: req.KeyPath,
	}
	if err := s.db.CreateServer(srv); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeError(w, http.StatusConflict, fmt.Sprintf("server %q already exists", req.Name))
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create server")
		return
	}

	audit.Log("server.add", req.Name, fmt.Sprintf("host=%s via=api", req.Host), true)
	writeJSON(w, map[string]string{"status": "created", "name": req.Name})
}

func (s *Server) handleDeleteServer(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !validServerName.MatchString(name) {
		writeError(w, http.StatusBadRequest, "invalid server name")
		return
	}

	if err := s.db.DeleteServer(name); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, "server not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	audit.Log("server.delete", name, "via=api", true)
	writeJSON(w, map[string]string{"status": "deleted"})
}

func (s *Server) handleCheckServer(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !validServerName.MatchString(name) {
		writeError(w, http.StatusBadRequest, "invalid server name")
		return
	}

	srv, err := s.db.GetServer(name)
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}

	// Quick TCP connectivity check
	addr := net.JoinHostPort(srv.Host, srv.Port)
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		s.db.UpdateServerStatus(name, "unreachable")
		writeJSON(w, map[string]string{"status": "unreachable", "error": err.Error()})
		return
	}
	conn.Close()

	s.db.UpdateServerStatus(name, "active")
	writeJSON(w, map[string]string{"status": "active"})
}

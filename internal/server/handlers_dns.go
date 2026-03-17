package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/fleetdeck/fleetdeck/internal/audit"
	"github.com/fleetdeck/fleetdeck/internal/dns"
)

func (s *Server) dnsProvider() (dns.Provider, error) {
	token := s.cfg.DNS.APIToken
	if token == "" {
		return nil, fmt.Errorf("DNS API token not configured")
	}
	return dns.GetProvider(s.cfg.DNS.Provider, token)
}

func (s *Server) handleListDNSRecords(w http.ResponseWriter, r *http.Request) {
	domain := r.PathValue("domain")
	if domain == "" {
		writeError(w, http.StatusBadRequest, "domain is required")
		return
	}

	provider, err := s.dnsProvider()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	records, err := provider.ListRecords(domain)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("listing records: %v", err))
		return
	}

	writeJSON(w, records)
}

func (s *Server) handleSetupDNS(w http.ResponseWriter, r *http.Request) {
	domain := r.PathValue("domain")
	if domain == "" {
		writeError(w, http.StatusBadRequest, "domain is required")
		return
	}

	var req struct {
		ServerIP string `json:"server_ip"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.ServerIP == "" {
		writeError(w, http.StatusBadRequest, "server_ip is required")
		return
	}

	provider, err := s.dnsProvider()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := dns.AutoConfigure(domain, req.ServerIP, provider); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("DNS auto-configuration failed: %v", err))
		return
	}

	audit.Log("dns.setup", domain, fmt.Sprintf("ip=%s via=api", req.ServerIP), true)
	writeJSON(w, map[string]string{"status": "configured", "domain": domain})
}

func (s *Server) handleDeleteDNSRecord(w http.ResponseWriter, r *http.Request) {
	domain := r.PathValue("domain")
	recordType := r.PathValue("type")
	record := r.PathValue("record")

	if domain == "" || recordType == "" || record == "" {
		writeError(w, http.StatusBadRequest, "domain, type, and record are required")
		return
	}

	provider, err := s.dnsProvider()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := provider.DeleteRecord(domain, recordType, record); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("deleting record: %v", err))
		return
	}

	audit.Log("dns.delete", domain, fmt.Sprintf("type=%s record=%s via=api", recordType, record), true)
	writeJSON(w, map[string]string{"status": "deleted"})
}

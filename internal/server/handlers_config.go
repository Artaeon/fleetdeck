package server

import (
	"net/http"
)

type apiConfig struct {
	BasePath     string `json:"base_path"`
	Domain       string `json:"domain"`
	DNSProvider  string `json:"dns_provider"`
	TraefikNet   string `json:"traefik_network"`
	BackupPath   string `json:"backup_path"`
	AuditEnabled bool   `json:"audit_enabled"`
	AuditPath    string `json:"audit_path"`
	Monitoring   bool   `json:"monitoring_enabled"`
	DeployStrat  string `json:"deploy_strategy"`
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	// Return sanitized config (no secrets)
	writeJSON(w, apiConfig{
		BasePath:     s.cfg.Server.BasePath,
		Domain:       s.cfg.Server.Domain,
		DNSProvider:  s.cfg.DNS.Provider,
		TraefikNet:   s.cfg.Traefik.Network,
		BackupPath:   s.cfg.Backup.BasePath,
		AuditEnabled: s.cfg.Audit.Enabled,
		AuditPath:    s.cfg.Audit.LogPath,
		Monitoring:   s.cfg.Monitoring.Enabled,
		DeployStrat:  s.cfg.Deploy.Strategy,
	})
}

func (s *Server) handleGetWebhookURL(w http.ResponseWriter, r *http.Request) {
	domain := s.cfg.Server.Domain
	webhookURL := ""
	if domain != "" {
		webhookURL = "https://" + domain + "/api/webhook/github"
	}
	writeJSON(w, map[string]string{"webhook_url": webhookURL})
}

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleGetConfig(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/config", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var cfg apiConfig
	if err := json.NewDecoder(w.Body).Decode(&cfg); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if cfg.BasePath == "" {
		t.Error("expected base_path to be set")
	}
	if cfg.TraefikNet == "" {
		t.Error("expected traefik_network to be set")
	}
	if cfg.DeployStrat == "" {
		t.Error("expected deploy_strategy to be set")
	}
}

func TestHandleGetConfigNoSecrets(t *testing.T) {
	srv, _ := setupTestServer(t)

	// Set some secrets that should NOT appear in the response
	srv.cfg.Server.APIToken = "secret-token"
	srv.cfg.Server.WebhookSecret = "secret-webhook"
	srv.cfg.DNS.APIToken = "secret-dns"

	req := httptest.NewRequest("GET", "/api/config", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	body := w.Body.String()
	if contains := "secret-token"; containsString(body, contains) {
		t.Error("config response should not contain API token")
	}
	if contains := "secret-webhook"; containsString(body, contains) {
		t.Error("config response should not contain webhook secret")
	}
	if contains := "secret-dns"; containsString(body, contains) {
		t.Error("config response should not contain DNS API token")
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestHandleGetWebhookURL(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/config/webhook-url", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	// Default config has no domain, so webhook URL should be empty
	if resp["webhook_url"] != "" {
		t.Errorf("expected empty webhook_url without domain, got %q", resp["webhook_url"])
	}
}

func TestHandleGetWebhookURLWithDomain(t *testing.T) {
	srv, _ := setupTestServer(t)
	srv.cfg.Server.Domain = "fleet.example.com"

	req := httptest.NewRequest("GET", "/api/config/webhook-url", nil)
	w := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(w, req)

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	expected := "https://fleet.example.com/api/webhook/github"
	if resp["webhook_url"] != expected {
		t.Errorf("expected %q, got %q", expected, resp["webhook_url"])
	}
}

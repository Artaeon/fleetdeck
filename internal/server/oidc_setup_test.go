package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fleetdeck/fleetdeck/internal/config"
	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

func TestAutheliaSetupIsPrivateAndConsistent(t *testing.T) {
	out := filepath.Join(t.TempDir(), "auth")
	settings := config.OIDCConfig{Issuer: "https://auth.example.invalid", PublicURL: "https://admin.example.invalid", AdminGroup: "admins"}
	if err := WriteAutheliaSetup(out, settings); err != nil {
		t.Fatal(err)
	}
	secret, err := readOIDCSecret(filepath.Join(out, "client-secret"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"client-secret", "authelia-client.yml", "fleetdeck-oidc.toml"} {
		info, err := os.Stat(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Fatal("setup file is not private")
		}
	}
	data, _ := os.ReadFile(filepath.Join(out, "fleetdeck-oidc.toml"))
	var cfg config.Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDashboardAuth(cfg.Server); err != nil {
		t.Fatal(err)
	}
	yamlData, _ := os.ReadFile(filepath.Join(out, "authelia-client.yml"))
	if strings.Contains(string(yamlData), secret) || strings.Contains(string(data), secret) {
		t.Fatal("plaintext secret duplicated into config")
	}
	var doc struct {
		Identity struct {
			OIDC struct {
				Clients []struct {
					ID        string   `yaml:"client_id"`
					Secret    string   `yaml:"client_secret"`
					PKCE      bool     `yaml:"require_pkce"`
					Policy    string   `yaml:"authorization_policy"`
					Redirects []string `yaml:"redirect_uris"`
				} `yaml:"clients"`
			} `yaml:"oidc"`
		} `yaml:"identity_providers"`
	}
	if err := yaml.Unmarshal(yamlData, &doc); err != nil {
		t.Fatal(err)
	}
	c := doc.Identity.OIDC.Clients[0]
	if c.ID != cfg.Server.OIDC.ClientID || !c.PKCE || c.Policy != "two_factor" || c.Redirects[0] != settings.PublicURL+"/auth/callback" || !strings.HasPrefix(c.Secret, "$pbkdf2-sha512$310000$") {
		t.Fatal("inconsistent client registration")
	}
	if err := WriteAutheliaSetup(out, settings); err == nil {
		t.Fatal("overwrote existing setup")
	}
	after, _ := readOIDCSecret(filepath.Join(out, "client-secret"))
	if after != secret {
		t.Fatal("existing credential changed")
	}
	repo := t.TempDir()
	os.Mkdir(filepath.Join(repo, ".git"), 0700)
	if err := WriteAutheliaSetup(filepath.Join(repo, "auth"), settings); err == nil {
		t.Fatal("wrote secrets inside a repository")
	}
}

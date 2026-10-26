package server

import (
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fleetdeck/fleetdeck/internal/config"
	"github.com/pelletier/go-toml/v2"
	"golang.org/x/crypto/pbkdf2"
	"gopkg.in/yaml.v3"
)

// WriteAutheliaSetup creates a new private directory, never edits live config,
// and refuses repository destinations. It returns no secret material.
func WriteAutheliaSetup(output string, settings config.OIDCConfig) error {
	parent, err := filepath.EvalSymlinks(filepath.Dir(output))
	if err != nil {
		return fmt.Errorf("setup parent directory must already exist: %w", err)
	}
	parent, err = filepath.Abs(parent)
	if err != nil {
		return err
	}
	for dir := parent; ; dir = filepath.Dir(dir) {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return fmt.Errorf("authentication files must be created outside Git repositories")
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	output = filepath.Join(parent, filepath.Base(output))
	settings.Enabled = true
	settings.Issuer = strings.TrimSuffix(settings.Issuer, "/")
	settings.PublicURL = strings.TrimSuffix(settings.PublicURL, "/")
	settings.ClientID, err = GenerateAPIToken()
	if err != nil {
		return err
	}
	settings.ClientSecretFile = filepath.Join(output, "client-secret")
	if settings.AuthenticationMethod == "" {
		settings.AuthenticationMethod = "webauthn"
	}
	if err := ValidateDashboardAuth(config.ServerConfig{OIDC: settings}); err != nil {
		return err
	}
	secret, err := GenerateAPIToken()
	if err != nil {
		return err
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	key := pbkdf2.Key([]byte(secret), salt, 310000, 64, sha512.New)
	encode := func(b []byte) string { return strings.ReplaceAll(base64.RawStdEncoding.EncodeToString(b), "+", ".") }
	digest := "$pbkdf2-sha512$310000$" + encode(salt) + "$" + encode(key)
	client := map[string]any{
		"client_id": settings.ClientID, "client_name": "FleetDeck Admin", "client_secret": digest,
		"public": false, "authorization_policy": "two_factor", "require_pkce": true, "pkce_challenge_method": "S256",
		"redirect_uris": []string{settings.PublicURL + "/auth/callback"}, "scopes": []string{"openid", "groups"},
		"grant_types": []string{"authorization_code"}, "response_types": []string{"code"}, "response_modes": []string{"query"},
		"token_endpoint_auth_method": "client_secret_basic", "id_token_signed_response_alg": "RS256",
	}
	clientYAML, err := yaml.Marshal(map[string]any{"identity_providers": map[string]any{"oidc": map[string]any{"clients": []any{client}}}})
	if err != nil {
		return err
	}
	fleetConfig, err := toml.Marshal(map[string]any{"server": map[string]any{"oidc": settings}})
	if err != nil {
		return err
	}
	if err := os.Mkdir(output, 0700); err != nil {
		return fmt.Errorf("setup requires a new directory: %w", err)
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(output)
		}
	}()
	for name, data := range map[string][]byte{"client-secret": []byte(secret + "\n"), "authelia-client.yml": clientYAML, "fleetdeck-oidc.toml": fleetConfig} {
		if err := os.WriteFile(filepath.Join(output, name), data, 0600); err != nil {
			return err
		}
	}
	complete = true
	return nil
}

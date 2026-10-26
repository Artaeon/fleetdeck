package server

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/fleetdeck/fleetdeck/internal/config"
)

func httpsURL(raw string, originOnly bool) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || (originOnly && u.Path != "" && u.Path != "/") {
		return nil, fmt.Errorf("an absolute HTTPS URL without credentials, query or fragment is required")
	}
	return u, nil
}

func ValidateDashboardAuth(cfg config.ServerConfig) error {
	if !cfg.OIDC.Enabled {
		return ValidateDashboardTokens(cfg.APIToken, cfg.MetricsToken)
	}
	o := cfg.OIDC
	if _, err := httpsURL(o.Issuer, false); err != nil {
		return fmt.Errorf("OIDC issuer: %w", err)
	}
	if _, err := httpsURL(o.PublicURL, true); err != nil {
		return fmt.Errorf("OIDC public_url: %w", err)
	}
	if strings.TrimSpace(o.ClientID) == "" || strings.TrimSpace(o.AdminGroup) == "" || o.ClientSecretFile == "" {
		return fmt.Errorf("OIDC requires client_id, client_secret_file and admin_group")
	}
	switch o.AuthenticationMethod {
	case "", "webauthn", "hardware", "mfa":
	default:
		return fmt.Errorf("OIDC authentication_method must be webauthn, hardware or mfa")
	}
	if cfg.MetricsToken != "" && (ValidateAPIToken(cfg.MetricsToken) != nil || cfg.MetricsToken == cfg.APIToken) {
		return fmt.Errorf("metrics token must be a distinct random credential of 32–1024 printable ASCII characters")
	}
	return nil
}

func readOIDCSecret(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return "", fmt.Errorf("OIDC client secret must be a private regular file (mode 0600 or 0400)")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("unable to read OIDC client secret")
	}
	secret := strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")
	if err := ValidateAPIToken(secret); err != nil {
		return "", fmt.Errorf("OIDC client secret must contain 32–1024 printable ASCII characters")
	}
	return secret, nil
}

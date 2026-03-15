package bootstrap

import (
	"strings"
	"testing"
)

func TestValidateBootstrapInputs(t *testing.T) {
	tests := []struct {
		name    string
		domain  string
		email   string
		network string
		wantErr bool
	}{
		// Valid inputs
		{
			name:    "valid typical inputs",
			domain:  "my-domain.com",
			email:   "admin@example.com",
			network: "traefik_default",
			wantErr: false,
		},
		{
			name:    "valid domain with dots and hyphens",
			domain:  "sub.my-domain.example.com",
			email:   "ops@fleet.io",
			network: "web",
			wantErr: false,
		},
		{
			name:    "valid network with underscores",
			domain:  "example.com",
			email:   "user@test.org",
			network: "traefik_network_1",
			wantErr: false,
		},
		{
			name:    "valid empty email",
			domain:  "example.com",
			email:   "",
			network: "traefik",
			wantErr: false,
		},

		// Invalid network names
		{
			name:    "invalid network with shell injection semicolon",
			domain:  "example.com",
			email:   "admin@example.com",
			network: "test; rm -rf /",
			wantErr: true,
		},
		{
			name:    "invalid network with command substitution",
			domain:  "example.com",
			email:   "admin@example.com",
			network: "test$(whoami)",
			wantErr: true,
		},
		{
			name:    "invalid empty network",
			domain:  "example.com",
			email:   "admin@example.com",
			network: "",
			wantErr: true,
		},
		{
			name:    "invalid network with spaces",
			domain:  "example.com",
			email:   "admin@example.com",
			network: "my network",
			wantErr: true,
		},
		{
			name:    "invalid network with backticks",
			domain:  "example.com",
			email:   "admin@example.com",
			network: "`whoami`",
			wantErr: true,
		},
		{
			name:    "invalid network starting with hyphen",
			domain:  "example.com",
			email:   "admin@example.com",
			network: "-badname",
			wantErr: true,
		},
		{
			name:    "invalid network with pipe",
			domain:  "example.com",
			email:   "admin@example.com",
			network: "net|cat",
			wantErr: true,
		},

		// Invalid domain names
		{
			name:    "invalid domain with shell injection",
			domain:  "test; cat /etc/passwd",
			email:   "admin@example.com",
			network: "traefik",
			wantErr: true,
		},
		{
			name:    "invalid domain with backticks",
			domain:  "`whoami`.example.com",
			email:   "admin@example.com",
			network: "traefik",
			wantErr: true,
		},
		{
			name:    "invalid empty domain",
			domain:  "",
			email:   "admin@example.com",
			network: "traefik",
			wantErr: true,
		},
		{
			name:    "invalid domain with spaces",
			domain:  "my domain.com",
			email:   "admin@example.com",
			network: "traefik",
			wantErr: true,
		},
		{
			name:    "invalid domain with dollar sign",
			domain:  "test$HOME.com",
			email:   "admin@example.com",
			network: "traefik",
			wantErr: true,
		},
		{
			name:    "invalid domain starting with dot",
			domain:  ".example.com",
			email:   "admin@example.com",
			network: "traefik",
			wantErr: true,
		},

		// Invalid email addresses
		{
			name:    "invalid email without at sign",
			domain:  "example.com",
			email:   "not-an-email",
			network: "traefik",
			wantErr: true,
		},
		{
			name:    "invalid email with shell injection",
			domain:  "example.com",
			email:   "test@foo;echo bad",
			network: "traefik",
			wantErr: true,
		},
		{
			name:    "invalid email with backticks",
			domain:  "example.com",
			email:   "test@`whoami`.com",
			network: "traefik",
			wantErr: true,
		},
		{
			name:    "invalid email with dollar sign",
			domain:  "example.com",
			email:   "test@$HOME.com",
			network: "traefik",
			wantErr: true,
		},
		{
			name:    "invalid email with single quotes",
			domain:  "example.com",
			email:   "test'@example.com",
			network: "traefik",
			wantErr: true,
		},
		{
			name:    "invalid email with double quotes",
			domain:  "example.com",
			email:   "test\"@example.com",
			network: "traefik",
			wantErr: true,
		},
		{
			name:    "invalid email with curly braces",
			domain:  "example.com",
			email:   "test@{example}.com",
			network: "traefik",
			wantErr: true,
		},
		{
			name:    "invalid email with parentheses",
			domain:  "example.com",
			email:   "test@example(bad).com",
			network: "traefik",
			wantErr: true,
		},
		{
			name:    "invalid email with backslash",
			domain:  "example.com",
			email:   "test\\@example.com",
			network: "traefik",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBootstrapInputs(tt.domain, tt.email, tt.network)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateBootstrapInputs(%q, %q, %q) error = %v, wantErr %v",
					tt.domain, tt.email, tt.network, err, tt.wantErr)
			}
		})
	}
}

func TestComposeFileGeneration(t *testing.T) {
	domain := "fleet.example.com"
	email := "admin@example.com"
	network := "traefik_default"

	output := composeFile(domain, email, network)

	// Verify it starts with services definition.
	if !strings.HasPrefix(output, "services:") {
		t.Error("expected compose file to start with 'services:'")
	}

	// Verify Traefik image.
	if !strings.Contains(output, "image: traefik:v3") {
		t.Error("expected compose file to contain traefik:v3 image")
	}

	// Verify port mappings.
	if !strings.Contains(output, `"80:80"`) {
		t.Error("expected compose file to contain port 80 mapping")
	}
	if !strings.Contains(output, `"443:443"`) {
		t.Error("expected compose file to contain port 443 mapping")
	}

	// Verify Docker socket volume.
	if !strings.Contains(output, "/var/run/docker.sock:/var/run/docker.sock:ro") {
		t.Error("expected compose file to mount Docker socket read-only")
	}

	// Verify acme.json volume.
	if !strings.Contains(output, "./acme.json:/acme.json") {
		t.Error("expected compose file to mount acme.json")
	}

	// Verify ACME email.
	if !strings.Contains(output, "acme.email="+email) {
		t.Errorf("expected compose file to contain ACME email %s", email)
	}

	// Verify dashboard is enabled.
	if !strings.Contains(output, "--api.dashboard=true") {
		t.Error("expected compose file to enable Traefik dashboard")
	}

	// Verify entrypoints.
	if !strings.Contains(output, "entrypoints.web.address=:80") {
		t.Error("expected compose file to define web entrypoint on port 80")
	}
	if !strings.Contains(output, "entrypoints.websecure.address=:443") {
		t.Error("expected compose file to define websecure entrypoint on port 443")
	}

	// Verify HTTP to HTTPS redirect.
	if !strings.Contains(output, "entrypoints.web.http.redirections.entrypoint.to=websecure") {
		t.Error("expected compose file to redirect HTTP to HTTPS")
	}

	// Verify Docker provider config.
	if !strings.Contains(output, "providers.docker=true") {
		t.Error("expected compose file to enable Docker provider")
	}
	if !strings.Contains(output, "providers.docker.exposedbydefault=false") {
		t.Error("expected compose file to disable exposed by default")
	}

	// Verify TLS challenge.
	if !strings.Contains(output, "acme.tlschallenge=true") {
		t.Error("expected compose file to enable TLS challenge")
	}

	// Verify restart policy.
	if !strings.Contains(output, "restart: unless-stopped") {
		t.Error("expected compose file to set restart policy to unless-stopped")
	}

	// Verify access log.
	if !strings.Contains(output, "--accesslog=true") {
		t.Error("expected compose file to enable access logging")
	}

	// Verify dashboard labels.
	if !strings.Contains(output, "traefik.enable=true") {
		t.Error("expected compose file to contain traefik.enable=true label")
	}
	if !strings.Contains(output, "traefik.http.routers.dashboard.entrypoints=websecure") {
		t.Error("expected compose file to set dashboard entrypoint to websecure")
	}
	if !strings.Contains(output, "traefik.http.routers.dashboard.tls.certresolver=letsencrypt") {
		t.Error("expected compose file to set dashboard cert resolver to letsencrypt")
	}
	if !strings.Contains(output, "traefik.http.routers.dashboard.service=api@internal") {
		t.Error("expected compose file to route dashboard to api@internal")
	}

	// Verify external network declaration.
	if !strings.Contains(output, "external: true") {
		t.Error("expected compose file to declare network as external")
	}
}

func TestComposeFileNetworkName(t *testing.T) {
	tests := []struct {
		name    string
		network string
	}{
		{"default network", "traefik_default"},
		{"custom network", "my-custom-net"},
		{"underscore network", "web_proxy"},
		{"simple network", "traefik"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := composeFile("example.com", "admin@example.com", tt.network)

			// The network name should appear in the Docker provider config.
			providerLine := "providers.docker.network=" + tt.network
			if !strings.Contains(output, providerLine) {
				t.Errorf("expected compose output to contain %q", providerLine)
			}

			// The network name should appear in the service networks section.
			// The compose file has "    - <network>" under the networks key.
			serviceNetworkLine := "      - " + tt.network
			if !strings.Contains(output, serviceNetworkLine) {
				t.Errorf("expected compose output to contain service network reference %q", serviceNetworkLine)
			}

			// The network name should appear in the top-level networks section.
			// The compose file has "  <network>:" followed by "    external: true".
			topLevelNetwork := "\n  " + tt.network + ":\n    external: true"
			if !strings.Contains(output, topLevelNetwork) {
				t.Errorf("expected compose output to contain top-level network declaration for %q", tt.network)
			}
		})
	}
}

func TestComposeFileDomainInLabels(t *testing.T) {
	tests := []struct {
		name   string
		domain string
	}{
		{"simple domain", "example.com"},
		{"subdomain", "fleet.example.com"},
		{"long subdomain", "app.staging.fleet.example.com"},
		{"hyphenated domain", "my-fleet.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := composeFile(tt.domain, "admin@example.com", "traefik")

			// The domain should appear in the router rule Host() directive,
			// wrapped in backticks as per Traefik label syntax.
			hostDirective := "Host(`" + tt.domain + "`)"
			if !strings.Contains(output, hostDirective) {
				t.Errorf("expected compose output to contain Host directive %q", hostDirective)
			}

			// Verify the PathPrefix directives are present alongside the domain.
			if !strings.Contains(output, "PathPrefix(`/api`)") {
				t.Error("expected compose output to contain PathPrefix for /api")
			}
			if !strings.Contains(output, "PathPrefix(`/dashboard`)") {
				t.Error("expected compose output to contain PathPrefix for /dashboard")
			}

			// Verify the complete router rule structure.
			if !strings.Contains(output, "traefik.http.routers.dashboard.rule=") {
				t.Error("expected compose output to contain dashboard router rule")
			}
		})
	}
}

func TestSafeNameRegex(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"traefik", true},
		{"traefik_default", true},
		{"my-network", true},
		{"web.proxy", true},
		{"net123", true},
		{"A-Za-z0-9", true},
		{"a", true},
		{"1start", true},

		// Invalid
		{"", false},
		{" spaces", false},
		{"-starts-with-hyphen", false},
		{"_starts-with-underscore", false},
		{".starts-with-dot", false},
		{"has space", false},
		{"semi;colon", false},
		{"back`tick", false},
		{"dollar$sign", false},
		{"paren(hesis)", false},
		{"single'quote", false},
		{"double\"quote", false},
		{"pipe|char", false},
		{"amper&sand", false},
		{"slash/char", false},
	}

	for _, tt := range tests {
		got := safeNameRe.MatchString(tt.input)
		if got != tt.valid {
			t.Errorf("safeNameRe.MatchString(%q) = %v, want %v", tt.input, got, tt.valid)
		}
	}
}

package profiles

import (
	"strings"
	"testing"
)

// TestRenderAllProfilesWithRealData renders each profile with realistic data
// and verifies: no template errors, output contains the project name and
// domain, and no leftover template markers ({{) remain.
func TestRenderAllProfilesWithRealData(t *testing.T) {
	profiles := List()

	data := ProfileData{
		Name:            "acme-app",
		Domain:          "acme-app.example.com",
		Port:            3000,
		PostgresVersion: "16",
		RedisVersion:    "7",
		AppType:         "node",
		Framework:       "Express",
	}

	for _, p := range profiles {
		t.Run(p.Name, func(t *testing.T) {
			rendered, err := Render(p.Compose, data)
			if err != nil {
				t.Fatalf("Render() for profile %q returned error: %v", p.Name, err)
			}
			if rendered == "" {
				t.Fatalf("Render() for profile %q returned empty string", p.Name)
			}

			// Output must contain the project name.
			if !strings.Contains(rendered, "acme-app") {
				t.Errorf("rendered output for profile %q does not contain project name 'acme-app'", p.Name)
			}

			// Output must contain the domain.
			if !strings.Contains(rendered, "acme-app.example.com") {
				// Worker profile does not use domain in its compose template.
				if p.Name != "worker" {
					t.Errorf("rendered output for profile %q does not contain domain 'acme-app.example.com'", p.Name)
				}
			}

			// No leftover template markers should remain.
			if strings.Contains(rendered, "{{") {
				t.Errorf("rendered output for profile %q contains leftover template marker '{{'", p.Name)
			}
			if strings.Contains(rendered, "}}") {
				t.Errorf("rendered output for profile %q contains leftover template marker '}}'", p.Name)
			}

			// Must be valid-looking YAML (starts with services:).
			if !strings.Contains(rendered, "services:") {
				t.Errorf("rendered output for profile %q does not contain 'services:' key", p.Name)
			}
		})
	}
}

// TestRenderProfilesWithSpecialDomains tests rendering with domains containing
// staging subdomains, hyphens, and unusual TLDs.
func TestRenderProfilesWithSpecialDomains(t *testing.T) {
	domains := []struct {
		name   string
		domain string
		appName string
	}{
		{
			name:    "staging subdomain",
			domain:  "my-app.staging.example.com",
			appName: "my-app",
		},
		{
			name:    "short TLD",
			domain:  "app-1.test.io",
			appName: "app-1",
		},
		{
			name:    "deep subdomain",
			domain:  "api.v2.prod.company.co.uk",
			appName: "api-service",
		},
		{
			name:    "numeric domain parts",
			domain:  "app42.region1.cloud9.dev",
			appName: "app42",
		},
	}

	profiles := List()

	for _, dd := range domains {
		for _, p := range profiles {
			t.Run(dd.name+"/"+p.Name, func(t *testing.T) {
				data := ProfileData{
					Name:            dd.appName,
					Domain:          dd.domain,
					Port:            8080,
					PostgresVersion: "16",
					RedisVersion:    "7",
				}

				rendered, err := Render(p.Compose, data)
				if err != nil {
					t.Fatalf("Render() returned error: %v", err)
				}

				// No leftover template markers.
				if strings.Contains(rendered, "{{") {
					t.Errorf("rendered output contains leftover template marker '{{'")
				}

				// The app name should appear.
				if !strings.Contains(rendered, dd.appName) {
					t.Errorf("rendered output does not contain app name %q", dd.appName)
				}

				// For profiles that expose domains (all except worker), the
				// domain should appear in the rendered output.
				if p.Name != "worker" {
					if !strings.Contains(rendered, dd.domain) {
						t.Errorf("rendered output for profile %q does not contain domain %q", p.Name, dd.domain)
					}
				}
			})
		}
	}
}

// TestRenderAllEnvTemplates renders env templates for all profiles and
// verifies there are no template errors.
func TestRenderAllEnvTemplates(t *testing.T) {
	profiles := List()

	data := ProfileData{
		Name:            "testservice",
		Domain:          "testservice.example.com",
		Port:            4000,
		PostgresVersion: "16",
		RedisVersion:    "7",
	}

	for _, p := range profiles {
		t.Run(p.Name, func(t *testing.T) {
			rendered, err := RenderEnv(p.EnvTemplate, data)
			if err != nil {
				t.Fatalf("RenderEnv() for profile %q returned error: %v", p.Name, err)
			}
			if rendered == "" {
				t.Fatalf("RenderEnv() for profile %q returned empty string", p.Name)
			}

			// No leftover template markers.
			if strings.Contains(rendered, "{{") {
				t.Errorf("rendered env for profile %q contains leftover template marker '{{'", p.Name)
			}
			if strings.Contains(rendered, "}}") {
				t.Errorf("rendered env for profile %q contains leftover template marker '}}'", p.Name)
			}
		})
	}
}

// TestBareProfileHasNoDatabase renders the bare profile and verifies that
// "postgres" does not appear anywhere in the output.
func TestBareProfileHasNoDatabase(t *testing.T) {
	p, err := Get("bare")
	if err != nil {
		t.Fatalf("Get(bare) error: %v", err)
	}

	data := ProfileData{
		Name:   "bareapp",
		Domain: "bareapp.example.com",
		Port:   3000,
	}

	rendered, err := Render(p.Compose, data)
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	if strings.Contains(strings.ToLower(rendered), "postgres") {
		t.Error("bare profile compose output should not contain 'postgres'")
	}
	if strings.Contains(strings.ToLower(rendered), "redis") {
		t.Error("bare profile compose output should not contain 'redis'")
	}
	if strings.Contains(strings.ToLower(rendered), "minio") {
		t.Error("bare profile compose output should not contain 'minio'")
	}
	if strings.Contains(strings.ToLower(rendered), "mailpit") {
		t.Error("bare profile compose output should not contain 'mailpit'")
	}

	// Also verify the env template has no database references.
	envRendered, err := RenderEnv(p.EnvTemplate, data)
	if err != nil {
		t.Fatalf("RenderEnv() error: %v", err)
	}
	if strings.Contains(strings.ToLower(envRendered), "postgres") {
		t.Error("bare profile env output should not contain 'postgres'")
	}
	if strings.Contains(strings.ToLower(envRendered), "database_url") {
		t.Error("bare profile env output should not contain 'database_url'")
	}
}

// TestServerProfileHasPostgresAndRedis renders the server profile and verifies
// that both postgres and redis services are present.
func TestServerProfileHasPostgresAndRedis(t *testing.T) {
	p, err := Get("server")
	if err != nil {
		t.Fatalf("Get(server) error: %v", err)
	}

	data := ProfileData{
		Name:            "apiserver",
		Domain:          "api.example.com",
		Port:            8080,
		PostgresVersion: "16",
	}

	rendered, err := Render(p.Compose, data)
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	checks := []struct {
		name     string
		contains string
	}{
		{"postgres service", "postgres:"},
		{"postgres image", "postgres:16"},
		{"postgres container", "apiserver-postgres"},
		{"postgres healthcheck", "pg_isready"},
		{"redis service", "redis:"},
		{"redis image", "redis:7-alpine"},
		{"redis container", "apiserver-redis"},
		{"redis healthcheck", "redis-cli"},
		{"DATABASE_URL", "DATABASE_URL"},
		{"REDIS_URL in app", "REDIS_URL: redis://redis:6379"},
		{"app container", "apiserver-app"},
		{"internal network", "apiserver-internal"},
	}

	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if !strings.Contains(rendered, check.contains) {
				t.Errorf("server profile compose does not contain %q", check.contains)
			}
		})
	}
}

// TestSaaSProfileHasAllServices renders the saas profile and verifies that
// postgres, redis, minio, and mailpit services are all present.
func TestSaaSProfileHasAllServices(t *testing.T) {
	p, err := Get("saas")
	if err != nil {
		t.Fatalf("Get(saas) error: %v", err)
	}

	data := ProfileData{
		Name:            "platform",
		Domain:          "platform.example.com",
		Port:            3000,
		PostgresVersion: "16",
	}

	rendered, err := Render(p.Compose, data)
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	serviceChecks := []struct {
		name     string
		contains string
	}{
		{"postgres service defined", "postgres:"},
		{"postgres image", "postgres:16"},
		{"postgres container", "platform-postgres"},
		{"redis service defined", "redis:"},
		{"redis image", "redis:7-alpine"},
		{"redis container", "platform-redis"},
		{"minio service defined", "minio:"},
		{"minio image", "minio/minio:latest"},
		{"minio container", "platform-minio"},
		{"minio S3 endpoint", "S3_ENDPOINT: http://minio:9000"},
		{"mailpit service defined", "mailpit:"},
		{"mailpit image", "axllent/mailpit:latest"},
		{"mailpit container", "platform-mailpit"},
		{"SMTP_HOST in app", "SMTP_HOST: mailpit"},
		{"SMTP_PORT in app", "SMTP_PORT: \"1025\""},
		{"s3 subdomain routing", "s3.platform.example.com"},
		{"mail subdomain routing", "mail.platform.example.com"},
	}

	for _, check := range serviceChecks {
		t.Run(check.name, func(t *testing.T) {
			if !strings.Contains(rendered, check.contains) {
				t.Errorf("saas profile compose does not contain %q", check.contains)
			}
		})
	}
}

// TestFullstackProfileHasFrontendAndBackend renders the fullstack profile and
// verifies that both Dockerfile.frontend and Dockerfile.backend are referenced.
func TestFullstackProfileHasFrontendAndBackend(t *testing.T) {
	p, err := Get("fullstack")
	if err != nil {
		t.Fatalf("Get(fullstack) error: %v", err)
	}

	data := ProfileData{
		Name:            "myplatform",
		Domain:          "myplatform.io",
		Port:            4000,
		PostgresVersion: "16",
	}

	rendered, err := Render(p.Compose, data)
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	checks := []struct {
		name     string
		contains string
	}{
		{"Dockerfile.frontend", "Dockerfile.frontend"},
		{"Dockerfile.backend", "Dockerfile.backend"},
		{"frontend service", "frontend:"},
		{"backend service", "backend:"},
		{"frontend image", "myplatform-frontend:local"},
		{"backend image", "myplatform-backend:local"},
		{"frontend container", "myplatform-frontend"},
		{"backend container", "myplatform-backend"},
		{"API subdomain", "api.myplatform.io"},
		{"frontend domain", "myplatform.io"},
		{"CORS_ORIGIN", "CORS_ORIGIN: https://myplatform.io"},
		{"NEXT_PUBLIC_API_URL", "NEXT_PUBLIC_API_URL: https://api.myplatform.io"},
		{"frontend port 3000", "PORT: \"3000\""},
		{"backend port 4000", "PORT: \"4000\""},
	}

	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if !strings.Contains(rendered, check.contains) {
				t.Errorf("fullstack profile compose does not contain %q\nGot:\n%s", check.contains, rendered)
			}
		})
	}
}

// TestWorkerProfileHasNoTraefikLabels renders the worker profile and verifies
// that the worker service has no traefik.enable=true label.
func TestWorkerProfileHasNoTraefikLabels(t *testing.T) {
	p, err := Get("worker")
	if err != nil {
		t.Fatalf("Get(worker) error: %v", err)
	}

	data := ProfileData{
		Name:            "bgworker",
		Domain:          "worker.example.com",
		Port:            3000,
		PostgresVersion: "16",
	}

	rendered, err := Render(p.Compose, data)
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	// The worker profile should not have traefik labels on any service.
	if strings.Contains(rendered, "traefik.enable=true") {
		t.Error("worker profile should not contain 'traefik.enable=true'")
	}
	if strings.Contains(rendered, "traefik.http.routers") {
		t.Error("worker profile should not contain traefik router labels")
	}

	// Worker should not use the traefik_default network.
	if strings.Contains(rendered, "traefik_default") {
		t.Error("worker profile should not reference traefik_default network")
	}

	// Worker should have its own internal network.
	if !strings.Contains(rendered, "bgworker-internal") {
		t.Error("worker profile should define 'bgworker-internal' network")
	}

	// Verify the worker service still has the correct environment.
	if !strings.Contains(rendered, "DATABASE_URL") {
		t.Error("worker profile should contain DATABASE_URL environment variable")
	}
	if !strings.Contains(rendered, "REDIS_URL: redis://redis:6379") {
		t.Error("worker profile should contain REDIS_URL environment variable")
	}
}

// TestStaticProfileHasNginxAndCacheHeaders renders the static profile and
// verifies it contains cache-control headers in the compose output.
func TestStaticProfileHasNginxAndCacheHeaders(t *testing.T) {
	p, err := Get("static")
	if err != nil {
		t.Fatalf("Get(static) error: %v", err)
	}

	data := ProfileData{
		Name:   "docs-site",
		Domain: "docs.example.com",
		Port:   80,
	}

	rendered, err := Render(p.Compose, data)
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	checks := []struct {
		name     string
		contains string
	}{
		{"nginx image", "nginx:alpine"},
		{"container name", "docs-site-nginx"},
		{"public volume", "./public:/usr/share/nginx/html:ro"},
		{"nginx conf", "./nginx.conf:/etc/nginx/conf.d/default.conf:ro"},
		{"traefik enabled", "traefik.enable=true"},
		{"domain routing", "docs.example.com"},
		{"Cache-Control header", "Cache-Control=public, max-age=31536000"},
		{"loadbalancer port 80", "loadbalancer.server.port=80"},
	}

	for _, check := range checks {
		t.Run("compose/"+check.name, func(t *testing.T) {
			if !strings.Contains(rendered, check.contains) {
				t.Errorf("static profile compose does not contain %q", check.contains)
			}
		})
	}

	// Verify the Nginx config field has cache-related directives.
	if p.Nginx == "" {
		t.Fatal("static profile Nginx config is empty")
	}

	nginxChecks := []struct {
		name     string
		contains string
	}{
		{"gzip on", "gzip on"},
		{"expires 1y", "expires 1y"},
		{"public immutable", "public, immutable"},
		{"try_files fallback", "try_files $uri $uri/ /index.html"},
		{"X-Frame-Options", "X-Frame-Options"},
		{"X-Content-Type-Options", "X-Content-Type-Options"},
	}

	for _, check := range nginxChecks {
		t.Run("nginx/"+check.name, func(t *testing.T) {
			if !strings.Contains(p.Nginx, check.contains) {
				t.Errorf("static profile Nginx config does not contain %q", check.contains)
			}
		})
	}
}

// TestRenderProfilesWithZeroPort verifies that profiles render properly even
// when port is zero (the default int value).
func TestRenderProfilesWithZeroPort(t *testing.T) {
	profiles := List()

	data := ProfileData{
		Name:            "zeroport",
		Domain:          "zeroport.example.com",
		PostgresVersion: "16",
		Port:            0,
	}

	for _, p := range profiles {
		t.Run(p.Name, func(t *testing.T) {
			rendered, err := Render(p.Compose, data)
			if err != nil {
				t.Fatalf("Render() for profile %q with zero port returned error: %v", p.Name, err)
			}
			if rendered == "" {
				t.Fatalf("Render() for profile %q returned empty string", p.Name)
			}
			// No template errors means no leftover markers.
			if strings.Contains(rendered, "{{") {
				t.Errorf("rendered output for profile %q contains leftover template marker '{{'", p.Name)
			}
		})
	}
}

// TestRenderProfilesWithEmptyName verifies that profiles render without
// template errors when the name is empty (which could produce odd YAML but
// should not cause a template parse/execution error).
func TestRenderProfilesWithEmptyName(t *testing.T) {
	profiles := List()

	data := ProfileData{
		Domain:          "example.com",
		Port:            8080,
		PostgresVersion: "16",
	}

	for _, p := range profiles {
		t.Run(p.Name, func(t *testing.T) {
			rendered, err := Render(p.Compose, data)
			if err != nil {
				t.Fatalf("Render() for profile %q with empty name returned error: %v", p.Name, err)
			}
			if rendered == "" {
				t.Fatalf("Render() for profile %q returned empty string", p.Name)
			}
			if strings.Contains(rendered, "{{") {
				t.Errorf("rendered output for profile %q contains leftover template marker '{{'", p.Name)
			}
		})
	}
}

// TestAllProfilesHaveRequiredServices verifies that every profile has at least
// one required service.
func TestAllProfilesHaveRequiredServices(t *testing.T) {
	profiles := List()

	for _, p := range profiles {
		t.Run(p.Name, func(t *testing.T) {
			hasRequired := false
			for _, s := range p.Services {
				if s.Required {
					hasRequired = true
					break
				}
			}
			if !hasRequired {
				t.Errorf("profile %q has no required services", p.Name)
			}
		})
	}
}

// TestAllServicesHaveDescriptions verifies that every service in every profile
// has a non-empty description.
func TestAllServicesHaveDescriptions(t *testing.T) {
	profiles := List()

	for _, p := range profiles {
		for _, s := range p.Services {
			t.Run(p.Name+"/"+s.Name, func(t *testing.T) {
				if s.Description == "" {
					t.Errorf("service %q in profile %q has no description", s.Name, p.Name)
				}
				if s.Image == "" {
					t.Errorf("service %q in profile %q has no image", s.Name, p.Name)
				}
			})
		}
	}
}

// TestAllProfilesHaveDescriptions verifies that every profile has a non-empty
// description.
func TestAllProfilesHaveDescriptions(t *testing.T) {
	profiles := List()

	for _, p := range profiles {
		t.Run(p.Name, func(t *testing.T) {
			if p.Description == "" {
				t.Errorf("profile %q has no description", p.Name)
			}
		})
	}
}

// TestRenderEnvContainsPortForNonStatic verifies that all non-static profiles
// include a PORT variable in their rendered env template.
func TestRenderEnvContainsPortForNonStatic(t *testing.T) {
	profiles := List()

	data := ProfileData{
		Name:            "portcheck",
		Domain:          "portcheck.example.com",
		Port:            9090,
		PostgresVersion: "16",
	}

	for _, p := range profiles {
		t.Run(p.Name, func(t *testing.T) {
			rendered, err := RenderEnv(p.EnvTemplate, data)
			if err != nil {
				t.Fatalf("RenderEnv() error: %v", err)
			}

			if p.Name == "static" {
				// Static profile has a comment-only env template.
				return
			}

			if !strings.Contains(rendered, "PORT=9090") {
				t.Errorf("env for profile %q does not contain 'PORT=9090'", p.Name)
			}
		})
	}
}

// TestProfilesWithDatabaseHaveDatabaseEnv verifies that profiles with a
// postgres service include POSTGRES_USER, POSTGRES_PASSWORD, and POSTGRES_DB
// in their env template.
func TestProfilesWithDatabaseHaveDatabaseEnv(t *testing.T) {
	profilesWithDB := []string{"server", "saas", "worker", "fullstack"}

	data := ProfileData{
		Name:            "dbapp",
		Domain:          "dbapp.example.com",
		Port:            3000,
		PostgresVersion: "16",
	}

	for _, profileName := range profilesWithDB {
		t.Run(profileName, func(t *testing.T) {
			p, err := Get(profileName)
			if err != nil {
				t.Fatalf("Get(%q) error: %v", profileName, err)
			}

			rendered, err := RenderEnv(p.EnvTemplate, data)
			if err != nil {
				t.Fatalf("RenderEnv() error: %v", err)
			}

			for _, envVar := range []string{"POSTGRES_USER=dbapp", "POSTGRES_DB=dbapp"} {
				if !strings.Contains(rendered, envVar) {
					t.Errorf("env for profile %q does not contain %q", profileName, envVar)
				}
			}

			// POSTGRES_PASSWORD should start with the project name.
			if !strings.Contains(rendered, "POSTGRES_PASSWORD=dbapp") {
				t.Errorf("env for profile %q does not have POSTGRES_PASSWORD starting with project name", profileName)
			}
		})
	}
}

// TestWorkerAndBareHaveDifferentNetworkSetups verifies that worker uses only
// an internal network while bare uses the traefik_default network.
func TestWorkerAndBareHaveDifferentNetworkSetups(t *testing.T) {
	data := ProfileData{
		Name:            "nettest",
		Domain:          "nettest.example.com",
		Port:            3000,
		PostgresVersion: "16",
	}

	bare, err := Get("bare")
	if err != nil {
		t.Fatalf("Get(bare) error: %v", err)
	}
	bareRendered, err := Render(bare.Compose, data)
	if err != nil {
		t.Fatalf("Render(bare) error: %v", err)
	}

	worker, err := Get("worker")
	if err != nil {
		t.Fatalf("Get(worker) error: %v", err)
	}
	workerRendered, err := Render(worker.Compose, data)
	if err != nil {
		t.Fatalf("Render(worker) error: %v", err)
	}

	// Bare should reference traefik_default.
	if !strings.Contains(bareRendered, "traefik_default") {
		t.Error("bare profile should reference traefik_default network")
	}

	// Worker should NOT reference traefik_default.
	if strings.Contains(workerRendered, "traefik_default") {
		t.Error("worker profile should NOT reference traefik_default network")
	}

	// Worker should have its own internal network.
	if !strings.Contains(workerRendered, "nettest-internal") {
		t.Error("worker profile should have 'nettest-internal' network")
	}
}

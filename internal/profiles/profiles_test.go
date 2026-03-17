package profiles

import (
	"strings"
	"testing"
)

func TestGetProfile(t *testing.T) {
	tests := []struct {
		name        string
		profileName string
		wantDesc    string
	}{
		{
			name:        "bare profile",
			profileName: "bare",
			wantDesc:    "App container only with Traefik routing. No database, no extras.",
		},
		{
			name:        "server profile",
			profileName: "server",
			wantDesc:    "App + PostgreSQL + Redis + automated backups. For APIs and backends.",
		},
		{
			name:        "saas profile",
			profileName: "saas",
			wantDesc:    "Full SaaS stack: App + PostgreSQL + Redis + S3 (MinIO) + email relay + cron.",
		},
		{
			name:        "static profile",
			profileName: "static",
			wantDesc:    "Nginx serving static files with CDN headers. For landing pages and docs.",
		},
		{
			name:        "worker profile",
			profileName: "worker",
			wantDesc:    "Background job runner with Redis queue. No HTTP exposure.",
		},
		{
			name:        "fullstack profile",
			profileName: "fullstack",
			wantDesc:    "Frontend + Backend + DB + Redis + S3. For monorepo SaaS applications.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := Get(tt.profileName)
			if err != nil {
				t.Fatalf("Get(%q) returned unexpected error: %v", tt.profileName, err)
			}
			if p == nil {
				t.Fatalf("Get(%q) returned nil profile", tt.profileName)
			}
			if p.Name != tt.profileName {
				t.Errorf("Get(%q).Name = %q, want %q", tt.profileName, p.Name, tt.profileName)
			}
			if p.Description != tt.wantDesc {
				t.Errorf("Get(%q).Description = %q, want %q", tt.profileName, p.Description, tt.wantDesc)
			}
			if p.Compose == "" {
				t.Errorf("Get(%q).Compose is empty", tt.profileName)
			}
			if p.EnvTemplate == "" {
				t.Errorf("Get(%q).EnvTemplate is empty", tt.profileName)
			}
		})
	}
}

func TestGetProfileNotFound(t *testing.T) {
	tests := []struct {
		name        string
		profileName string
	}{
		{name: "empty name", profileName: ""},
		{name: "unknown profile", profileName: "nonexistent"},
		{name: "typo in bare", profileName: "bares"},
		{name: "uppercase", profileName: "BARE"},
		{name: "mixed case", profileName: "Server"},
		{name: "special characters", profileName: "bare!@#"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := Get(tt.profileName)
			if err == nil {
				t.Fatalf("Get(%q) expected error, got profile %v", tt.profileName, p)
			}
			if p != nil {
				t.Errorf("Get(%q) expected nil profile on error, got %v", tt.profileName, p)
			}
			if !strings.Contains(err.Error(), "not found") {
				t.Errorf("Get(%q) error = %q, want it to contain 'not found'", tt.profileName, err.Error())
			}
		})
	}
}

func TestListProfiles(t *testing.T) {
	profiles := List()

	if len(profiles) != 6 {
		t.Fatalf("List() returned %d profiles, want 6", len(profiles))
	}

	expectedNames := map[string]bool{
		"bare":      false,
		"server":    false,
		"saas":      false,
		"static":    false,
		"worker":    false,
		"fullstack": false,
	}

	for _, p := range profiles {
		if _, ok := expectedNames[p.Name]; !ok {
			t.Errorf("List() returned unexpected profile %q", p.Name)
			continue
		}
		expectedNames[p.Name] = true
	}

	for name, found := range expectedNames {
		if !found {
			t.Errorf("List() missing expected profile %q", name)
		}
	}
}

func TestProfileServices(t *testing.T) {
	tests := []struct {
		name             string
		profileName      string
		expectedServices []string
		expectedCount    int
	}{
		{
			name:             "bare has only app",
			profileName:      "bare",
			expectedServices: []string{"app"},
			expectedCount:    1,
		},
		{
			name:             "server has app, postgres, redis",
			profileName:      "server",
			expectedServices: []string{"app", "postgres", "redis"},
			expectedCount:    3,
		},
		{
			name:             "saas has app, postgres, redis, minio, mailpit",
			profileName:      "saas",
			expectedServices: []string{"app", "postgres", "redis", "minio", "mailpit"},
			expectedCount:    5,
		},
		{
			name:             "static has only nginx",
			profileName:      "static",
			expectedServices: []string{"nginx"},
			expectedCount:    1,
		},
		{
			name:             "worker has worker, redis, postgres",
			profileName:      "worker",
			expectedServices: []string{"worker", "redis", "postgres"},
			expectedCount:    3,
		},
		{
			name:             "fullstack has frontend, backend, postgres, redis, minio",
			profileName:      "fullstack",
			expectedServices: []string{"frontend", "backend", "postgres", "redis", "minio"},
			expectedCount:    5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := Get(tt.profileName)
			if err != nil {
				t.Fatalf("Get(%q) returned error: %v", tt.profileName, err)
			}

			if len(p.Services) != tt.expectedCount {
				t.Fatalf("profile %q has %d services, want %d", tt.profileName, len(p.Services), tt.expectedCount)
			}

			serviceMap := make(map[string]bool)
			for _, s := range p.Services {
				serviceMap[s.Name] = true
			}

			for _, expected := range tt.expectedServices {
				if !serviceMap[expected] {
					t.Errorf("profile %q missing expected service %q; has %v", tt.profileName, expected, p.ServiceNames())
				}
			}
		})
	}
}

func TestProfileServiceNames(t *testing.T) {
	tests := []struct {
		name        string
		profileName string
		wantNames   []string
	}{
		{
			name:        "bare service names",
			profileName: "bare",
			wantNames:   []string{"app"},
		},
		{
			name:        "server service names",
			profileName: "server",
			wantNames:   []string{"app", "postgres", "redis"},
		},
		{
			name:        "saas service names",
			profileName: "saas",
			wantNames:   []string{"app", "postgres", "redis", "minio", "mailpit"},
		},
		{
			name:        "static service names",
			profileName: "static",
			wantNames:   []string{"nginx"},
		},
		{
			name:        "worker service names",
			profileName: "worker",
			wantNames:   []string{"worker", "redis", "postgres"},
		},
		{
			name:        "fullstack service names",
			profileName: "fullstack",
			wantNames:   []string{"frontend", "backend", "postgres", "redis", "minio"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := Get(tt.profileName)
			if err != nil {
				t.Fatalf("Get(%q) returned error: %v", tt.profileName, err)
			}

			names := p.ServiceNames()
			if len(names) != len(tt.wantNames) {
				t.Fatalf("ServiceNames() returned %d names, want %d: got %v", len(names), len(tt.wantNames), names)
			}

			// Verify order matches the registration order
			for i, want := range tt.wantNames {
				if names[i] != want {
					t.Errorf("ServiceNames()[%d] = %q, want %q", i, names[i], want)
				}
			}
		})
	}
}

func TestProfileHasService(t *testing.T) {
	tests := []struct {
		name        string
		profileName string
		service     string
		want        bool
	}{
		// bare profile
		{name: "bare has app", profileName: "bare", service: "app", want: true},
		{name: "bare no postgres", profileName: "bare", service: "postgres", want: false},
		{name: "bare no redis", profileName: "bare", service: "redis", want: false},
		{name: "bare no nginx", profileName: "bare", service: "nginx", want: false},

		// server profile
		{name: "server has app", profileName: "server", service: "app", want: true},
		{name: "server has postgres", profileName: "server", service: "postgres", want: true},
		{name: "server has redis", profileName: "server", service: "redis", want: true},
		{name: "server no minio", profileName: "server", service: "minio", want: false},
		{name: "server no nginx", profileName: "server", service: "nginx", want: false},

		// saas profile
		{name: "saas has app", profileName: "saas", service: "app", want: true},
		{name: "saas has postgres", profileName: "saas", service: "postgres", want: true},
		{name: "saas has redis", profileName: "saas", service: "redis", want: true},
		{name: "saas has minio", profileName: "saas", service: "minio", want: true},
		{name: "saas has mailpit", profileName: "saas", service: "mailpit", want: true},
		{name: "saas no nginx", profileName: "saas", service: "nginx", want: false},

		// static profile
		{name: "static has nginx", profileName: "static", service: "nginx", want: true},
		{name: "static no app", profileName: "static", service: "app", want: false},
		{name: "static no postgres", profileName: "static", service: "postgres", want: false},

		// worker profile
		{name: "worker has worker", profileName: "worker", service: "worker", want: true},
		{name: "worker has redis", profileName: "worker", service: "redis", want: true},
		{name: "worker has postgres", profileName: "worker", service: "postgres", want: true},
		{name: "worker no app", profileName: "worker", service: "app", want: false},
		{name: "worker no minio", profileName: "worker", service: "minio", want: false},

		// fullstack profile
		{name: "fullstack has frontend", profileName: "fullstack", service: "frontend", want: true},
		{name: "fullstack has backend", profileName: "fullstack", service: "backend", want: true},
		{name: "fullstack has postgres", profileName: "fullstack", service: "postgres", want: true},
		{name: "fullstack has redis", profileName: "fullstack", service: "redis", want: true},
		{name: "fullstack has minio", profileName: "fullstack", service: "minio", want: true},
		{name: "fullstack no mailpit", profileName: "fullstack", service: "mailpit", want: false},

		// edge cases
		{name: "empty service name", profileName: "bare", service: "", want: false},
		{name: "service name with spaces", profileName: "bare", service: " app ", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := Get(tt.profileName)
			if err != nil {
				t.Fatalf("Get(%q) returned error: %v", tt.profileName, err)
			}

			got := p.HasService(tt.service)
			if got != tt.want {
				t.Errorf("profile %q HasService(%q) = %v, want %v", tt.profileName, tt.service, got, tt.want)
			}
		})
	}
}

func TestRenderCompose(t *testing.T) {
	p, err := Get("bare")
	if err != nil {
		t.Fatalf("Get(bare) returned error: %v", err)
	}

	data := ProfileData{
		Name:   "myapp",
		Domain: "myapp.example.com",
		Port:   3000,
	}

	rendered, err := Render(p.Compose, data)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	tests := []struct {
		name     string
		contains string
	}{
		{name: "contains app name in image", contains: "myapp:local"},
		{name: "contains container name", contains: "myapp-app"},
		{name: "contains domain", contains: "myapp.example.com"},
		{name: "contains port", contains: "3000"},
		{name: "contains traefik enable", contains: "traefik.enable=true"},
		{name: "contains services key", contains: "services:"},
		{name: "contains networks key", contains: "networks:"},
		{name: "contains traefik_default", contains: "traefik_default"},
		{name: "contains router rule", contains: "traefik.http.routers.myapp.rule=Host"},
		{name: "contains TLS config", contains: "traefik.http.routers.myapp.tls=true"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(rendered, tt.contains) {
				t.Errorf("Render() output does not contain %q\nGot:\n%s", tt.contains, rendered)
			}
		})
	}
}

func TestRenderEnv(t *testing.T) {
	p, err := Get("server")
	if err != nil {
		t.Fatalf("Get(server) returned error: %v", err)
	}

	data := ProfileData{
		Name:   "myapi",
		Domain: "api.example.com",
		Port:   8080,
	}

	rendered, err := RenderEnv(p.EnvTemplate, data)
	if err != nil {
		t.Fatalf("RenderEnv() returned error: %v", err)
	}

	tests := []struct {
		name     string
		contains string
	}{
		{name: "contains POSTGRES_USER", contains: "POSTGRES_USER=myapi"},
		{name: "contains POSTGRES_PASSWORD prefix", contains: "POSTGRES_PASSWORD=myapi_CHANGEME_"},
		{name: "contains POSTGRES_DB", contains: "POSTGRES_DB=myapi"},
		{name: "contains REDIS_URL", contains: "REDIS_URL=redis://redis:6379"},
		{name: "contains PORT", contains: "PORT=8080"},
		{name: "contains NODE_ENV", contains: "NODE_ENV=production"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(rendered, tt.contains) {
				t.Errorf("RenderEnv() output does not contain %q\nGot:\n%s", tt.contains, rendered)
			}
		})
	}
}

func TestRenderWithSpecialChars(t *testing.T) {
	tests := []struct {
		name       string
		data       ProfileData
		wantInOutput []string
		wantErr    bool
	}{
		{
			name: "domain with subdomain",
			data: ProfileData{
				Name:   "myapp",
				Domain: "app.sub.example.com",
				Port:   3000,
			},
			wantInOutput: []string{"app.sub.example.com"},
			wantErr:      false,
		},
		{
			name: "domain with hyphen",
			data: ProfileData{
				Name:   "my-app",
				Domain: "my-app.example.com",
				Port:   3000,
			},
			wantInOutput: []string{"my-app.example.com", "my-app:local", "my-app-app"},
			wantErr:      false,
		},
		{
			name: "domain with numbers",
			data: ProfileData{
				Name:   "app123",
				Domain: "app123.example.com",
				Port:   9999,
			},
			wantInOutput: []string{"app123.example.com", "app123:local", "9999"},
			wantErr:      false,
		},
		{
			name: "name with underscores",
			data: ProfileData{
				Name:   "my_app",
				Domain: "my-app.example.com",
				Port:   3000,
			},
			wantInOutput: []string{"my_app:local", "my_app-app"},
			wantErr:      false,
		},
		{
			name: "port boundary low",
			data: ProfileData{
				Name:   "testapp",
				Domain: "test.example.com",
				Port:   1,
			},
			wantInOutput: []string{"server.port=1"},
			wantErr:      false,
		},
		{
			name: "port boundary high",
			data: ProfileData{
				Name:   "testapp",
				Domain: "test.example.com",
				Port:   65535,
			},
			wantInOutput: []string{"server.port=65535"},
			wantErr:      false,
		},
	}

	p, err := Get("bare")
	if err != nil {
		t.Fatalf("Get(bare) returned error: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rendered, err := Render(p.Compose, tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Render() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			for _, want := range tt.wantInOutput {
				if !strings.Contains(rendered, want) {
					t.Errorf("Render() output does not contain %q\nGot:\n%s", want, rendered)
				}
			}
		})
	}
}

func TestProfileOrder(t *testing.T) {
	profiles := List()

	expectedOrder := []string{"bare", "server", "saas", "static", "worker", "fullstack"}

	if len(profiles) < len(expectedOrder) {
		t.Fatalf("List() returned %d profiles, want at least %d", len(profiles), len(expectedOrder))
	}

	for i, expected := range expectedOrder {
		if profiles[i].Name != expected {
			t.Errorf("List()[%d].Name = %q, want %q", i, profiles[i].Name, expected)
		}
	}
}

func TestSaaSProfileHasMinIO(t *testing.T) {
	p, err := Get("saas")
	if err != nil {
		t.Fatalf("Get(saas) returned error: %v", err)
	}

	if !p.HasService("minio") {
		t.Error("saas profile does not have minio service")
	}

	// Verify minio service details
	var minioService *Service
	for i := range p.Services {
		if p.Services[i].Name == "minio" {
			minioService = &p.Services[i]
			break
		}
	}

	if minioService == nil {
		t.Fatal("minio service not found in saas profile services slice")
	}

	if minioService.Image != "minio/minio:latest" {
		t.Errorf("minio service image = %q, want %q", minioService.Image, "minio/minio:latest")
	}

	// Verify minio appears in the compose template
	data := ProfileData{
		Name:            "testapp",
		Domain:          "test.example.com",
		Port:            3000,
		PostgresVersion: "16",
	}

	rendered, err := Render(p.Compose, data)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	minioChecks := []struct {
		name     string
		contains string
	}{
		{name: "minio service definition", contains: "minio:"},
		{name: "minio image", contains: "minio/minio:latest"},
		{name: "minio container name", contains: "testapp-minio"},
		{name: "minio data volume", contains: "minio_data:/data"},
		{name: "minio server command", contains: "server /data"},
		{name: "minio console address", contains: ":9001"},
		{name: "minio healthcheck", contains: "mc"},
		{name: "S3 endpoint in app", contains: "S3_ENDPOINT: http://minio:9000"},
		{name: "traefik s3 router", contains: "traefik.http.routers.testapp-s3"},
		{name: "s3 subdomain", contains: "s3.test.example.com"},
	}

	for _, check := range minioChecks {
		t.Run(check.name, func(t *testing.T) {
			if !strings.Contains(rendered, check.contains) {
				t.Errorf("rendered saas compose does not contain %q", check.contains)
			}
		})
	}
}

func TestWorkerProfileNoTraefik(t *testing.T) {
	p, err := Get("worker")
	if err != nil {
		t.Fatalf("Get(worker) returned error: %v", err)
	}

	data := ProfileData{
		Name:            "bgworker",
		Domain:          "worker.example.com",
		Port:            3000,
		PostgresVersion: "16",
	}

	rendered, err := Render(p.Compose, data)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	// The worker profile should have no traefik labels
	if strings.Contains(rendered, "traefik.enable") {
		t.Error("worker profile compose contains 'traefik.enable' but should not expose via traefik")
	}

	if strings.Contains(rendered, "traefik.http.routers") {
		t.Error("worker profile compose contains traefik router labels but should not")
	}

	// Worker should not be on the traefik_default network
	if strings.Contains(rendered, "traefik_default") {
		t.Error("worker profile compose references traefik_default network but should not")
	}

	// Worker should use its own internal network only
	if !strings.Contains(rendered, "bgworker-internal") {
		t.Error("worker profile compose does not contain internal network 'bgworker-internal'")
	}

	// Verify the worker service has no labels section at all (no exposed ports)
	lines := strings.Split(rendered, "\n")
	inWorkerService := false
	inNextService := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "worker:" {
			inWorkerService = true
			continue
		}
		if inWorkerService && (trimmed == "redis:" || trimmed == "postgres:" || trimmed == "networks:") {
			inNextService = true
		}
		if inWorkerService && !inNextService && strings.Contains(trimmed, "labels:") {
			t.Error("worker service definition contains 'labels:' section but should not")
		}
	}
}

func TestStaticProfileHasNginx(t *testing.T) {
	p, err := Get("static")
	if err != nil {
		t.Fatalf("Get(static) returned error: %v", err)
	}

	// Verify nginx service exists
	if !p.HasService("nginx") {
		t.Error("static profile does not have nginx service")
	}

	// Verify nginx service details
	var nginxService *Service
	for i := range p.Services {
		if p.Services[i].Name == "nginx" {
			nginxService = &p.Services[i]
			break
		}
	}

	if nginxService == nil {
		t.Fatal("nginx service not found in static profile services slice")
	}

	if nginxService.Image != "nginx:alpine" {
		t.Errorf("nginx service image = %q, want %q", nginxService.Image, "nginx:alpine")
	}

	if !nginxService.Required {
		t.Error("nginx service should be required in static profile")
	}

	// Verify Nginx config field is populated
	if p.Nginx == "" {
		t.Fatal("static profile Nginx config is empty")
	}

	nginxConfigChecks := []struct {
		name     string
		contains string
	}{
		{name: "listen directive", contains: "listen 80"},
		{name: "root directive", contains: "root /usr/share/nginx/html"},
		{name: "index directive", contains: "index index.html"},
		{name: "gzip enabled", contains: "gzip on"},
		{name: "gzip types", contains: "gzip_types"},
		{name: "SPA fallback", contains: "try_files $uri $uri/ /index.html"},
		{name: "X-Frame-Options header", contains: "X-Frame-Options"},
		{name: "X-Content-Type-Options header", contains: "X-Content-Type-Options"},
		{name: "cache control for static assets", contains: "expires 1y"},
		{name: "immutable cache header", contains: "public, immutable"},
	}

	for _, check := range nginxConfigChecks {
		t.Run(check.name, func(t *testing.T) {
			if !strings.Contains(p.Nginx, check.contains) {
				t.Errorf("static profile Nginx config does not contain %q", check.contains)
			}
		})
	}

	// Verify the compose template uses nginx image
	data := ProfileData{
		Name:   "docs-site",
		Domain: "docs.example.com",
		Port:   80,
	}

	rendered, err := Render(p.Compose, data)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	composeChecks := []struct {
		name     string
		contains string
	}{
		{name: "nginx image in compose", contains: "nginx:alpine"},
		{name: "nginx container name", contains: "docs-site-nginx"},
		{name: "public volume mount", contains: "./public:/usr/share/nginx/html:ro"},
		{name: "nginx conf mount", contains: "./nginx.conf:/etc/nginx/conf.d/default.conf:ro"},
		{name: "traefik routing", contains: "traefik.enable=true"},
		{name: "domain in router", contains: "docs.example.com"},
		{name: "cache control header middleware", contains: "Cache-Control=public, max-age=31536000"},
	}

	for _, check := range composeChecks {
		t.Run("compose_"+check.name, func(t *testing.T) {
			if !strings.Contains(rendered, check.contains) {
				t.Errorf("rendered static compose does not contain %q", check.contains)
			}
		})
	}
}

func TestRenderAllProfilesCompose(t *testing.T) {
	profiles := List()

	data := ProfileData{
		Name:            "testproject",
		Domain:          "testproject.example.com",
		Port:            8080,
		PostgresVersion: "16",
		RedisVersion:    "7",
	}

	for _, p := range profiles {
		t.Run(p.Name, func(t *testing.T) {
			rendered, err := Render(p.Compose, data)
			if err != nil {
				t.Fatalf("Render() for profile %q returned error: %v", p.Name, err)
			}
			if rendered == "" {
				t.Errorf("Render() for profile %q returned empty string", p.Name)
			}
			if !strings.Contains(rendered, "services:") {
				t.Errorf("Render() for profile %q does not contain 'services:' key", p.Name)
			}
		})
	}
}

func TestRenderAllProfilesEnv(t *testing.T) {
	profiles := List()

	data := ProfileData{
		Name:            "testproject",
		Domain:          "testproject.example.com",
		Port:            8080,
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
				t.Errorf("RenderEnv() for profile %q returned empty string", p.Name)
			}
		})
	}
}

func TestRenderInvalidTemplate(t *testing.T) {
	tests := []struct {
		name string
		tmpl string
	}{
		{name: "unclosed action", tmpl: "{{.Name"},
		{name: "unknown function", tmpl: "{{unknownFunc .Name}}"},
	}

	data := ProfileData{
		Name:   "test",
		Domain: "test.example.com",
		Port:   3000,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Render(tt.tmpl, data)
			if err == nil {
				t.Error("Render() with invalid template expected error, got nil")
			}
		})
	}
}

func TestProfileServiceRequiredFlag(t *testing.T) {
	tests := []struct {
		name        string
		profileName string
		service     string
		required    bool
	}{
		// bare
		{name: "bare app required", profileName: "bare", service: "app", required: true},

		// server
		{name: "server app required", profileName: "server", service: "app", required: true},
		{name: "server postgres required", profileName: "server", service: "postgres", required: true},
		{name: "server redis optional", profileName: "server", service: "redis", required: false},

		// saas
		{name: "saas app required", profileName: "saas", service: "app", required: true},
		{name: "saas postgres required", profileName: "saas", service: "postgres", required: true},
		{name: "saas redis required", profileName: "saas", service: "redis", required: true},
		{name: "saas minio optional", profileName: "saas", service: "minio", required: false},
		{name: "saas mailpit optional", profileName: "saas", service: "mailpit", required: false},

		// static
		{name: "static nginx required", profileName: "static", service: "nginx", required: true},

		// worker
		{name: "worker worker required", profileName: "worker", service: "worker", required: true},
		{name: "worker redis required", profileName: "worker", service: "redis", required: true},
		{name: "worker postgres optional", profileName: "worker", service: "postgres", required: false},

		// fullstack
		{name: "fullstack frontend required", profileName: "fullstack", service: "frontend", required: true},
		{name: "fullstack backend required", profileName: "fullstack", service: "backend", required: true},
		{name: "fullstack postgres required", profileName: "fullstack", service: "postgres", required: true},
		{name: "fullstack redis required", profileName: "fullstack", service: "redis", required: true},
		{name: "fullstack minio optional", profileName: "fullstack", service: "minio", required: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := Get(tt.profileName)
			if err != nil {
				t.Fatalf("Get(%q) returned error: %v", tt.profileName, err)
			}

			var found bool
			for _, s := range p.Services {
				if s.Name == tt.service {
					found = true
					if s.Required != tt.required {
						t.Errorf("service %q in profile %q: Required = %v, want %v",
							tt.service, tt.profileName, s.Required, tt.required)
					}
					break
				}
			}
			if !found {
				t.Errorf("service %q not found in profile %q", tt.service, tt.profileName)
			}
		})
	}
}

func TestFullstackProfileComposeRender(t *testing.T) {
	p, err := Get("fullstack")
	if err != nil {
		t.Fatalf("Get(fullstack) returned error: %v", err)
	}

	data := ProfileData{
		Name:            "webapp",
		Domain:          "webapp.io",
		Port:            4000,
		PostgresVersion: "16",
	}

	rendered, err := Render(p.Compose, data)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	checks := []struct {
		name     string
		contains string
	}{
		{name: "frontend service", contains: "frontend:"},
		{name: "backend service", contains: "backend:"},
		{name: "frontend image", contains: "webapp-frontend:local"},
		{name: "backend image", contains: "webapp-backend:local"},
		{name: "frontend domain", contains: "webapp.io"},
		{name: "api subdomain", contains: "api.webapp.io"},
		{name: "CORS origin", contains: "CORS_ORIGIN: https://webapp.io"},
		{name: "frontend port 3000", contains: "PORT: \"3000\""},
		{name: "backend port", contains: "PORT: \"4000\""},
		{name: "api router", contains: "traefik.http.routers.webapp-api"},
		{name: "frontend router", contains: "traefik.http.routers.webapp.rule"},
		{name: "internal network", contains: "webapp-internal"},
		{name: "Dockerfile.frontend", contains: "Dockerfile.frontend"},
		{name: "Dockerfile.backend", contains: "Dockerfile.backend"},
	}

	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if !strings.Contains(rendered, check.contains) {
				t.Errorf("rendered fullstack compose does not contain %q\nGot:\n%s", check.contains, rendered)
			}
		})
	}
}

func TestSaaSProfileEnvRender(t *testing.T) {
	p, err := Get("saas")
	if err != nil {
		t.Fatalf("Get(saas) returned error: %v", err)
	}

	data := ProfileData{
		Name:   "platform",
		Domain: "platform.example.com",
		Port:   3000,
	}

	rendered, err := RenderEnv(p.EnvTemplate, data)
	if err != nil {
		t.Fatalf("RenderEnv() returned error: %v", err)
	}

	checks := []struct {
		name     string
		contains string
	}{
		{name: "POSTGRES_USER", contains: "POSTGRES_USER=platform"},
		{name: "POSTGRES_DB", contains: "POSTGRES_DB=platform"},
		{name: "REDIS_URL", contains: "REDIS_URL=redis://redis:6379"},
		{name: "MINIO_ROOT_USER", contains: "MINIO_ROOT_USER=platform_admin"},
		{name: "MINIO_ROOT_PASSWORD prefix", contains: "MINIO_ROOT_PASSWORD=platform_minio_CHANGEME_"},
		{name: "S3_BUCKET", contains: "S3_BUCKET=platform-uploads"},
		{name: "SMTP_HOST", contains: "SMTP_HOST=mailpit"},
		{name: "SMTP_PORT", contains: "SMTP_PORT=1025"},
		{name: "PORT", contains: "PORT=3000"},
		{name: "NODE_ENV", contains: "NODE_ENV=production"},
	}

	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if !strings.Contains(rendered, check.contains) {
				t.Errorf("rendered saas env does not contain %q\nGot:\n%s", check.contains, rendered)
			}
		})
	}
}

func TestProfileNginxConfigPresence(t *testing.T) {
	tests := []struct {
		name        string
		profileName string
		wantNginx   bool
	}{
		{name: "bare has no nginx config", profileName: "bare", wantNginx: false},
		{name: "server has no nginx config", profileName: "server", wantNginx: false},
		{name: "saas has no nginx config", profileName: "saas", wantNginx: false},
		{name: "static has nginx config", profileName: "static", wantNginx: true},
		{name: "worker has no nginx config", profileName: "worker", wantNginx: false},
		{name: "fullstack has no nginx config", profileName: "fullstack", wantNginx: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := Get(tt.profileName)
			if err != nil {
				t.Fatalf("Get(%q) returned error: %v", tt.profileName, err)
			}

			hasNginx := p.Nginx != ""
			if hasNginx != tt.wantNginx {
				t.Errorf("profile %q Nginx config present = %v, want %v", tt.profileName, hasNginx, tt.wantNginx)
			}
		})
	}
}

func TestRegisterAndGetCustomProfile(t *testing.T) {
	customProfile := &Profile{
		Name:        "custom-test-profile",
		Description: "A custom profile for testing",
		Services: []Service{
			{Name: "testservice", Image: "test:latest", Description: "Test service", Required: true},
		},
		Compose:     "services:\n  test:\n    image: {{.Name}}:test\n",
		EnvTemplate: "TEST_VAR={{.Name}}\n",
	}

	Register(customProfile)

	// Retrieve it
	p, err := Get("custom-test-profile")
	if err != nil {
		t.Fatalf("Get(custom-test-profile) returned error: %v", err)
	}

	if p.Name != "custom-test-profile" {
		t.Errorf("profile name = %q, want %q", p.Name, "custom-test-profile")
	}

	if p.Description != "A custom profile for testing" {
		t.Errorf("profile description = %q, want %q", p.Description, "A custom profile for testing")
	}

	if len(p.Services) != 1 {
		t.Fatalf("profile has %d services, want 1", len(p.Services))
	}

	if p.Services[0].Name != "testservice" {
		t.Errorf("service name = %q, want %q", p.Services[0].Name, "testservice")
	}

	// Render compose
	rendered, err := Render(p.Compose, ProfileData{Name: "mytest"})
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}
	if !strings.Contains(rendered, "mytest:test") {
		t.Errorf("rendered compose does not contain 'mytest:test'")
	}

	// Clean up: remove from registry so it doesn't affect other tests
	delete(registry, "custom-test-profile")
}

func TestProfilesUseLetsencryptCertResolver(t *testing.T) {
	// All profiles that have traefik labels should use "letsencrypt" certresolver.
	for _, name := range []string{"bare", "server", "saas", "static", "fullstack"} {
		p, err := Get(name)
		if err != nil {
			t.Fatalf("Get(%q) error: %v", name, err)
		}
		if strings.Contains(p.Compose, "certresolver=myresolver") {
			t.Errorf("profile %q still uses deprecated certresolver=myresolver", name)
		}
		if strings.Contains(p.Compose, "traefik.http.routers") && !strings.Contains(p.Compose, "certresolver=letsencrypt") {
			t.Errorf("profile %q with traefik routing should use certresolver=letsencrypt", name)
		}
	}
}

func TestRenderEnvIsSameAsRender(t *testing.T) {
	// RenderEnv delegates to Render, verify they produce the same output
	tmpl := "DB_NAME={{.Name}}\nDOMAIN={{.Domain}}\nPORT={{.Port}}\n"
	data := ProfileData{
		Name:   "test",
		Domain: "test.example.com",
		Port:   3000,
	}

	renderResult, err := Render(tmpl, data)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	renderEnvResult, err := RenderEnv(tmpl, data)
	if err != nil {
		t.Fatalf("RenderEnv() returned error: %v", err)
	}

	if renderResult != renderEnvResult {
		t.Errorf("Render() and RenderEnv() produced different output:\nRender:    %q\nRenderEnv: %q", renderResult, renderEnvResult)
	}
}

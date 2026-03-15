package discover

import (
	"encoding/json"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Compose file scanner: nested dirs, excluded paths, file name variants
// ---------------------------------------------------------------------------

func TestScanComposeFilesNestedDirs(t *testing.T) {
	dir := t.TempDir()

	// Create compose files at various nesting depths
	level1 := filepath.Join(dir, "apps", "webapp")
	level2 := filepath.Join(dir, "apps", "services", "api")
	level3 := filepath.Join(dir, "org", "team", "proj", "backend")

	for _, d := range []string{level1, level2, level3} {
		os.MkdirAll(d, 0755)
		os.WriteFile(
			filepath.Join(d, "docker-compose.yml"),
			[]byte("services:\n  app:\n    image: nginx\n"),
			0644,
		)
	}

	projects, err := ScanComposeFiles([]string{dir}, nil)
	if err != nil {
		t.Fatalf("ScanComposeFiles: %v", err)
	}
	if len(projects) != 3 {
		names := make([]string, len(projects))
		for i, p := range projects {
			names[i] = p.Name + " @ " + p.Dir
		}
		t.Errorf("expected 3 nested projects, got %d: %v", len(projects), names)
	}
}

func TestScanComposeFilesDepthLimit(t *testing.T) {
	dir := t.TempDir()

	// Create a compose file deeper than 4 levels (should be skipped)
	deepDir := filepath.Join(dir, "a", "b", "c", "d", "e", "project")
	os.MkdirAll(deepDir, 0755)
	os.WriteFile(
		filepath.Join(deepDir, "docker-compose.yml"),
		[]byte("services:\n  app:\n    image: nginx\n"),
		0644,
	)

	// Create one within depth limit
	shallowDir := filepath.Join(dir, "a", "shallow")
	os.MkdirAll(shallowDir, 0755)
	os.WriteFile(
		filepath.Join(shallowDir, "docker-compose.yml"),
		[]byte("services:\n  web:\n    image: node:20\n"),
		0644,
	)

	projects, err := ScanComposeFiles([]string{dir}, nil)
	if err != nil {
		t.Fatalf("ScanComposeFiles: %v", err)
	}

	// Only the shallow one should be found
	if len(projects) != 1 {
		names := make([]string, len(projects))
		for i, p := range projects {
			names[i] = p.Name + " @ " + p.Dir
		}
		t.Errorf("expected 1 project (depth-limited), got %d: %v", len(projects), names)
	}
}

func TestScanComposeFilesSkipExcludedPaths(t *testing.T) {
	dir := t.TempDir()

	// Create a valid project
	validDir := filepath.Join(dir, "valid-app")
	os.MkdirAll(validDir, 0755)
	os.WriteFile(
		filepath.Join(validDir, "docker-compose.yml"),
		[]byte("services:\n  app:\n    image: node:20\n"),
		0644,
	)

	// Create projects in excluded dirs (built-in skip dirs)
	for _, skipDir := range []string{".git", "node_modules", "vendor", ".cache", "__pycache__", ".venv", "venv"} {
		excluded := filepath.Join(dir, skipDir, "project")
		os.MkdirAll(excluded, 0755)
		os.WriteFile(
			filepath.Join(excluded, "docker-compose.yml"),
			[]byte("services:\n  skip:\n    image: skip\n"),
			0644,
		)
	}

	projects, err := ScanComposeFiles([]string{dir}, nil)
	if err != nil {
		t.Fatalf("ScanComposeFiles: %v", err)
	}
	if len(projects) != 1 {
		names := make([]string, len(projects))
		for i, p := range projects {
			names[i] = p.Name + " @ " + p.Dir
		}
		t.Errorf("expected 1 project (skip dirs excluded), got %d: %v", len(projects), names)
	}
	if projects[0].Name != "valid-app" {
		t.Errorf("expected project name 'valid-app', got %s", projects[0].Name)
	}
}

func TestScanComposeFilesCustomExcludePaths(t *testing.T) {
	dir := t.TempDir()

	// Create two projects
	keep := filepath.Join(dir, "keep")
	skip := filepath.Join(dir, "skip-me")
	for _, d := range []string{keep, skip} {
		os.MkdirAll(d, 0755)
		os.WriteFile(
			filepath.Join(d, "docker-compose.yml"),
			[]byte("services:\n  app:\n    image: nginx\n"),
			0644,
		)
	}

	projects, err := ScanComposeFiles([]string{dir}, []string{"skip-me"})
	if err != nil {
		t.Fatalf("ScanComposeFiles: %v", err)
	}
	if len(projects) != 1 {
		t.Errorf("expected 1 project (custom exclude), got %d", len(projects))
	}
	if len(projects) > 0 && projects[0].Name != "keep" {
		t.Errorf("expected project 'keep', got %s", projects[0].Name)
	}
}

func TestScanComposeFilesMultipleSearchPaths(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	for _, d := range []string{
		filepath.Join(dir1, "app1"),
		filepath.Join(dir2, "app2"),
	} {
		os.MkdirAll(d, 0755)
		os.WriteFile(
			filepath.Join(d, "docker-compose.yml"),
			[]byte("services:\n  svc:\n    image: nginx\n"),
			0644,
		)
	}

	projects, err := ScanComposeFiles([]string{dir1, dir2}, nil)
	if err != nil {
		t.Fatalf("ScanComposeFiles: %v", err)
	}
	if len(projects) != 2 {
		t.Errorf("expected 2 projects from multiple search paths, got %d", len(projects))
	}
}

func TestScanComposeFilesDedupSameDir(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "dedup-proj")
	os.MkdirAll(projDir, 0755)

	// Write two compose files in the same directory
	os.WriteFile(filepath.Join(projDir, "docker-compose.yml"),
		[]byte("services:\n  app:\n    image: node:20\n"), 0644)
	os.WriteFile(filepath.Join(projDir, "compose.yml"),
		[]byte("services:\n  web:\n    image: nginx\n"), 0644)

	projects, err := ScanComposeFiles([]string{dir}, nil)
	if err != nil {
		t.Fatalf("ScanComposeFiles: %v", err)
	}
	// Should only find 1 project per directory (dedup by dir)
	if len(projects) != 1 {
		t.Errorf("expected 1 project (dedup same dir), got %d", len(projects))
	}
}

func TestScanComposeFilesAllNameVariants(t *testing.T) {
	// Each variant in its own directory
	dir := t.TempDir()
	variants := []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"}

	for i, variant := range variants {
		projDir := filepath.Join(dir, "variant-"+string(rune('a'+i)))
		os.MkdirAll(projDir, 0755)
		os.WriteFile(
			filepath.Join(projDir, variant),
			[]byte("services:\n  svc:\n    image: nginx\n"),
			0644,
		)
	}

	projects, err := ScanComposeFiles([]string{dir}, nil)
	if err != nil {
		t.Fatalf("ScanComposeFiles: %v", err)
	}
	if len(projects) != 4 {
		t.Errorf("expected 4 projects (one per variant), got %d", len(projects))
	}
}

func TestParseComposeProjectDBDetection(t *testing.T) {
	tests := []struct {
		name       string
		yaml       string
		wantHasDB  bool
		wantDBType string
	}{
		{
			name:       "postgres",
			yaml:       "services:\n  db:\n    image: postgres:15-alpine\n",
			wantHasDB:  true,
			wantDBType: "postgres",
		},
		{
			name:       "mysql",
			yaml:       "services:\n  db:\n    image: mysql:8\n",
			wantHasDB:  true,
			wantDBType: "mysql",
		},
		{
			name:       "mariadb",
			yaml:       "services:\n  db:\n    image: mariadb:10\n",
			wantHasDB:  true,
			wantDBType: "mysql",
		},
		{
			name:       "mongo",
			yaml:       "services:\n  db:\n    image: mongo:6\n",
			wantHasDB:  true,
			wantDBType: "mongo",
		},
		{
			name:       "redis only",
			yaml:       "services:\n  cache:\n    image: redis:7-alpine\n",
			wantHasDB:  true,
			wantDBType: "redis",
		},
		{
			name:       "redis with postgres",
			yaml:       "services:\n  db:\n    image: postgres:15\n  cache:\n    image: redis:7\n",
			wantHasDB:  true,
			wantDBType: "postgres",
		},
		{
			name:       "no database",
			yaml:       "services:\n  app:\n    image: node:20\n  web:\n    image: nginx\n",
			wantHasDB:  false,
			wantDBType: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(tt.yaml), 0644)

			cp, err := parseComposeProject(filepath.Join(dir, "docker-compose.yml"))
			if err != nil {
				t.Fatalf("parseComposeProject: %v", err)
			}
			if cp.HasDB != tt.wantHasDB {
				t.Errorf("HasDB: got %v, want %v", cp.HasDB, tt.wantHasDB)
			}
			if cp.DBType != tt.wantDBType {
				t.Errorf("DBType: got %s, want %s", cp.DBType, tt.wantDBType)
			}
		})
	}
}

func TestParseComposeProjectInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("{{invalid yaml"), 0644)

	_, err := parseComposeProject(filepath.Join(dir, "docker-compose.yml"))
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestParseComposeProjectMissingFile(t *testing.T) {
	_, err := parseComposeProject("/nonexistent/path/docker-compose.yml")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestParseComposeProjectNameFromDir(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "my-cool-project")
	os.MkdirAll(projDir, 0755)
	os.WriteFile(
		filepath.Join(projDir, "docker-compose.yml"),
		[]byte("services:\n  app:\n    image: nginx\n"),
		0644,
	)

	cp, err := parseComposeProject(filepath.Join(projDir, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("parseComposeProject: %v", err)
	}
	if cp.Name != "my-cool-project" {
		t.Errorf("expected project name 'my-cool-project' (from dir), got %s", cp.Name)
	}
}

func TestParseComposeProjectListsServices(t *testing.T) {
	dir := t.TempDir()
	yaml := "services:\n  web:\n    image: nginx\n  api:\n    image: node:20\n  worker:\n    image: python:3.12\n"
	os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(yaml), 0644)

	cp, err := parseComposeProject(filepath.Join(dir, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("parseComposeProject: %v", err)
	}
	if len(cp.Services) != 3 {
		t.Errorf("expected 3 services, got %d", len(cp.Services))
	}
}

func TestParseComposeProjectEmptyServices(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services:\n"), 0644)

	cp, err := parseComposeProject(filepath.Join(dir, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("parseComposeProject: %v", err)
	}
	if len(cp.Services) != 0 {
		t.Errorf("expected 0 services, got %d", len(cp.Services))
	}
}

// ---------------------------------------------------------------------------
// Traefik label parser: various Host() rule formats
// ---------------------------------------------------------------------------

func TestExtractDomainHostWithPath(t *testing.T) {
	labels := map[string]string{
		"traefik.http.routers.app.rule": "Host(`app.example.com`) && PathPrefix(`/api`)",
	}
	got := ExtractDomain(labels)
	if got != "app.example.com" {
		t.Errorf("expected app.example.com, got %q", got)
	}
}

func TestExtractDomainMultipleRouters(t *testing.T) {
	labels := map[string]string{
		"traefik.http.routers.web.rule":  "Host(`web.example.com`)",
		"traefik.http.routers.api.rule":  "Host(`api.example.com`)",
		"traefik.http.services.web.port": "3000",
	}
	got := ExtractDomain(labels)
	// Should find at least one domain
	if got != "web.example.com" && got != "api.example.com" {
		t.Errorf("expected one of the domains, got %q", got)
	}
}

func TestExtractDomainNilMap(t *testing.T) {
	got := ExtractDomain(nil)
	if got != "" {
		t.Errorf("expected empty string for nil map, got %q", got)
	}
}

func TestExtractDomainSubdomain(t *testing.T) {
	labels := map[string]string{
		"traefik.http.routers.myapp.rule": "Host(`staging.myapp.example.com`)",
	}
	got := ExtractDomain(labels)
	if got != "staging.myapp.example.com" {
		t.Errorf("expected staging.myapp.example.com, got %q", got)
	}
}

func TestExtractDomainNonRuleKey(t *testing.T) {
	// Key contains "traefik.http.routers" but does not end with ".rule"
	labels := map[string]string{
		"traefik.http.routers.myapp.tls":         "true",
		"traefik.http.routers.myapp.entrypoints":  "websecure",
	}
	got := ExtractDomain(labels)
	if got != "" {
		t.Errorf("expected empty string for non-.rule keys, got %q", got)
	}
}

func TestExtractDomainFromLabelsListWithSpaces(t *testing.T) {
	labels := []string{
		" traefik.http.routers.myapp.rule = Host(`spaced.example.com`) ",
	}
	got := ExtractDomainFromLabelsList(labels)
	if got != "spaced.example.com" {
		t.Errorf("expected spaced.example.com, got %q", got)
	}
}

func TestExtractDomainFromLabelsListHostWithPathPrefix(t *testing.T) {
	labels := []string{
		"traefik.http.routers.api.rule=Host(`api.test.io`) && PathPrefix(`/v2`)",
	}
	got := ExtractDomainFromLabelsList(labels)
	if got != "api.test.io" {
		t.Errorf("expected api.test.io, got %q", got)
	}
}

func TestExtractDomainFromLabelsListNoEqualsSign(t *testing.T) {
	labels := []string{
		"traefik.http.routers.app.rule",
	}
	got := ExtractDomainFromLabelsList(labels)
	if got != "" {
		t.Errorf("expected empty for label without =, got %q", got)
	}
}

func TestExtractDomainFromLabelsListHostWithoutBackticks(t *testing.T) {
	// Host() without backticks should not match
	labels := []string{
		"traefik.http.routers.app.rule=Host(no-backticks.com)",
	}
	got := ExtractDomainFromLabelsList(labels)
	if got != "" {
		t.Errorf("expected empty for Host() without backticks, got %q", got)
	}
}

func TestExtractDomainFromLabelsListMixedLabels(t *testing.T) {
	labels := []string{
		"traefik.enable=true",
		"com.docker.compose.project=myapp",
		"traefik.http.routers.myapp.rule=Host(`myapp.prod.com`)",
		"traefik.http.routers.myapp.tls=true",
		"traefik.http.services.myapp.loadbalancer.server.port=8080",
	}
	got := ExtractDomainFromLabelsList(labels)
	if got != "myapp.prod.com" {
		t.Errorf("expected myapp.prod.com, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// extractDomainFromService: both label formats
// ---------------------------------------------------------------------------

func TestExtractDomainFromServiceListLabels(t *testing.T) {
	// Simulate []interface{} labels (YAML list format)
	labels := []interface{}{
		"traefik.enable=true",
		"traefik.http.routers.svc.rule=Host(`svc.example.com`)",
	}

	got := extractDomainFromService(labels)
	if got != "svc.example.com" {
		t.Errorf("expected svc.example.com, got %q", got)
	}
}

func TestExtractDomainFromServiceMapLabels(t *testing.T) {
	// Simulate map[string]interface{} labels (YAML map format)
	labels := map[string]interface{}{
		"traefik.enable":                   "true",
		"traefik.http.routers.svc.rule":    "Host(`svc-map.example.com`)",
	}

	got := extractDomainFromService(labels)
	if got != "svc-map.example.com" {
		t.Errorf("expected svc-map.example.com, got %q", got)
	}
}

func TestExtractDomainFromServiceNilLabels(t *testing.T) {
	got := extractDomainFromService(nil)
	if got != "" {
		t.Errorf("expected empty for nil labels, got %q", got)
	}
}

func TestExtractDomainFromServiceUnknownType(t *testing.T) {
	got := extractDomainFromService(42)
	if got != "" {
		t.Errorf("expected empty for unsupported type, got %q", got)
	}
}

func TestExtractDomainFromServiceListWithNonStringItems(t *testing.T) {
	labels := []interface{}{
		42,
		true,
		"traefik.http.routers.app.rule=Host(`mixed.example.com`)",
	}

	got := extractDomainFromService(labels)
	if got != "mixed.example.com" {
		t.Errorf("expected mixed.example.com, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// User detection
// ---------------------------------------------------------------------------

func TestDetectProjectOwnerCurrentUser(t *testing.T) {
	dir := t.TempDir()

	owner, err := DetectProjectOwner(dir)
	if err != nil {
		t.Fatalf("DetectProjectOwner: %v", err)
	}

	currentUser, err := user.Current()
	if err != nil {
		t.Fatalf("user.Current: %v", err)
	}

	if owner != currentUser.Username {
		t.Errorf("expected owner %q (current user), got %q", currentUser.Username, owner)
	}
}

func TestDetectProjectOwnerNestedDir(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "a", "b", "c")
	os.MkdirAll(nested, 0755)

	owner, err := DetectProjectOwner(nested)
	if err != nil {
		t.Fatalf("DetectProjectOwner nested: %v", err)
	}
	if owner == "" {
		t.Error("expected non-empty owner for nested dir")
	}
}

func TestDetectProjectOwnerFileInsteadOfDir(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "regular-file.txt")
	os.WriteFile(filePath, []byte("content"), 0644)

	// DetectProjectOwner uses os.Stat which works on files too
	owner, err := DetectProjectOwner(filePath)
	if err != nil {
		t.Fatalf("DetectProjectOwner on file: %v", err)
	}
	if owner == "" {
		t.Error("expected non-empty owner for file")
	}
}

// ---------------------------------------------------------------------------
// Container parsing: parseLabels edge cases
// ---------------------------------------------------------------------------

func TestParseLabelsMultipleEquals(t *testing.T) {
	// Value contains '=' characters
	got := parseLabels("key=val=ue=with=equals")
	if got["key"] != "val=ue=with=equals" {
		t.Errorf("expected 'val=ue=with=equals', got %q", got["key"])
	}
}

func TestParseLabelsOnlyCommas(t *testing.T) {
	got := parseLabels(",,,")
	// All entries are empty or have no '=' so should result in empty map
	for k, v := range got {
		if k != "" {
			t.Errorf("unexpected label key %q=%q", k, v)
		}
	}
}

func TestParseLabelsUnicodeValues(t *testing.T) {
	got := parseLabels("label.name=value-with-unicode-\u00e9\u00e8\u00ea")
	expected := "value-with-unicode-\u00e9\u00e8\u00ea"
	if got["label.name"] != expected {
		t.Errorf("expected %q, got %q", expected, got["label.name"])
	}
}

// ---------------------------------------------------------------------------
// Container JSON parsing: additional scenarios
// ---------------------------------------------------------------------------

func TestSimulateContainerParsingMultipleContainers(t *testing.T) {
	lines := []string{
		`{"ID":"c1","Names":"app-web-1","Image":"node:20","State":"running","Status":"Up 2h","Labels":"com.docker.compose.project=app,com.docker.compose.service=web,com.docker.compose.project.working_dir=/opt/app"}`,
		`{"ID":"c2","Names":"app-db-1","Image":"postgres:15","State":"running","Status":"Up 2h","Labels":"com.docker.compose.project=app,com.docker.compose.service=db,com.docker.compose.project.working_dir=/opt/app"}`,
		`{"ID":"c3","Names":"monitoring","Image":"grafana/grafana","State":"running","Status":"Up 5d","Labels":""}`,
	}

	groups := make(map[string][]ContainerInfo)
	for _, line := range lines {
		ci := simulateContainerParsing(t, line)
		key := ci.ProjectDir
		if key == "" {
			key = "__standalone__"
		}
		groups[key] = append(groups[key], ci)
	}

	if len(groups["/opt/app"]) != 2 {
		t.Errorf("expected 2 containers in /opt/app group, got %d", len(groups["/opt/app"]))
	}
	if len(groups["__standalone__"]) != 1 {
		t.Errorf("expected 1 standalone container, got %d", len(groups["__standalone__"]))
	}
}

func TestSimulateContainerParsingTraefikLabels(t *testing.T) {
	jsonLine := `{"ID":"t1","Names":"myapp-web-1","Image":"node:20","State":"running","Status":"Up 1h","Labels":"traefik.enable=true,traefik.http.routers.myapp.rule=Host(` + "`" + `myapp.example.com` + "`" + `),com.docker.compose.project=myapp,com.docker.compose.service=web,com.docker.compose.project.working_dir=/opt/myapp"}`

	ci := simulateContainerParsing(t, jsonLine)

	if ci.Labels["traefik.enable"] != "true" {
		t.Errorf("expected traefik.enable=true, got %s", ci.Labels["traefik.enable"])
	}

	domain := ExtractDomain(ci.Labels)
	if domain != "myapp.example.com" {
		t.Errorf("expected domain myapp.example.com from container labels, got %s", domain)
	}
}

func TestSimulateContainerParsingCreatedState(t *testing.T) {
	jsonLine := `{"ID":"created1","Names":"new-container","Image":"alpine:latest","State":"created","Status":"Created","Labels":""}`

	ci := simulateContainerParsing(t, jsonLine)
	if ci.State != "created" {
		t.Errorf("expected state 'created', got %s", ci.State)
	}
}

func TestSimulateContainerParsingPausedState(t *testing.T) {
	jsonLine := `{"ID":"paused1","Names":"paused-container","Image":"nginx:alpine","State":"paused","Status":"Up 1h (Paused)","Labels":"com.docker.compose.project=paused-app,com.docker.compose.service=web,com.docker.compose.project.working_dir=/opt/paused"}`

	ci := simulateContainerParsing(t, jsonLine)
	if ci.State != "paused" {
		t.Errorf("expected state 'paused', got %s", ci.State)
	}
	if ci.ComposeProject != "paused-app" {
		t.Errorf("expected ComposeProject 'paused-app', got %s", ci.ComposeProject)
	}
}

func TestSimulateContainerParsingEmptyLabelsField(t *testing.T) {
	jsonLine := `{"ID":"empty1","Names":"no-labels","Image":"alpine","State":"running","Status":"Up","Labels":""}`

	ci := simulateContainerParsing(t, jsonLine)
	if ci.ProjectDir != "" {
		t.Errorf("expected empty ProjectDir, got %s", ci.ProjectDir)
	}
	if ci.ComposeProject != "" {
		t.Errorf("expected empty ComposeProject, got %s", ci.ComposeProject)
	}
	if ci.Service != "" {
		t.Errorf("expected empty Service, got %s", ci.Service)
	}
}

func TestContainerJSONMalformedSkipped(t *testing.T) {
	// Simulate the parsing loop logic from ScanRunningContainers
	output := "not valid json\n{also bad}\n"

	var parsed int
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		var raw struct {
			ID string `json:"ID"`
		}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		parsed++
	}
	if parsed != 0 {
		t.Errorf("expected 0 successfully parsed lines, got %d", parsed)
	}
}

func TestContainerJSONMixedValidAndInvalid(t *testing.T) {
	lines := []string{
		`{"ID":"valid1","Names":"good","Image":"nginx","State":"running","Status":"Up","Labels":""}`,
		`{bad json}`,
		`{"ID":"valid2","Names":"also-good","Image":"alpine","State":"exited","Status":"Exited","Labels":""}`,
	}

	var containers []ContainerInfo
	for _, line := range lines {
		var raw struct {
			ID     string `json:"ID"`
			Names  string `json:"Names"`
			Image  string `json:"Image"`
			State  string `json:"State"`
			Status string `json:"Status"`
			Labels string `json:"Labels"`
		}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		labels := parseLabels(raw.Labels)
		containers = append(containers, ContainerInfo{
			ID:    raw.ID,
			Name:  raw.Names,
			Image: raw.Image,
			State: raw.State,
		})
		_ = labels
	}

	if len(containers) != 2 {
		t.Errorf("expected 2 valid containers, got %d", len(containers))
	}
}

// ---------------------------------------------------------------------------
// isComposeFile additional cases
// ---------------------------------------------------------------------------

func TestIsComposeFileEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"docker-compose.yml", true},
		{"docker-compose.yaml", true},
		{"compose.yml", true},
		{"compose.yaml", true},
		{"Docker-Compose.yml", false},     // case sensitive
		{"DOCKER-COMPOSE.YML", false},     // uppercase
		{"docker-compose.yml.bak", false}, // with suffix
		{".docker-compose.yml", false},    // with dot prefix
		{"docker-compose", false},         // no extension
		{"", false},                       // empty string
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isComposeFile(tt.name); got != tt.expected {
				t.Errorf("isComposeFile(%q) = %v, want %v", tt.name, got, tt.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Compose with labels: list format domain extraction
// ---------------------------------------------------------------------------

func TestParseComposeProjectWithListLabelsTraefikDomain(t *testing.T) {
	dir := t.TempDir()
	yaml := `services:
  app:
    image: myapp:latest
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.app.rule=Host(` + "`" + `list-label.example.com` + "`" + `)"
      - "traefik.http.routers.app.tls=true"
`
	os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(yaml), 0644)

	cp, err := parseComposeProject(filepath.Join(dir, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("parseComposeProject: %v", err)
	}
	if cp.Domain != "list-label.example.com" {
		t.Errorf("expected domain list-label.example.com, got %q", cp.Domain)
	}
}

func TestParseComposeProjectNoLabels(t *testing.T) {
	dir := t.TempDir()
	yaml := "services:\n  app:\n    image: nginx:alpine\n"
	os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(yaml), 0644)

	cp, err := parseComposeProject(filepath.Join(dir, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("parseComposeProject: %v", err)
	}
	if cp.Domain != "" {
		t.Errorf("expected empty domain when no labels, got %q", cp.Domain)
	}
}

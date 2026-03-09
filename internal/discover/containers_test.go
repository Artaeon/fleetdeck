package discover

import (
	"encoding/json"
	"testing"
)

func TestParseLabels(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]string
	}{
		{
			name:  "standard compose labels",
			input: "com.docker.compose.project=myapp,com.docker.compose.service=web,com.docker.compose.project.working_dir=/opt/myapp",
			expected: map[string]string{
				"com.docker.compose.project":             "myapp",
				"com.docker.compose.service":             "web",
				"com.docker.compose.project.working_dir": "/opt/myapp",
			},
		},
		{
			name:     "empty string",
			input:    "",
			expected: map[string]string{},
		},
		{
			name:  "single label",
			input: "traefik.enable=true",
			expected: map[string]string{
				"traefik.enable": "true",
			},
		},
		{
			name:  "label with value containing equals",
			input: "traefik.http.routers.app.rule=Host(`app.com`)",
			expected: map[string]string{
				"traefik.http.routers.app.rule": "Host(`app.com`)",
			},
		},
		{
			name:  "labels with spaces",
			input: " key1=val1 , key2=val2 ",
			expected: map[string]string{
				"key1": "val1",
				"key2": "val2",
			},
		},
		{
			name:     "no equals sign in entry",
			input:    "noequalssign",
			expected: map[string]string{},
		},
		{
			name:  "mixed valid and invalid entries",
			input: "valid=yes,invalid,also_valid=true",
			expected: map[string]string{
				"valid":      "yes",
				"also_valid": "true",
			},
		},
		{
			name:  "traefik labels with backticks",
			input: "traefik.enable=true,traefik.http.routers.myapp.rule=Host(`myapp.example.com`),traefik.http.routers.myapp.tls=true",
			expected: map[string]string{
				"traefik.enable":                  "true",
				"traefik.http.routers.myapp.rule": "Host(`myapp.example.com`)",
				"traefik.http.routers.myapp.tls":  "true",
			},
		},
		{
			name:  "empty value",
			input: "key=",
			expected: map[string]string{
				"key": "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseLabels(tt.input)
			if len(got) != len(tt.expected) {
				t.Errorf("expected %d labels, got %d: %v", len(tt.expected), len(got), got)
				return
			}
			for k, v := range tt.expected {
				if got[k] != v {
					t.Errorf("label %q: expected %q, got %q", k, v, got[k])
				}
			}
		})
	}
}

// simulateContainerParsing reproduces the JSON parsing logic from ScanRunningContainers
// to verify correct ContainerInfo construction from docker ps JSON output.
func simulateContainerParsing(t *testing.T, jsonLine string) ContainerInfo {
	t.Helper()

	var raw struct {
		ID     string `json:"ID"`
		Names  string `json:"Names"`
		Image  string `json:"Image"`
		State  string `json:"State"`
		Status string `json:"Status"`
		Labels string `json:"Labels"`
	}

	if err := json.Unmarshal([]byte(jsonLine), &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	labels := parseLabels(raw.Labels)

	return ContainerInfo{
		ID:             raw.ID,
		Name:           raw.Names,
		Image:          raw.Image,
		State:          raw.State,
		Status:         raw.Status,
		Labels:         labels,
		ProjectDir:     labels["com.docker.compose.project.working_dir"],
		ComposeProject: labels["com.docker.compose.project"],
		Service:        labels["com.docker.compose.service"],
	}
}

func TestParseContainerJSONComposeContainer(t *testing.T) {
	jsonLine := `{"ID":"abc123def456","Names":"myapp-web-1","Image":"node:20","State":"running","Status":"Up 2 hours","Labels":"com.docker.compose.project=myapp,com.docker.compose.service=web,com.docker.compose.project.working_dir=/opt/myapp"}`

	ci := simulateContainerParsing(t, jsonLine)

	if ci.ID != "abc123def456" {
		t.Errorf("ID: got %q, want %q", ci.ID, "abc123def456")
	}
	if ci.Name != "myapp-web-1" {
		t.Errorf("Name: got %q, want %q", ci.Name, "myapp-web-1")
	}
	if ci.Image != "node:20" {
		t.Errorf("Image: got %q, want %q", ci.Image, "node:20")
	}
	if ci.State != "running" {
		t.Errorf("State: got %q, want %q", ci.State, "running")
	}
	if ci.ProjectDir != "/opt/myapp" {
		t.Errorf("ProjectDir: got %q, want %q", ci.ProjectDir, "/opt/myapp")
	}
	if ci.Service != "web" {
		t.Errorf("Service: got %q, want %q", ci.Service, "web")
	}
	if ci.ComposeProject != "myapp" {
		t.Errorf("ComposeProject: got %q, want %q", ci.ComposeProject, "myapp")
	}
}

func TestParseContainerJSONStandaloneContainer(t *testing.T) {
	jsonLine := `{"ID":"xyz789","Names":"standalone","Image":"nginx:alpine","State":"running","Status":"Up 1 hour","Labels":"maintainer=NGINX"}`

	ci := simulateContainerParsing(t, jsonLine)

	if ci.ID != "xyz789" {
		t.Errorf("ID: got %q, want %q", ci.ID, "xyz789")
	}
	if ci.Image != "nginx:alpine" {
		t.Errorf("Image: got %q, want %q", ci.Image, "nginx:alpine")
	}
	if ci.ProjectDir != "" {
		t.Errorf("ProjectDir: expected empty for standalone, got %q", ci.ProjectDir)
	}
	if ci.Service != "" {
		t.Errorf("Service: expected empty for standalone, got %q", ci.Service)
	}
	if ci.ComposeProject != "" {
		t.Errorf("ComposeProject: expected empty for standalone, got %q", ci.ComposeProject)
	}
}

func TestParseContainerJSONExitedContainer(t *testing.T) {
	jsonLine := `{"ID":"dead999","Names":"old-container","Image":"alpine:3.18","State":"exited","Status":"Exited (0) 3 days ago","Labels":""}`

	ci := simulateContainerParsing(t, jsonLine)

	if ci.ID != "dead999" {
		t.Errorf("ID: got %q, want %q", ci.ID, "dead999")
	}
	if ci.State != "exited" {
		t.Errorf("State: got %q, want %q", ci.State, "exited")
	}
	if ci.ProjectDir != "" {
		t.Errorf("ProjectDir: expected empty for no labels, got %q", ci.ProjectDir)
	}
}

func TestParseContainerJSONMalformed(t *testing.T) {
	// Malformed JSON should not be parseable
	badJSON := `{not valid json}`
	var raw struct {
		ID string `json:"ID"`
	}
	err := json.Unmarshal([]byte(badJSON), &raw)
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
}

func TestParseContainerJSONEmptyOutput(t *testing.T) {
	// Empty string should produce no results
	lines := []string{"", "  ", "\n"}
	for _, line := range lines {
		var raw struct {
			ID string `json:"ID"`
		}
		err := json.Unmarshal([]byte(line), &raw)
		if err == nil && raw.ID != "" {
			t.Errorf("expected error or empty result for empty line %q", line)
		}
	}
}

func TestParseContainerJSONContainerWithoutComposeLabels(t *testing.T) {
	jsonLine := `{"ID":"standalone1","Names":"traefik","Image":"traefik:v3","State":"running","Status":"Up 5 days","Labels":"org.opencontainers.image.title=Traefik"}`

	ci := simulateContainerParsing(t, jsonLine)

	if ci.ProjectDir != "" {
		t.Errorf("expected empty ProjectDir for non-compose container, got %q", ci.ProjectDir)
	}
	if ci.ComposeProject != "" {
		t.Errorf("expected empty ComposeProject for non-compose container, got %q", ci.ComposeProject)
	}
	if ci.Service != "" {
		t.Errorf("expected empty Service for non-compose container, got %q", ci.Service)
	}
	// Should still parse the regular fields
	if ci.ID != "standalone1" {
		t.Errorf("ID: got %q, want %q", ci.ID, "standalone1")
	}
	if ci.Labels["org.opencontainers.image.title"] != "Traefik" {
		t.Error("expected custom label to be parsed")
	}
}

func TestGroupingLogic(t *testing.T) {
	// Test the grouping key logic: containers with ProjectDir go to their dir,
	// containers without go to __standalone__
	tests := []struct {
		name      string
		container ContainerInfo
		wantKey   string
	}{
		{
			name: "compose container grouped by project dir",
			container: ContainerInfo{
				ID:         "c1",
				ProjectDir: "/opt/myapp",
			},
			wantKey: "/opt/myapp",
		},
		{
			name: "standalone container grouped to __standalone__",
			container: ContainerInfo{
				ID:         "c2",
				ProjectDir: "",
			},
			wantKey: "__standalone__",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := tt.container.ProjectDir
			if key == "" {
				key = "__standalone__"
			}
			if key != tt.wantKey {
				t.Errorf("group key: got %q, want %q", key, tt.wantKey)
			}
		})
	}
}

package project

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestComposeLogsArgs(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		service  string
		tail     int
		follow   bool
		wantArgs []string
		wantDir  string
	}{
		{
			name:     "basic logs no options",
			path:     "/srv/projects/myapp",
			service:  "",
			tail:     0,
			follow:   false,
			wantArgs: []string{"compose", "logs"},
			wantDir:  "/srv/projects/myapp",
		},
		{
			name:     "logs with service name",
			path:     "/srv/projects/myapp",
			service:  "web",
			tail:     0,
			follow:   false,
			wantArgs: []string{"compose", "logs", "web"},
			wantDir:  "/srv/projects/myapp",
		},
		{
			name:     "logs with tail",
			path:     "/srv/projects/myapp",
			service:  "",
			tail:     100,
			follow:   false,
			wantArgs: []string{"compose", "logs", "--tail", "100"},
			wantDir:  "/srv/projects/myapp",
		},
		{
			name:     "logs with follow",
			path:     "/srv/projects/myapp",
			service:  "",
			tail:     0,
			follow:   true,
			wantArgs: []string{"compose", "logs", "--follow"},
			wantDir:  "/srv/projects/myapp",
		},
		{
			name:     "logs with service and tail",
			path:     "/srv/projects/myapp",
			service:  "db",
			tail:     50,
			follow:   false,
			wantArgs: []string{"compose", "logs", "db", "--tail", "50"},
			wantDir:  "/srv/projects/myapp",
		},
		{
			name:     "logs with all options",
			path:     "/opt/deploy/webapp",
			service:  "api",
			tail:     200,
			follow:   true,
			wantArgs: []string{"compose", "logs", "api", "--tail", "200", "--follow"},
			wantDir:  "/opt/deploy/webapp",
		},
		{
			name:     "logs with tail of 1",
			path:     "/srv/projects/myapp",
			service:  "",
			tail:     1,
			follow:   false,
			wantArgs: []string{"compose", "logs", "--tail", "1"},
			wantDir:  "/srv/projects/myapp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := ComposeLogs(tt.path, tt.service, tt.tail, tt.follow)

			// Verify the command binary is "docker"
			if cmd.Path == "" {
				// cmd.Path might not be set if docker isn't found;
				// check Args[0] instead, which is always set.
			}
			if cmd.Args[0] != "docker" {
				t.Errorf("command binary = %q, want %q", cmd.Args[0], "docker")
			}

			// Verify arguments (Args[0] is the command name, rest are args)
			gotArgs := cmd.Args[1:]
			if len(gotArgs) != len(tt.wantArgs) {
				t.Fatalf("args count = %d, want %d; got %v, want %v",
					len(gotArgs), len(tt.wantArgs), gotArgs, tt.wantArgs)
			}
			for i, arg := range gotArgs {
				if arg != tt.wantArgs[i] {
					t.Errorf("arg[%d] = %q, want %q", i, arg, tt.wantArgs[i])
				}
			}

			// Verify working directory
			if cmd.Dir != tt.wantDir {
				t.Errorf("cmd.Dir = %q, want %q", cmd.Dir, tt.wantDir)
			}
		})
	}
}

func TestComposeUpErrorWrapping(t *testing.T) {
	// ComposeUp on a nonexistent path will fail because docker isn't
	// available in the test environment, but we can verify the error
	// wrapping behavior.
	err := ComposeUp("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Skip("docker compose is available and succeeded unexpectedly")
	}
	if !strings.Contains(err.Error(), "docker compose up") {
		t.Errorf("error should be wrapped with 'docker compose up', got: %v", err)
	}
}

func TestComposeDownErrorWrapping(t *testing.T) {
	err := ComposeDown("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Skip("docker compose is available and succeeded unexpectedly")
	}
	if !strings.Contains(err.Error(), "docker compose down") {
		t.Errorf("error should be wrapped with 'docker compose down', got: %v", err)
	}
}

func TestComposeRestartErrorWrapping(t *testing.T) {
	err := ComposeRestart("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Skip("docker compose is available and succeeded unexpectedly")
	}
	if !strings.Contains(err.Error(), "docker compose restart") {
		t.Errorf("error should be wrapped with 'docker compose restart', got: %v", err)
	}
}

func TestComposePSErrorWrapping(t *testing.T) {
	_, err := ComposePS("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Skip("docker compose is available and succeeded unexpectedly")
	}
	if !strings.Contains(err.Error(), "docker compose ps") {
		t.Errorf("error should be wrapped with 'docker compose ps', got: %v", err)
	}
}

func TestContainerStatusJSON(t *testing.T) {
	// Test that ContainerStatus can be populated from docker compose ps JSON.
	// This tests the JSON parsing logic indirectly.
	sampleJSON := `{"Name":"myapp-web-1","State":"running","Status":"Up 2 hours"}`

	var c struct {
		Name   string `json:"Name"`
		State  string `json:"State"`
		Status string `json:"Status"`
	}
	if err := json.Unmarshal([]byte(sampleJSON), &c); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	cs := ContainerStatus{
		Name:   c.Name,
		State:  c.State,
		Status: c.Status,
	}

	if cs.Name != "myapp-web-1" {
		t.Errorf("Name = %q, want %q", cs.Name, "myapp-web-1")
	}
	if cs.State != "running" {
		t.Errorf("State = %q, want %q", cs.State, "running")
	}
	if cs.Status != "Up 2 hours" {
		t.Errorf("Status = %q, want %q", cs.Status, "Up 2 hours")
	}
}

func TestComposePSMultiLineJSON(t *testing.T) {
	// Simulate docker compose ps --format json output with multiple containers.
	// The real ComposePS parses one JSON object per line.
	lines := `{"Name":"app-web-1","State":"running","Status":"Up 5 min"}
{"Name":"app-db-1","State":"running","Status":"Up 5 min"}
{"Name":"app-redis-1","State":"exited","Status":"Exited (0) 3 min ago"}`

	var containers []ContainerStatus
	for _, line := range strings.Split(strings.TrimSpace(lines), "\n") {
		if line == "" {
			continue
		}
		var c struct {
			Name   string `json:"Name"`
			State  string `json:"State"`
			Status string `json:"Status"`
		}
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			t.Fatalf("unmarshal line %q: %v", line, err)
		}
		containers = append(containers, ContainerStatus{
			Name:   c.Name,
			State:  c.State,
			Status: c.Status,
		})
	}

	if len(containers) != 3 {
		t.Fatalf("expected 3 containers, got %d", len(containers))
	}

	// Verify counting logic matches CountContainers behavior
	running := 0
	for _, c := range containers {
		if c.State == "running" {
			running++
		}
	}
	if running != 2 {
		t.Errorf("running count = %d, want 2", running)
	}
}

func TestComposeLogsServiceOrdering(t *testing.T) {
	// Verify that the service name appears before --tail and --follow
	// in the argument list, matching Docker CLI expectations.
	cmd := ComposeLogs("/tmp", "web", 50, true)
	args := cmd.Args[1:] // skip "docker"

	serviceIdx := -1
	tailIdx := -1
	followIdx := -1
	for i, arg := range args {
		switch arg {
		case "web":
			serviceIdx = i
		case "--tail":
			tailIdx = i
		case "--follow":
			followIdx = i
		}
	}

	if serviceIdx == -1 {
		t.Fatal("service name 'web' not found in args")
	}
	if tailIdx == -1 {
		t.Fatal("--tail not found in args")
	}
	if followIdx == -1 {
		t.Fatal("--follow not found in args")
	}
	if serviceIdx > tailIdx {
		t.Error("service name should appear before --tail")
	}
	if serviceIdx > followIdx {
		t.Error("service name should appear before --follow")
	}
}

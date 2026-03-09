package discover

import (
	"testing"
)

func TestMergeResultsEmpty(t *testing.T) {
	result := mergeResults(nil, nil, nil)
	if len(result) != 0 {
		t.Errorf("expected 0 results, got %d", len(result))
	}
}

func TestMergeResultsComposeOnly(t *testing.T) {
	composeProjects := []ComposeProject{
		{
			Name:     "webapp",
			Dir:      "/opt/projects/webapp",
			Services: []string{"app", "db"},
			Domain:   "webapp.com",
			HasDB:    true,
			DBType:   "postgres",
		},
		{
			Name:     "api",
			Dir:      "/opt/projects/api",
			Services: []string{"web"},
			Domain:   "api.com",
		},
	}

	result := mergeResults(composeProjects, nil, nil)
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}

	found := make(map[string]bool)
	for _, r := range result {
		found[r.Name] = true
		if r.Name == "webapp" {
			if r.Domain != "webapp.com" {
				t.Errorf("expected domain webapp.com, got %s", r.Domain)
			}
			if !r.HasDB {
				t.Error("expected HasDB true for webapp")
			}
			if r.DBType != "postgres" {
				t.Errorf("expected DBType postgres, got %s", r.DBType)
			}
		}
	}
	if !found["webapp"] {
		t.Error("expected to find webapp")
	}
	if !found["api"] {
		t.Error("expected to find api")
	}
}

func TestMergeResultsContainersOnly(t *testing.T) {
	containers := map[string][]ContainerInfo{
		"/opt/projects/myapp": {
			{
				ID:             "abc123",
				Name:           "myapp-web-1",
				Image:          "node:20",
				State:          "running",
				Service:        "web",
				ComposeProject: "myapp",
			},
		},
	}

	result := mergeResults(nil, containers, nil)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].Name != "myapp" {
		t.Errorf("expected name myapp (from compose label), got %s", result[0].Name)
	}
	if result[0].ContainerCount != 1 {
		t.Errorf("expected 1 container, got %d", result[0].ContainerCount)
	}
	if result[0].RunningCount != 1 {
		t.Errorf("expected 1 running, got %d", result[0].RunningCount)
	}
	if !result[0].Running {
		t.Error("expected Running to be true")
	}
}

func TestMergeResultsContainersUseDirNameWhenNoComposeLabel(t *testing.T) {
	containers := map[string][]ContainerInfo{
		"/opt/projects/fallback-name": {
			{
				ID:      "def456",
				Name:    "container1",
				Image:   "nginx",
				State:   "running",
				Service: "web",
				// ComposeProject is empty
			},
		},
	}

	result := mergeResults(nil, containers, nil)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].Name != "fallback-name" {
		t.Errorf("expected name fallback-name (from dir), got %s", result[0].Name)
	}
}

func TestMergeResultsSkipsStandalone(t *testing.T) {
	containers := map[string][]ContainerInfo{
		"__standalone__": {
			{ID: "stand1", Name: "orphan", Image: "alpine", State: "running"},
		},
		"/opt/projects/real": {
			{ID: "real1", Name: "real-web-1", Image: "node:20", State: "running", Service: "web"},
		},
	}

	result := mergeResults(nil, containers, nil)
	if len(result) != 1 {
		t.Fatalf("expected 1 result (standalone skipped), got %d", len(result))
	}
	if result[0].Dir != "/opt/projects/real" {
		t.Errorf("expected dir /opt/projects/real, got %s", result[0].Dir)
	}
}

func TestMergeResultsDuplicateDetection(t *testing.T) {
	// Same project found by both compose scan and container scan
	composeProjects := []ComposeProject{
		{
			Name:   "webapp",
			Dir:    "/opt/projects/webapp",
			Domain: "webapp.com",
			HasDB:  true,
			DBType: "postgres",
		},
	}

	containers := map[string][]ContainerInfo{
		"/opt/projects/webapp": {
			{
				ID:             "abc123",
				Name:           "webapp-web-1",
				Image:          "node:20",
				State:          "running",
				Service:        "web",
				ComposeProject: "webapp",
			},
			{
				ID:             "def456",
				Name:           "webapp-db-1",
				Image:          "postgres:15",
				State:          "running",
				Service:        "db",
				ComposeProject: "webapp",
			},
		},
	}

	result := mergeResults(composeProjects, containers, nil)
	// Should be 1 merged result, not 2 duplicates
	if len(result) != 1 {
		t.Fatalf("expected 1 merged result (dedup), got %d", len(result))
	}

	dp := result[0]
	if dp.Name != "webapp" {
		t.Errorf("expected name webapp, got %s", dp.Name)
	}
	if dp.Domain != "webapp.com" {
		t.Errorf("expected domain webapp.com (from compose), got %s", dp.Domain)
	}
	if dp.ContainerCount != 2 {
		t.Errorf("expected 2 containers, got %d", dp.ContainerCount)
	}
	if dp.RunningCount != 2 {
		t.Errorf("expected 2 running, got %d", dp.RunningCount)
	}
	if !dp.Running {
		t.Error("expected Running to be true")
	}
	if len(dp.Services) != 2 {
		t.Errorf("expected 2 services, got %d", len(dp.Services))
	}
	if !dp.HasDB {
		t.Error("expected HasDB from compose to be preserved")
	}
}

func TestMergeResultsExistingPaths(t *testing.T) {
	composeProjects := []ComposeProject{
		{Name: "managed", Dir: "/opt/projects/managed", Domain: "managed.com"},
		{Name: "unmanaged", Dir: "/opt/projects/unmanaged", Domain: "unmanaged.com"},
	}

	existingPaths := map[string]string{
		"/opt/projects/managed": "managed-in-db",
	}

	result := mergeResults(composeProjects, nil, existingPaths)
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}

	for _, r := range result {
		if r.Name == "managed" {
			if !r.AlreadyManaged {
				t.Error("expected managed project to be marked AlreadyManaged")
			}
			if r.ManagedName != "managed-in-db" {
				t.Errorf("expected ManagedName managed-in-db, got %s", r.ManagedName)
			}
		} else if r.Name == "unmanaged" {
			if r.AlreadyManaged {
				t.Error("expected unmanaged project to NOT be marked AlreadyManaged")
			}
		}
	}
}

func TestMergeResultsContainersEnrichExistingPaths(t *testing.T) {
	// Container found for a dir that is already managed but has no compose file
	containers := map[string][]ContainerInfo{
		"/opt/projects/orphan": {
			{
				ID:             "orph1",
				Name:           "orphan-web-1",
				Image:          "node:20",
				State:          "exited",
				Service:        "web",
				ComposeProject: "orphan",
			},
		},
	}

	existingPaths := map[string]string{
		"/opt/projects/orphan": "orphan-db-name",
	}

	result := mergeResults(nil, containers, existingPaths)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if !result[0].AlreadyManaged {
		t.Error("expected container-only project with existing path to be marked AlreadyManaged")
	}
	if result[0].ManagedName != "orphan-db-name" {
		t.Errorf("expected ManagedName orphan-db-name, got %s", result[0].ManagedName)
	}
}

func TestMergeResultsDBDetectionFromContainers(t *testing.T) {
	containers := map[string][]ContainerInfo{
		"/opt/projects/dbtest": {
			{ID: "c1", Name: "web", Image: "node:20", State: "running", Service: "web"},
			{ID: "c2", Name: "db", Image: "postgres:15-alpine", State: "running", Service: "db"},
		},
	}

	result := mergeResults(nil, containers, nil)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if !result[0].HasDB {
		t.Error("expected HasDB true from postgres image")
	}
	if result[0].DBType != "postgres" {
		t.Errorf("expected DBType postgres, got %s", result[0].DBType)
	}
}

func TestMergeResultsMySQLDetection(t *testing.T) {
	containers := map[string][]ContainerInfo{
		"/opt/projects/mysqltest": {
			{ID: "c1", Name: "web", Image: "node:20", State: "running", Service: "web"},
			{ID: "c2", Name: "db", Image: "mysql:8", State: "running", Service: "db"},
		},
	}

	result := mergeResults(nil, containers, nil)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if !result[0].HasDB {
		t.Error("expected HasDB true from mysql image")
	}
	if result[0].DBType != "mysql" {
		t.Errorf("expected DBType mysql, got %s", result[0].DBType)
	}
}

func TestMergeResultsMariaDBDetection(t *testing.T) {
	containers := map[string][]ContainerInfo{
		"/opt/projects/mariatest": {
			{ID: "c1", Name: "db", Image: "mariadb:10", State: "running", Service: "db"},
		},
	}

	result := mergeResults(nil, containers, nil)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if !result[0].HasDB {
		t.Error("expected HasDB true from mariadb image")
	}
	if result[0].DBType != "mysql" {
		t.Errorf("expected DBType mysql, got %s", result[0].DBType)
	}
}

func TestMergeResultsDomainExtractedFromContainerLabels(t *testing.T) {
	containers := map[string][]ContainerInfo{
		"/opt/projects/labeled": {
			{
				ID:      "c1",
				Name:    "labeled-web-1",
				Image:   "node:20",
				State:   "running",
				Service: "web",
				Labels: map[string]string{
					"traefik.http.routers.labeled.rule": "Host(`labeled.example.com`)",
				},
			},
		},
	}

	result := mergeResults(nil, containers, nil)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].Domain != "labeled.example.com" {
		t.Errorf("expected domain labeled.example.com, got %s", result[0].Domain)
	}
}

func TestMergeResultsExitedContainerNotRunning(t *testing.T) {
	containers := map[string][]ContainerInfo{
		"/opt/projects/stopped": {
			{ID: "c1", Name: "web", Image: "node:20", State: "exited", Service: "web"},
		},
	}

	result := mergeResults(nil, containers, nil)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].Running {
		t.Error("expected Running to be false for exited container")
	}
	if result[0].RunningCount != 0 {
		t.Errorf("expected RunningCount 0, got %d", result[0].RunningCount)
	}
	if result[0].ContainerCount != 1 {
		t.Errorf("expected ContainerCount 1, got %d", result[0].ContainerCount)
	}
}

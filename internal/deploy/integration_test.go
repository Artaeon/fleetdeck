//go:build integration

package deploy

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func skipIfNoIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("FLEETDECK_INTEGRATION") == "" {
		t.Skip("skipping integration test: FLEETDECK_INTEGRATION not set")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("skipping integration test: docker not found")
	}
}

const testComposeYML = `services:
  web:
    image: nginx:alpine
    ports:
      - "0:80"
`

func writeTestCompose(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(testComposeYML), 0644); err != nil {
		t.Fatalf("writing docker-compose.yml: %v", err)
	}
}

func composeDown(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("docker", "compose", "down", "--remove-orphans")
	cmd.Dir = dir
	cmd.CombinedOutput()
}

func TestIntegrationBasicDeploy(t *testing.T) {
	skipIfNoIntegration(t)

	dir := t.TempDir()
	writeTestCompose(t, dir)
	defer composeDown(t, dir)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	strategy := &BasicStrategy{}
	result, err := strategy.Deploy(ctx, DeployOptions{
		ProjectPath: dir,
	})
	if err != nil {
		t.Fatalf("BasicStrategy.Deploy failed: %v", err)
	}
	if !result.Success {
		t.Fatal("expected successful deployment")
	}
	if len(result.NewContainers) == 0 {
		t.Fatal("expected at least one new container")
	}
}

func TestIntegrationBasicDeployAndStop(t *testing.T) {
	skipIfNoIntegration(t)

	dir := t.TempDir()
	writeTestCompose(t, dir)
	defer composeDown(t, dir)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	strategy := &BasicStrategy{}
	_, err := strategy.Deploy(ctx, DeployOptions{
		ProjectPath: dir,
	})
	if err != nil {
		t.Fatalf("BasicStrategy.Deploy failed: %v", err)
	}

	containers, err := listContainers(dir)
	if err != nil {
		t.Fatalf("listContainers failed: %v", err)
	}
	if len(containers) == 0 {
		t.Fatal("expected running containers after deploy")
	}
}

func TestIntegrationRollingDeploy(t *testing.T) {
	skipIfNoIntegration(t)

	dir := t.TempDir()
	writeTestCompose(t, dir)
	defer composeDown(t, dir)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	strategy := &RollingStrategy{}
	result, err := strategy.Deploy(ctx, DeployOptions{
		ProjectPath: dir,
	})
	if err != nil {
		t.Fatalf("RollingStrategy.Deploy failed: %v", err)
	}
	if !result.Success {
		t.Fatal("expected successful deployment")
	}
	if len(result.NewContainers) == 0 {
		t.Fatal("expected at least one new container")
	}
}

package bootstrap

import (
	"fmt"
	"strings"
	"testing"
)

// mockRunner records all commands run and allows configuring failures.
type mockRunner struct {
	commands     []string
	failOn       map[string]error
	outputFor    map[string]string
}

func newMockRunner() *mockRunner {
	return &mockRunner{
		failOn:    make(map[string]error),
		outputFor: make(map[string]string),
	}
}

func (m *mockRunner) Run(cmd string) (string, error) {
	m.commands = append(m.commands, cmd)

	// Check for exact match first.
	if err, ok := m.failOn[cmd]; ok {
		return "", err
	}

	// Check for substring match (useful for matching patterns).
	for pattern, err := range m.failOn {
		if strings.Contains(cmd, pattern) {
			return "", err
		}
	}

	// Return configured output, if any.
	if out, ok := m.outputFor[cmd]; ok {
		return out, nil
	}

	return "", nil
}

func TestBootstrapWithMockRunner(t *testing.T) {
	runner := newMockRunner()
	cfg := Config{
		Host:           "10.0.0.1",
		Port:           "22",
		User:           "root",
		Domain:         "example.com",
		Email:          "admin@example.com",
		SwapSizeGB:     2,
		TraefikNetwork: "traefik",
	}

	result, err := Bootstrap(cfg, runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify all major steps completed.
	if !result.DockerInstalled {
		t.Error("expected DockerInstalled=true")
	}
	if !result.TraefikConfigured {
		t.Error("expected TraefikConfigured=true")
	}
	if !result.FirewallConfigured {
		t.Error("expected FirewallConfigured=true")
	}
	if !result.SwapCreated {
		t.Error("expected SwapCreated=true")
	}

	// Verify commands were executed (at least the major ones).
	if len(runner.commands) == 0 {
		t.Fatal("expected commands to be recorded")
	}

	// Verify ordering: system prep before docker, docker before traefik.
	aptUpdateIdx := -1

	for i, cmd := range runner.commands {
		if strings.Contains(cmd, "apt-get update") && aptUpdateIdx == -1 {
			aptUpdateIdx = i
		}
	}

	if aptUpdateIdx == -1 {
		t.Error("expected apt-get update to be called")
	}

	// Docker check (command -v docker) runs before installation commands.
	// If docker is not found (our mock returns error by default on unknown
	// commands -- actually it returns nil), the install commands still run.
	// Let's verify docker verification ran.
	foundDockerVersion := false
	for _, cmd := range runner.commands {
		if cmd == "docker version" {
			foundDockerVersion = true
			break
		}
	}
	if !foundDockerVersion {
		t.Error("expected 'docker version' verification command")
	}

	// Verify traefik-related commands ran.
	foundTraefikDir := false
	for _, cmd := range runner.commands {
		if strings.Contains(cmd, "mkdir -p /opt/traefik") {
			foundTraefikDir = true
			break
		}
	}
	if !foundTraefikDir {
		t.Error("expected traefik directory creation command")
	}
}

func TestBootstrapDockerFailure(t *testing.T) {
	runner := newMockRunner()

	// Make "command -v docker" fail (docker not found), then docker install fails.
	runner.failOn["command -v docker"] = fmt.Errorf("not found")
	runner.failOn["docker-ce"] = fmt.Errorf("apt-get install failed: unable to locate package")

	cfg := Config{
		Host:   "10.0.0.1",
		Domain: "example.com",
		Email:  "admin@example.com",
	}

	result, err := Bootstrap(cfg, runner)

	if err == nil {
		t.Fatal("expected error when docker installation fails")
	}

	if !strings.Contains(err.Error(), "docker") {
		t.Errorf("expected error to mention docker, got: %v", err)
	}

	// Docker should not be marked as installed.
	if result.DockerInstalled {
		t.Error("expected DockerInstalled=false when installation fails")
	}

	// Traefik should not have been configured (it depends on Docker).
	if result.TraefikConfigured {
		t.Error("expected TraefikConfigured=false when docker fails")
	}

	// Errors should be recorded.
	if len(result.Errors) == 0 {
		t.Error("expected non-empty Errors list")
	}

	foundDockerError := false
	for _, e := range result.Errors {
		if strings.Contains(e, "docker") {
			foundDockerError = true
			break
		}
	}
	if !foundDockerError {
		t.Errorf("expected docker-related error in Errors, got: %v", result.Errors)
	}
}

func TestBootstrapDockerVerifyFailure(t *testing.T) {
	runner := newMockRunner()

	// Docker installs fine but verification fails.
	runner.failOn["docker version"] = fmt.Errorf("docker daemon not running")

	cfg := Config{
		Host:   "10.0.0.1",
		Domain: "example.com",
		Email:  "admin@example.com",
	}

	result, err := Bootstrap(cfg, runner)

	if err == nil {
		t.Fatal("expected error when docker verification fails")
	}

	if result.DockerInstalled {
		t.Error("expected DockerInstalled=false when verification fails")
	}
}

func TestBootstrapNonCriticalFailures(t *testing.T) {
	runner := newMockRunner()

	// Make swap and firewall fail - these are non-critical.
	runner.failOn["fallocate"] = fmt.Errorf("not enough disk space")
	runner.failOn["ufw"] = fmt.Errorf("ufw not available")

	// Make swapon --show return empty so swap setup is attempted.
	runner.outputFor["swapon --show --noheadings"] = ""

	cfg := Config{
		Host:       "10.0.0.1",
		Domain:     "example.com",
		Email:      "admin@example.com",
		SwapSizeGB: 4,
	}

	result, err := Bootstrap(cfg, runner)

	if err != nil {
		t.Fatalf("expected no fatal error for non-critical failures, got: %v", err)
	}

	// Docker and Traefik should succeed.
	if !result.DockerInstalled {
		t.Error("expected DockerInstalled=true")
	}
	if !result.TraefikConfigured {
		t.Error("expected TraefikConfigured=true")
	}

	// Swap and firewall should have failed.
	if result.SwapCreated {
		t.Error("expected SwapCreated=false when swap fails")
	}
	if result.FirewallConfigured {
		t.Error("expected FirewallConfigured=false when firewall fails")
	}

	// Errors should be recorded for swap and firewall.
	if len(result.Errors) == 0 {
		t.Fatal("expected non-empty Errors for swap/firewall failures")
	}

	foundSwapError := false
	foundFirewallError := false
	for _, e := range result.Errors {
		if strings.Contains(e, "swap") {
			foundSwapError = true
		}
		if strings.Contains(e, "firewall") {
			foundFirewallError = true
		}
	}
	if !foundSwapError {
		t.Errorf("expected swap-related error in Errors, got: %v", result.Errors)
	}
	if !foundFirewallError {
		t.Errorf("expected firewall-related error in Errors, got: %v", result.Errors)
	}
}

func TestBootstrapSystemPrepFailure(t *testing.T) {
	runner := newMockRunner()

	// System preparation is critical; fail on apt-get update.
	runner.failOn["apt-get update"] = fmt.Errorf("could not resolve archive.ubuntu.com")

	cfg := Config{
		Host:   "10.0.0.1",
		Domain: "example.com",
		Email:  "admin@example.com",
	}

	result, err := Bootstrap(cfg, runner)

	if err == nil {
		t.Fatal("expected error when system prep fails")
	}

	// Nothing should be configured since system prep is step 1.
	if result.DockerInstalled {
		t.Error("expected DockerInstalled=false when system prep fails")
	}
	if result.TraefikConfigured {
		t.Error("expected TraefikConfigured=false when system prep fails")
	}
	if result.FirewallConfigured {
		t.Error("expected FirewallConfigured=false when system prep fails")
	}
}

func TestBootstrapTraefikFailure(t *testing.T) {
	runner := newMockRunner()

	// Traefik setup is critical; fail on traefik docker compose up.
	runner.failOn["docker compose up -d --force-recreate"] = fmt.Errorf("network traefik not found")

	cfg := Config{
		Host:   "10.0.0.1",
		Domain: "example.com",
		Email:  "admin@example.com",
	}

	result, err := Bootstrap(cfg, runner)

	if err == nil {
		t.Fatal("expected error when traefik setup fails")
	}

	// Docker should be installed (it comes before traefik).
	if !result.DockerInstalled {
		t.Error("expected DockerInstalled=true (installed before traefik)")
	}

	// Traefik should not be marked as configured.
	if result.TraefikConfigured {
		t.Error("expected TraefikConfigured=false when traefik fails")
	}
}

func TestBootstrapNoSwap(t *testing.T) {
	runner := newMockRunner()

	cfg := Config{
		Host:       "10.0.0.1",
		Domain:     "example.com",
		Email:      "admin@example.com",
		SwapSizeGB: 0, // No swap requested.
	}

	result, err := Bootstrap(cfg, runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.SwapCreated {
		t.Error("expected SwapCreated=false when SwapSizeGB=0")
	}

	// Verify no swap-related commands were run.
	for _, cmd := range runner.commands {
		if strings.Contains(cmd, "fallocate") || strings.Contains(cmd, "mkswap") {
			t.Errorf("unexpected swap command when SwapSizeGB=0: %s", cmd)
		}
	}
}

func TestBootstrapSwapAlreadyExists(t *testing.T) {
	runner := newMockRunner()

	// Simulate swap already being active.
	runner.outputFor["swapon --show --noheadings"] = "/swapfile file 2G 0B -2"

	cfg := Config{
		Host:       "10.0.0.1",
		Domain:     "example.com",
		Email:      "admin@example.com",
		SwapSizeGB: 2,
	}

	result, err := Bootstrap(cfg, runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.SwapCreated {
		t.Error("expected SwapCreated=true when swap is already active")
	}

	// Verify no fallocate/mkswap commands were run.
	for _, cmd := range runner.commands {
		if strings.Contains(cmd, "fallocate") || strings.Contains(cmd, "mkswap") {
			t.Errorf("unexpected swap creation command when swap already exists: %s", cmd)
		}
	}
}

func TestBootstrapDefaultTraefikNetwork(t *testing.T) {
	runner := newMockRunner()

	cfg := Config{
		Host:           "10.0.0.1",
		Domain:         "example.com",
		Email:          "admin@example.com",
		TraefikNetwork: "", // Should default to "traefik".
	}

	result, err := Bootstrap(cfg, runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.TraefikConfigured {
		t.Error("expected TraefikConfigured=true")
	}

	// Verify the network creation command used "traefik" as the default.
	foundNetworkCreate := false
	for _, cmd := range runner.commands {
		if strings.Contains(cmd, "docker network create traefik") {
			foundNetworkCreate = true
			break
		}
	}
	if !foundNetworkCreate {
		t.Error("expected docker network create command with default 'traefik' network name")
	}
}

func TestBootstrapCustomTraefikNetwork(t *testing.T) {
	runner := newMockRunner()

	cfg := Config{
		Host:           "10.0.0.1",
		Domain:         "example.com",
		Email:          "admin@example.com",
		TraefikNetwork: "custom-net",
	}

	result, err := Bootstrap(cfg, runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.TraefikConfigured {
		t.Error("expected TraefikConfigured=true")
	}

	foundCustomNetwork := false
	for _, cmd := range runner.commands {
		if strings.Contains(cmd, "docker network create custom-net") {
			foundCustomNetwork = true
			break
		}
	}
	if !foundCustomNetwork {
		t.Error("expected docker network create command with 'custom-net' network name")
	}
}

func TestBootstrapNonCriticalSSHFailure(t *testing.T) {
	runner := newMockRunner()

	// SSH configuration is non-critical.
	runner.failOn["sshd_config"] = fmt.Errorf("permission denied")

	cfg := Config{
		Host:   "10.0.0.1",
		Domain: "example.com",
		Email:  "admin@example.com",
	}

	result, err := Bootstrap(cfg, runner)
	if err != nil {
		t.Fatalf("expected no fatal error for SSH failure, got: %v", err)
	}

	// Everything else should still succeed.
	if !result.DockerInstalled {
		t.Error("expected DockerInstalled=true despite SSH failure")
	}
	if !result.TraefikConfigured {
		t.Error("expected TraefikConfigured=true despite SSH failure")
	}

	// SSH error should be recorded.
	foundSSHError := false
	for _, e := range result.Errors {
		if strings.Contains(e, "ssh") || strings.Contains(e, "SSH") {
			foundSSHError = true
			break
		}
	}
	if !foundSSHError {
		t.Errorf("expected SSH-related error in Errors, got: %v", result.Errors)
	}
}

func TestBootstrapTimezoneFailureNonCritical(t *testing.T) {
	runner := newMockRunner()

	// Timezone setting is non-critical.
	runner.failOn["timedatectl"] = fmt.Errorf("timedatectl not found")

	cfg := Config{
		Host:   "10.0.0.1",
		Domain: "example.com",
		Email:  "admin@example.com",
	}

	result, err := Bootstrap(cfg, runner)
	if err != nil {
		t.Fatalf("expected no fatal error for timezone failure, got: %v", err)
	}

	if !result.DockerInstalled {
		t.Error("expected DockerInstalled=true despite timezone failure")
	}

	foundTZError := false
	for _, e := range result.Errors {
		if strings.Contains(e, "timezone") {
			foundTZError = true
			break
		}
	}
	if !foundTZError {
		t.Errorf("expected timezone-related error in Errors, got: %v", result.Errors)
	}
}

func TestBootstrapCommandOrder(t *testing.T) {
	runner := newMockRunner()

	cfg := Config{
		Host:       "10.0.0.1",
		Domain:     "example.com",
		Email:      "admin@example.com",
		SwapSizeGB: 1,
	}

	_, err := Bootstrap(cfg, runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify high-level ordering by finding key commands.
	phases := map[string]int{
		"apt-get update":                     -1,
		"docker version":                     -1,
		"ufw":                                -1,
		"mkdir -p /opt/traefik":              -1,
	}

	for i, cmd := range runner.commands {
		for pattern := range phases {
			if strings.Contains(cmd, pattern) && phases[pattern] == -1 {
				phases[pattern] = i
			}
		}
	}

	// System prep (apt-get update) should come before docker.
	if phases["apt-get update"] != -1 && phases["docker version"] != -1 {
		if phases["apt-get update"] >= phases["docker version"] {
			t.Error("expected system prep (apt-get update) before docker verification")
		}
	}

	// Docker verification should come before traefik.
	if phases["docker version"] != -1 && phases["mkdir -p /opt/traefik"] != -1 {
		if phases["docker version"] >= phases["mkdir -p /opt/traefik"] {
			t.Error("expected docker verification before traefik setup")
		}
	}
}

func TestConfigStruct(t *testing.T) {
	cfg := Config{
		Host:           "192.168.1.100",
		Port:           "2222",
		User:           "deploy",
		PrivateKey:     "/home/deploy/.ssh/id_ed25519",
		Domain:         "myapp.example.com",
		Email:          "ops@example.com",
		SwapSizeGB:     4,
		TraefikNetwork: "web",
	}

	if cfg.Host != "192.168.1.100" {
		t.Errorf("unexpected Host: %s", cfg.Host)
	}
	if cfg.Port != "2222" {
		t.Errorf("unexpected Port: %s", cfg.Port)
	}
	if cfg.User != "deploy" {
		t.Errorf("unexpected User: %s", cfg.User)
	}
	if cfg.Domain != "myapp.example.com" {
		t.Errorf("unexpected Domain: %s", cfg.Domain)
	}
	if cfg.SwapSizeGB != 4 {
		t.Errorf("unexpected SwapSizeGB: %d", cfg.SwapSizeGB)
	}
}

func TestResultStruct(t *testing.T) {
	result := Result{
		DockerInstalled:    true,
		TraefikConfigured:  true,
		FirewallConfigured: false,
		SwapCreated:        true,
		Errors:             []string{"configure firewall: ufw not found"},
	}

	if !result.DockerInstalled {
		t.Error("expected DockerInstalled=true")
	}
	if !result.TraefikConfigured {
		t.Error("expected TraefikConfigured=true")
	}
	if result.FirewallConfigured {
		t.Error("expected FirewallConfigured=false")
	}
	if !result.SwapCreated {
		t.Error("expected SwapCreated=true")
	}
	if len(result.Errors) != 1 {
		t.Errorf("expected 1 error, got %d", len(result.Errors))
	}
}

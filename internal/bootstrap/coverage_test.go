package bootstrap

import (
	"fmt"
	"strings"
	"testing"
)

// TestBootstrapTimezoneFailureNonFatal verifies that a timezone failure is
// non-fatal: Bootstrap still succeeds but the error is recorded in Result.Errors.
func TestBootstrapTimezoneFailureNonFatal(t *testing.T) {
	runner := newMockRunner()
	runner.failOn["timedatectl"] = fmt.Errorf("timedatectl: command not found")

	cfg := Config{
		Host:   "10.0.0.1",
		Domain: "example.com",
		Email:  "admin@example.com",
	}

	result, err := Bootstrap(cfg, runner)
	if err != nil {
		t.Fatalf("expected Bootstrap to succeed despite timezone failure, got: %v", err)
	}

	if !result.DockerInstalled {
		t.Error("expected DockerInstalled=true despite timezone failure")
	}
	if !result.TraefikConfigured {
		t.Error("expected TraefikConfigured=true despite timezone failure")
	}

	// The timezone error should be recorded in Result.Errors.
	foundTZError := false
	for _, e := range result.Errors {
		if strings.Contains(e, "timezone") {
			foundTZError = true
			break
		}
	}
	if !foundTZError {
		t.Errorf("expected timezone error in Result.Errors, got: %v", result.Errors)
	}
}

// TestBootstrapSSHConfigFailureNonFatal verifies that an SSH configuration
// failure is non-fatal and Bootstrap continues through remaining steps.
func TestBootstrapSSHConfigFailureNonFatal(t *testing.T) {
	runner := newMockRunner()
	runner.failOn["sshd_config"] = fmt.Errorf("permission denied")

	cfg := Config{
		Host:       "10.0.0.1",
		Domain:     "example.com",
		Email:      "admin@example.com",
		SwapSizeGB: 1,
	}

	result, err := Bootstrap(cfg, runner)
	if err != nil {
		t.Fatalf("expected Bootstrap to succeed despite SSH failure, got: %v", err)
	}

	if !result.DockerInstalled {
		t.Error("expected DockerInstalled=true despite SSH failure")
	}
	if !result.TraefikConfigured {
		t.Error("expected TraefikConfigured=true despite SSH failure")
	}
	if !result.FirewallConfigured {
		t.Error("expected FirewallConfigured=true despite SSH failure")
	}

	// Verify SSH error is recorded.
	foundSSHError := false
	for _, e := range result.Errors {
		if strings.Contains(e, "ssh") {
			foundSSHError = true
			break
		}
	}
	if !foundSSHError {
		t.Errorf("expected ssh-related error in Result.Errors, got: %v", result.Errors)
	}
}

// TestBootstrapSwapFailureNonFatal verifies that a swap configuration failure
// is non-fatal, Bootstrap continues, and SwapCreated=false.
func TestBootstrapSwapFailureNonFatal(t *testing.T) {
	runner := newMockRunner()
	runner.outputFor["swapon --show --noheadings"] = ""
	runner.failOn["fallocate"] = fmt.Errorf("not enough disk space")

	cfg := Config{
		Host:       "10.0.0.1",
		Domain:     "example.com",
		Email:      "admin@example.com",
		SwapSizeGB: 2,
	}

	result, err := Bootstrap(cfg, runner)
	if err != nil {
		t.Fatalf("expected Bootstrap to succeed despite swap failure, got: %v", err)
	}

	if result.SwapCreated {
		t.Error("expected SwapCreated=false when swap configuration fails")
	}
	if !result.DockerInstalled {
		t.Error("expected DockerInstalled=true despite swap failure")
	}
	if !result.TraefikConfigured {
		t.Error("expected TraefikConfigured=true despite swap failure")
	}

	foundSwapError := false
	for _, e := range result.Errors {
		if strings.Contains(e, "swap") {
			foundSwapError = true
			break
		}
	}
	if !foundSwapError {
		t.Errorf("expected swap-related error in Result.Errors, got: %v", result.Errors)
	}
}

// TestBootstrapFirewallFailureNonFatal verifies that a firewall configuration
// failure is non-fatal, Bootstrap continues, and FirewallConfigured=false.
func TestBootstrapFirewallFailureNonFatal(t *testing.T) {
	runner := newMockRunner()
	runner.failOn["ufw"] = fmt.Errorf("ufw: command not found")

	cfg := Config{
		Host:   "10.0.0.1",
		Domain: "example.com",
		Email:  "admin@example.com",
	}

	result, err := Bootstrap(cfg, runner)
	if err != nil {
		t.Fatalf("expected Bootstrap to succeed despite firewall failure, got: %v", err)
	}

	if result.FirewallConfigured {
		t.Error("expected FirewallConfigured=false when firewall fails")
	}
	if !result.DockerInstalled {
		t.Error("expected DockerInstalled=true despite firewall failure")
	}
	if !result.TraefikConfigured {
		t.Error("expected TraefikConfigured=true despite firewall failure")
	}

	foundFirewallError := false
	for _, e := range result.Errors {
		if strings.Contains(e, "firewall") {
			foundFirewallError = true
			break
		}
	}
	if !foundFirewallError {
		t.Errorf("expected firewall-related error in Result.Errors, got: %v", result.Errors)
	}
}

// TestBootstrapDefaultNetwork verifies that when TraefikNetwork is empty,
// the default "traefik" network name is used.
func TestBootstrapDefaultNetwork(t *testing.T) {
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

	// The default "traefik" network should be used in the docker network create command.
	foundDefault := false
	for _, cmd := range runner.commands {
		if strings.Contains(cmd, "docker network create traefik") {
			foundDefault = true
			break
		}
	}
	if !foundDefault {
		t.Error("expected 'docker network create traefik' with default network name")
	}
}

// TestBootstrapCustomNetwork verifies that a custom network name is passed
// through to the traefik setup commands.
func TestBootstrapCustomNetwork(t *testing.T) {
	runner := newMockRunner()

	cfg := Config{
		Host:           "10.0.0.1",
		Domain:         "example.com",
		Email:          "admin@example.com",
		TraefikNetwork: "my-custom-network",
	}

	result, err := Bootstrap(cfg, runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.TraefikConfigured {
		t.Error("expected TraefikConfigured=true")
	}

	foundCustom := false
	for _, cmd := range runner.commands {
		if strings.Contains(cmd, "docker network create my-custom-network") {
			foundCustom = true
			break
		}
	}
	if !foundCustom {
		t.Error("expected 'docker network create my-custom-network' with custom network name")
	}
}

// TestBootstrapZeroSwap verifies that when SwapSizeGB=0 the swap step is
// skipped entirely and no swap-related commands are executed.
func TestBootstrapZeroSwap(t *testing.T) {
	runner := newMockRunner()

	cfg := Config{
		Host:       "10.0.0.1",
		Domain:     "example.com",
		Email:      "admin@example.com",
		SwapSizeGB: 0,
	}

	result, err := Bootstrap(cfg, runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.SwapCreated {
		t.Error("expected SwapCreated=false when SwapSizeGB=0")
	}

	// Verify no swap-related commands were issued at all, including swapon --show.
	for _, cmd := range runner.commands {
		if strings.Contains(cmd, "fallocate") || strings.Contains(cmd, "mkswap") || strings.Contains(cmd, "swapon") {
			t.Errorf("unexpected swap-related command when SwapSizeGB=0: %s", cmd)
		}
	}
}

// TestBootstrapAllStepsSucceed verifies that when every step succeeds, all
// result booleans are true and no errors are recorded.
func TestBootstrapAllStepsSucceed(t *testing.T) {
	runner := newMockRunner()
	runner.outputFor["swapon --show --noheadings"] = ""

	cfg := Config{
		Host:           "10.0.0.1",
		Domain:         "example.com",
		Email:          "admin@example.com",
		SwapSizeGB:     2,
		TraefikNetwork: "traefik",
	}

	result, err := Bootstrap(cfg, runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

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
	if len(result.Errors) != 0 {
		t.Errorf("expected no errors, got: %v", result.Errors)
	}
}

// TestBootstrapErrorsContainContext verifies that error messages recorded in
// Result.Errors contain the step name for context.
func TestBootstrapErrorsContainContext(t *testing.T) {
	runner := newMockRunner()
	runner.outputFor["swapon --show --noheadings"] = ""
	runner.failOn["timedatectl"] = fmt.Errorf("tz error")
	runner.failOn["sshd_config"] = fmt.Errorf("ssh error")
	runner.failOn["fallocate"] = fmt.Errorf("disk error")
	runner.failOn["ufw"] = fmt.Errorf("firewall error")

	cfg := Config{
		Host:       "10.0.0.1",
		Domain:     "example.com",
		Email:      "admin@example.com",
		SwapSizeGB: 1,
	}

	result, err := Bootstrap(cfg, runner)
	if err != nil {
		t.Fatalf("unexpected fatal error: %v", err)
	}

	// Each recorded error should contain the step name as a prefix for context.
	expectedPrefixes := []string{
		"set timezone:",
		"configure ssh:",
		"configure swap:",
		"configure firewall:",
	}

	for _, prefix := range expectedPrefixes {
		found := false
		for _, e := range result.Errors {
			if strings.Contains(e, prefix) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected an error containing %q in Result.Errors, got: %v", prefix, result.Errors)
		}
	}
}

// TestBootstrapStepsOrder verifies that system preparation runs before docker
// installation, and docker installation runs before traefik setup.
func TestBootstrapStepsOrder(t *testing.T) {
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

	// Find indices of key commands for each phase.
	systemPrepIdx := -1
	dockerIdx := -1
	traefikIdx := -1

	for i, cmd := range runner.commands {
		if strings.Contains(cmd, "apt-get update") && systemPrepIdx == -1 {
			systemPrepIdx = i
		}
		if strings.Contains(cmd, "docker version") && dockerIdx == -1 {
			dockerIdx = i
		}
		if strings.Contains(cmd, "mkdir -p /opt/traefik") && traefikIdx == -1 {
			traefikIdx = i
		}
	}

	if systemPrepIdx == -1 {
		t.Fatal("expected system prep command (apt-get update)")
	}
	if dockerIdx == -1 {
		t.Fatal("expected docker verification command (docker version)")
	}
	if traefikIdx == -1 {
		t.Fatal("expected traefik setup command (mkdir -p /opt/traefik)")
	}

	if systemPrepIdx >= dockerIdx {
		t.Errorf("expected system prep (idx=%d) before docker (idx=%d)", systemPrepIdx, dockerIdx)
	}
	if dockerIdx >= traefikIdx {
		t.Errorf("expected docker (idx=%d) before traefik (idx=%d)", dockerIdx, traefikIdx)
	}
}

// TestBootstrapTraefikDefaultNetwork verifies that when TraefikNetwork is
// empty, the "traefik" default is passed to setupTraefik and appears in the
// compose file written to the server.
func TestBootstrapTraefikDefaultNetwork(t *testing.T) {
	runner := newMockRunner()

	cfg := Config{
		Host:           "10.0.0.1",
		Domain:         "example.com",
		Email:          "admin@example.com",
		TraefikNetwork: "",
	}

	result, err := Bootstrap(cfg, runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.TraefikConfigured {
		t.Error("expected TraefikConfigured=true")
	}

	// Verify the compose file written contains the default "traefik" network.
	foundComposeWithDefault := false
	for _, cmd := range runner.commands {
		if strings.Contains(cmd, "docker-compose.yml") && strings.Contains(cmd, "providers.docker.network=traefik") {
			foundComposeWithDefault = true
			break
		}
	}
	if !foundComposeWithDefault {
		t.Error("expected docker-compose.yml to reference default 'traefik' network in providers.docker.network")
	}
}

// TestBootstrapResultZeroValue verifies that a freshly created Result struct
// has all booleans false and Errors nil.
func TestBootstrapResultZeroValue(t *testing.T) {
	var result Result

	if result.DockerInstalled {
		t.Error("expected DockerInstalled=false for zero-value Result")
	}
	if result.TraefikConfigured {
		t.Error("expected TraefikConfigured=false for zero-value Result")
	}
	if result.FirewallConfigured {
		t.Error("expected FirewallConfigured=false for zero-value Result")
	}
	if result.SwapCreated {
		t.Error("expected SwapCreated=false for zero-value Result")
	}
	if result.Errors != nil {
		t.Errorf("expected Errors=nil for zero-value Result, got: %v", result.Errors)
	}
}

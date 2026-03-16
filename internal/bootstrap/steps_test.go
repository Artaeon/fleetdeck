package bootstrap

import (
	"fmt"
	"strings"
	"testing"
)

// ---------- Test: prepareSystem ----------

func TestPrepareSystem(t *testing.T) {
	runner := newMockRunner()

	err := prepareSystem(runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(runner.commands) != 3 {
		t.Fatalf("expected 3 commands, got %d: %v", len(runner.commands), runner.commands)
	}

	// Verify apt-get update is the first command.
	if !strings.Contains(runner.commands[0], "apt-get update") {
		t.Errorf("expected first command to contain 'apt-get update', got %q", runner.commands[0])
	}

	// Verify apt-get upgrade runs.
	if !strings.Contains(runner.commands[1], "apt-get upgrade") {
		t.Errorf("expected second command to contain 'apt-get upgrade', got %q", runner.commands[1])
	}

	// Verify essential packages are installed.
	installCmd := runner.commands[2]
	essentialPackages := []string{"curl", "git", "htop", "unzip", "fail2ban"}
	for _, pkg := range essentialPackages {
		if !strings.Contains(installCmd, pkg) {
			t.Errorf("expected install command to contain %q, got %q", pkg, installCmd)
		}
	}

	// Verify DEBIAN_FRONTEND=noninteractive is set for all commands.
	for i, cmd := range runner.commands {
		if !strings.Contains(cmd, "DEBIAN_FRONTEND=noninteractive") {
			t.Errorf("command %d missing DEBIAN_FRONTEND=noninteractive: %q", i, cmd)
		}
	}
}

func TestPrepareSystemFailure(t *testing.T) {
	runner := newMockRunner()
	runner.failOn["apt-get update"] = fmt.Errorf("could not resolve archive.ubuntu.com")

	err := prepareSystem(runner)
	if err == nil {
		t.Fatal("expected error when apt-get update fails")
	}
	if !strings.Contains(err.Error(), "apt-get update") {
		t.Errorf("expected error to mention failing command, got: %v", err)
	}
}

func TestPrepareSystemUpgradeFailure(t *testing.T) {
	runner := newMockRunner()
	runner.failOn["apt-get upgrade"] = fmt.Errorf("upgrade failed: broken packages")

	err := prepareSystem(runner)
	if err == nil {
		t.Fatal("expected error when apt-get upgrade fails")
	}

	// Verify update ran but upgrade failed, so install did not run.
	foundUpdate := false
	foundInstall := false
	for _, cmd := range runner.commands {
		if strings.Contains(cmd, "apt-get update") {
			foundUpdate = true
		}
		if strings.Contains(cmd, "apt-get install") {
			foundInstall = true
		}
	}
	if !foundUpdate {
		t.Error("expected apt-get update to have run")
	}
	if foundInstall {
		t.Error("expected apt-get install NOT to run after upgrade failure")
	}
}

// ---------- Test: configureSwap ----------

func TestConfigureSwapAlreadyExists(t *testing.T) {
	runner := newMockRunner()
	runner.outputFor["swapon --show --noheadings"] = "/swapfile file 2G 0B -2"

	err := configureSwap(runner, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// When swap exists, no creation commands should run.
	for _, cmd := range runner.commands {
		if strings.Contains(cmd, "fallocate") {
			t.Errorf("unexpected fallocate command when swap exists: %s", cmd)
		}
		if strings.Contains(cmd, "mkswap") {
			t.Errorf("unexpected mkswap command when swap exists: %s", cmd)
		}
		if strings.Contains(cmd, "swapon /swapfile") {
			t.Errorf("unexpected swapon command when swap exists: %s", cmd)
		}
	}
}

func TestConfigureSwapCreatesNew(t *testing.T) {
	runner := newMockRunner()
	// swapon --show returns empty => no swap
	runner.outputFor["swapon --show --noheadings"] = ""

	err := configureSwap(runner, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedCommands := []string{
		"fallocate -l 2G /swapfile",
		"chmod 600 /swapfile",
		"mkswap /swapfile",
		"swapon /swapfile",
	}

	for _, expected := range expectedCommands {
		found := false
		for _, cmd := range runner.commands {
			if strings.Contains(cmd, expected) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected command %q to be run, commands were: %v", expected, runner.commands)
		}
	}

	// Verify fstab persistence.
	foundFstab := false
	for _, cmd := range runner.commands {
		if strings.Contains(cmd, "/etc/fstab") && strings.Contains(cmd, "/swapfile") {
			foundFstab = true
			break
		}
	}
	if !foundFstab {
		t.Error("expected fstab persistence command")
	}
}

func TestConfigureSwapCustomSize(t *testing.T) {
	sizes := []int{1, 4, 8}

	for _, size := range sizes {
		t.Run(fmt.Sprintf("%dGB", size), func(t *testing.T) {
			runner := newMockRunner()
			runner.outputFor["swapon --show --noheadings"] = ""

			err := configureSwap(runner, size)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			expectedFallocate := fmt.Sprintf("fallocate -l %dG /swapfile", size)
			found := false
			for _, cmd := range runner.commands {
				if cmd == expectedFallocate {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected %q command, got: %v", expectedFallocate, runner.commands)
			}
		})
	}
}

func TestConfigureSwapFallocateFailure(t *testing.T) {
	runner := newMockRunner()
	runner.outputFor["swapon --show --noheadings"] = ""
	runner.failOn["fallocate"] = fmt.Errorf("not enough disk space")

	err := configureSwap(runner, 2)
	if err == nil {
		t.Fatal("expected error when fallocate fails")
	}
	if !strings.Contains(err.Error(), "fallocate") {
		t.Errorf("expected error to mention fallocate, got: %v", err)
	}
}

func TestConfigureSwapMkswapFailure(t *testing.T) {
	runner := newMockRunner()
	runner.outputFor["swapon --show --noheadings"] = ""
	runner.failOn["mkswap"] = fmt.Errorf("mkswap failed")

	err := configureSwap(runner, 2)
	if err == nil {
		t.Fatal("expected error when mkswap fails")
	}
}

func TestConfigureSwapFstabFailure(t *testing.T) {
	runner := newMockRunner()
	runner.outputFor["swapon --show --noheadings"] = ""
	runner.failOn["/etc/fstab"] = fmt.Errorf("permission denied")

	err := configureSwap(runner, 2)
	if err == nil {
		t.Fatal("expected error when fstab write fails")
	}
	if !strings.Contains(err.Error(), "fstab") {
		t.Errorf("expected error to mention fstab, got: %v", err)
	}
}

// ---------- Test: setTimezone ----------

func TestSetTimezone(t *testing.T) {
	runner := newMockRunner()

	err := setTimezone(runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(runner.commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(runner.commands))
	}
	if runner.commands[0] != "timedatectl set-timezone UTC" {
		t.Errorf("expected 'timedatectl set-timezone UTC', got %q", runner.commands[0])
	}
}

func TestSetTimezoneFailure(t *testing.T) {
	runner := newMockRunner()
	runner.failOn["timedatectl"] = fmt.Errorf("timedatectl not found")

	err := setTimezone(runner)
	if err == nil {
		t.Fatal("expected error when timedatectl fails")
	}
	if !strings.Contains(err.Error(), "timezone") {
		t.Errorf("expected error to mention timezone, got: %v", err)
	}
}

// ---------- Test: configureSSH ----------

func TestConfigureSSH(t *testing.T) {
	runner := newMockRunner()

	err := configureSSH(runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Expect 2 sed commands + 1 service restart = 3 commands.
	if len(runner.commands) != 3 {
		t.Fatalf("expected 3 commands, got %d: %v", len(runner.commands), runner.commands)
	}

	// Verify PasswordAuthentication is disabled.
	foundPasswordAuth := false
	for _, cmd := range runner.commands {
		if strings.Contains(cmd, "PasswordAuthentication no") && strings.Contains(cmd, "sshd_config") {
			foundPasswordAuth = true
			break
		}
	}
	if !foundPasswordAuth {
		t.Error("expected sed command to disable PasswordAuthentication")
	}

	// Verify PermitRootLogin is disabled.
	foundRootLogin := false
	for _, cmd := range runner.commands {
		if strings.Contains(cmd, "PermitRootLogin prohibit-password") && strings.Contains(cmd, "sshd_config") {
			foundRootLogin = true
			break
		}
	}
	if !foundRootLogin {
		t.Error("expected sed command to disable PermitRootLogin")
	}

	// Verify service restart.
	restartCmd := runner.commands[2]
	if !strings.Contains(restartCmd, "restart sshd") && !strings.Contains(restartCmd, "ssh restart") {
		t.Errorf("expected SSH service restart command, got %q", restartCmd)
	}
}

func TestConfigureSSHSedFailure(t *testing.T) {
	runner := newMockRunner()
	runner.failOn["sshd_config"] = fmt.Errorf("permission denied")

	err := configureSSH(runner)
	if err == nil {
		t.Fatal("expected error when sed fails on sshd_config")
	}
}

func TestConfigureSSHRestartFailure(t *testing.T) {
	runner := newMockRunner()
	runner.failOn["restart sshd"] = fmt.Errorf("systemctl: command not found")

	err := configureSSH(runner)
	if err == nil {
		t.Fatal("expected error when sshd restart fails")
	}
	if !strings.Contains(err.Error(), "sshd") {
		t.Errorf("expected error to mention sshd, got: %v", err)
	}
}

// ---------- Test: installDocker ----------

func TestInstallDockerAlreadyInstalled(t *testing.T) {
	runner := newMockRunner()
	// "command -v docker" succeeds (returns no error) => docker already installed
	runner.outputFor["command -v docker"] = "/usr/bin/docker"

	err := installDocker(runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have run "command -v docker" and nothing else.
	if len(runner.commands) != 1 {
		t.Errorf("expected 1 command (just the check), got %d: %v", len(runner.commands), runner.commands)
	}
	if runner.commands[0] != "command -v docker" {
		t.Errorf("expected 'command -v docker', got %q", runner.commands[0])
	}
}

func TestInstallDockerFreshInstall(t *testing.T) {
	runner := newMockRunner()
	// "command -v docker" fails => docker not installed
	runner.failOn["command -v docker"] = fmt.Errorf("not found")

	err := installDocker(runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the check was the first command.
	if runner.commands[0] != "command -v docker" {
		t.Errorf("expected first command to be 'command -v docker', got %q", runner.commands[0])
	}

	// Verify key installation steps ran.
	expectedSubstrings := []string{
		"apt-get install",
		"docker.asc",
		"docker-ce",
		"systemctl enable docker",
		"systemctl start docker",
	}
	for _, substr := range expectedSubstrings {
		found := false
		for _, cmd := range runner.commands {
			if strings.Contains(cmd, substr) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected a command containing %q during fresh install", substr)
		}
	}

	// Verify docker-compose-plugin is installed.
	foundCompose := false
	for _, cmd := range runner.commands {
		if strings.Contains(cmd, "docker-compose-plugin") {
			foundCompose = true
			break
		}
	}
	if !foundCompose {
		t.Error("expected docker-compose-plugin in install command")
	}
}

func TestInstallDockerInstallFailure(t *testing.T) {
	runner := newMockRunner()
	runner.failOn["command -v docker"] = fmt.Errorf("not found")
	runner.failOn["docker-ce"] = fmt.Errorf("E: Unable to locate package docker-ce")

	err := installDocker(runner)
	if err == nil {
		t.Fatal("expected error when docker installation fails")
	}
}

// ---------- Test: verifyDocker ----------

func TestVerifyDocker(t *testing.T) {
	runner := newMockRunner()

	err := verifyDocker(runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(runner.commands) != 2 {
		t.Fatalf("expected 2 commands, got %d: %v", len(runner.commands), runner.commands)
	}
	if runner.commands[0] != "docker version" {
		t.Errorf("expected 'docker version', got %q", runner.commands[0])
	}
	if runner.commands[1] != "docker compose version" {
		t.Errorf("expected 'docker compose version', got %q", runner.commands[1])
	}
}

func TestVerifyDockerFailure(t *testing.T) {
	runner := newMockRunner()
	runner.failOn["docker version"] = fmt.Errorf("Cannot connect to the Docker daemon")

	err := verifyDocker(runner)
	if err == nil {
		t.Fatal("expected error when docker version fails")
	}
	if !strings.Contains(err.Error(), "docker version") {
		t.Errorf("expected error to mention 'docker version', got: %v", err)
	}

	// "docker compose version" should not run after "docker version" fails.
	foundComposeVersion := false
	for _, cmd := range runner.commands {
		if cmd == "docker compose version" {
			foundComposeVersion = true
		}
	}
	if foundComposeVersion {
		t.Error("expected 'docker compose version' NOT to run after 'docker version' failure")
	}
}

func TestVerifyDockerComposeFailure(t *testing.T) {
	runner := newMockRunner()
	runner.failOn["docker compose version"] = fmt.Errorf("docker compose not found")

	err := verifyDocker(runner)
	if err == nil {
		t.Fatal("expected error when docker compose version fails")
	}
	if !strings.Contains(err.Error(), "docker compose") {
		t.Errorf("expected error to mention 'docker compose', got: %v", err)
	}
}

// ---------- Test: configureFirewall ----------

func TestConfigureFirewall(t *testing.T) {
	runner := newMockRunner()

	err := configureFirewall(runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify UFW installation command.
	foundUFWInstall := false
	for _, cmd := range runner.commands {
		if strings.Contains(cmd, "apt-get install") && strings.Contains(cmd, "ufw") {
			foundUFWInstall = true
			break
		}
	}
	if !foundUFWInstall {
		t.Error("expected UFW installation command")
	}

	// Verify default policies.
	expectedRules := []string{
		"ufw default deny incoming",
		"ufw default allow outgoing",
		"ufw allow 22/tcp",
		"ufw allow 80/tcp",
		"ufw allow 443/tcp",
	}
	for _, rule := range expectedRules {
		found := false
		for _, cmd := range runner.commands {
			if cmd == rule {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected command %q to be run", rule)
		}
	}

	// Verify UFW is enabled.
	foundEnable := false
	for _, cmd := range runner.commands {
		if strings.Contains(cmd, "ufw enable") {
			foundEnable = true
			break
		}
	}
	if !foundEnable {
		t.Error("expected 'ufw enable' command")
	}
}

func TestConfigureFirewallInstallFailure(t *testing.T) {
	runner := newMockRunner()
	runner.failOn["apt-get install"] = fmt.Errorf("E: Unable to locate package ufw")

	err := configureFirewall(runner)
	if err == nil {
		t.Fatal("expected error when UFW installation fails")
	}
	if !strings.Contains(err.Error(), "ufw") {
		t.Errorf("expected error to mention ufw, got: %v", err)
	}

	// No firewall rules should have been attempted.
	for _, cmd := range runner.commands {
		if strings.HasPrefix(cmd, "ufw default") || strings.HasPrefix(cmd, "ufw allow") {
			t.Errorf("unexpected firewall rule command after install failure: %s", cmd)
		}
	}
}

func TestConfigureFirewallRuleFailure(t *testing.T) {
	runner := newMockRunner()
	runner.failOn["ufw allow 80/tcp"] = fmt.Errorf("ufw: error applying rule")

	err := configureFirewall(runner)
	if err == nil {
		t.Fatal("expected error when firewall rule fails")
	}
}

func TestConfigureFirewallEnableFailure(t *testing.T) {
	runner := newMockRunner()
	runner.failOn["ufw enable"] = fmt.Errorf("ufw: unable to enable")

	err := configureFirewall(runner)
	if err == nil {
		t.Fatal("expected error when ufw enable fails")
	}
	if !strings.Contains(err.Error(), "enabling ufw") {
		t.Errorf("expected error to mention enabling ufw, got: %v", err)
	}
}

func TestConfigureFirewallCommandOrder(t *testing.T) {
	runner := newMockRunner()

	err := configureFirewall(runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify ordering: install comes first, then rules, then enable.
	installIdx := -1
	denyIdx := -1
	enableIdx := -1

	for i, cmd := range runner.commands {
		if strings.Contains(cmd, "apt-get install") && strings.Contains(cmd, "ufw") && installIdx == -1 {
			installIdx = i
		}
		if cmd == "ufw default deny incoming" && denyIdx == -1 {
			denyIdx = i
		}
		if strings.Contains(cmd, "ufw enable") && enableIdx == -1 {
			enableIdx = i
		}
	}

	if installIdx == -1 || denyIdx == -1 || enableIdx == -1 {
		t.Fatalf("missing expected commands: install=%d, deny=%d, enable=%d", installIdx, denyIdx, enableIdx)
	}

	if installIdx >= denyIdx {
		t.Error("expected UFW install before firewall rules")
	}
	if denyIdx >= enableIdx {
		t.Error("expected firewall rules before ufw enable")
	}
}

// ---------- Test: configureSwap command ordering ----------

func TestConfigureSwapCommandOrder(t *testing.T) {
	runner := newMockRunner()
	runner.outputFor["swapon --show --noheadings"] = ""

	err := configureSwap(runner, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the order: fallocate -> chmod -> mkswap -> swapon -> fstab
	fallocateIdx := -1
	chmodIdx := -1
	mkswapIdx := -1
	swaponIdx := -1
	fstabIdx := -1

	for i, cmd := range runner.commands {
		switch {
		case strings.Contains(cmd, "fallocate") && fallocateIdx == -1:
			fallocateIdx = i
		case strings.Contains(cmd, "chmod 600") && chmodIdx == -1:
			chmodIdx = i
		case strings.Contains(cmd, "mkswap") && mkswapIdx == -1:
			mkswapIdx = i
		case cmd == "swapon /swapfile" && swaponIdx == -1:
			swaponIdx = i
		case strings.Contains(cmd, "/etc/fstab") && fstabIdx == -1:
			fstabIdx = i
		}
	}

	if fallocateIdx == -1 || chmodIdx == -1 || mkswapIdx == -1 || swaponIdx == -1 || fstabIdx == -1 {
		t.Fatalf("missing expected swap commands, got: %v", runner.commands)
	}

	if !(fallocateIdx < chmodIdx && chmodIdx < mkswapIdx && mkswapIdx < swaponIdx && swaponIdx < fstabIdx) {
		t.Errorf("swap commands out of order: fallocate=%d, chmod=%d, mkswap=%d, swapon=%d, fstab=%d",
			fallocateIdx, chmodIdx, mkswapIdx, swaponIdx, fstabIdx)
	}
}

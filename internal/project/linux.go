package project

import (
	"fmt"
	"os/exec"
	"os/user"
	"strings"
)

func LinuxUserName(projectName string) string {
	return "fleetdeck-" + projectName
}

func CreateLinuxUser(projectName, projectPath string) error {
	username := LinuxUserName(projectName)

	if _, err := user.Lookup(username); err == nil {
		return fmt.Errorf("user %q already exists", username)
	}

	cmd := exec.Command("useradd",
		"--system",
		"--shell", "/bin/bash",
		"--home-dir", projectPath,
		"--create-home",
		username,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("creating user %s: %s: %w", username, strings.TrimSpace(string(out)), err)
	}

	cmd = exec.Command("usermod", "-aG", "docker", username)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("adding %s to docker group: %s: %w", username, strings.TrimSpace(string(out)), err)
	}

	return nil
}

func DeleteLinuxUser(projectName string) error {
	username := LinuxUserName(projectName)

	if _, err := user.Lookup(username); err != nil {
		return nil // user doesn't exist, nothing to do
	}

	cmd := exec.Command("userdel", "--remove", username)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("deleting user %s: %s: %w", username, strings.TrimSpace(string(out)), err)
	}

	return nil
}

func SetupAuthorizedKeys(projectPath, publicKey string) error {
	cmd := exec.Command("bash", "-c", fmt.Sprintf(
		`mkdir -p %s/.ssh && echo %q > %s/.ssh/authorized_keys && chmod 700 %s/.ssh && chmod 600 %s/.ssh/authorized_keys`,
		projectPath, publicKey, projectPath, projectPath, projectPath,
	))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("setting up authorized_keys: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func ChownProjectDir(projectName, projectPath string) error {
	username := LinuxUserName(projectName)
	cmd := exec.Command("chown", "-R", username+":"+username, projectPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("chown: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

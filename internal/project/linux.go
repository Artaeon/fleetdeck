package project

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
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
		"--shell", "/usr/sbin/nologin",
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
	sshDir := filepath.Join(projectPath, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return fmt.Errorf("creating .ssh directory: %w", err)
	}

	authKeysPath := filepath.Join(sshDir, "authorized_keys")
	// Restrict the key to only allow non-interactive commands (no shell access)
	restrictedKey := fmt.Sprintf("restrict,command=\"/usr/bin/docker compose\" %s", publicKey)
	if err := os.WriteFile(authKeysPath, []byte(restrictedKey+"\n"), 0600); err != nil {
		return fmt.Errorf("writing authorized_keys: %w", err)
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

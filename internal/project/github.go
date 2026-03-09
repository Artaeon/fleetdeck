package project

import (
	"fmt"
	"os/exec"
	"strings"
)

func CreateGitHubRepo(org, name string, private bool) (string, error) {
	repoName := name
	if org != "" {
		repoName = org + "/" + name
	}

	args := []string{"repo", "create", repoName, "--confirm"}
	if private {
		args = append(args, "--private")
	} else {
		args = append(args, "--public")
	}

	cmd := exec.Command("gh", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("creating GitHub repo: %s: %w", strings.TrimSpace(string(out)), err)
	}

	return strings.TrimSpace(string(out)), nil
}

func SetGitHubSecret(repo, key, value string) error {
	cmd := exec.Command("gh", "secret", "set", key, "--repo", repo, "--body", value)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("setting secret %s: %s: %w", key, strings.TrimSpace(string(out)), err)
	}
	return nil
}

func DeleteGitHubRepo(repo string) error {
	cmd := exec.Command("gh", "repo", "delete", repo, "--yes")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("deleting GitHub repo: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func GetServerIP() (string, error) {
	cmd := exec.Command("hostname", "-I")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("getting server IP: %w", err)
	}
	parts := strings.Fields(string(out))
	if len(parts) == 0 {
		return "", fmt.Errorf("no IP addresses found")
	}
	return parts[0], nil
}

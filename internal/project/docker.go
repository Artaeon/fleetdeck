package project

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type ContainerStatus struct {
	Name   string
	State  string
	Status string
}

func ComposeUp(projectPath string) error {
	cmd := exec.Command("docker", "compose", "up", "-d")
	cmd.Dir = projectPath
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker compose up: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func ComposeDown(projectPath string) error {
	cmd := exec.Command("docker", "compose", "down")
	cmd.Dir = projectPath
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker compose down: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func ComposeRestart(projectPath string) error {
	cmd := exec.Command("docker", "compose", "restart")
	cmd.Dir = projectPath
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker compose restart: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func ComposeLogs(projectPath string, service string, tail int, follow bool) *exec.Cmd {
	args := []string{"compose", "logs"}
	if service != "" {
		args = append(args, service)
	}
	if tail > 0 {
		args = append(args, "--tail", fmt.Sprintf("%d", tail))
	}
	if follow {
		args = append(args, "--follow")
	}

	cmd := exec.Command("docker", args...)
	cmd.Dir = projectPath
	return cmd
}

func ComposePS(projectPath string) ([]ContainerStatus, error) {
	cmd := exec.Command("docker", "compose", "ps", "--format", "json")
	cmd.Dir = projectPath
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker compose ps: %w", err)
	}

	if len(strings.TrimSpace(string(out))) == 0 {
		return nil, nil
	}

	var containers []ContainerStatus
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		var c struct {
			Name   string `json:"Name"`
			State  string `json:"State"`
			Status string `json:"Status"`
		}
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			continue
		}
		containers = append(containers, ContainerStatus{
			Name:   c.Name,
			State:  c.State,
			Status: c.Status,
		})
	}

	return containers, nil
}

func CountContainers(projectPath string) (running, total int) {
	containers, err := ComposePS(projectPath)
	if err != nil {
		return 0, 0
	}
	total = len(containers)
	for _, c := range containers {
		if c.State == "running" {
			running++
		}
	}
	return
}

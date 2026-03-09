package project

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fleetdeck/fleetdeck/internal/templates"
)

func ScaffoldProject(projectPath string, tmpl *templates.Template, data templates.TemplateData) error {
	// Create directory structure
	dirs := []string{
		filepath.Join(projectPath, ".github", "workflows"),
		filepath.Join(projectPath, "deployments"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("creating directory %s: %w", d, err)
		}
	}

	// Render and write Dockerfile
	dockerfile, err := templates.Render(tmpl.Dockerfile, data)
	if err != nil {
		return fmt.Errorf("rendering Dockerfile: %w", err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, "Dockerfile"), []byte(dockerfile), 0644); err != nil {
		return err
	}

	// Render and write docker-compose.yml
	compose, err := templates.Render(tmpl.Compose, data)
	if err != nil {
		return fmt.Errorf("rendering docker-compose.yml: %w", err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, "docker-compose.yml"), []byte(compose), 0644); err != nil {
		return err
	}

	// Write GitHub Actions workflow
	if err := os.WriteFile(filepath.Join(projectPath, ".github", "workflows", "deploy.yml"), []byte(tmpl.Workflow), 0644); err != nil {
		return err
	}

	// Write .gitignore
	if err := os.WriteFile(filepath.Join(projectPath, ".gitignore"), []byte(tmpl.GitIgnore), 0644); err != nil {
		return err
	}

	return nil
}

func InitAndPushRepo(projectPath, repoURL string) error {
	commands := [][]string{
		{"git", "init"},
		{"git", "add", "."},
		{"git", "commit", "-m", "Initial scaffold from FleetDeck"},
		{"git", "branch", "-M", "main"},
		{"git", "remote", "add", "origin", repoURL},
		{"git", "push", "-u", "origin", "main"},
	}

	for _, args := range commands {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = projectPath
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("running %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(string(out)), err)
		}
	}

	return nil
}

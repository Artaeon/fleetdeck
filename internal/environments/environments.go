// Package environments manages per-project deployment environments such as
// staging, production, and preview. Each environment is a separate docker
// compose project with its own domain prefix.
package environments

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Environment describes a single deployment environment for a project.
type Environment struct {
	Name        string    `json:"name"`         // "staging", "production", "preview-<id>"
	Domain      string    `json:"domain"`       // e.g. "staging.app.example.com"
	Branch      string    `json:"branch"`       // git branch associated with this environment
	ProjectName string    `json:"project_name"` // parent project
	Status      string    `json:"status"`       // "running", "stopped", "creating", "error"
	CreatedAt   time.Time `json:"created_at"`
}

// Manager handles CRUD operations for project environments.
type Manager struct {
	basePath string
}

// NewManager returns a Manager that stores environment data under basePath.
func NewManager(basePath string) *Manager {
	return &Manager{basePath: basePath}
}

// Create provisions a new environment for the given project. It copies the
// project's docker-compose.yml into a dedicated directory, adjusting the
// domain and environment variables for the target environment.
func (m *Manager) Create(projectName, envName, domain, branch string) (*Environment, error) {
	projectPath := filepath.Join(m.basePath, projectName)
	envPath := m.GetEnvPath(projectName, envName)

	// Ensure the source project exists.
	srcCompose := filepath.Join(projectPath, "docker-compose.yml")
	if _, err := os.Stat(srcCompose); err != nil {
		return nil, fmt.Errorf("source project not found: %w", err)
	}

	// Create the environment directory.
	if err := os.MkdirAll(envPath, 0755); err != nil {
		return nil, fmt.Errorf("creating environment directory: %w", err)
	}

	// Read and adjust the compose file for this environment.
	composeData, err := os.ReadFile(srcCompose)
	if err != nil {
		return nil, fmt.Errorf("reading source compose file: %w", err)
	}

	adjusted := adjustComposeForEnv(string(composeData), projectName, envName, domain)
	dstCompose := filepath.Join(envPath, "docker-compose.yml")
	if err := os.WriteFile(dstCompose, []byte(adjusted), 0644); err != nil {
		return nil, fmt.Errorf("writing environment compose file: %w", err)
	}

	// Copy .env file if present, adjusting domain references.
	srcEnv := filepath.Join(projectPath, ".env")
	if data, err := os.ReadFile(srcEnv); err == nil {
		envData := adjustEnvFile(string(data), domain)
		if err := os.WriteFile(filepath.Join(envPath, ".env"), []byte(envData), 0644); err != nil {
			return nil, fmt.Errorf("writing environment .env file: %w", err)
		}
	}

	env := &Environment{
		Name:        envName,
		Domain:      domain,
		Branch:      branch,
		ProjectName: projectName,
		Status:      "creating",
		CreatedAt:   time.Now().UTC(),
	}

	// Persist metadata.
	if err := m.saveMetadata(envPath, env); err != nil {
		return nil, fmt.Errorf("saving environment metadata: %w", err)
	}

	return env, nil
}

// List returns all environments for the given project.
func (m *Manager) List(projectName string) ([]Environment, error) {
	envsDir := filepath.Join(m.basePath, projectName, "environments")
	entries, err := os.ReadDir(envsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("listing environments: %w", err)
	}

	var envs []Environment
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		env, err := m.loadMetadata(filepath.Join(envsDir, entry.Name()))
		if err != nil {
			continue // skip corrupted entries
		}
		envs = append(envs, *env)
	}
	return envs, nil
}

// Promote copies configuration and images from one environment to another.
// Typically used for staging → production promotion.
func (m *Manager) Promote(projectName, fromEnv, toEnv string) error {
	fromPath := m.GetEnvPath(projectName, fromEnv)
	toPath := m.GetEnvPath(projectName, toEnv)

	// Verify source environment exists.
	if _, err := os.Stat(fromPath); err != nil {
		return fmt.Errorf("source environment %q not found: %w", fromEnv, err)
	}

	// Create or overwrite destination environment directory.
	if err := os.MkdirAll(toPath, 0755); err != nil {
		return fmt.Errorf("creating destination environment: %w", err)
	}

	// Load source metadata to determine domain pattern.
	srcMeta, err := m.loadMetadata(fromPath)
	if err != nil {
		return fmt.Errorf("reading source environment metadata: %w", err)
	}

	// Copy compose file, adjusting the environment name in labels/domains.
	srcCompose := filepath.Join(fromPath, "docker-compose.yml")
	composeData, err := os.ReadFile(srcCompose)
	if err != nil {
		return fmt.Errorf("reading source compose file: %w", err)
	}

	// Derive the target domain by replacing the environment prefix.
	toDomain := replaceEnvPrefix(srcMeta.Domain, fromEnv, toEnv)
	adjusted := strings.ReplaceAll(string(composeData), srcMeta.Domain, toDomain)
	adjusted = strings.ReplaceAll(adjusted, fromEnv+"-"+projectName, toEnv+"-"+projectName)

	if err := os.WriteFile(filepath.Join(toPath, "docker-compose.yml"), []byte(adjusted), 0644); err != nil {
		return fmt.Errorf("writing destination compose file: %w", err)
	}

	// Copy .env if present.
	if data, err := os.ReadFile(filepath.Join(fromPath, ".env")); err == nil {
		envData := strings.ReplaceAll(string(data), srcMeta.Domain, toDomain)
		if err := os.WriteFile(filepath.Join(toPath, ".env"), []byte(envData), 0644); err != nil {
			return fmt.Errorf("writing destination .env file: %w", err)
		}
	}

	// Copy docker images by tagging from source to destination project.
	copyImages(projectName, fromEnv, toEnv)

	// Save metadata for the promoted environment.
	promoted := &Environment{
		Name:        toEnv,
		Domain:      toDomain,
		Branch:      srcMeta.Branch,
		ProjectName: projectName,
		Status:      "creating",
		CreatedAt:   time.Now().UTC(),
	}
	return m.saveMetadata(toPath, promoted)
}

// Delete removes an environment and stops its containers.
func (m *Manager) Delete(projectName, envName string) error {
	envPath := m.GetEnvPath(projectName, envName)

	// Stop containers if compose file exists.
	composePath := filepath.Join(envPath, "docker-compose.yml")
	if _, err := os.Stat(composePath); err == nil {
		cmd := exec.Command("docker", "compose", "down", "--remove-orphans")
		cmd.Dir = envPath
		cmd.CombinedOutput() // best-effort
	}

	if err := os.RemoveAll(envPath); err != nil {
		return fmt.Errorf("removing environment directory: %w", err)
	}
	return nil
}

// GetEnvPath returns the filesystem path for a project's environment.
func (m *Manager) GetEnvPath(projectName, envName string) string {
	return filepath.Join(m.basePath, projectName, "environments", envName)
}

// saveMetadata writes the environment metadata as JSON to the environment
// directory.
func (m *Manager) saveMetadata(envPath string, env *Environment) error {
	data, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(envPath, "environment.json"), data, 0644)
}

// loadMetadata reads environment metadata from the environment directory.
func (m *Manager) loadMetadata(envPath string) (*Environment, error) {
	data, err := os.ReadFile(filepath.Join(envPath, "environment.json"))
	if err != nil {
		return nil, err
	}
	var env Environment
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	return &env, nil
}

// adjustComposeForEnv replaces domain and project-name references in a
// docker-compose.yml to isolate the environment as a separate compose project.
func adjustComposeForEnv(compose, projectName, envName, domain string) string {
	// Replace any Host rule references with the environment domain.
	result := compose

	// Set the compose project name so containers are namespaced.
	projectPrefix := fmt.Sprintf("name: %s-%s\n", envName, projectName)
	if !strings.Contains(result, "name:") {
		result = projectPrefix + result
	}

	// Replace domain references. The source compose typically has the
	// production domain; we swap it with the environment-specific one.
	// This is a best-effort text replacement — templates use Host(`domain`).
	lines := strings.Split(result, "\n")
	for i, line := range lines {
		if strings.Contains(line, "Host(") {
			// Replace the Host rule domain with the environment domain.
			lines[i] = replaceHostRule(line, domain)
		}
	}

	return strings.Join(lines, "\n")
}

// replaceHostRule replaces the domain inside a Traefik Host() rule with the
// given domain.
func replaceHostRule(line, domain string) string {
	start := strings.Index(line, "Host(`")
	if start == -1 {
		return line
	}
	start += len("Host(`")
	end := strings.Index(line[start:], "`)")
	if end == -1 {
		return line
	}
	return line[:start] + domain + line[start+end:]
}

// adjustEnvFile replaces domain references in a .env file.
func adjustEnvFile(envContent, domain string) string {
	lines := strings.Split(envContent, "\n")
	for i, line := range lines {
		if strings.Contains(strings.ToUpper(line), "DOMAIN") && strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			lines[i] = parts[0] + "=" + domain
		}
	}
	return strings.Join(lines, "\n")
}

// replaceEnvPrefix swaps the environment prefix in a domain.
// e.g. "staging.app.example.com" with fromEnv="staging", toEnv="production"
// becomes "production.app.example.com".
func replaceEnvPrefix(domain, fromEnv, toEnv string) string {
	if strings.HasPrefix(domain, fromEnv+".") {
		return toEnv + "." + domain[len(fromEnv)+1:]
	}
	// If the domain doesn't have the expected prefix, prepend the target env.
	return toEnv + "." + domain
}

// copyImages attempts to re-tag docker images from the source environment
// project to the destination environment project. This is best-effort.
func copyImages(projectName, fromEnv, toEnv string) {
	srcProject := fromEnv + "-" + projectName
	dstProject := toEnv + "-" + projectName

	// List images for the source project.
	cmd := exec.Command("docker", "images", "--format", "{{.Repository}}:{{.Tag}}", "--filter", "reference="+srcProject+"*")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return
	}

	for _, img := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if img == "" {
			continue
		}
		newTag := strings.Replace(img, srcProject, dstProject, 1)
		exec.Command("docker", "tag", img, newTag).Run()
	}
}

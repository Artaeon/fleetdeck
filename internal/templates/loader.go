package templates

import (
	"fmt"
	"os"
	"path/filepath"
)

func LoadCustomTemplates(basePath string) error {
	templatesDir := filepath.Join(basePath, "templates")
	entries, err := os.ReadDir(templatesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading templates directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		if _, exists := registry[name]; exists {
			continue // don't override built-in templates
		}

		dir := filepath.Join(templatesDir, name)
		tmpl := &Template{
			Name:        name,
			Description: fmt.Sprintf("Custom template: %s", name),
		}

		if data, err := os.ReadFile(filepath.Join(dir, "Dockerfile")); err == nil {
			tmpl.Dockerfile = string(data)
		}
		if data, err := os.ReadFile(filepath.Join(dir, "docker-compose.yml")); err == nil {
			tmpl.Compose = string(data)
		}
		if data, err := os.ReadFile(filepath.Join(dir, "deploy.yml")); err == nil {
			tmpl.Workflow = string(data)
		} else {
			tmpl.Workflow = SharedWorkflow
		}
		if data, err := os.ReadFile(filepath.Join(dir, ".env.template")); err == nil {
			tmpl.EnvTemplate = string(data)
		}
		if data, err := os.ReadFile(filepath.Join(dir, ".gitignore")); err == nil {
			tmpl.GitIgnore = string(data)
		}

		Register(tmpl)
	}

	return nil
}

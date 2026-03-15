package project

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fleetdeck/fleetdeck/internal/profiles"
	"github.com/fleetdeck/fleetdeck/internal/templates"
)

// ScaffoldFromProfile creates project files using a deployment profile.
// It generates docker-compose.yml from the profile, uses the code template
// for the Dockerfile, and writes the profile's env template.
func ScaffoldFromProfile(projectPath string, profile *profiles.Profile, tmpl *templates.Template, data profiles.ProfileData, tmplData templates.TemplateData) error {
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

	// Render and write Dockerfile from the code template
	dockerfile, err := templates.Render(tmpl.Dockerfile, tmplData)
	if err != nil {
		return fmt.Errorf("rendering Dockerfile: %w", err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, "Dockerfile"), []byte(dockerfile), 0644); err != nil {
		return err
	}

	// Render and write docker-compose.yml from the PROFILE (not code template)
	compose, err := profiles.Render(profile.Compose, data)
	if err != nil {
		return fmt.Errorf("rendering docker-compose.yml from profile: %w", err)
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

	// Write nginx config for static profile
	if profile.Nginx != "" {
		if err := os.MkdirAll(projectPath, 0755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(projectPath, "nginx.conf"), []byte(profile.Nginx), 0644); err != nil {
			return err
		}
	}

	// Create public directory for static profile
	if profile.Name == "static" {
		publicDir := filepath.Join(projectPath, "public")
		if err := os.MkdirAll(publicDir, 0755); err != nil {
			return err
		}
		indexHTML := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>` + data.Name + `</title>
</head>
<body>
    <h1>Welcome to ` + data.Name + `</h1>
    <p>Deployed with FleetDeck.</p>
</body>
</html>
`
		if err := os.WriteFile(filepath.Join(publicDir, "index.html"), []byte(indexHTML), 0644); err != nil {
			return err
		}
	}

	return nil
}

// GenerateEnvFromProfile generates a .env file from a profile's env template.
func GenerateEnvFromProfile(projectPath string, profile *profiles.Profile, data profiles.ProfileData) error {
	content, err := profiles.RenderEnv(profile.EnvTemplate, data)
	if err != nil {
		return fmt.Errorf("rendering profile env template: %w", err)
	}

	// Replace placeholder passwords with generated secrets
	content = replacePasswordPlaceholders(content, data.Name)

	return os.WriteFile(filepath.Join(projectPath, ".env"), []byte(content), 0600)
}

func replacePasswordPlaceholders(content, name string) string {
	// Replace all CHANGEME placeholders with real secrets
	for {
		idx := indexOf(content, "CHANGEME")
		if idx == -1 {
			break
		}
		// Find the full placeholder (everything after = up to newline that contains CHANGEME)
		secret := GenerateSecret(16)
		// Replace just the first occurrence of the pattern
		before := content[:idx]
		after := content[idx+len("CHANGEME"):]
		// Also consume $(openssl rand -hex 16) if present
		if len(after) > 0 && after[0] == '_' {
			// Format: CHANGEME_$(openssl rand -hex 16)
			if shellIdx := indexOf(after, ")"); shellIdx != -1 {
				after = after[shellIdx+1:]
			}
		}
		content = before + secret + after
	}
	return content
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

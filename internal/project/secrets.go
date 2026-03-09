package project

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/fleetdeck/fleetdeck/internal/templates"
)

var validNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$|^[a-z0-9]$`)

// ValidateName checks that a project name is safe for use as a Linux user,
// Docker Compose project, GitHub repo, and filesystem path.
func ValidateName(name string) error {
	if len(name) < 1 || len(name) > 63 {
		return fmt.Errorf("project name must be 1-63 characters, got %d", len(name))
	}
	if !validNameRe.MatchString(name) {
		return fmt.Errorf("project name must contain only lowercase letters, numbers, and hyphens (cannot start/end with hyphen)")
	}
	if strings.Contains(name, "--") {
		return fmt.Errorf("project name cannot contain consecutive hyphens")
	}
	return nil
}

func GenerateSecret(length int) string {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

func GenerateEnvFile(projectPath string, tmpl *templates.Template, data templates.TemplateData) error {
	content, err := templates.Render(tmpl.EnvTemplate, data)
	if err != nil {
		return fmt.Errorf("rendering env template: %w", err)
	}

	// Replace placeholder passwords with generated secrets
	content = strings.ReplaceAll(content, data.Name+"_CHANGEME", GenerateSecret(16))

	return os.WriteFile(projectPath+"/.env", []byte(content), 0600)
}

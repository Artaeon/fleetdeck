package project

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/fleetdeck/fleetdeck/internal/templates"
)

func GenerateSecret(length int) string {
	b := make([]byte, length)
	rand.Read(b)
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

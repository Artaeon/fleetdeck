package sourcedeploy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Native backups address the directory's Compose project. Do not let source
// settings silently select a different project for backup or restoration.
func validateComposeIdentity(dir, file, name string) error {
	data, err := os.ReadFile(filepath.Join(dir, file))
	if err != nil {
		return err
	}
	var c struct {
		Name string `yaml:"name"`
	}
	if yaml.Unmarshal(data, &c) != nil {
		return fmt.Errorf("invalid Compose identity")
	}
	if c.Name != "" && c.Name != name {
		return fmt.Errorf("Compose name must match the linked project name")
	}
	return validateComposeEnvironment(dir)
}

func validateComposeEnvironment(dir string) error {
	// A conservative refusal avoids maintaining a second dotenv parser. These
	// orchestration controls belong in a separately reviewed server setup.
	for _, value := range os.Environ() {
		if strings.HasPrefix(value, "COMPOSE_") {
			return fmt.Errorf("clear COMPOSE_* environment overrides before source deployment")
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, ".env"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if strings.Contains(string(data), "COMPOSE_") {
		return fmt.Errorf("source deployment does not accept COMPOSE_* orchestration overrides in .env")
	}
	return nil
}

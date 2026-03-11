package compose

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateMissingFile(t *testing.T) {
	dir := t.TempDir()
	// No compose file exists — validation should fail.
	err := Validate(dir)
	if err == nil {
		t.Error("expected error for directory without a compose file")
	}
}

func TestValidateInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	// Write an invalid compose file.
	err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("not: [valid: yaml: :::"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	err = Validate(dir)
	if err == nil {
		t.Error("expected error for invalid compose file")
	}
}

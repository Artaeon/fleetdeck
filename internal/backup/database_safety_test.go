package backup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDatabaseDumpFailureReachesBackupCaller(t *testing.T) {
	for _, image := range []string{"postgres:17", "mysql:8", "mariadb:11"} {
		t.Run(image, func(t *testing.T) {
			putFakeDockerOnPath(t, "printf 'partial SQL output'\nexit 23\n")
			projectDir, backupDir := t.TempDir(), t.TempDir()
			os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte("services:\n  db:\n    image: "+image+"\n"), 0600)
			os.WriteFile(filepath.Join(projectDir, ".env"), []byte("MYSQL_DATABASE=fixture\n"), 0600)
			components, err := BackupDatabases(projectDir, backupDir)
			if err == nil || len(components) != 0 || !strings.Contains(err.Error(), "database service db") {
				t.Fatalf("failed dump reported as successful: components=%v error=%v", components, err)
			}
		})
	}
}

func TestInvalidComposeCannotHideDatabaseServices(t *testing.T) {
	projectDir := t.TempDir()
	os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte("services: [invalid"), 0600)
	// Do not silently fall back to another file after failing to parse the first.
	os.WriteFile(filepath.Join(projectDir, "compose.yaml"), []byte("services: {}"), 0600)
	if _, err := BackupDatabases(projectDir, t.TempDir()); err == nil {
		t.Fatal("invalid Compose accepted")
	}
}

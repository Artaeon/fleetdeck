package backup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnvironmentFilesArePrivateAndRestorable(t *testing.T) {
	project, backup, restored := t.TempDir(), t.TempDir(), t.TempDir()
	os.Mkdir(filepath.Join(project, "secrets"), 0700)
	os.WriteFile(filepath.Join(project, "secrets", "database.env"), []byte("POSTGRES_USER=fixture\n"), 0644)
	os.WriteFile(filepath.Join(project, "compose.yaml"), []byte("services:\n  db:\n    env_file:\n      - path: ./secrets/database.env\n      - path: absent.env\n        required: false\n  app:\n    env_file: secrets/database.env\n"), 0644)
	components, err := BackupConfigFiles(project, backup)
	if err != nil {
		t.Fatal(err)
	}
	if len(components) != 2 {
		t.Fatalf("expected deduplicated config and env: %v", components)
	}
	source, _ := os.OpenRoot(backup)
	defer source.Close()
	dest, _ := os.OpenRoot(restored)
	defer dest.Close()
	for _, component := range components {
		if err := restoreConfigFile(source, dest, component); err != nil {
			t.Fatal(err)
		}
	}
	info, err := os.Stat(filepath.Join(restored, "secrets", "database.env"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("environment permissions: %v %v", info, err)
	}
}

func TestUnsafeOrMissingEnvironmentFileFailsBackup(t *testing.T) {
	for _, ref := range []string{"missing.env", "../outside.env", "/etc/passwd", "link.env", "${ENV_PATH}"} {
		t.Run(ref, func(t *testing.T) {
			project := t.TempDir()
			os.Symlink("/etc/passwd", filepath.Join(project, "link.env"))
			os.WriteFile(filepath.Join(project, "compose.yaml"), []byte("services:\n  app:\n    env_file: '"+ref+"'\n"), 0600)
			if _, err := BackupConfigFiles(project, t.TempDir()); err == nil {
				t.Fatal("incomplete or unsafe backup accepted")
			}
		})
	}
}

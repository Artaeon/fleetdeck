package backup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBindRestoreReplacesNestedDirectory(t *testing.T) {
	source, project := t.TempDir(), t.TempDir()
	os.Mkdir(filepath.Join(source, "state"), 0700)
	os.WriteFile(filepath.Join(source, "state", "knowledge.sqlite3"), []byte("snapshot"), 0600)
	archive := filepath.Join(t.TempDir(), "state.tar.gz")
	if err := archiveDirectory(filepath.Join(source, "state"), archive); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(project, "nested", "state")
	os.MkdirAll(target, 0700)
	os.WriteFile(filepath.Join(target, "knowledge.sqlite3-wal"), []byte("stale WAL"), 0600)
	if err := restoreProjectBindMount(archive, project, "nested/state"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(target, "knowledge.sqlite3"))
	if err != nil || string(data) != "snapshot" {
		t.Fatal("snapshot missing", err)
	}
	if _, err := os.Stat(filepath.Join(target, "knowledge.sqlite3-wal")); !os.IsNotExist(err) {
		t.Fatal("stale WAL survived")
	}
}

func TestBindRestoreRejectsLinksAndKeepsOriginal(t *testing.T) {
	source, project := t.TempDir(), t.TempDir()
	os.Mkdir(filepath.Join(source, "state"), 0700)
	os.Symlink("/etc/passwd", filepath.Join(source, "state", "escape"))
	archive := filepath.Join(t.TempDir(), "state.tar.gz")
	if err := archiveDirectory(filepath.Join(source, "state"), archive); err != nil {
		t.Fatal(err)
	}
	os.Mkdir(filepath.Join(project, "state"), 0700)
	marker := filepath.Join(project, "state", "original")
	os.WriteFile(marker, []byte("keep"), 0600)
	if err := restoreProjectBindMount(archive, project, "state"); err == nil {
		t.Fatal("accepted link")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("original lost", err)
	}
}

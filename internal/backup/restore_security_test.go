package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func writeSecurityManifest(t *testing.T, dir string, comps ...ComponentInfo) {
	t.Helper()
	data, err := json.Marshal(Manifest{Version: "1", Components: comps})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestUnsafeManifestRejectedBeforeDocker(t *testing.T) {
	for _, tc := range []struct {
		name string
		comp ComponentInfo
	}{
		{"source traversal", ComponentInfo{Type: "config", Name: "config.yml", Path: "../outside"}},
		{"absolute source", ComponentInfo{Type: "config", Name: "config.yml", Path: "/etc/passwd"}},
		{"target traversal", ComponentInfo{Type: "config", Name: "../outside", Path: "saved"}},
		{"absolute target", ComponentInfo{Type: "config", Name: "/etc/config", Path: "saved"}},
		{"portable traversal", ComponentInfo{Type: "config", Name: `..\outside`, Path: "saved"}},
		{"unknown type", ComponentInfo{Type: "custom", Name: "config", Path: "saved"}},
		{"host bind disguised as named volume", ComponentInfo{Type: "volume", Name: "/etc (named volume)", Path: "volumes/namedvol_data.tar.gz"}},
		{"volume mount options", ComponentInfo{Type: "volume", Name: "data:/host (named volume)", Path: "volumes/namedvol_data.tar.gz"}},
		{"database option", ComponentInfo{Type: "database", Name: "--all (PostgreSQL)", Path: "dump.gz"}},
		{"unknown database", ComponentInfo{Type: "database", Name: "db (Unknown)", Path: "dump.gz"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backupDir, projectDir := t.TempDir(), t.TempDir()
			marker := filepath.Join(t.TempDir(), "docker-called")
			putFakeDockerOnPath(t, "touch "+shellQuote(marker)+"\nexit 0\n")
			os.WriteFile(filepath.Join(backupDir, "saved"), []byte("config"), 0600)
			writeSecurityManifest(t, backupDir, tc.comp)
			if err := RestoreBackup(backupDir, projectDir, RestoreOptions{NoStart: true}); err == nil {
				t.Fatal("unsafe backup accepted")
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatal("Docker invoked before rejecting unsafe backup")
			}
		})
	}
}

func TestVerifyRejectsSourceLinksAndNonRegularFiles(t *testing.T) {
	for _, mode := range []string{"file link", "parent link", "directory", "manifest link"} {
		t.Run(mode, func(t *testing.T) {
			backupDir, outside := t.TempDir(), t.TempDir()
			content := []byte("outside content")
			hash := sha256.Sum256(content)
			os.WriteFile(filepath.Join(outside, "saved"), content, 0600)
			comp := ComponentInfo{Type: "config", Name: "config.yml", Path: "saved", Checksum: hex.EncodeToString(hash[:])}
			switch mode {
			case "file link":
				os.Symlink(filepath.Join(outside, "saved"), filepath.Join(backupDir, "saved"))
			case "parent link":
				os.Symlink(outside, filepath.Join(backupDir, "link"))
				comp.Path = "link/saved"
			case "directory":
				os.Mkdir(filepath.Join(backupDir, "saved"), 0700)
			case "manifest link":
				writeSecurityManifest(t, outside, comp)
				os.Symlink(filepath.Join(outside, "manifest.json"), filepath.Join(backupDir, "manifest.json"))
			}
			if mode != "manifest link" {
				writeSecurityManifest(t, backupDir, comp)
			}
			results, err := VerifyBackup(backupDir)
			if err == nil && !HasFailures(results) {
				t.Fatal("unsafe source accepted")
			}
		})
	}
}

func TestConfigRestoreRejectsDestinationLinksBeforeStopping(t *testing.T) {
	for _, parentLink := range []bool{false, true} {
		t.Run(fmt.Sprint("parent=", parentLink), func(t *testing.T) {
			backupDir, projectDir, outside := t.TempDir(), t.TempDir(), t.TempDir()
			marker := filepath.Join(t.TempDir(), "docker-called")
			putFakeDockerOnPath(t, "touch "+shellQuote(marker)+"\nexit 0\n")
			original := filepath.Join(outside, "config")
			os.WriteFile(original, []byte("keep outside"), 0600)
			os.WriteFile(filepath.Join(projectDir, "first"), []byte("keep project"), 0600)
			content := []byte("restore")
			hash := sha256.Sum256(content)
			os.WriteFile(filepath.Join(backupDir, "saved"), content, 0644)
			first := ComponentInfo{Type: "config", Name: "first", Path: "saved", Checksum: hex.EncodeToString(hash[:])}
			second := first
			second.Name = "linked"
			if parentLink {
				os.Symlink(outside, filepath.Join(projectDir, "linked"))
				second.Name = "linked/config"
			} else {
				os.Symlink(original, filepath.Join(projectDir, "linked"))
			}
			writeSecurityManifest(t, backupDir, first, second)
			if err := RestoreBackup(backupDir, projectDir, RestoreOptions{FilesOnly: true}); err == nil {
				t.Fatal("destination link accepted")
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatal("Docker invoked")
			}
			for path, want := range map[string]string{original: "keep outside", filepath.Join(projectDir, "first"): "keep project"} {
				got, err := os.ReadFile(path)
				if err != nil || string(got) != want {
					t.Fatalf("modified %s: %s (%v)", path, got, err)
				}
			}
		})
	}
}

func TestConfigRestoreReplacesHardlinkWithoutChangingOutsideFile(t *testing.T) {
	backupDir, projectDir, outside := t.TempDir(), t.TempDir(), t.TempDir()
	putFakeDockerOnPath(t, "exit 0\n")
	original := filepath.Join(outside, "config")
	os.WriteFile(original, []byte("outside"), 0600)
	if err := os.Link(original, filepath.Join(projectDir, "nginx.conf")); err != nil {
		t.Fatal(err)
	}
	content := []byte("restore")
	hash := sha256.Sum256(content)
	os.WriteFile(filepath.Join(backupDir, "saved"), content, 0644)
	writeSecurityManifest(t, backupDir, ComponentInfo{Type: "config", Name: "nginx.conf", Path: "saved", Checksum: hex.EncodeToString(hash[:])})
	if err := RestoreBackup(backupDir, projectDir, RestoreOptions{FilesOnly: true, NoStart: true}); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{original: "outside", filepath.Join(projectDir, "nginx.conf"): "restore"} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("unexpected content: %s (%v)", got, err)
		}
	}
	info, _ := os.Stat(filepath.Join(projectDir, "nginx.conf"))
	if info.Mode().Perm() != 0644 {
		t.Fatal("lost read-only container config permissions")
	}
}

func TestConfigCopyRejectsChangedSourceWithoutReplacingTarget(t *testing.T) {
	sourceDir, projectDir := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(sourceDir, "saved"), []byte("changed"), 0600)
	os.WriteFile(filepath.Join(projectDir, "config"), []byte("keep"), 0600)
	source, _ := os.OpenRoot(sourceDir)
	defer source.Close()
	dest, _ := os.OpenRoot(projectDir)
	defer dest.Close()
	hash := sha256.Sum256([]byte("original"))
	if err := restoreConfigFile(source, dest, ComponentInfo{Name: "config", Path: "saved", Checksum: hex.EncodeToString(hash[:])}); err == nil {
		t.Fatal("changed source accepted")
	}
	got, _ := os.ReadFile(filepath.Join(projectDir, "config"))
	entries, _ := os.ReadDir(projectDir)
	if string(got) != "keep" || len(entries) != 1 {
		t.Fatal("target changed or temporary file leaked")
	}
}

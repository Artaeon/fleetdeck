package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

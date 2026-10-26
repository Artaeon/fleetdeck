package backup

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func fakeVolumeModel(t *testing.T, model any, afterConfig string) {
	t.Helper()
	data, err := json.Marshal(model)
	if err != nil {
		t.Fatal(err)
	}
	putFakeDockerOnPath(t, "if [ \"$1 $2 $3 $4\" = 'compose config --format json' ]; then printf '%s' "+shellQuote(string(data))+"; exit 0; fi\n"+afterConfig)
}

func fakeBindMounts(t *testing.T, project string, sources ...string) {
	t.Helper()
	mounts := []composeMount{}
	for _, source := range sources {
		mounts = append(mounts, composeMount{Type: "bind", Source: filepath.Join(project, source), Target: "/data/" + source})
	}
	fakeVolumeModel(t, map[string]any{"services": map[string]any{"app": map[string]any{"volumes": mounts}}}, "exit 1\n")
}

func namedVolumeModel() map[string]any {
	return map[string]any{
		"services": map[string]any{
			"app":    map[string]any{"volumes": []composeMount{{Type: "volume", Source: "data"}}},
			"worker": map[string]any{"volumes": []composeMount{{Type: "volume", Source: "data"}}},
		},
		"volumes": map[string]any{"data": map[string]any{"name": "example_data"}, "unused": map[string]any{"name": "not_created"}},
	}
}

func TestNamedVolumeUsesResolvedNameAndDeduplicates(t *testing.T) {
	project, backup := t.TempDir(), t.TempDir()
	fixture := filepath.Join(t.TempDir(), "archive.tar.gz")
	raw, err := exec.Command("tar", "cf", "-", "-C", t.TempDir(), ".").Output()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixture, raw, 0600); err != nil {
		t.Fatal(err)
	}
	calls := filepath.Join(t.TempDir(), "calls")
	fakeVolumeModel(t, namedVolumeModel(), `if [ "$1 $2" = 'volume inspect' ]; then test "$3" = example_data; exit $?; fi
if [ "$1" = run ]; then
 echo run >> `+shellQuote(calls)+`
 cat `+shellQuote(fixture)+`
 exit $?
fi
exit 99
`)
	components, err := BackupVolumes(project, backup)
	if err != nil {
		t.Fatal(err)
	}
	if len(components) != 1 || components[0].Name != "example_data (named volume)" {
		t.Fatalf("incorrect inventory: %#v", components)
	}
	data, _ := os.ReadFile(calls)
	if string(data) != "run\n" {
		t.Fatalf("shared volume archived more than once: %q", data)
	}
}

func TestNamedVolumeFailureCannotBecomeSuccessfulBackup(t *testing.T) {
	for _, failure := range []string{"inspect", "archive"} {
		t.Run(failure, func(t *testing.T) {
			script := "exit 1\n"
			if failure == "archive" {
				script = "if [ \"$1 $2\" = 'volume inspect' ]; then exit 0; fi\nexit 1\n"
			}
			fakeVolumeModel(t, namedVolumeModel(), script)
			if _, err := BackupVolumes(t.TempDir(), t.TempDir()); err == nil {
				t.Fatal("volume error ignored")
			}
		})
	}
}

func TestVolumeInventoryRejectsUnsafeAndUnsupportedSources(t *testing.T) {
	for _, kind := range []string{"symlink", "outside", "unknown"} {
		t.Run(kind, func(t *testing.T) {
			project := t.TempDir()
			source := t.TempDir()
			mount := composeMount{Type: "bind", Source: source}
			if kind == "symlink" {
				if err := os.Symlink(source, filepath.Join(project, "link")); err != nil {
					t.Fatal(err)
				}
				mount.Source = filepath.Join(project, "link")
			}
			if kind == "unknown" {
				mount.Type = "unsupported"
			}
			fakeVolumeModel(t, map[string]any{"services": map[string]any{"app": map[string]any{"volumes": []composeMount{mount}}}}, "exit 99\n")
			if _, err := BackupVolumes(project, t.TempDir()); err == nil {
				t.Fatal("unsafe or unsupported source accepted")
			}
		})
	}
}

func TestVolumeInventorySkipsOnlyReadOnlyAndTmpfsMounts(t *testing.T) {
	mounts := []composeMount{{Type: "bind", Source: "/missing", ReadOnly: true}, {Type: "volume", Source: "missing", ReadOnly: true}, {Type: "tmpfs", Target: "/tmp"}}
	fakeVolumeModel(t, map[string]any{"services": map[string]any{"app": map[string]any{"volumes": mounts}}}, "exit 99\n")
	components, err := BackupVolumes(t.TempDir(), t.TempDir())
	if err != nil || len(components) != 0 {
		t.Fatalf("unexpected archive: %v %v", components, err)
	}
}

func TestDatabaseDetectionAcceptsLongMountSyntax(t *testing.T) {
	project := t.TempDir()
	os.WriteFile(filepath.Join(project, "compose.yaml"), []byte("services:\n  app:\n    image: example/app\n    volumes:\n      - type: bind\n        source: ./data\n        target: /data\n"), 0600)
	if _, err := BackupDatabases(project, t.TempDir()); err != nil {
		t.Fatal(err)
	}
}

func TestVolumeArchiveFailureRemovesPartialFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "partial.tar.gz")
	if err := writeVolumeArchive(exec.Command("sh", "-c", "printf partial; exit 1"), path); err == nil {
		t.Fatal("partial archive accepted")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("partial archive remains")
	}
}

func TestVolumeArchiveIsPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.tar.gz")
	if err := archiveDirectory(t.TempDir(), path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("archive is not private: %v %v", info, err)
	}
}

func TestVolumeChecksumRejectsAnotherValidArchive(t *testing.T) {
	project, backup := t.TempDir(), t.TempDir()
	os.Mkdir(filepath.Join(project, "data"), 0700)
	os.WriteFile(filepath.Join(project, "data", "proof"), []byte("original"), 0600)
	fakeBindMounts(t, project, "data")
	components, err := BackupVolumes(project, backup)
	if err != nil {
		t.Fatal(err)
	}
	if len(components) != 1 || len(components[0].Checksum) != 64 {
		t.Fatalf("missing checksum: %v", components)
	}
	writeSecurityManifest(t, backup, components...)
	results, err := VerifyBackup(backup)
	if err != nil || HasFailures(results) {
		t.Fatalf("valid backup failed: %v %v", results, err)
	}
	os.WriteFile(filepath.Join(project, "data", "proof"), []byte("changed"), 0600)
	archive := filepath.Join(backup, components[0].Path)
	if err := os.Remove(archive); err != nil {
		t.Fatal(err)
	}
	if err := archiveDirectory(filepath.Join(project, "data"), archive); err != nil {
		t.Fatal(err)
	}
	results, err = VerifyBackup(backup)
	if err != nil || !HasFailures(results) {
		t.Fatalf("replaced valid archive passed: %v %v", results, err)
	}
	// Legacy archives retain gzip verification without a recorded SHA256.
	components[0].Checksum = ""
	writeSecurityManifest(t, backup, components...)
	results, err = VerifyBackup(backup)
	if err != nil || HasFailures(results) {
		t.Fatalf("legacy archive rejected: %v %v", results, err)
	}
}

package backup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// AddSourceFiles adds previous versions of files about to be replaced to an
// existing snapshot, using the same traversal checks and checksums as configs.
// Call before VerifyBackup and before changing the project. Missing paths are
// new source files and have no previous content to snapshot.
func AddSourceFiles(projectPath, backupPath string, names []string) error {
	manifest, err := ReadManifest(backupPath)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(projectPath)
	if err != nil {
		return err
	}
	defer root.Close()
	seen := map[string]bool{}
	for _, c := range manifest.Components {
		if c.Type == "config" {
			seen[c.Name] = true
		}
	}
	for _, name := range names {
		if seen[name] {
			continue
		}
		if info, err := root.Lstat(name); err == nil && info.IsDir() {
			continue
		}
		if err = checkPath(root, name, false, false); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		f, err := root.Open(name)
		if err != nil {
			return err
		}
		dest := filepath.Join("config", name)
		size, checksum, err := copyOpenedFileWithChecksum(f, filepath.Join(backupPath, dest), false)
		f.Close()
		if err != nil {
			return fmt.Errorf("snapshot source file: %w", err)
		}
		manifest.Components = append(manifest.Components, ComponentInfo{Type: "config", Name: name, Path: dest, SizeBytes: size, Checksum: checksum})
		seen[name] = true
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(backupPath, ".source-manifest-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err = temp.Write(data); err == nil {
		err = temp.Sync()
	}
	closeErr := temp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(temp.Name(), filepath.Join(backupPath, "manifest.json"))
}

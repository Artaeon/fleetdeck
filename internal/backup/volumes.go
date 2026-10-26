package backup

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func BackupVolumes(projectPath, backupDir string) ([]ComponentInfo, error) {
	volDir := filepath.Join(backupDir, "volumes")
	if err := os.MkdirAll(volDir, 0700); err != nil {
		return nil, err
	}
	config, err := resolveVolumeConfig(projectPath)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(projectPath)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	absoluteProject, err := filepath.Abs(projectPath)
	if err != nil {
		return nil, err
	}
	var components []ComponentInfo
	seen := make(map[string]bool)
	for _, service := range volumeServiceNames(config) {
		for _, mount := range config.Services[service].Volumes {
			if mount.ReadOnly || mount.Type == "tmpfs" {
				continue
			}
			var component ComponentInfo
			var archiveName, key string
			switch mount.Type {
			case "bind":
				source := mount.Source
				if source == "" {
					return nil, fmt.Errorf("bind mount for %s has no source", service)
				}
				if !filepath.IsAbs(source) {
					source = filepath.Join(absoluteProject, source)
				}
				relative, err := filepath.Rel(absoluteProject, source)
				if err != nil {
					return nil, err
				}
				if err := checkPath(root, relative, true, false); err != nil {
					return nil, fmt.Errorf("writable bind for %s must be an existing real directory inside the project: %w", service, err)
				}
				key = "bind:" + relative
				if seen[key] {
					continue
				}
				digest := sha256.Sum256([]byte(relative))
				archiveName = fmt.Sprintf("bind_%x.tar.gz", digest[:])
				if err := archiveDirectory(source, filepath.Join(volDir, archiveName)); err != nil {
					return nil, err
				}
				component = ComponentInfo{Type: "volume", Name: service + "/" + filepath.Base(relative), RestorePath: relative}
			case "volume":
				volume, ok := config.Volumes[mount.Source]
				if mount.Source == "" || !ok || !dockerObjectName.MatchString(volume.Name) {
					return nil, fmt.Errorf("writable volume for %s has no valid resolved Docker name (anonymous volumes are unsupported)", service)
				}
				key = "volume:" + volume.Name
				if seen[key] {
					continue
				}
				if err := exec.Command("docker", "volume", "inspect", volume.Name).Run(); err != nil {
					return nil, fmt.Errorf("required volume %s is unavailable: %w", volume.Name, err)
				}
				digest := sha256.Sum256([]byte(volume.Name))
				archiveName = fmt.Sprintf("namedvol_%x.tar.gz", digest[:])
				cmd := exec.Command("docker", "run", "--rm", "-v", volume.Name+":/data:ro", "-v", volDir+":/backup", "alpine", "tar", "czf", "/backup/"+archiveName, "-C", "/data", ".")
				if _, err := cmd.CombinedOutput(); err != nil {
					return nil, fmt.Errorf("archiving volume %s: %w", volume.Name, err)
				}
				component = ComponentInfo{Type: "volume", Name: volume.Name + " (named volume)"}
			default:
				return nil, fmt.Errorf("unsupported writable mount type %q for %s", mount.Type, service)
			}
			info, err := os.Stat(filepath.Join(volDir, archiveName))
			if err != nil {
				return nil, fmt.Errorf("checking volume archive: %w", err)
			}
			component.Path = filepath.Join("volumes", archiveName)
			component.SizeBytes = info.Size()
			components = append(components, component)
			seen[key] = true
		}
	}
	return components, nil
}

func archiveDirectory(srcDir, archivePath string) error {
	cmd := exec.Command("tar", "czf", archivePath, "-C", filepath.Dir(srcDir), filepath.Base(srcDir))
	cmd.Env = append(os.Environ(), "COPYFILE_DISABLE=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tar: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func sanitizeName(name string) string {
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, ".", "_")
	name = strings.ReplaceAll(name, " ", "_")
	return name
}

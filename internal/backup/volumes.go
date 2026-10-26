package backup

import (
	"compress/gzip"
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
				cmd := exec.Command("docker", "run", "--rm", "--network", "none", "--read-only",
					"--cap-drop=ALL", "--cap-add=DAC_OVERRIDE", "--security-opt=no-new-privileges:true",
					"--pids-limit=32", "--memory=128m",
					"--mount", "type=volume,src="+volume.Name+",dst=/data,readonly",
					"alpine", "tar", "cf", "-", "-C", "/data", ".")
				if err := writeVolumeArchive(cmd, filepath.Join(volDir, archiveName)); err != nil {
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
			component.Checksum, err = fileSHA256(filepath.Join(volDir, archiveName))
			if err != nil {
				return nil, fmt.Errorf("checksumming volume archive: %w", err)
			}
			components = append(components, component)
			seen[key] = true
		}
	}
	return components, nil
}

func archiveDirectory(srcDir, archivePath string) error {
	cmd := exec.Command("tar", "cf", "-", "-C", filepath.Dir(srcDir), filepath.Base(srcDir))
	cmd.Env = append(os.Environ(), "COPYFILE_DISABLE=1")
	return writeVolumeArchive(cmd, archivePath)
}

// Stream to a private file on the client. The helper container only receives a
// read-only data mount, never a writable host backup directory.
func writeVolumeArchive(cmd *exec.Cmd, path string) error {
	output, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	complete := false
	compressed := gzip.NewWriter(output)
	defer func() {
		compressed.Close()
		output.Close()
		if !complete {
			_ = os.Remove(path)
		}
	}()
	cmd.Stdout = compressed
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("archive command failed: %w", err)
	}
	if err := compressed.Close(); err != nil {
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	complete = true
	return nil
}

func sanitizeName(name string) string {
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, ".", "_")
	name = strings.ReplaceAll(name, " ", "_")
	return name
}

package backup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var skipVolumeDirs = map[string]bool{
	"node_modules": true,
	".git":         true,
	".cache":       true,
	"tmp":          true,
	"__pycache__":  true,
}

func BackupVolumes(projectPath, backupDir string) ([]ComponentInfo, error) {
	volDir := filepath.Join(backupDir, "volumes")
	if err := os.MkdirAll(volDir, 0700); err != nil {
		return nil, err
	}

	compose, err := parseComposeFile(projectPath)
	if err != nil {
		return nil, nil
	}

	var components []ComponentInfo

	for serviceName, svc := range compose.Services {
		for _, vol := range svc.Volumes {
			parts := strings.SplitN(vol, ":", 2)
			if len(parts) < 2 {
				continue
			}
			hostPath := parts[0]

			// Resolve relative paths
			if !filepath.IsAbs(hostPath) {
				hostPath = filepath.Join(projectPath, hostPath)
			}

			// Skip non-existent paths and ephemeral directories
			info, err := os.Stat(hostPath)
			if err != nil || !info.IsDir() {
				continue
			}
			if skipVolumeDirs[filepath.Base(hostPath)] {
				continue
			}

			archiveName := fmt.Sprintf("%s_%s.tar.gz", serviceName, sanitizeName(filepath.Base(hostPath)))
			archivePath := filepath.Join(volDir, archiveName)

			if err := archiveDirectory(hostPath, archivePath); err != nil {
				continue
			}

			archiveInfo, err := os.Stat(archivePath)
			if err != nil {
				continue
			}

			components = append(components, ComponentInfo{
				Type:      "volume",
				Name:      fmt.Sprintf("%s/%s", serviceName, filepath.Base(parts[0])),
				Path:      filepath.Join("volumes", archiveName),
				SizeBytes: archiveInfo.Size(),
			})
		}
	}

	// Also detect Docker named volumes via docker compose
	namedVolComponents, err := backupNamedVolumes(projectPath, volDir)
	if err == nil {
		components = append(components, namedVolComponents...)
	}

	return components, nil
}

func archiveDirectory(srcDir, archivePath string) error {
	cmd := exec.Command("tar", "czf", archivePath, "-C", filepath.Dir(srcDir), filepath.Base(srcDir))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tar: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func backupNamedVolumes(projectPath, volDir string) ([]ComponentInfo, error) {
	// Get named volumes from docker compose config
	cmd := exec.Command("docker", "compose", "config", "--volumes")
	cmd.Dir = projectPath
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var components []ComponentInfo
	for _, volName := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		volName = strings.TrimSpace(volName)
		if volName == "" {
			continue
		}

		// Check if this volume exists as a Docker volume
		checkCmd := exec.Command("docker", "volume", "inspect", volName)
		if err := checkCmd.Run(); err != nil {
			continue
		}

		archiveName := fmt.Sprintf("namedvol_%s.tar.gz", sanitizeName(volName))
		archivePath := filepath.Join(volDir, archiveName)

		// Use a temporary container to archive the volume
		tarCmd := exec.Command("docker", "run", "--rm",
			"-v", volName+":/data:ro",
			"-v", volDir+":/backup",
			"alpine",
			"tar", "czf", "/backup/"+archiveName, "-C", "/data", ".")
		if _, err := tarCmd.CombinedOutput(); err != nil {
			continue
		}

		archiveInfo, err := os.Stat(archivePath)
		if err != nil {
			continue
		}

		components = append(components, ComponentInfo{
			Type:      "volume",
			Name:      volName + " (named volume)",
			Path:      filepath.Join("volumes", archiveName),
			SizeBytes: archiveInfo.Size(),
		})
	}

	return components, nil
}

func sanitizeName(name string) string {
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, ".", "_")
	name = strings.ReplaceAll(name, " ", "_")
	return name
}

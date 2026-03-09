package backup

import (
	"os"
	"path/filepath"
)

var configFilePatterns = []string{
	"docker-compose.yml",
	"docker-compose.yaml",
	"compose.yml",
	"compose.yaml",
	".env",
	"Dockerfile",
	"Dockerfile.*",
	"*.dockerfile",
	"nginx.conf",
	"Caddyfile",
	"Makefile",
}

func BackupConfigFiles(projectPath, backupDir string) ([]ComponentInfo, error) {
	configDir := filepath.Join(backupDir, "config")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, err
	}

	var components []ComponentInfo

	for _, pattern := range configFilePatterns {
		matches, err := filepath.Glob(filepath.Join(projectPath, pattern))
		if err != nil {
			continue
		}
		for _, match := range matches {
			info, err := os.Stat(match)
			if err != nil || info.IsDir() {
				continue
			}

			relName := filepath.Base(match)
			dst := filepath.Join(configDir, relName)
			size, checksum, err := copyFileWithChecksum(match, dst)
			if err != nil {
				continue
			}

			components = append(components, ComponentInfo{
				Type:      "config",
				Name:      relName,
				Path:      filepath.Join("config", relName),
				SizeBytes: size,
				Checksum:  checksum,
			})
		}
	}

	// Also backup .github/workflows/ if present
	workflowDir := filepath.Join(projectPath, ".github", "workflows")
	if info, err := os.Stat(workflowDir); err == nil && info.IsDir() {
		entries, _ := os.ReadDir(workflowDir)
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			src := filepath.Join(workflowDir, entry.Name())
			dstRel := filepath.Join("config", ".github", "workflows", entry.Name())
			dst := filepath.Join(backupDir, dstRel)

			size, checksum, err := copyFileWithChecksum(src, dst)
			if err != nil {
				continue
			}

			components = append(components, ComponentInfo{
				Type:      "config",
				Name:      ".github/workflows/" + entry.Name(),
				Path:      dstRel,
				SizeBytes: size,
				Checksum:  checksum,
			})
		}
	}

	return components, nil
}

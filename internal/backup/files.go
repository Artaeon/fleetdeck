package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var configFilePatterns = []string{
	"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml",
	".env", "Dockerfile", "Dockerfile.*", "*.dockerfile", "nginx.conf", "Caddyfile", "Makefile",
	".github/workflows/*",
}

func BackupConfigFiles(projectPath, backupDir string) ([]ComponentInfo, error) {
	root, err := os.OpenRoot(projectPath)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	files := make(map[string]bool) // true marks a private environment file
	for _, pattern := range configFilePatterns {
		matches, err := filepath.Glob(filepath.Join(projectPath, pattern))
		if err != nil {
			return nil, err
		}
		for _, match := range matches {
			name, err := filepath.Rel(projectPath, match)
			if err != nil {
				return nil, err
			}
			info, err := root.Lstat(name)
			if err != nil {
				return nil, err
			}
			if info.IsDir() {
				continue
			}
			if err := checkPath(root, name, false, false); err != nil {
				return nil, err
			}
			files[name] = name == ".env"
		}
	}
	// Separate env_file references may be nested and have arbitrary names.
	// Refuse unsupported paths rather than produce an incomplete recovery set.
	for _, name := range configFilePatterns[:4] {
		if _, exists := files[name]; !exists {
			continue
		}
		data, err := root.ReadFile(name)
		if err != nil {
			return nil, err
		}
		var config struct {
			Services map[string]struct {
				EnvFile yaml.Node `yaml:"env_file"`
			} `yaml:"services"`
		}
		if yaml.Unmarshal(data, &config) != nil {
			return nil, fmt.Errorf("invalid Compose file %s", name)
		}
		for _, service := range config.Services {
			refs, err := environmentFiles(service.EnvFile)
			if err != nil {
				return nil, err
			}
			for _, ref := range refs {
				if strings.Contains(ref.Path, "$") {
					return nil, fmt.Errorf("env_file paths must be literal project-relative paths for backup")
				}
				path := filepath.Clean(ref.Path)
				if err := checkPath(root, path, false, false); err != nil {
					if !ref.Required && os.IsNotExist(err) {
						continue
					}
					return nil, fmt.Errorf("required environment file: %w", err)
				}
				files[path] = true
			}
		}
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	var components []ComponentInfo
	for _, name := range names {
		in, err := root.Open(name)
		if err != nil {
			return nil, err
		}
		dst := filepath.Join("config", name)
		size, checksum, err := copyOpenedFileWithChecksum(in, filepath.Join(backupDir, dst), files[name])
		in.Close()
		if err != nil {
			return nil, fmt.Errorf("backing up configuration %s: %w", name, err)
		}
		components = append(components, ComponentInfo{Type: "config", Name: name, Path: dst, SizeBytes: size, Checksum: checksum})
	}
	return components, nil
}

type environmentFile struct {
	Path     string `yaml:"path"`
	Required bool   `yaml:"required"`
}

func environmentFiles(node yaml.Node) ([]environmentFile, error) {
	if node.Kind == 0 {
		return nil, nil
	}
	if node.Kind == yaml.SequenceNode {
		var result []environmentFile
		for _, entry := range node.Content {
			refs, err := environmentFiles(*entry)
			if err != nil {
				return nil, err
			}
			result = append(result, refs...)
		}
		return result, nil
	}
	ref := environmentFile{Required: true}
	switch node.Kind {
	case yaml.ScalarNode:
		if node.Tag != "!!str" {
			return nil, fmt.Errorf("invalid Compose env_file")
		}
		ref.Path = node.Value
	case yaml.MappingNode:
		if node.Decode(&ref) != nil {
			return nil, fmt.Errorf("invalid Compose env_file")
		}
	default:
		return nil, fmt.Errorf("invalid Compose env_file")
	}
	if ref.Path == "" {
		return nil, fmt.Errorf("empty Compose env_file path")
	}
	return []environmentFile{ref}, nil
}

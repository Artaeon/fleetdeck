package discover

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type ComposeProject struct {
	Name      string
	Dir       string
	FilePath  string
	Services  []string
	Domain    string
	HasDB     bool
	DBType    string
}

type composeYAML struct {
	Services map[string]struct {
		Image  string      `yaml:"image"`
		Labels interface{} `yaml:"labels"`
	} `yaml:"services"`
}

var composeFileNames = []string{
	"docker-compose.yml",
	"docker-compose.yaml",
	"compose.yml",
	"compose.yaml",
}

var skipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	".cache":       true,
	".local":       true,
	"__pycache__":  true,
	".venv":        true,
	"venv":         true,
	".npm":         true,
	".config":      true,
	"snap":         true,
}

func ScanComposeFiles(searchPaths []string, excludePaths []string) ([]ComposeProject, error) {
	excludeSet := make(map[string]bool)
	for _, p := range excludePaths {
		excludeSet[p] = true
	}
	for k, v := range skipDirs {
		excludeSet[k] = v
	}

	var projects []ComposeProject
	seen := make(map[string]bool)

	for _, searchPath := range searchPaths {
		filepath.WalkDir(searchPath, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // skip permission errors etc.
			}

			if d.IsDir() {
				base := filepath.Base(path)
				if excludeSet[base] {
					return filepath.SkipDir
				}
				// Limit depth: don't recurse more than 4 levels deep
				rel, _ := filepath.Rel(searchPath, path)
				if strings.Count(rel, string(os.PathSeparator)) > 4 {
					return filepath.SkipDir
				}
				return nil
			}

			if !isComposeFile(d.Name()) {
				return nil
			}

			dir := filepath.Dir(path)
			if seen[dir] {
				return nil
			}
			seen[dir] = true

			cp, err := parseComposeProject(path)
			if err != nil {
				return nil
			}
			projects = append(projects, *cp)
			return nil
		})
	}

	return projects, nil
}

func isComposeFile(name string) bool {
	for _, n := range composeFileNames {
		if name == n {
			return true
		}
	}
	return false
}

func parseComposeProject(filePath string) (*ComposeProject, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var cf composeYAML
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return nil, err
	}

	dir := filepath.Dir(filePath)
	cp := &ComposeProject{
		Name:     filepath.Base(dir),
		Dir:      dir,
		FilePath: filePath,
	}

	for svcName, svc := range cf.Services {
		cp.Services = append(cp.Services, svcName)

		// Detect database services
		image := strings.ToLower(svc.Image)
		if strings.HasPrefix(image, "postgres") {
			cp.HasDB = true
			cp.DBType = "postgres"
		} else if strings.HasPrefix(image, "mysql") || strings.HasPrefix(image, "mariadb") {
			cp.HasDB = true
			cp.DBType = "mysql"
		} else if strings.HasPrefix(image, "mongo") {
			cp.HasDB = true
			cp.DBType = "mongo"
		} else if strings.HasPrefix(image, "redis") {
			cp.HasDB = true
			if cp.DBType == "" {
				cp.DBType = "redis"
			}
		}

		// Extract Traefik domain from labels
		domain := extractDomainFromService(svc.Labels)
		if domain != "" {
			cp.Domain = domain
		}
	}

	return cp, nil
}

func extractDomainFromService(labels interface{}) string {
	switch l := labels.(type) {
	case []interface{}:
		var strLabels []string
		for _, item := range l {
			if s, ok := item.(string); ok {
				strLabels = append(strLabels, s)
			}
		}
		return ExtractDomainFromLabelsList(strLabels)
	case map[string]interface{}:
		m := make(map[string]string)
		for k, v := range l {
			if s, ok := v.(string); ok {
				m[k] = s
			}
		}
		return ExtractDomain(m)
	}
	return ""
}

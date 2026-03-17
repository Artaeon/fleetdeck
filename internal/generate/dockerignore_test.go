package generate

import (
	"strings"
	"testing"

	"github.com/fleetdeck/fleetdeck/internal/detect"
)

func TestDockerignore(t *testing.T) {
	tests := []struct {
		name     string
		appType  detect.AppType
		required []string
		excluded []string
	}{
		{
			name:     "nextjs includes node and next patterns",
			appType:  detect.AppTypeNextJS,
			required: []string{"node_modules", ".next", ".git", "*.db", ".env", ".env.local", ".env*.local", "npm-debug.log*", ".DS_Store", "coverage", ".turbo"},
			excluded: []string{"dist"},
		},
		{
			name:     "node includes standard node patterns",
			appType:  detect.AppTypeNode,
			required: []string{"node_modules", ".next", ".git", "*.db", ".env", ".env.local", ".env*.local", "npm-debug.log*", ".DS_Store", "coverage", ".turbo"},
			excluded: []string{"dist"},
		},
		{
			name:     "nestjs includes dist",
			appType:  detect.AppTypeNestJS,
			required: []string{"node_modules", ".next", ".git", "*.db", ".env", ".env.local", ".env*.local", "npm-debug.log*", ".DS_Store", "coverage", ".turbo", "dist"},
		},
		{
			name:     "python includes python-specific patterns",
			appType:  detect.AppTypePython,
			required: []string{"__pycache__", "*.pyc", ".git", ".env", ".env.local", "venv", ".venv", "*.db", ".DS_Store", ".pytest_cache", ".mypy_cache", "htmlcov"},
		},
		{
			name:     "go includes go-specific patterns",
			appType:  detect.AppTypeGo,
			required: []string{".git", "*.db", ".env", ".DS_Store", "tmp", "vendor"},
		},
		{
			name:     "rust includes target directory",
			appType:  detect.AppTypeRust,
			required: []string{".git", "target", "*.db", ".env", ".DS_Store"},
		},
		{
			name:     "static includes minimal patterns",
			appType:  detect.AppTypeStatic,
			required: []string{".git", ".env", ".DS_Store", "node_modules"},
		},
		{
			name:     "unknown includes base patterns",
			appType:  detect.AppTypeUnknown,
			required: []string{".git", ".env", ".DS_Store"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Dockerignore(tt.appType)

			for _, pattern := range tt.required {
				if !containsLine(result, pattern) {
					t.Errorf("expected pattern %q in output for %s, got:\n%s", pattern, tt.appType, result)
				}
			}

			for _, pattern := range tt.excluded {
				if containsLine(result, pattern) {
					t.Errorf("unexpected pattern %q in output for %s, got:\n%s", pattern, tt.appType, result)
				}
			}

			if !strings.HasSuffix(result, "\n") {
				t.Errorf("expected output to end with newline for %s", tt.appType)
			}
		})
	}
}

func containsLine(content, pattern string) bool {
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == pattern {
			return true
		}
	}
	return false
}

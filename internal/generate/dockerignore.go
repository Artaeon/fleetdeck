package generate

import (
	"strings"

	"github.com/fleetdeck/fleetdeck/internal/detect"
)

// Dockerignore returns framework-specific .dockerignore content based on the
// detected application type.
func Dockerignore(appType detect.AppType) string {
	var patterns []string

	switch appType {
	case detect.AppTypeNextJS:
		patterns = []string{
			"node_modules",
			".next",
			".git",
			"*.db",
			".env",
			".env.local",
			".env*.local",
			"npm-debug.log*",
			".DS_Store",
			"coverage",
			".turbo",
		}
	case detect.AppTypeNode:
		patterns = []string{
			"node_modules",
			".next",
			".git",
			"*.db",
			".env",
			".env.local",
			".env*.local",
			"npm-debug.log*",
			".DS_Store",
			"coverage",
			".turbo",
		}
	case detect.AppTypeNestJS:
		patterns = []string{
			"node_modules",
			".next",
			".git",
			"*.db",
			".env",
			".env.local",
			".env*.local",
			"npm-debug.log*",
			".DS_Store",
			"coverage",
			".turbo",
			"dist",
		}
	case detect.AppTypePython:
		patterns = []string{
			"__pycache__",
			"*.pyc",
			".git",
			".env",
			".env.local",
			"venv",
			".venv",
			"*.db",
			".DS_Store",
			".pytest_cache",
			".mypy_cache",
			"htmlcov",
		}
	case detect.AppTypeGo:
		patterns = []string{
			".git",
			"*.db",
			".env",
			".DS_Store",
			"tmp",
			"vendor",
		}
	case detect.AppTypeRust:
		patterns = []string{
			".git",
			"target",
			"*.db",
			".env",
			".DS_Store",
		}
	case detect.AppTypeStatic:
		patterns = []string{
			".git",
			".env",
			".DS_Store",
			"node_modules",
		}
	default:
		patterns = []string{
			".git",
			".env",
			".DS_Store",
		}
	}

	return strings.Join(patterns, "\n") + "\n"
}

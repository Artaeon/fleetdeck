package detect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// AppType represents a detected application type.
type AppType string

const (
	AppTypeNode    AppType = "node"
	AppTypeNextJS  AppType = "nextjs"
	AppTypeNestJS  AppType = "nestjs"
	AppTypePython  AppType = "python"
	AppTypeGo      AppType = "go"
	AppTypeStatic  AppType = "static"
	AppTypeRust    AppType = "rust"
	AppTypeUnknown AppType = "unknown"
)

// Result holds the full detection analysis for a project directory.
type Result struct {
	AppType     AppType  `json:"app_type"`
	Framework   string   `json:"framework,omitempty"`
	Language    string   `json:"language"`
	HasDB       bool     `json:"has_db"`
	HasRedis    bool     `json:"has_redis"`
	HasDocker   bool     `json:"has_docker"`
	EntryPoint  string   `json:"entry_point,omitempty"`
	Port        int      `json:"port,omitempty"`
	Profile     string   `json:"recommended_profile"`
	Confidence  float64  `json:"confidence"`
	Indicators  []string `json:"indicators"`
	Warnings    []string `json:"warnings,omitempty"`
}

// Detect analyzes a directory and returns detection results.
func Detect(dir string) (*Result, error) {
	r := &Result{
		AppType:    AppTypeUnknown,
		Language:   "unknown",
		Confidence: 0,
	}

	// Check for existing Docker setup
	if fileExists(filepath.Join(dir, "Dockerfile")) || fileExists(filepath.Join(dir, "docker-compose.yml")) {
		r.HasDocker = true
		r.Indicators = append(r.Indicators, "has existing Docker configuration")
	}

	// Detect by language/framework (order matters - more specific first)
	detectors := []func(string, *Result){
		detectNextJS,
		detectNestJS,
		detectNode,
		detectPython,
		detectGo,
		detectRust,
		detectStatic,
	}

	for _, detect := range detectors {
		detect(dir, r)
		if r.AppType != AppTypeUnknown {
			break
		}
	}

	// Detect services
	detectServices(dir, r)

	// Recommend profile
	r.Profile = recommendProfile(r)

	return r, nil
}

func detectNextJS(dir string, r *Result) {
	pkgJSON := readPackageJSON(dir)
	if pkgJSON == nil {
		return
	}

	deps := mergeMaps(pkgJSON.Dependencies, pkgJSON.DevDependencies)
	if _, ok := deps["next"]; !ok {
		return
	}

	r.AppType = AppTypeNextJS
	r.Language = "javascript"
	r.Framework = "Next.js"
	r.Port = 3000
	r.Confidence = 0.95
	r.Indicators = append(r.Indicators, "found next in package.json dependencies")

	if fileExists(filepath.Join(dir, "app")) || fileExists(filepath.Join(dir, "src", "app")) {
		r.Framework = "Next.js (App Router)"
		r.Indicators = append(r.Indicators, "using App Router")
	} else if fileExists(filepath.Join(dir, "pages")) || fileExists(filepath.Join(dir, "src", "pages")) {
		r.Framework = "Next.js (Pages Router)"
		r.Indicators = append(r.Indicators, "using Pages Router")
	}

	if _, ok := deps["typescript"]; ok {
		r.Language = "typescript"
		r.Indicators = append(r.Indicators, "TypeScript project")
	}

	// Check for standalone output configuration
	standaloneFound := false
	for _, configName := range []string{"next.config.js", "next.config.ts", "next.config.mjs"} {
		data, err := os.ReadFile(filepath.Join(dir, configName))
		if err != nil {
			continue
		}
		content := string(data)
		standaloneRe := regexp.MustCompile(`output\s*[:=]\s*["']standalone["']`)
		if standaloneRe.MatchString(content) {
			standaloneFound = true
			r.Indicators = append(r.Indicators, "standalone output configured")
			break
		}
	}
	if !standaloneFound {
		r.Warnings = append(r.Warnings, "Next.js standalone output not detected; add output: \"standalone\" to next.config for optimized Docker builds")
	}

	// Check for Prisma usage
	if _, ok := deps["@prisma/client"]; ok {
		r.HasDB = true
		r.Indicators = append(r.Indicators, "uses Prisma (database)")
	}
}

func detectNestJS(dir string, r *Result) {
	pkgJSON := readPackageJSON(dir)
	if pkgJSON == nil {
		return
	}

	deps := mergeMaps(pkgJSON.Dependencies, pkgJSON.DevDependencies)
	if _, ok := deps["@nestjs/core"]; !ok {
		return
	}

	r.AppType = AppTypeNestJS
	r.Language = "typescript"
	r.Framework = "NestJS"
	r.Port = 3000
	r.Confidence = 0.95
	r.Indicators = append(r.Indicators, "found @nestjs/core in package.json dependencies")

	if _, ok := deps["@nestjs/typeorm"]; ok {
		r.HasDB = true
		r.Indicators = append(r.Indicators, "uses TypeORM (database)")
	}
	if _, ok := deps["@prisma/client"]; ok {
		r.HasDB = true
		r.Indicators = append(r.Indicators, "uses Prisma (database)")
	}
}

func detectNode(dir string, r *Result) {
	pkgJSON := readPackageJSON(dir)
	if pkgJSON == nil {
		return
	}

	r.AppType = AppTypeNode
	r.Language = "javascript"
	r.Port = 3000
	r.Confidence = 0.85
	r.Indicators = append(r.Indicators, "found package.json")

	deps := mergeMaps(pkgJSON.Dependencies, pkgJSON.DevDependencies)

	if _, ok := deps["express"]; ok {
		r.Framework = "Express"
		r.Confidence = 0.90
		r.Indicators = append(r.Indicators, "uses Express framework")
	} else if _, ok := deps["fastify"]; ok {
		r.Framework = "Fastify"
		r.Confidence = 0.90
		r.Indicators = append(r.Indicators, "uses Fastify framework")
	} else if _, ok := deps["koa"]; ok {
		r.Framework = "Koa"
		r.Confidence = 0.90
		r.Indicators = append(r.Indicators, "uses Koa framework")
	}

	if _, ok := deps["typescript"]; ok {
		r.Language = "typescript"
		r.Indicators = append(r.Indicators, "TypeScript project")
	}

	if pkgJSON.Main != "" {
		r.EntryPoint = pkgJSON.Main
	}
}

func detectPython(dir string, r *Result) {
	hasPyProject := fileExists(filepath.Join(dir, "pyproject.toml"))
	hasRequirements := fileExists(filepath.Join(dir, "requirements.txt"))
	hasPipfile := fileExists(filepath.Join(dir, "Pipfile"))
	hasSetupPy := fileExists(filepath.Join(dir, "setup.py"))

	if !hasPyProject && !hasRequirements && !hasPipfile && !hasSetupPy {
		return
	}

	r.AppType = AppTypePython
	r.Language = "python"
	r.Port = 8000
	r.Confidence = 0.80
	r.Indicators = append(r.Indicators, "found Python project files")

	// Try to detect framework from requirements
	reqContent := ""
	if hasRequirements {
		if data, err := os.ReadFile(filepath.Join(dir, "requirements.txt")); err == nil {
			reqContent = string(data)
		}
	}

	if strings.Contains(reqContent, "fastapi") || fileExists(filepath.Join(dir, "main.py")) {
		r.Framework = "FastAPI"
		r.Confidence = 0.90
		r.Indicators = append(r.Indicators, "uses FastAPI")
	} else if strings.Contains(reqContent, "django") || fileExists(filepath.Join(dir, "manage.py")) {
		r.Framework = "Django"
		r.Port = 8000
		r.Confidence = 0.90
		r.Indicators = append(r.Indicators, "uses Django")
	} else if strings.Contains(reqContent, "flask") {
		r.Framework = "Flask"
		r.Port = 5000
		r.Confidence = 0.90
		r.Indicators = append(r.Indicators, "uses Flask")
	}

	if strings.Contains(reqContent, "sqlalchemy") || strings.Contains(reqContent, "psycopg") {
		r.HasDB = true
		r.Indicators = append(r.Indicators, "uses database libraries")
	}
}

func detectGo(dir string, r *Result) {
	if !fileExists(filepath.Join(dir, "go.mod")) {
		return
	}

	r.AppType = AppTypeGo
	r.Language = "go"
	r.Port = 8080
	r.Confidence = 0.90
	r.Indicators = append(r.Indicators, "found go.mod")

	if data, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil {
		content := string(data)
		if strings.Contains(content, "github.com/gin-gonic/gin") {
			r.Framework = "Gin"
			r.Indicators = append(r.Indicators, "uses Gin framework")
		} else if strings.Contains(content, "github.com/labstack/echo") {
			r.Framework = "Echo"
			r.Indicators = append(r.Indicators, "uses Echo framework")
		} else if strings.Contains(content, "github.com/gofiber/fiber") {
			r.Framework = "Fiber"
			r.Indicators = append(r.Indicators, "uses Fiber framework")
		}

		if strings.Contains(content, "gorm.io") || strings.Contains(content, "sqlx") || strings.Contains(content, "pgx") {
			r.HasDB = true
			r.Indicators = append(r.Indicators, "uses database libraries")
		}
	}

	if fileExists(filepath.Join(dir, "main.go")) {
		r.EntryPoint = "main.go"
	} else if fileExists(filepath.Join(dir, "cmd")) {
		r.EntryPoint = "cmd/"
		r.Indicators = append(r.Indicators, "uses cmd/ layout")
	}
}

func detectRust(dir string, r *Result) {
	if !fileExists(filepath.Join(dir, "Cargo.toml")) {
		return
	}

	r.AppType = AppTypeRust
	r.Language = "rust"
	r.Port = 8080
	r.Confidence = 0.90
	r.Indicators = append(r.Indicators, "found Cargo.toml")

	if data, err := os.ReadFile(filepath.Join(dir, "Cargo.toml")); err == nil {
		content := string(data)
		if strings.Contains(content, "actix-web") {
			r.Framework = "Actix Web"
			r.Indicators = append(r.Indicators, "uses Actix Web framework")
		} else if strings.Contains(content, "axum") {
			r.Framework = "Axum"
			r.Indicators = append(r.Indicators, "uses Axum framework")
		} else if strings.Contains(content, "rocket") {
			r.Framework = "Rocket"
			r.Indicators = append(r.Indicators, "uses Rocket framework")
		}

		if strings.Contains(content, "diesel") || strings.Contains(content, "sqlx") || strings.Contains(content, "sea-orm") {
			r.HasDB = true
			r.Indicators = append(r.Indicators, "uses database libraries")
		}
	}
}

func detectStatic(dir string, r *Result) {
	hasIndex := fileExists(filepath.Join(dir, "index.html")) ||
		fileExists(filepath.Join(dir, "public", "index.html")) ||
		fileExists(filepath.Join(dir, "dist", "index.html")) ||
		fileExists(filepath.Join(dir, "build", "index.html"))

	if !hasIndex {
		return
	}

	r.AppType = AppTypeStatic
	r.Language = "html"
	r.Port = 80
	r.Confidence = 0.70
	r.Indicators = append(r.Indicators, "found index.html")
}

func detectServices(dir string, r *Result) {
	// Check for Redis usage
	patterns := []string{"redis", "ioredis", "bull", "bullmq"}
	if matchesAnyInProject(dir, patterns) {
		r.HasRedis = true
		r.Indicators = append(r.Indicators, "uses Redis")
	}

	// Check for database from existing compose file
	if data, err := os.ReadFile(filepath.Join(dir, "docker-compose.yml")); err == nil {
		content := string(data)
		if strings.Contains(content, "postgres") || strings.Contains(content, "mysql") || strings.Contains(content, "mariadb") {
			r.HasDB = true
		}
		if strings.Contains(content, "redis") {
			r.HasRedis = true
		}
	}
}

func recommendProfile(r *Result) string {
	if r.AppType == AppTypeStatic {
		return "static"
	}

	if r.HasDB && r.HasRedis {
		return "saas"
	}

	if r.HasDB {
		return "server"
	}

	return "bare"
}

// Helper types and functions

type packageJSON struct {
	Name            string            `json:"name"`
	Main            string            `json:"main"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
	Scripts         map[string]string `json:"scripts"`
}

func readPackageJSON(dir string) *packageJSON {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return nil
	}
	var pkg packageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil
	}
	return &pkg
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func mergeMaps(a, b map[string]string) map[string]string {
	result := make(map[string]string)
	for k, v := range a {
		result[k] = v
	}
	for k, v := range b {
		result[k] = v
	}
	return result
}

func matchesAnyInProject(dir string, patterns []string) bool {
	// Check package.json dependencies
	if pkg := readPackageJSON(dir); pkg != nil {
		deps := mergeMaps(pkg.Dependencies, pkg.DevDependencies)
		for _, p := range patterns {
			if _, ok := deps[p]; ok {
				return true
			}
		}
	}

	// Check requirements.txt
	if data, err := os.ReadFile(filepath.Join(dir, "requirements.txt")); err == nil {
		content := strings.ToLower(string(data))
		for _, p := range patterns {
			if strings.Contains(content, p) {
				return true
			}
		}
	}

	// Check go.mod
	if data, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil {
		content := strings.ToLower(string(data))
		for _, p := range patterns {
			if strings.Contains(content, p) {
				return true
			}
		}
	}

	return false
}

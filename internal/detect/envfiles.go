package detect

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EnvFileRequirement describes an env file that a docker-compose.yml expects.
type EnvFileRequirement struct {
	// Required is the filename referenced in docker-compose.yml (e.g. ".env.production").
	Required string
	// Exists is true if the file already exists in the project directory.
	Exists bool
	// Example is the path to an example file that can be used as a template
	// (e.g. ".env.example" or ".env.production.example").
	Example string
}

// DetectEnvFiles scans a project directory for docker-compose.yml env_file
// references and checks whether those files exist. If an example file is
// found, it is recorded so callers can generate a production file from it.
func DetectEnvFiles(dir string) ([]EnvFileRequirement, error) {
	composePaths := []string{
		filepath.Join(dir, "docker-compose.yml"),
		filepath.Join(dir, "docker-compose.yaml"),
		filepath.Join(dir, "compose.yml"),
		filepath.Join(dir, "compose.yaml"),
	}

	var composePath string
	for _, p := range composePaths {
		if _, err := os.Stat(p); err == nil {
			composePath = p
			break
		}
	}
	if composePath == "" {
		return nil, nil // no compose file, nothing to check
	}

	data, err := os.ReadFile(composePath)
	if err != nil {
		return nil, fmt.Errorf("reading compose file: %w", err)
	}

	// Extract env_file references from compose content.
	requiredFiles := parseEnvFileRefs(string(data))

	var results []EnvFileRequirement
	seen := make(map[string]bool)

	for _, ref := range requiredFiles {
		if seen[ref] {
			continue
		}
		seen[ref] = true

		req := EnvFileRequirement{Required: ref}
		fullPath := filepath.Join(dir, ref)

		if _, err := os.Stat(fullPath); err == nil {
			req.Exists = true
		}

		// Look for example files that could serve as a template.
		candidates := []string{
			fullPath + ".example",
			fullPath + ".sample",
			fullPath + ".template",
			filepath.Join(dir, ".env.example"),
			filepath.Join(dir, ".env.sample"),
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				req.Example = c
				break
			}
		}

		results = append(results, req)
	}

	return results, nil
}

// GenerateEnvFromExample reads an example env file and generates a production
// version by replacing placeholder values with generated secrets.
func GenerateEnvFromExample(examplePath, outputPath string) error {
	f, err := os.Open(examplePath)
	if err != nil {
		return fmt.Errorf("opening example env file: %w", err)
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, processEnvLine(line))
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading example env file: %w", err)
	}

	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(outputPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("writing env file: %w", err)
	}

	return nil
}

// processEnvLine handles a single line from an example env file.
// It replaces common placeholder patterns with generated values.
func processEnvLine(line string) string {
	trimmed := strings.TrimSpace(line)

	// Keep comments and empty lines as-is.
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return line
	}

	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return line
	}

	key := parts[0]
	value := strings.TrimSpace(parts[1])
	value = strings.Trim(value, "\"'")

	keyUpper := strings.ToUpper(key)

	// Generate secrets for known secret-like keys.
	if isSecretKey(keyUpper) && isPlaceholderValue(value) {
		return key + "=" + generateSecret()
	}

	// Keep the original value for non-secret keys.
	return line
}

// isSecretKey returns true if the key name suggests it holds a secret.
func isSecretKey(key string) bool {
	secretPatterns := []string{
		"SECRET", "PASSWORD", "KEY", "TOKEN",
	}
	for _, p := range secretPatterns {
		if strings.Contains(key, p) {
			return true
		}
	}
	return false
}

// isPlaceholderValue returns true if the value looks like a placeholder.
func isPlaceholderValue(value string) bool {
	if value == "" {
		return true
	}
	lower := strings.ToLower(value)
	placeholders := []string{
		"your-", "changeme", "placeholder", "xxx", "...",
		"replace", "generate", "todo", "fixme", "secret-here",
		"sk_test_", "pk_test_", "whsec_",
	}
	for _, p := range placeholders {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

func generateSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(b)
}

// parseEnvFileRefs extracts env_file references from a docker-compose.yml.
// Handles both single-value and list formats:
//
//	env_file: .env.production
//	env_file:
//	  - .env
//	  - .env.production
func parseEnvFileRefs(content string) []string {
	var refs []string
	lines := strings.Split(content, "\n")

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if !strings.HasPrefix(trimmed, "env_file:") {
			continue
		}

		// Single-value format: env_file: .env.production
		after := strings.TrimPrefix(trimmed, "env_file:")
		after = strings.TrimSpace(after)
		if after != "" {
			refs = append(refs, strings.Trim(after, "\"'"))
			continue
		}

		// List format: look at subsequent indented lines starting with "-"
		for j := i + 1; j < len(lines); j++ {
			nextTrimmed := strings.TrimSpace(lines[j])
			if nextTrimmed == "" {
				continue
			}
			if !strings.HasPrefix(nextTrimmed, "-") {
				break
			}
			val := strings.TrimPrefix(nextTrimmed, "-")
			val = strings.TrimSpace(val)
			val = strings.Trim(val, "\"'")
			if val != "" {
				refs = append(refs, val)
			}
		}
	}

	return refs
}

package detect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectEnvFilesSimple(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(`services:
  app:
    env_file: .env.production
`), 0644)

	reqs, err := DetectEnvFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(reqs) != 1 {
		t.Fatalf("expected 1 requirement, got %d", len(reqs))
	}
	if reqs[0].Required != ".env.production" {
		t.Errorf("required = %q, want .env.production", reqs[0].Required)
	}
	if reqs[0].Exists {
		t.Error("file should not exist")
	}
}

func TestDetectEnvFilesWithExample(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(`services:
  app:
    env_file: .env.production
`), 0644)
	os.WriteFile(filepath.Join(dir, ".env.production.example"), []byte("SECRET=placeholder\n"), 0644)

	reqs, err := DetectEnvFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if reqs[0].Example == "" {
		t.Error("should find .env.production.example")
	}
}

func TestDetectEnvFilesAlreadyExists(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(`services:
  app:
    env_file: .env
`), 0644)
	os.WriteFile(filepath.Join(dir, ".env"), []byte("KEY=value\n"), 0644)

	reqs, err := DetectEnvFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reqs[0].Exists {
		t.Error("file should exist")
	}
}

func TestDetectEnvFilesListFormat(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(`services:
  app:
    env_file:
      - .env
      - .env.production
`), 0644)

	reqs, err := DetectEnvFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requirements, got %d", len(reqs))
	}
}

func TestDetectEnvFilesNoCompose(t *testing.T) {
	dir := t.TempDir()
	reqs, err := DetectEnvFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if reqs != nil {
		t.Error("expected nil for no compose file")
	}
}

func TestDetectEnvFilesFallbackToGenericExample(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(`services:
  app:
    env_file: .env.production
`), 0644)
	os.WriteFile(filepath.Join(dir, ".env.example"), []byte("DB_URL=placeholder\n"), 0644)

	reqs, err := DetectEnvFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if reqs[0].Example == "" {
		t.Error("should fall back to .env.example")
	}
}

func TestGenerateEnvFromExample(t *testing.T) {
	dir := t.TempDir()
	example := filepath.Join(dir, ".env.example")
	output := filepath.Join(dir, ".env.production")

	os.WriteFile(example, []byte(`# Database
DATABASE_URL="file:./dev.db"

# Auth
NEXTAUTH_SECRET="your-secret-here-generate-with-openssl"
API_KEY="sk_test_placeholder"

# Normal config
PORT=3000
NODE_ENV=production
`), 0644)

	if err := GenerateEnvFromExample(example, output); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	// Secrets should be replaced
	if strings.Contains(content, "your-secret-here") {
		t.Error("NEXTAUTH_SECRET placeholder should be replaced")
	}
	if strings.Contains(content, "sk_test_placeholder") {
		t.Error("API_KEY placeholder should be replaced")
	}

	// Non-secrets should be preserved
	if !strings.Contains(content, "PORT=3000") {
		t.Error("PORT should be preserved")
	}
	if !strings.Contains(content, "NODE_ENV=production") {
		t.Error("NODE_ENV should be preserved")
	}
	if !strings.Contains(content, "DATABASE_URL") {
		t.Error("DATABASE_URL should be preserved")
	}

	// Comments should be preserved
	if !strings.Contains(content, "# Database") {
		t.Error("comments should be preserved")
	}

	// File permissions should be 0600
	info, _ := os.Stat(output)
	if info.Mode().Perm() != 0600 {
		t.Errorf("permissions = %o, want 0600", info.Mode().Perm())
	}
}

func TestGenerateEnvSecretsAreUnique(t *testing.T) {
	dir := t.TempDir()
	example := filepath.Join(dir, ".env.example")
	os.WriteFile(example, []byte(`SECRET_A="changeme"
SECRET_B="changeme"
SECRET_C="changeme"
`), 0644)

	out1 := filepath.Join(dir, "out1")
	if err := GenerateEnvFromExample(example, out1); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(out1)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")

	secrets := make(map[string]bool)
	for _, line := range lines {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			secrets[parts[1]] = true
		}
	}
	if len(secrets) < 3 {
		t.Error("each secret should be unique")
	}
}

func TestIsSecretKey(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"NEXTAUTH_SECRET", true},
		{"DATABASE_PASSWORD", true},
		{"API_KEY", true},
		{"STRIPE_SECRET_KEY", true},
		{"ACCESS_TOKEN", true},
		{"PORT", false},
		{"NODE_ENV", false},
		{"DATABASE_URL", false},
		{"HOSTNAME", false},
	}
	for _, tt := range tests {
		if got := isSecretKey(tt.key); got != tt.want {
			t.Errorf("isSecretKey(%q) = %v, want %v", tt.key, got, tt.want)
		}
	}
}

func TestIsPlaceholderValue(t *testing.T) {
	tests := []struct {
		val  string
		want bool
	}{
		{"your-secret-here", true},
		{"changeme", true},
		{"placeholder", true},
		{"sk_test_xxx", true},
		{"...", true},
		{"", true},
		{"actual-production-value-abc123", false},
		{"3000", false},
		{"production", false},
	}
	for _, tt := range tests {
		if got := isPlaceholderValue(tt.val); got != tt.want {
			t.Errorf("isPlaceholderValue(%q) = %v, want %v", tt.val, got, tt.want)
		}
	}
}

func TestParseEnvFileRefsDeduplicated(t *testing.T) {
	content := `services:
  app:
    env_file: .env.production
  worker:
    env_file: .env.production
`
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(content), 0644)

	reqs, _ := DetectEnvFiles(dir)
	if len(reqs) != 1 {
		t.Errorf("expected 1 deduplicated requirement, got %d", len(reqs))
	}
}

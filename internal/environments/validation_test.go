package environments

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateEnvName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		// --- Valid names ---
		{name: "simple staging", input: "staging", wantErr: false},
		{name: "production", input: "production", wantErr: false},
		{name: "preview", input: "preview", wantErr: false},
		{name: "single char", input: "a", wantErr: false},
		{name: "two chars", input: "ab", wantErr: false},
		{name: "hyphenated with digits", input: "my-env-1", wantErr: false},
		{name: "all digits", input: "123", wantErr: false},
		{name: "digit-letter mix", input: "1a2b", wantErr: false},
		{name: "max length 63", input: strings.Repeat("a", 63), wantErr: false},
		{name: "preview with id", input: "preview-42", wantErr: false},

		// --- Empty / too long ---
		{name: "empty string", input: "", wantErr: true},
		{name: "over 63 chars", input: strings.Repeat("a", 64), wantErr: true},
		{name: "way over 63 chars", input: strings.Repeat("x", 200), wantErr: true},

		// --- Path traversal attacks ---
		{name: "parent dir traversal", input: "../escape", wantErr: true},
		{name: "deep traversal", input: "../../etc/passwd", wantErr: true},
		{name: "dot-dot only", input: "..", wantErr: true},
		{name: "single dot", input: ".", wantErr: true},
		{name: "slash in name", input: "env/name", wantErr: true},
		{name: "backslash in name", input: "env\\name", wantErr: true},
		{name: "absolute path", input: "/etc/passwd", wantErr: true},

		// --- Shell injection attempts ---
		{name: "space in name", input: "env name", wantErr: true},
		{name: "semicolon injection", input: "env;cmd", wantErr: true},
		{name: "command substitution dollar", input: "env$(cmd)", wantErr: true},
		{name: "command substitution backtick", input: "env`cmd`", wantErr: true},
		{name: "pipe injection", input: "env|cmd", wantErr: true},
		{name: "ampersand injection", input: "env&&cmd", wantErr: true},
		{name: "redirect injection", input: "env>file", wantErr: true},
		{name: "dollar sign alone", input: "env$var", wantErr: true},
		{name: "backtick alone", input: "`id`", wantErr: true},

		// --- Hidden/special files ---
		{name: "hidden directory", input: ".hidden", wantErr: true},
		{name: "dotfile", input: ".env", wantErr: true},

		// --- Hyphen edge cases ---
		{name: "leading hyphen", input: "-leading", wantErr: true},
		{name: "trailing hyphen", input: "trailing-", wantErr: true},
		{name: "only hyphens", input: "---", wantErr: true},
		{name: "single hyphen", input: "-", wantErr: true},

		// --- Uppercase ---
		{name: "uppercase letters", input: "ENV", wantErr: true},
		{name: "mixed case", input: "MyEnv", wantErr: true},
		{name: "camelCase", input: "myEnv", wantErr: true},

		// --- Double dots ---
		{name: "double dots embedded", input: "a..b", wantErr: true},

		// --- Underscore (not in allowed set) ---
		{name: "underscore", input: "my_env", wantErr: true},

		// --- Whitespace variants ---
		{name: "tab in name", input: "env\tname", wantErr: true},
		{name: "newline in name", input: "env\nname", wantErr: true},
		{name: "carriage return", input: "env\rname", wantErr: true},

		// --- Null byte ---
		{name: "null byte", input: "env\x00name", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEnvName(tt.input)
			if tt.wantErr && err == nil {
				t.Errorf("validateEnvName(%q) = nil, want error", tt.input)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("validateEnvName(%q) = %v, want nil", tt.input, err)
			}
		})
	}
}

func TestValidateEnvNameErrorMessages(t *testing.T) {
	t.Run("empty name mentions empty", func(t *testing.T) {
		err := validateEnvName("")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "empty") {
			t.Errorf("error should mention 'empty', got: %v", err)
		}
	})

	t.Run("too long mentions length", func(t *testing.T) {
		err := validateEnvName(strings.Repeat("a", 100))
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "1-63") {
			t.Errorf("error should mention '1-63' character limit, got: %v", err)
		}
	})

	t.Run("invalid chars mentions allowed pattern", func(t *testing.T) {
		err := validateEnvName("UPPER")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "lowercase") {
			t.Errorf("error should mention 'lowercase', got: %v", err)
		}
	})
}

// setupProjectForValidation creates a minimal project structure needed for
// Manager operations. It returns the Manager and the temporary base directory.
func setupProjectForValidation(t *testing.T, projectName string) (*Manager, string) {
	t.Helper()

	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, projectName)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("creating project dir: %v", err)
	}

	composeContent := "services:\n  app:\n    image: myapp:latest\n"
	if err := os.WriteFile(filepath.Join(projectDir, "docker-compose.yml"), []byte(composeContent), 0644); err != nil {
		t.Fatalf("writing docker-compose.yml: %v", err)
	}

	return NewManager(tmpDir), tmpDir
}

func TestCreateRejectsInvalidEnvName(t *testing.T) {
	pathTraversalNames := []struct {
		name    string
		envName string
	}{
		{name: "parent directory", envName: "../escape"},
		{name: "deep traversal", envName: "../../etc/passwd"},
		{name: "dot-dot", envName: ".."},
		{name: "semicolon injection", envName: "env;rm -rf /"},
		{name: "command substitution", envName: "$(cat /etc/shadow)"},
		{name: "backtick injection", envName: "`whoami`"},
		{name: "empty name", envName: ""},
		{name: "slash traversal", envName: "foo/bar"},
		{name: "uppercase", envName: "STAGING"},
		{name: "leading hyphen", envName: "-bad"},
		{name: "trailing hyphen", envName: "bad-"},
		{name: "space", envName: "my env"},
		{name: "hidden", envName: ".secret"},
	}

	for _, tt := range pathTraversalNames {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := setupProjectForValidation(t, "myapp")
			env, err := m.Create("myapp", tt.envName, "staging.example.com", "main")
			if err == nil {
				t.Errorf("Create() with envName=%q should return error, got env=%+v", tt.envName, env)
			}
			if env != nil {
				t.Errorf("Create() with invalid envName should return nil environment, got %+v", env)
			}
		})
	}
}

func TestCreateRejectsInvalidEnvNameNoDirectoryCreated(t *testing.T) {
	// Verify that when Create rejects an invalid env name, no directory is
	// created on the filesystem -- especially important for traversal attacks.
	m, tmpDir := setupProjectForValidation(t, "myapp")

	_, err := m.Create("myapp", "../escape", "evil.example.com", "main")
	if err == nil {
		t.Fatal("Create() should reject path traversal")
	}

	// Ensure no "escape" directory was created at the parent level.
	escapePath := filepath.Join(tmpDir, "escape")
	if _, statErr := os.Stat(escapePath); !os.IsNotExist(statErr) {
		t.Errorf("path traversal attempt should not create directory at %s", escapePath)
	}
}

func TestDeleteRejectsInvalidEnvName(t *testing.T) {
	invalidNames := []struct {
		name    string
		envName string
	}{
		{name: "empty", envName: ""},
		{name: "parent traversal", envName: "../escape"},
		{name: "deep traversal", envName: "../../etc"},
		{name: "shell injection semicolon", envName: "env;rm -rf /"},
		{name: "command substitution", envName: "$(whoami)"},
		{name: "uppercase", envName: "PROD"},
		{name: "hidden dir", envName: ".hidden"},
		{name: "leading hyphen", envName: "-bad"},
		{name: "trailing hyphen", envName: "bad-"},
		{name: "slash", envName: "a/b"},
		{name: "space", envName: "a b"},
		{name: "backtick", envName: "`id`"},
	}

	for _, tt := range invalidNames {
		t.Run(tt.name, func(t *testing.T) {
			m := NewManager(t.TempDir())
			err := m.Delete("myapp", tt.envName)
			if err == nil {
				t.Errorf("Delete() with envName=%q should return error", tt.envName)
			}
			if !strings.Contains(err.Error(), "invalid environment name") {
				t.Errorf("Delete() error should mention 'invalid environment name', got: %v", err)
			}
		})
	}
}

func TestPromoteRejectsInvalidEnvNames(t *testing.T) {
	tests := []struct {
		name    string
		fromEnv string
		toEnv   string
		errPart string // substring expected in error
	}{
		{
			name:    "invalid source empty",
			fromEnv: "",
			toEnv:   "production",
			errPart: "source environment",
		},
		{
			name:    "invalid source traversal",
			fromEnv: "../etc",
			toEnv:   "production",
			errPart: "source environment",
		},
		{
			name:    "invalid source injection",
			fromEnv: "staging;evil",
			toEnv:   "production",
			errPart: "source environment",
		},
		{
			name:    "invalid target empty",
			fromEnv: "staging",
			toEnv:   "",
			errPart: "target environment",
		},
		{
			name:    "invalid target traversal",
			fromEnv: "staging",
			toEnv:   "../../root",
			errPart: "target environment",
		},
		{
			name:    "invalid target injection",
			fromEnv: "staging",
			toEnv:   "prod$(rm -rf /)",
			errPart: "target environment",
		},
		{
			name:    "invalid target uppercase",
			fromEnv: "staging",
			toEnv:   "PRODUCTION",
			errPart: "target environment",
		},
		{
			name:    "both invalid",
			fromEnv: "../bad",
			toEnv:   "../../worse",
			errPart: "source environment", // source validated first
		},
		{
			name:    "invalid source backtick",
			fromEnv: "`id`",
			toEnv:   "production",
			errPart: "source environment",
		},
		{
			name:    "invalid target hidden",
			fromEnv: "staging",
			toEnv:   ".hidden",
			errPart: "target environment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewManager(t.TempDir())
			err := m.Promote("myapp", tt.fromEnv, tt.toEnv)
			if err == nil {
				t.Errorf("Promote(%q, %q) should return error", tt.fromEnv, tt.toEnv)
			}
			if !strings.Contains(err.Error(), tt.errPart) {
				t.Errorf("Promote() error should contain %q, got: %v", tt.errPart, err)
			}
		})
	}
}

func TestPromoteAcceptsValidEnvNames(t *testing.T) {
	// Verify that valid env names pass validation (the Promote call will
	// still fail because the source environment directory doesn't exist,
	// but the error should NOT be about invalid environment names).
	m := NewManager(t.TempDir())

	err := m.Promote("myapp", "staging", "production")
	if err == nil {
		t.Fatal("Promote() should fail (source env doesn't exist), but not due to validation")
	}
	// The error should be about the source env not being found, not validation.
	if strings.Contains(err.Error(), "source environment:") && strings.Contains(err.Error(), "lowercase") {
		t.Errorf("Promote() with valid names should not fail validation, got: %v", err)
	}
}

func TestCreateRejectsEmptyProjectName(t *testing.T) {
	m := NewManager(t.TempDir())

	env, err := m.Create("", "staging", "staging.example.com", "main")
	if err == nil {
		t.Error("Create() with empty project name should return error")
	}
	if env != nil {
		t.Error("Create() with empty project name should return nil environment")
	}
	if err != nil && !strings.Contains(err.Error(), "project name") {
		t.Errorf("error should mention 'project name', got: %v", err)
	}
}

func TestCreateRejectsEmptyProjectNameBeforeEnvValidation(t *testing.T) {
	// Even with an invalid env name, the project name check should come first
	// (or at least one of the two should trigger).
	m := NewManager(t.TempDir())

	_, err := m.Create("", "../evil", "evil.example.com", "main")
	if err == nil {
		t.Error("Create() with empty project name and invalid env name should return error")
	}
}

func TestGetEnvPathNoTraversal(t *testing.T) {
	// While GetEnvPath itself doesn't validate, verify that the path
	// construction is safe when used with validated names.
	m := NewManager("/data/projects")

	path := m.GetEnvPath("myapp", "staging")
	if !strings.HasPrefix(path, "/data/projects/myapp/environments/") {
		t.Errorf("GetEnvPath should produce path under basePath, got: %s", path)
	}
	if strings.Contains(path, "..") {
		t.Errorf("GetEnvPath should not contain '..', got: %s", path)
	}
}

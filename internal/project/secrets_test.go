package project

import (
	"testing"
)

func TestGenerateSecret(t *testing.T) {
	s1 := GenerateSecret(16)
	s2 := GenerateSecret(16)

	if len(s1) != 32 { // hex encoding doubles the length
		t.Errorf("expected 32 char hex string, got %d chars", len(s1))
	}
	if s1 == s2 {
		t.Error("two generated secrets should not be identical")
	}
}

func TestGenerateSecretLengths(t *testing.T) {
	tests := []struct {
		byteLen    int
		expectedHex int
	}{
		{8, 16},
		{16, 32},
		{32, 64},
	}

	for _, tt := range tests {
		s := GenerateSecret(tt.byteLen)
		if len(s) != tt.expectedHex {
			t.Errorf("GenerateSecret(%d): expected %d hex chars, got %d", tt.byteLen, tt.expectedHex, len(s))
		}
	}
}

func TestValidateName(t *testing.T) {
	valid := []string{"myapp", "a", "test-app", "app123", "a1b2c3"}
	for _, name := range valid {
		if err := ValidateName(name); err != nil {
			t.Errorf("ValidateName(%q) should be valid, got error: %v", name, err)
		}
	}

	invalid := []string{
		"",                                                                  // empty
		"-myapp",                                                            // starts with hyphen
		"myapp-",                                                            // ends with hyphen
		"my--app",                                                           // consecutive hyphens
		"My App",                                                            // spaces and uppercase
		"my_app",                                                            // underscore
		"my.app",                                                            // dot
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",    // 64 chars — too long
		"../etc/passwd",                                                     // path traversal
	}
	for _, name := range invalid {
		if err := ValidateName(name); err == nil {
			t.Errorf("ValidateName(%q) should be invalid, got nil error", name)
		}
	}
}

func TestLinuxUserName(t *testing.T) {
	tests := []struct {
		project  string
		expected string
	}{
		{"myapp", "fleetdeck-myapp"},
		{"test-project", "fleetdeck-test-project"},
		{"a", "fleetdeck-a"},
	}

	for _, tt := range tests {
		got := LinuxUserName(tt.project)
		if got != tt.expected {
			t.Errorf("LinuxUserName(%q) = %q, want %q", tt.project, got, tt.expected)
		}
	}
}

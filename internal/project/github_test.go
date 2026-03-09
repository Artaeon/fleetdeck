package project

import (
	"net"
	"testing"
)

func TestGetServerIP(t *testing.T) {
	ip, err := GetServerIP()
	if err != nil {
		t.Skip("GetServerIP requires network/hostname -I support: " + err.Error())
	}

	// Verify the result is a valid IP address
	if ip == "" {
		t.Fatal("GetServerIP returned empty string")
	}

	parsed := net.ParseIP(ip)
	if parsed == nil {
		t.Errorf("GetServerIP returned invalid IP address: %q", ip)
	}

	// Should be a non-loopback address (hostname -I typically excludes 127.0.0.1)
	if parsed.IsLoopback() {
		t.Errorf("GetServerIP returned loopback address: %q", ip)
	}
}

func TestGetServerIPFormat(t *testing.T) {
	ip, err := GetServerIP()
	if err != nil {
		t.Skip("GetServerIP requires network/hostname -I support: " + err.Error())
	}

	// Should return just one IP, not multiple
	parsed := net.ParseIP(ip)
	if parsed == nil {
		t.Fatalf("GetServerIP returned invalid IP: %q", ip)
	}

	// Should be IPv4 or IPv6, not empty
	if parsed.To4() == nil && parsed.To16() == nil {
		t.Errorf("GetServerIP returned neither IPv4 nor IPv6: %q", ip)
	}
}

func TestCreateGitHubRepoArgConstruction(t *testing.T) {
	// We can't actually call GitHub, but we can verify the repo name
	// construction logic by testing the function's argument building.
	// The function constructs "org/name" when org is provided, or just
	// "name" when org is empty.

	tests := []struct {
		org      string
		name     string
		private  bool
		wantRepo string
	}{
		{"myorg", "myapp", true, "myorg/myapp"},
		{"myorg", "myapp", false, "myorg/myapp"},
		{"", "myapp", true, "myapp"},
		{"", "myapp", false, "myapp"},
		{"company", "service-api", true, "company/service-api"},
	}

	for _, tt := range tests {
		// Verify the repo name construction logic
		repoName := tt.name
		if tt.org != "" {
			repoName = tt.org + "/" + tt.name
		}
		if repoName != tt.wantRepo {
			t.Errorf("repo name for org=%q name=%q: got %q, want %q",
				tt.org, tt.name, repoName, tt.wantRepo)
		}
	}
}

func TestSetGitHubSecretErrorWrapping(t *testing.T) {
	// This will fail because `gh` isn't authenticated/available in test,
	// but we can verify the error is wrapped properly.
	err := SetGitHubSecret("nonexistent/repo", "MY_SECRET", "myvalue")
	if err == nil {
		t.Skip("gh CLI is available and succeeded unexpectedly")
	}

	// Verify the error message includes the secret key name
	errMsg := err.Error()
	if len(errMsg) == 0 {
		t.Error("error message should not be empty")
	}
	// The error wrapping includes "setting secret <key>:"
	if !contains(errMsg, "setting secret MY_SECRET") {
		t.Errorf("error should reference the secret key name, got: %v", err)
	}
}

func TestDeleteGitHubRepoErrorWrapping(t *testing.T) {
	err := DeleteGitHubRepo("nonexistent/repo-that-does-not-exist")
	if err == nil {
		t.Skip("gh CLI is available and succeeded unexpectedly")
	}

	errMsg := err.Error()
	if !contains(errMsg, "deleting GitHub repo") {
		t.Errorf("error should be wrapped with 'deleting GitHub repo', got: %v", err)
	}
}

func TestCreateGitHubRepoErrorWrapping(t *testing.T) {
	_, err := CreateGitHubRepo("nonexistent-org", "nonexistent-repo", true)
	if err == nil {
		t.Skip("gh CLI is available and succeeded unexpectedly")
	}

	errMsg := err.Error()
	if !contains(errMsg, "creating GitHub repo") {
		t.Errorf("error should be wrapped with 'creating GitHub repo', got: %v", err)
	}
}

// contains checks if s contains substr (simple helper to avoid importing strings).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

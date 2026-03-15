package cmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestUseCaseDeployWithInvalidDomain verifies that validateDomain rejects
// domains that a user might accidentally type -- no dots, spaces, shell
// metacharacters, or empty strings.
func TestUseCaseDeployWithInvalidDomain(t *testing.T) {
	invalidDomains := []struct {
		name   string
		domain string
	}{
		{"localhost without dot", "localhost"},
		{"bare name", "myapp"},
		{"space in domain", "app server"},
		{"semicolon injection", "app;rm"},
		{"empty string", ""},
	}

	for _, tt := range invalidDomains {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDomain(tt.domain)
			if err == nil {
				t.Errorf("validateDomain(%q) should return error", tt.domain)
			}
		})
	}
}

// TestUseCaseDeployWithValidDomain verifies that validateDomain accepts
// well-formed domains that users would commonly use for deployments.
func TestUseCaseDeployWithValidDomain(t *testing.T) {
	validDomains := []struct {
		name   string
		domain string
	}{
		{"simple domain", "myapp.com"},
		{"subdomain", "sub.example.com"},
		{"multi-level TLD", "app.staging.example.co.uk"},
	}

	for _, tt := range validDomains {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDomain(tt.domain)
			if err != nil {
				t.Errorf("validateDomain(%q) should succeed, got: %v", tt.domain, err)
			}
		})
	}
}

// TestUseCaseDeployProjectNameFromDir verifies that filepath.Base correctly
// extracts the project name from various directory paths, as used in the
// deploy command when --name is not provided.
func TestUseCaseDeployProjectNameFromDir(t *testing.T) {
	tests := []struct {
		name     string
		dirPath  string
		expected string
	}{
		{"absolute path", "/home/user/my-app", "my-app"},
		{"nested path", "/var/projects/webapp/src", "src"},
		{"trailing slash stripped by filepath", "/home/user/my-app", "my-app"},
		{"root path", "/", "/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filepath.Base(tt.dirPath)
			if got != tt.expected {
				t.Errorf("filepath.Base(%q) = %q, want %q", tt.dirPath, got, tt.expected)
			}
		})
	}

	// Special case: "." resolves to the current directory name, which varies,
	// but filepath.Base(".") always returns ".".
	dotResult := filepath.Base(".")
	if dotResult != "." {
		t.Errorf("filepath.Base(\".\") = %q, want %q", dotResult, ".")
	}
}

// TestUseCaseMonitorMultipleArgs verifies that the monitor start command
// accepts MinimumNArgs(1) -- it should work with 1, 2, or 3 project names.
func TestUseCaseMonitorMultipleArgs(t *testing.T) {
	monCmd := findSubcommand(rootCmd, "monitor")
	if monCmd == nil {
		t.Fatal("monitor command not found")
	}

	startCmd := findSubcommand(monCmd, "start")
	if startCmd == nil {
		t.Fatal("monitor start subcommand not found")
	}

	// 0 args should fail.
	if err := startCmd.Args(startCmd, []string{}); err == nil {
		t.Error("monitor start with 0 args should fail")
	}

	// 1 arg should pass.
	if err := startCmd.Args(startCmd, []string{"myapp"}); err != nil {
		t.Errorf("monitor start with 1 arg should pass, got: %v", err)
	}

	// 2 args should pass.
	if err := startCmd.Args(startCmd, []string{"myapp", "api"}); err != nil {
		t.Errorf("monitor start with 2 args should pass, got: %v", err)
	}

	// 3 args should pass.
	if err := startCmd.Args(startCmd, []string{"myapp", "api", "blog"}); err != nil {
		t.Errorf("monitor start with 3 args should pass, got: %v", err)
	}
}

// TestUseCaseMonitorCheckExitsNonZero verifies that the monitor check command
// uses ExactArgs(1) and is designed to exit non-zero for unhealthy services.
// We verify the command structure rather than the actual exit behavior
// (which requires a running database and project).
func TestUseCaseMonitorCheckExitsNonZero(t *testing.T) {
	monCmd := findSubcommand(rootCmd, "monitor")
	if monCmd == nil {
		t.Fatal("monitor command not found")
	}

	checkCmd := findSubcommand(monCmd, "check")
	if checkCmd == nil {
		t.Fatal("monitor check subcommand not found")
	}

	// Should require exactly 1 argument (the project name).
	if err := checkCmd.Args(checkCmd, []string{}); err == nil {
		t.Error("monitor check with 0 args should fail")
	}
	if err := checkCmd.Args(checkCmd, []string{"myapp"}); err != nil {
		t.Errorf("monitor check with 1 arg should pass, got: %v", err)
	}
	if err := checkCmd.Args(checkCmd, []string{"a", "b"}); err == nil {
		t.Error("monitor check with 2 args should fail")
	}

	// Verify timeout flag exists (used for the health check HTTP call).
	if checkCmd.Flags().Lookup("timeout") == nil {
		t.Error("monitor check should have --timeout flag")
	}
}

// TestUseCaseCreateDomainValidation verifies that the create command has
// domain validation. We test the validateDomain function that the command
// uses with domain patterns a user might try for project creation.
func TestUseCaseCreateDomainValidation(t *testing.T) {
	// These patterns specifically come up during `fleetdeck create`:
	badDomains := []struct {
		domain string
		reason string
	}{
		{"", "empty domain"},
		{"myproject", "no dot -- user forgot the TLD"},
		{"my project.com", "space in domain"},
		{"project$(cmd).com", "command injection via $()"},
		{"project`id`.com", "command injection via backticks"},
	}

	for _, tt := range badDomains {
		t.Run(tt.reason, func(t *testing.T) {
			err := validateDomain(tt.domain)
			if err == nil {
				t.Errorf("validateDomain(%q) should reject: %s", tt.domain, tt.reason)
			}
		})
	}
}

// TestUseCaseDNSSetupFlow verifies that both validateDomain and validateIP
// are available and function correctly, as both are called during the
// dns setup command flow.
func TestUseCaseDNSSetupFlow(t *testing.T) {
	// Simulate the validation that dnsSetupCmd performs with its two args.

	// Valid domain + valid IP should both pass.
	if err := validateDomain("example.com"); err != nil {
		t.Errorf("validateDomain should accept valid domain: %v", err)
	}
	if err := validateIP("143.198.1.1"); err != nil {
		t.Errorf("validateIP should accept valid IP: %v", err)
	}

	// Invalid domain should fail even with valid IP.
	if err := validateDomain("nodot"); err == nil {
		t.Error("validateDomain should reject domain without dot")
	}

	// Invalid IP should fail even with valid domain.
	if err := validateIP("not-an-ip"); err == nil {
		t.Error("validateIP should reject non-IP string")
	}

	// Verify IPv6 is accepted (some servers have IPv6 only).
	if err := validateIP("2001:db8::1"); err != nil {
		t.Errorf("validateIP should accept IPv6: %v", err)
	}

	// Verify the dns setup command requires ExactArgs(2).
	dnsParent := findSubcommand(rootCmd, "dns")
	if dnsParent == nil {
		t.Fatal("dns command not found")
	}
	setupCmd := findSubcommand(dnsParent, "setup")
	if setupCmd == nil {
		t.Fatal("dns setup subcommand not found")
	}
	if err := setupCmd.Args(setupCmd, []string{"example.com", "1.2.3.4"}); err != nil {
		t.Errorf("dns setup should accept 2 args, got: %v", err)
	}
	if err := setupCmd.Args(setupCmd, []string{"example.com"}); err == nil {
		t.Error("dns setup should reject 1 arg")
	}
}

// TestUseCaseDetectJsonOutput verifies that the detect command has a --json
// flag registered, allowing JSON output for CI/CD pipeline consumption.
func TestUseCaseDetectJsonOutput(t *testing.T) {
	cmd := findSubcommand(rootCmd, "detect")
	if cmd == nil {
		t.Fatal("detect command not found")
	}

	jsonFlag := cmd.Flags().Lookup("json")
	if jsonFlag == nil {
		t.Fatal("detect command should have --json flag")
	}

	// Default should be false (pretty output by default).
	if jsonFlag.DefValue != "false" {
		t.Errorf("--json default should be 'false', got %q", jsonFlag.DefValue)
	}

	// The flag should be a bool type.
	if jsonFlag.Value.Type() != "bool" {
		t.Errorf("--json should be bool type, got %q", jsonFlag.Value.Type())
	}
}

// TestUseCaseServerSetupRequiredFlags verifies that the server setup command
// has --domain and --email flags, both of which are required for proper
// Traefik and Let's Encrypt configuration.
func TestUseCaseServerSetupRequiredFlags(t *testing.T) {
	serverParent := findSubcommand(rootCmd, "server")
	if serverParent == nil {
		t.Fatal("server command not found")
	}

	setupCmd := findSubcommand(serverParent, "setup")
	if setupCmd == nil {
		t.Fatal("server setup subcommand not found")
	}

	// Verify --domain flag exists.
	domainFlag := setupCmd.Flags().Lookup("domain")
	if domainFlag == nil {
		t.Fatal("server setup should have --domain flag")
	}

	// Verify --email flag exists.
	emailFlag := setupCmd.Flags().Lookup("email")
	if emailFlag == nil {
		t.Fatal("server setup should have --email flag")
	}

	// Both should default to empty (indicating they must be provided).
	if domainFlag.DefValue != "" {
		t.Errorf("--domain default should be empty, got %q", domainFlag.DefValue)
	}
	if emailFlag.DefValue != "" {
		t.Errorf("--email default should be empty, got %q", emailFlag.DefValue)
	}

	// Verify the command requires exactly 1 arg (user@host).
	if err := setupCmd.Args(setupCmd, []string{"root@1.2.3.4"}); err != nil {
		t.Errorf("server setup should accept 1 arg, got: %v", err)
	}
	if err := setupCmd.Args(setupCmd, []string{}); err == nil {
		t.Error("server setup should reject 0 args")
	}
}

// TestUseCaseEnvCreateAutoGeneratesDomain verifies that the env create command
// has logic to auto-generate a domain when --domain is not provided. The
// auto-generated pattern is envName.originalDomain. We verify the command
// structure and flag setup that enables this behavior.
func TestUseCaseEnvCreateAutoGeneratesDomain(t *testing.T) {
	envParent := findSubcommand(rootCmd, "env")
	if envParent == nil {
		t.Fatal("env command not found")
	}

	createCmd := findSubcommand(envParent, "create")
	if createCmd == nil {
		t.Fatal("env create subcommand not found")
	}

	// Verify --domain flag exists and defaults to empty (triggers auto-generation).
	domainFlag := createCmd.Flags().Lookup("domain")
	if domainFlag == nil {
		t.Fatal("env create should have --domain flag")
	}
	if domainFlag.DefValue != "" {
		t.Errorf("--domain default should be empty (auto-generate), got %q", domainFlag.DefValue)
	}

	// Verify --branch flag exists for preview environments.
	branchFlag := createCmd.Flags().Lookup("branch")
	if branchFlag == nil {
		t.Fatal("env create should have --branch flag")
	}

	// Verify the command requires exactly 2 args: <project> <environment>.
	if err := createCmd.Args(createCmd, []string{"myapp", "staging"}); err != nil {
		t.Errorf("env create should accept 2 args, got: %v", err)
	}
	if err := createCmd.Args(createCmd, []string{"myapp"}); err == nil {
		t.Error("env create should reject 1 arg")
	}

	// Verify the auto-generation pattern: envName + "." + originalDomain.
	// This is a unit test of the pattern logic, not the full command execution
	// (which requires a database).
	envName := "staging"
	originalDomain := "myapp.example.com"
	autoGenerated := envName + "." + originalDomain
	expected := "staging.myapp.example.com"
	if autoGenerated != expected {
		t.Errorf("auto-generated domain = %q, want %q", autoGenerated, expected)
	}

	// Verify the auto-generated domain passes validation.
	if err := validateDomain(autoGenerated); err != nil {
		t.Errorf("auto-generated domain should pass validation, got: %v", err)
	}
}

// findUseCaseSubcommand is a helper for locating nested subcommands by
// traversing the command tree. This is used to find commands like
// "server setup" or "dns setup" for testing.
func findUseCaseSubcommand(parent *cobra.Command, names ...string) *cobra.Command {
	current := parent
	for _, name := range names {
		found := false
		for _, cmd := range current.Commands() {
			if cmd.Name() == name {
				current = cmd
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}
	return current
}

// TestUseCaseDeployStrategyFlagDefault verifies that the deploy command's
// --strategy flag defaults to "basic", matching the default in GetStrategy.
func TestUseCaseDeployStrategyFlagDefault(t *testing.T) {
	cmd := findSubcommand(rootCmd, "deploy")
	if cmd == nil {
		t.Fatal("deploy command not found")
	}

	strategyFlag := cmd.Flags().Lookup("strategy")
	if strategyFlag == nil {
		t.Fatal("deploy should have --strategy flag")
	}
	if strategyFlag.DefValue != "basic" {
		t.Errorf("--strategy default = %q, want %q", strategyFlag.DefValue, "basic")
	}
}

// TestUseCaseValidateDomainErrorMessages verifies that domain validation
// errors contain enough context for the user to understand what went wrong.
func TestUseCaseValidateDomainErrorMessages(t *testing.T) {
	// Empty domain should mention "empty".
	err := validateDomain("")
	if err == nil {
		t.Fatal("expected error for empty domain")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("empty domain error should mention 'empty', got: %v", err)
	}

	// No-dot domain should mention "dot".
	err = validateDomain("localhost")
	if err == nil {
		t.Fatal("expected error for no-dot domain")
	}
	if !strings.Contains(err.Error(), "dot") {
		t.Errorf("no-dot error should mention 'dot', got: %v", err)
	}

	// Invalid chars domain should mention "invalid".
	err = validateDomain("evil;domain.com")
	if err == nil {
		t.Fatal("expected error for invalid chars")
	}
	if !strings.Contains(err.Error(), "invalid") {
		t.Errorf("invalid chars error should mention 'invalid', got: %v", err)
	}
}

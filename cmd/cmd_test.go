package cmd

import (
	"testing"

	"github.com/fleetdeck/fleetdeck/internal/config"
	"github.com/spf13/cobra"
)

// --- Command registration tests ---

func TestAllCommandsRegistered(t *testing.T) {
	expectedCommands := []string{
		"create",
		"list",
		"info",
		"status",
		"start",
		"stop",
		"restart",
		"destroy",
		"backup",
		"snapshot",
		"rollback",
		"logs",
		"import",
		"discover",
		"dashboard",
		"init",
		"upgrade",
		"schedule",
		"audit",
		"sync",
		"templates",
	}

	registered := make(map[string]bool)
	for _, cmd := range rootCmd.Commands() {
		registered[cmd.Name()] = true
	}

	for _, expected := range expectedCommands {
		if !registered[expected] {
			t.Errorf("expected command %q to be registered on rootCmd", expected)
		}
	}
}

func TestRootCmdHasConfigFlag(t *testing.T) {
	f := rootCmd.PersistentFlags().Lookup("config")
	if f == nil {
		t.Fatal("expected --config persistent flag on rootCmd")
	}
	if f.DefValue != "" {
		t.Errorf("expected default empty, got %q", f.DefValue)
	}
}

// --- Create command flag tests ---

func TestCreateCommandDomainFlagRequired(t *testing.T) {
	var createFound bool
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "create" {
			createFound = true

			domainFlag := cmd.Flags().Lookup("domain")
			if domainFlag == nil {
				t.Fatal("expected --domain flag on create command")
			}

			// Check that the flag is marked as required
			annotations := domainFlag.Annotations
			if annotations == nil {
				t.Fatal("expected --domain flag to have annotations (required)")
			}
			required, exists := annotations["cobra_annotation_bash_completion_one_required_flag"]
			if !exists || len(required) == 0 {
				t.Error("expected --domain flag to be marked as required")
			}
			break
		}
	}
	if !createFound {
		t.Fatal("create command not found on rootCmd")
	}
}

func TestCreateCommandHasExpectedFlags(t *testing.T) {
	var createCmd *cobra.Command
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "create" {
			createCmd = cmd
			break
		}
	}
	if createCmd == nil {
		t.Fatal("create command not found")
	}

	expectedFlags := []string{"domain", "github-org", "template", "skip-github"}
	for _, name := range expectedFlags {
		if createCmd.Flags().Lookup(name) == nil {
			t.Errorf("expected flag --%s on create command", name)
		}
	}
}

func TestCreateCommandRequiresExactlyOneArg(t *testing.T) {
	var createCmd *cobra.Command
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "create" {
			createCmd = cmd
			break
		}
	}
	if createCmd == nil {
		t.Fatal("create command not found")
	}

	// 0 args should fail
	if err := createCmd.Args(createCmd, []string{}); err == nil {
		t.Error("expected error with 0 args")
	}

	// 1 arg should pass
	if err := createCmd.Args(createCmd, []string{"myproject"}); err != nil {
		t.Errorf("expected no error with 1 arg, got %v", err)
	}

	// 2 args should fail
	if err := createCmd.Args(createCmd, []string{"a", "b"}); err == nil {
		t.Error("expected error with 2 args")
	}
}

// --- minInt tests ---

func TestMinInt(t *testing.T) {
	tests := []struct {
		a, b, want int
	}{
		{1, 2, 1},
		{2, 1, 1},
		{5, 5, 5},
		{0, 10, 0},
		{-1, 0, -1},
		{-5, -3, -5},
		{100, 0, 0},
	}

	for _, tt := range tests {
		got := minInt(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("minInt(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

// --- autoSnapshot tests ---

func TestAutoSnapshotNilConfig(t *testing.T) {
	// Save and restore global cfg
	originalCfg := cfg
	cfg = nil
	defer func() { cfg = originalCfg }()

	// Should not panic when cfg is nil
	autoSnapshot("anyproject", "stop")
}

func TestAutoSnapshotDisabled(t *testing.T) {
	// Save and restore global cfg
	originalCfg := cfg
	defer func() { cfg = originalCfg }()

	// Load a default config with AutoSnapshot disabled
	cfg = &config.Config{
		Backup: config.BackupConfig{
			AutoSnapshot: false,
		},
	}

	// Should return early when auto-snapshot is disabled
	// (no panic, no error — just a no-op)
	autoSnapshot("anyproject", "restart")
}

// --- countContainersForProject tests ---

func TestCountContainersForProjectEmptyPath(t *testing.T) {
	// Empty path should return 0, 0 (docker compose will fail)
	running, total := countContainersForProject("")
	if running != 0 {
		t.Errorf("expected 0 running, got %d", running)
	}
	if total != 0 {
		t.Errorf("expected 0 total, got %d", total)
	}
}

func TestCountContainersForProjectNonexistentPath(t *testing.T) {
	running, total := countContainersForProject("/nonexistent/path/xyz")
	if running != 0 {
		t.Errorf("expected 0 running, got %d", running)
	}
	if total != 0 {
		t.Errorf("expected 0 total, got %d", total)
	}
}

// --- List command alias test ---

func TestListCommandHasAlias(t *testing.T) {
	var listCmd *cobra.Command
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "list" {
			listCmd = cmd
			break
		}
	}
	if listCmd == nil {
		t.Fatal("list command not found")
	}

	foundAlias := false
	for _, alias := range listCmd.Aliases {
		if alias == "ls" {
			foundAlias = true
			break
		}
	}
	if !foundAlias {
		t.Error("expected 'ls' alias on list command")
	}
}

// --- Info command arg validation ---

func TestInfoCommandRequiresExactlyOneArg(t *testing.T) {
	var infoCmd *cobra.Command
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "info" {
			infoCmd = cmd
			break
		}
	}
	if infoCmd == nil {
		t.Fatal("info command not found")
	}

	if err := infoCmd.Args(infoCmd, []string{}); err == nil {
		t.Error("expected error with 0 args")
	}
	if err := infoCmd.Args(infoCmd, []string{"myproject"}); err != nil {
		t.Errorf("expected no error with 1 arg, got %v", err)
	}
}

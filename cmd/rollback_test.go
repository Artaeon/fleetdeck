package cmd

import (
	"testing"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/db"
)

func TestFindBackupByPrefix(t *testing.T) {
	backups := []*db.BackupRecord{
		{ID: "aaaa-1111-2222-3333", Type: "manual", CreatedAt: time.Now()},
		{ID: "bbbb-4444-5555-6666", Type: "snapshot", CreatedAt: time.Now()},
		{ID: "cccc-7777-8888-9999", Type: "manual", CreatedAt: time.Now()},
	}

	tests := []struct {
		name   string
		prefix string
		wantID string
		found  bool
	}{
		{"full match", "aaaa-1111-2222-3333", "aaaa-1111-2222-3333", true},
		{"short prefix", "bbbb", "bbbb-4444-5555-6666", true},
		{"12 char prefix", "cccc-7777-888", "cccc-7777-8888-9999", true},
		{"no match", "zzzz", "", false},
		{"empty prefix matches first", "", "aaaa-1111-2222-3333", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findBackupByPrefix(backups, tt.prefix)
			if tt.found {
				if result == nil {
					t.Fatalf("expected to find backup with prefix %q, got nil", tt.prefix)
				}
				if result.ID != tt.wantID {
					t.Errorf("expected ID %s, got %s", tt.wantID, result.ID)
				}
			} else {
				if result != nil {
					t.Errorf("expected nil for prefix %q, got %s", tt.prefix, result.ID)
				}
			}
		})
	}
}

func TestFindBackupByPrefixEmpty(t *testing.T) {
	result := findBackupByPrefix(nil, "anything")
	if result != nil {
		t.Errorf("expected nil for empty backup list, got %v", result)
	}
}

func TestRollbackCommandFlags(t *testing.T) {
	// Verify the rollback command is properly registered on rootCmd
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "rollback <project-name>" {
			found = true

			// Check --latest flag exists
			f := cmd.Flags().Lookup("latest")
			if f == nil {
				t.Error("expected --latest flag to be defined")
			} else if f.DefValue != "false" {
				t.Errorf("expected --latest default false, got %s", f.DefValue)
			}

			// Check --backup-id flag exists
			f = cmd.Flags().Lookup("backup-id")
			if f == nil {
				t.Error("expected --backup-id flag to be defined")
			} else if f.DefValue != "" {
				t.Errorf("expected --backup-id default empty, got %s", f.DefValue)
			}

			// Check --no-snapshot flag exists
			f = cmd.Flags().Lookup("no-snapshot")
			if f == nil {
				t.Error("expected --no-snapshot flag to be defined")
			} else if f.DefValue != "false" {
				t.Errorf("expected --no-snapshot default false, got %s", f.DefValue)
			}

			break
		}
	}
	if !found {
		t.Error("rollback command not found on rootCmd")
	}
}

func TestRollbackRequiresExactlyOneArg(t *testing.T) {
	// The command has Args: cobra.ExactArgs(1), which means it should reject 0 or 2+ args.
	cmd := rollbackCmd

	// With no args
	err := cmd.Args(cmd, []string{})
	if err == nil {
		t.Error("expected error with 0 args")
	}

	// With one arg (should pass)
	err = cmd.Args(cmd, []string{"myproject"})
	if err != nil {
		t.Errorf("expected no error with 1 arg, got %v", err)
	}

	// With two args
	err = cmd.Args(cmd, []string{"myproject", "extra"})
	if err == nil {
		t.Error("expected error with 2 args")
	}
}

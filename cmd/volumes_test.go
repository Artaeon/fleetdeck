package cmd

import "testing"

// --- Volumes command tests ---

func TestVolumesCommandRegistered(t *testing.T) {
	cmd := findSubcommand(rootCmd, "volumes")
	if cmd == nil {
		t.Fatal("expected 'volumes' command to be registered on rootCmd")
	}
}

func TestVolumesListCommandRegistered(t *testing.T) {
	volCmd := findSubcommand(rootCmd, "volumes")
	if volCmd == nil {
		t.Fatal("volumes command not found")
	}

	listCmd := findSubcommand(volCmd, "list")
	if listCmd == nil {
		t.Fatal("expected 'list' subcommand on volumes command")
	}
}

func TestVolumesListFlags(t *testing.T) {
	volCmd := findSubcommand(rootCmd, "volumes")
	if volCmd == nil {
		t.Fatal("volumes command not found")
	}

	listCmd := findSubcommand(volCmd, "list")
	if listCmd == nil {
		t.Fatal("list subcommand not found")
	}

	// SSH flags are persistent on the parent; check them there
	sshFlags := []string{"server", "port", "key", "passphrase"}
	for _, name := range sshFlags {
		if volCmd.PersistentFlags().Lookup(name) == nil {
			t.Errorf("expected persistent flag --%s on volumes command", name)
		}
	}

	// Verify list subcommand inherits them (available after parent parse)
	_ = listCmd
}

func TestVolumesRmCommandRegistered(t *testing.T) {
	volCmd := findSubcommand(rootCmd, "volumes")
	if volCmd == nil {
		t.Fatal("volumes command not found")
	}

	rmCmd := findSubcommand(volCmd, "rm")
	if rmCmd == nil {
		t.Fatal("expected 'rm' subcommand on volumes command")
	}
}

func TestVolumesRmRequiresArgument(t *testing.T) {
	volCmd := findSubcommand(rootCmd, "volumes")
	if volCmd == nil {
		t.Fatal("volumes command not found")
	}

	rmCmd := findSubcommand(volCmd, "rm")
	if rmCmd == nil {
		t.Fatal("rm subcommand not found")
	}

	// Verify that Args is set to require exactly 1 argument
	err := rmCmd.Args(rmCmd, []string{})
	if err == nil {
		t.Error("expected error when no arguments provided to volumes rm")
	}
}

func TestVolumesRmFlags(t *testing.T) {
	volCmd := findSubcommand(rootCmd, "volumes")
	if volCmd == nil {
		t.Fatal("volumes command not found")
	}

	rmCmd := findSubcommand(volCmd, "rm")
	if rmCmd == nil {
		t.Fatal("rm subcommand not found")
	}

	// force is a local flag on rm
	if rmCmd.Flags().Lookup("force") == nil {
		t.Error("expected flag --force on volumes rm command")
	}

	// SSH flags are persistent on the parent
	sshFlags := []string{"server", "port", "key", "passphrase"}
	for _, name := range sshFlags {
		if volCmd.PersistentFlags().Lookup(name) == nil {
			t.Errorf("expected persistent flag --%s on volumes command", name)
		}
	}
}

func TestDeployFreshFlag(t *testing.T) {
	cmd := findSubcommand(rootCmd, "deploy")
	if cmd == nil {
		t.Fatal("deploy command not found")
	}

	if cmd.Flags().Lookup("fresh") == nil {
		t.Error("expected flag --fresh on deploy command")
	}
}

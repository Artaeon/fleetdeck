package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

// findSubcommand searches a command's subcommands for one with the given name.
func findSubcommand(parent *cobra.Command, name string) *cobra.Command {
	for _, cmd := range parent.Commands() {
		if cmd.Name() == name {
			return cmd
		}
	}
	return nil
}

// --- Deploy command tests ---

func TestDeployCommandRegistered(t *testing.T) {
	cmd := findSubcommand(rootCmd, "deploy")
	if cmd == nil {
		t.Fatal("expected 'deploy' command to be registered on rootCmd")
	}
}

func TestDeployCommandFlags(t *testing.T) {
	cmd := findSubcommand(rootCmd, "deploy")
	if cmd == nil {
		t.Fatal("deploy command not found")
	}

	expectedFlags := []string{
		"domain", "server", "port", "key", "profile",
		"strategy", "name", "timeout", "insecure", "no-cache",
	}
	for _, name := range expectedFlags {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("expected flag --%s on deploy command", name)
		}
	}
}

func TestDeployNoCacheFlag(t *testing.T) {
	cmd := findSubcommand(rootCmd, "deploy")
	if cmd == nil {
		t.Fatal("deploy command not found")
	}

	f := cmd.Flags().Lookup("no-cache")
	if f == nil {
		t.Fatal("expected --no-cache flag on deploy command")
	}
	if f.DefValue != "false" {
		t.Errorf("expected --no-cache default to be \"false\", got %q", f.DefValue)
	}
	if f.Usage == "" {
		t.Error("expected --no-cache to have a usage description")
	}
}

// --- Detect command tests ---

func TestDetectCommandRegistered(t *testing.T) {
	cmd := findSubcommand(rootCmd, "detect")
	if cmd == nil {
		t.Fatal("expected 'detect' command to be registered on rootCmd")
	}
}

func TestDetectCommandFlags(t *testing.T) {
	cmd := findSubcommand(rootCmd, "detect")
	if cmd == nil {
		t.Fatal("detect command not found")
	}

	if cmd.Flags().Lookup("json") == nil {
		t.Error("expected flag --json on detect command")
	}
}

// --- Profiles command tests ---

func TestProfilesCommandRegistered(t *testing.T) {
	cmd := findSubcommand(rootCmd, "profiles")
	if cmd == nil {
		t.Fatal("expected 'profiles' command to be registered on rootCmd")
	}
}

// --- Profile info command tests ---

func TestProfileInfoCommandRegistered(t *testing.T) {
	cmd := findSubcommand(rootCmd, "profile")
	if cmd == nil {
		t.Fatal("expected 'profile' command to be registered on rootCmd")
	}
}

func TestProfileInfoFlags(t *testing.T) {
	cmd := findSubcommand(rootCmd, "profile")
	if cmd == nil {
		t.Fatal("profile command not found")
	}

	if cmd.Flags().Lookup("compose") == nil {
		t.Error("expected flag --compose on profile command")
	}
}

// --- Server command tests ---

func TestServerCommandRegistered(t *testing.T) {
	cmd := findSubcommand(rootCmd, "server")
	if cmd == nil {
		t.Fatal("expected 'server' command to be registered on rootCmd")
	}
}

func TestServerSetupFlags(t *testing.T) {
	serverCmd := findSubcommand(rootCmd, "server")
	if serverCmd == nil {
		t.Fatal("server command not found")
	}

	setupCmd := findSubcommand(serverCmd, "setup")
	if setupCmd == nil {
		t.Fatal("expected 'setup' subcommand on server command")
	}

	expectedFlags := []string{
		"port", "key", "domain", "email", "swap",
		"traefik-network", "insecure",
	}
	for _, name := range expectedFlags {
		if setupCmd.Flags().Lookup(name) == nil {
			t.Errorf("expected flag --%s on server setup command", name)
		}
	}
}

// --- Monitor command tests ---

func TestMonitorCommandRegistered(t *testing.T) {
	cmd := findSubcommand(rootCmd, "monitor")
	if cmd == nil {
		t.Fatal("expected 'monitor' command to be registered on rootCmd")
	}
}

func TestMonitorStartFlags(t *testing.T) {
	monCmd := findSubcommand(rootCmd, "monitor")
	if monCmd == nil {
		t.Fatal("monitor command not found")
	}

	startCmd := findSubcommand(monCmd, "start")
	if startCmd == nil {
		t.Fatal("expected 'start' subcommand on monitor command")
	}

	expectedFlags := []string{
		"interval", "timeout", "webhook", "slack", "threshold",
	}
	for _, name := range expectedFlags {
		if startCmd.Flags().Lookup(name) == nil {
			t.Errorf("expected flag --%s on monitor start command", name)
		}
	}
}

func TestMonitorCheckFlags(t *testing.T) {
	monCmd := findSubcommand(rootCmd, "monitor")
	if monCmd == nil {
		t.Fatal("monitor command not found")
	}

	checkCmd := findSubcommand(monCmd, "check")
	if checkCmd == nil {
		t.Fatal("expected 'check' subcommand on monitor command")
	}

	if checkCmd.Flags().Lookup("timeout") == nil {
		t.Error("expected flag --timeout on monitor check command")
	}
}

// --- Env command tests ---

func TestEnvCommandRegistered(t *testing.T) {
	cmd := findSubcommand(rootCmd, "env")
	if cmd == nil {
		t.Fatal("expected 'env' command to be registered on rootCmd")
	}
}

func TestEnvCreateFlags(t *testing.T) {
	envCmd := findSubcommand(rootCmd, "env")
	if envCmd == nil {
		t.Fatal("env command not found")
	}

	createCmd := findSubcommand(envCmd, "create")
	if createCmd == nil {
		t.Fatal("expected 'create' subcommand on env command")
	}

	expectedFlags := []string{"domain", "branch"}
	for _, name := range expectedFlags {
		if createCmd.Flags().Lookup(name) == nil {
			t.Errorf("expected flag --%s on env create command", name)
		}
	}
}

// --- DNS command tests ---

func TestDNSCommandRegistered(t *testing.T) {
	cmd := findSubcommand(rootCmd, "dns")
	if cmd == nil {
		t.Fatal("expected 'dns' command to be registered on rootCmd")
	}
}

func TestDNSPersistentFlags(t *testing.T) {
	cmd := findSubcommand(rootCmd, "dns")
	if cmd == nil {
		t.Fatal("dns command not found")
	}

	expectedFlags := []string{"provider", "token"}
	for _, name := range expectedFlags {
		if cmd.PersistentFlags().Lookup(name) == nil {
			t.Errorf("expected persistent flag --%s on dns command", name)
		}
	}
}

// --- parseTarget tests ---

func TestParseTarget(t *testing.T) {
	tests := []struct {
		input    string
		wantHost string
		wantUser string
	}{
		{"root@1.2.3.4", "1.2.3.4", "root"},
		{"ubuntu@myserver.com", "myserver.com", "ubuntu"},
		{"1.2.3.4", "1.2.3.4", "root"},
		{"deploy@server", "server", "deploy"},
	}

	for _, tt := range tests {
		host, user := parseTarget(tt.input)
		if host != tt.wantHost {
			t.Errorf("parseTarget(%q): host = %q, want %q", tt.input, host, tt.wantHost)
		}
		if user != tt.wantUser {
			t.Errorf("parseTarget(%q): user = %q, want %q", tt.input, user, tt.wantUser)
		}
	}
}

// --- defaultPortForTemplate tests ---

func TestDefaultPortForTemplate(t *testing.T) {
	tests := []struct {
		template string
		wantPort int
	}{
		{"go", 8080},
		{"python", 8000},
		{"static", 80},
		{"node", 3000},
		{"nextjs", 3000},
		{"nestjs", 3000},
		{"custom", 3000},
		{"", 3000},
		{"unknown", 3000},
	}

	for _, tt := range tests {
		got := defaultPortForTemplate(tt.template)
		if got != tt.wantPort {
			t.Errorf("defaultPortForTemplate(%q) = %d, want %d", tt.template, got, tt.wantPort)
		}
	}
}

// --- boolToYesNo tests ---

func TestBoolToYesNo(t *testing.T) {
	if got := boolToYesNo(true); got != "yes" {
		t.Errorf("boolToYesNo(true) = %q, want %q", got, "yes")
	}
	if got := boolToYesNo(false); got != "no" {
		t.Errorf("boolToYesNo(false) = %q, want %q", got, "no")
	}
}

// --- validateDomain edge case tests ---

func TestValidateDomainEdgeCases(t *testing.T) {
	tests := []struct {
		domain  string
		wantErr bool
	}{
		// Valid domains
		{"example.com", false},
		{"sub.example.com", false},
		{"a.b.c.d.example.com", false},
		{"my-site.example.com", false},

		// Invalid: empty
		{"", true},

		// Invalid: no dot
		{"localhost", true},
		{"example", true},

		// Invalid: special characters
		{"example .com", true},
		{"example\t.com", true},
		{"example\n.com", true},
		{"example\".com", true},
		{"example'.com", true},
		{"example`.com", true},
		{"example;.com", true},
		{"example$.com", true},
		{"example\\.com", true},
		{"example{.com", true},
		{"example}.com", true},
		{"example(.com", true},
		{"example).com", true},
	}

	for _, tt := range tests {
		err := validateDomain(tt.domain)
		if (err != nil) != tt.wantErr {
			t.Errorf("validateDomain(%q): err = %v, wantErr = %v", tt.domain, err, tt.wantErr)
		}
	}
}

// --- validateIP edge case tests ---

func TestValidateIPEdgeCases(t *testing.T) {
	tests := []struct {
		ip      string
		wantErr bool
	}{
		// Valid IPv4
		{"1.2.3.4", false},
		{"192.168.1.1", false},
		{"0.0.0.0", false},
		{"255.255.255.255", false},

		// Valid IPv6
		{"::1", false},
		{"2001:db8::1", false},
		{"fe80::1", false},

		// Invalid
		{"", true},
		{"not-an-ip", true},
		{"999.999.999.999", true},
		{"1.2.3", true},
		{"1.2.3.4.5", true},
		{"abc.def.ghi.jkl", true},
	}

	for _, tt := range tests {
		err := validateIP(tt.ip)
		if (err != nil) != tt.wantErr {
			t.Errorf("validateIP(%q): err = %v, wantErr = %v", tt.ip, err, tt.wantErr)
		}
	}
}

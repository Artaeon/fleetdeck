package cmd

import "testing"

func TestParseBranchMap(t *testing.T) {
	tests := []struct {
		input string
		want  map[string]string
	}{
		{"main:production", map[string]string{"main": "production"}},
		{"main:production,develop:staging", map[string]string{"main": "production", "develop": "staging"}},
		{"main:production, develop:staging, feature:preview", map[string]string{"main": "production", "develop": "staging", "feature": "preview"}},
		{"", map[string]string{}},
	}

	for _, tt := range tests {
		got := parseBranchMap(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("parseBranchMap(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for k, v := range tt.want {
			if got[k] != v {
				t.Errorf("parseBranchMap(%q)[%q] = %q, want %q", tt.input, k, got[k], v)
			}
		}
	}
}

func TestSetupCDCommandRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "setup-cd" {
			found = true
			break
		}
	}
	if !found {
		t.Error("setup-cd command not registered")
	}
}

package server

import (
	"testing"

	"github.com/fleetdeck/fleetdeck/internal/db"
)

func TestResolveBranchEnvironment(t *testing.T) {
	tests := []struct {
		name     string
		project  *db.Project
		branch   string
		expected string
	}{
		{
			name:     "main branch defaults to production",
			project:  &db.Project{},
			branch:   "main",
			expected: "production",
		},
		{
			name:     "master branch defaults to production",
			project:  &db.Project{},
			branch:   "master",
			expected: "production",
		},
		{
			name:     "unmapped branch returns empty",
			project:  &db.Project{},
			branch:   "feature/login",
			expected: "",
		},
		{
			name: "explicit mapping overrides default",
			project: &db.Project{
				BranchMappings: `{"develop":"staging","main":"production"}`,
			},
			branch:   "develop",
			expected: "staging",
		},
		{
			name: "main with explicit mapping",
			project: &db.Project{
				BranchMappings: `{"main":"production","develop":"staging"}`,
			},
			branch:   "main",
			expected: "production",
		},
		{
			name: "unmapped branch with mappings configured",
			project: &db.Project{
				BranchMappings: `{"main":"production"}`,
			},
			branch:   "feature/test",
			expected: "",
		},
		{
			name: "invalid JSON mappings falls through to default",
			project: &db.Project{
				BranchMappings: "not-json",
			},
			branch:   "main",
			expected: "production",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveBranchEnvironment(tt.project, tt.branch)
			if got != tt.expected {
				t.Errorf("resolveBranchEnvironment(%q) = %q, want %q", tt.branch, got, tt.expected)
			}
		})
	}
}

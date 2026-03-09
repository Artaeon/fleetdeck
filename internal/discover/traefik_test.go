package discover

import (
	"testing"
)

func TestExtractDomain(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		expected string
	}{
		{
			name: "standard host rule",
			labels: map[string]string{
				"traefik.http.routers.myapp.rule": "Host(`myapp.example.com`)",
			},
			expected: "myapp.example.com",
		},
		{
			name: "multiple labels",
			labels: map[string]string{
				"traefik.enable":                  "true",
				"traefik.http.routers.web.rule":   "Host(`web.test.com`)",
				"traefik.http.services.web.port":  "3000",
			},
			expected: "web.test.com",
		},
		{
			name: "no traefik labels",
			labels: map[string]string{
				"com.docker.compose.project": "myapp",
			},
			expected: "",
		},
		{
			name:     "empty labels",
			labels:   map[string]string{},
			expected: "",
		},
		{
			name: "host rule without backticks",
			labels: map[string]string{
				"traefik.http.routers.app.rule": "Host(app.com)",
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractDomain(tt.labels)
			if got != tt.expected {
				t.Errorf("ExtractDomain() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestExtractDomainFromLabelsList(t *testing.T) {
	tests := []struct {
		name     string
		labels   []string
		expected string
	}{
		{
			name: "standard compose labels",
			labels: []string{
				"traefik.enable=true",
				"traefik.http.routers.myapp.rule=Host(`myapp.de`)",
				"traefik.http.routers.myapp.tls=true",
			},
			expected: "myapp.de",
		},
		{
			name: "no host rule",
			labels: []string{
				"traefik.enable=true",
			},
			expected: "",
		},
		{
			name:     "empty labels",
			labels:   []string{},
			expected: "",
		},
		{
			name:     "nil labels",
			labels:   nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractDomainFromLabelsList(tt.labels)
			if got != tt.expected {
				t.Errorf("ExtractDomainFromLabelsList() = %q, want %q", got, tt.expected)
			}
		})
	}
}

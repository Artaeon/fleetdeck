package dns

import "testing"

func TestIsMultiLevelTLD(t *testing.T) {
	tests := []struct {
		tld  string
		want bool
	}{
		{"co.uk", true},
		{"com.au", true},
		{"org.uk", true},
		{"co.jp", true},
		{"com.br", true},
		{"CO.UK", true},   // case insensitive
		{"Com.Au", true},  // mixed case
		{"com", false},
		{"uk", false},
		{"org", false},
		{"io", false},
		{"co.io", false},  // not in list
		{"", false},
		{"...", false},
	}

	for _, tt := range tests {
		t.Run(tt.tld, func(t *testing.T) {
			got := isMultiLevelTLD(tt.tld)
			if got != tt.want {
				t.Errorf("isMultiLevelTLD(%q) = %v, want %v", tt.tld, got, tt.want)
			}
		})
	}
}

func TestRootDomainMultiLevelTLD(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// Multi-level TLDs
		{"example.co.uk", "example.co.uk"},
		{"app.example.co.uk", "example.co.uk"},
		{"deep.sub.example.co.uk", "example.co.uk"},
		{"example.com.au", "example.com.au"},
		{"app.example.com.au", "example.com.au"},
		{"app.example.org.uk", "example.org.uk"},
		{"shop.example.co.jp", "example.co.jp"},

		// Standard TLDs (unchanged behavior)
		{"example.com", "example.com"},
		{"app.example.com", "example.com"},
		{"deep.sub.example.com", "example.com"},
		{"example.io", "example.io"},
		{"app.example.io", "example.io"},

		// Edge cases
		{"localhost", "localhost"},
		{"co.uk", "co.uk"},  // just the TLD itself
		{"a.co.uk", "a.co.uk"},  // minimal multi-level
		{"example.com.br", "example.com.br"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := rootDomain(tt.input)
			if got != tt.expected {
				t.Errorf("rootDomain(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

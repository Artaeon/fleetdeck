package cmd

import (
	"strings"
	"testing"
)

func TestValidateDomain(t *testing.T) {
	tests := []struct {
		name    string
		domain  string
		wantErr bool
	}{
		// --- Valid domains ---
		{name: "simple domain", domain: "example.com", wantErr: false},
		{name: "subdomain", domain: "sub.example.com", wantErr: false},
		{name: "hyphenated subdomain", domain: "my-app.example.co", wantErr: false},
		{name: "deeply nested subdomain", domain: "a.b.c.d.example.com", wantErr: false},
		{name: "two-letter TLD", domain: "example.io", wantErr: false},
		{name: "numeric subdomain", domain: "123.example.com", wantErr: false},
		{name: "long domain", domain: "very-long-subdomain.with-many-parts.example.com", wantErr: false},
		{name: "wildcard subdomain", domain: "*.example.com", wantErr: false},
		{name: "country TLD", domain: "example.co.uk", wantErr: false},

		// --- Invalid: empty ---
		{name: "empty string", domain: "", wantErr: true},

		// --- Invalid: no dot ---
		{name: "no dot", domain: "nodot", wantErr: true},
		{name: "single word", domain: "localhost", wantErr: true},

		// --- Invalid: dangerous characters ---
		{name: "space", domain: "has space.com", wantErr: true},
		{name: "tab", domain: "has\tspace.com", wantErr: true},
		{name: "newline", domain: "has\nnewline.com", wantErr: true},
		{name: "semicolon", domain: "semi;colon.com", wantErr: true},
		{name: "backtick", domain: "back`tick.com", wantErr: true},
		{name: "dollar sign", domain: "dollar$sign.com", wantErr: true},
		{name: "open paren", domain: "paren(.com", wantErr: true},
		{name: "close paren", domain: "paren).com", wantErr: true},
		{name: "double quote", domain: "quote\".com", wantErr: true},
		{name: "single quote", domain: "quote'.com", wantErr: true},
		{name: "backslash", domain: "back\\slash.com", wantErr: true},
		{name: "curly brace open", domain: "brace{.com", wantErr: true},
		{name: "curly brace close", domain: "brace}.com", wantErr: true},

		// --- Invalid: injection attempts ---
		{name: "command injection semicolon", domain: "example.com; rm -rf /", wantErr: true},
		{name: "command substitution", domain: "$(whoami).com", wantErr: true},
		{name: "backtick substitution", domain: "`id`.com", wantErr: true},
		{name: "pipe injection", domain: "example.com|cat /etc/passwd", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDomain(tt.domain)
			if tt.wantErr && err == nil {
				t.Errorf("validateDomain(%q) = nil, want error", tt.domain)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("validateDomain(%q) = %v, want nil", tt.domain, err)
			}
		})
	}
}

func TestValidateDomainErrorMessages(t *testing.T) {
	t.Run("empty domain mentions empty", func(t *testing.T) {
		err := validateDomain("")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "empty") {
			t.Errorf("error should mention 'empty', got: %v", err)
		}
	})

	t.Run("no dot mentions dot", func(t *testing.T) {
		err := validateDomain("nodot")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "dot") {
			t.Errorf("error should mention 'dot', got: %v", err)
		}
	})

	t.Run("invalid chars mentions invalid", func(t *testing.T) {
		err := validateDomain("semi;colon.com")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "invalid") {
			t.Errorf("error should mention 'invalid', got: %v", err)
		}
	})
}

func TestValidateIP(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		wantErr bool
	}{
		// --- Valid IPv4 ---
		{name: "simple IPv4", ip: "1.2.3.4", wantErr: false},
		{name: "private class A", ip: "10.0.0.1", wantErr: false},
		{name: "private class C", ip: "192.168.1.1", wantErr: false},
		{name: "all 255s", ip: "255.255.255.255", wantErr: false},
		{name: "all zeros", ip: "0.0.0.0", wantErr: false},
		{name: "loopback", ip: "127.0.0.1", wantErr: false},
		{name: "class B", ip: "172.16.0.1", wantErr: false},

		// --- Valid IPv6 ---
		{name: "IPv6 loopback", ip: "::1", wantErr: false},
		{name: "IPv6 documentation", ip: "2001:db8::1", wantErr: false},
		{name: "IPv6 full", ip: "2001:0db8:85a3:0000:0000:8a2e:0370:7334", wantErr: false},
		{name: "IPv6 all zeros", ip: "::", wantErr: false},
		{name: "IPv6 link-local", ip: "fe80::1", wantErr: false},

		// --- Invalid ---
		{name: "empty string", ip: "", wantErr: true},
		{name: "not an IP", ip: "not-an-ip", wantErr: true},
		{name: "octets too large", ip: "999.999.999.999", wantErr: true},
		{name: "too few octets", ip: "1.2.3", wantErr: true},
		{name: "too many octets", ip: "1.2.3.4.5", wantErr: true},
		{name: "alphabetic", ip: "abc", wantErr: true},
		{name: "IP with port", ip: "1.2.3.4:80", wantErr: true},
		{name: "domain name", ip: "example.com", wantErr: true},
		{name: "IPv4 with leading zeros", ip: "01.02.03.04", wantErr: true},
		{name: "CIDR notation", ip: "192.168.1.0/24", wantErr: true},
		{name: "negative octet", ip: "-1.2.3.4", wantErr: true},
		{name: "IPv4 single octet too large", ip: "256.1.1.1", wantErr: true},
		{name: "spaces", ip: " 1.2.3.4 ", wantErr: true},
		{name: "IPv6 with port", ip: "[::1]:80", wantErr: true},

		// --- Injection attempts via IP field ---
		{name: "semicolon injection", ip: "1.2.3.4; rm -rf /", wantErr: true},
		{name: "command substitution", ip: "$(whoami)", wantErr: true},
		{name: "backtick injection", ip: "`id`", wantErr: true},
		{name: "pipe injection", ip: "1.2.3.4|cat /etc/passwd", wantErr: true},
		{name: "newline injection", ip: "1.2.3.4\n127.0.0.1", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateIP(tt.ip)
			if tt.wantErr && err == nil {
				t.Errorf("validateIP(%q) = nil, want error", tt.ip)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("validateIP(%q) = %v, want nil", tt.ip, err)
			}
		})
	}
}

func TestValidateIPErrorMessage(t *testing.T) {
	err := validateIP("not-valid")
	if err == nil {
		t.Fatal("expected error")
	}
	// Error should include the invalid input.
	if !strings.Contains(err.Error(), "not-valid") {
		t.Errorf("error should include the invalid input, got: %v", err)
	}
	if !strings.Contains(err.Error(), "not a valid IP") {
		t.Errorf("error should mention 'not a valid IP', got: %v", err)
	}
}

func TestValidateDomainAndIPTogether(t *testing.T) {
	// Simulate the validation flow from dnsSetupCmd:
	// both domain and IP must be valid for the command to proceed.
	validPairs := []struct {
		domain string
		ip     string
	}{
		{"example.com", "1.2.3.4"},
		{"sub.example.com", "10.0.0.1"},
		{"app.example.co.uk", "::1"},
	}

	for _, pair := range validPairs {
		if err := validateDomain(pair.domain); err != nil {
			t.Errorf("validateDomain(%q) should pass, got: %v", pair.domain, err)
		}
		if err := validateIP(pair.ip); err != nil {
			t.Errorf("validateIP(%q) should pass, got: %v", pair.ip, err)
		}
	}

	// Invalid domain with valid IP should still fail domain validation.
	if err := validateDomain(""); err == nil {
		t.Error("empty domain should fail validation")
	}
	if err := validateIP("1.2.3.4"); err != nil {
		t.Error("valid IP should pass when domain fails")
	}

	// Valid domain with invalid IP should still fail IP validation.
	if err := validateDomain("example.com"); err != nil {
		t.Error("valid domain should pass when IP fails")
	}
	if err := validateIP("not-valid"); err == nil {
		t.Error("invalid IP should fail validation")
	}
}

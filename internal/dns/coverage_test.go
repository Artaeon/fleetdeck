package dns

import (
	"fmt"
	"strings"
	"testing"
)

// TestAutoConfigureBothRecordsSucceed verifies that when both CreateRecord
// calls succeed, AutoConfigure returns no error.
func TestAutoConfigureBothRecordsSucceed(t *testing.T) {
	mock := &mockDNSProvider{}

	err := AutoConfigure("example.com", "10.0.0.1", mock)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(mock.createCalls) != 2 {
		t.Fatalf("expected 2 CreateRecord calls, got %d", len(mock.createCalls))
	}
}

// TestAutoConfigureRootFails verifies that when the first CreateRecord call
// (root domain) fails, the error mentions the root domain.
func TestAutoConfigureRootFails(t *testing.T) {
	mock := &mockDNSProvider{
		createErr: fmt.Errorf("API timeout"),
	}

	err := AutoConfigure("example.com", "10.0.0.1", mock)
	if err == nil {
		t.Fatal("expected error when root record creation fails")
	}

	if !strings.Contains(err.Error(), "example.com") {
		t.Errorf("expected error to mention root domain 'example.com', got: %v", err)
	}

	// Only one call should have been made (failed on first).
	if len(mock.createCalls) != 1 {
		t.Errorf("expected 1 CreateRecord call, got %d", len(mock.createCalls))
	}
}

// TestAutoConfigureWildcardFails verifies that when the first call succeeds
// but the second (wildcard) fails, the error mentions the wildcard.
func TestAutoConfigureWildcardFails(t *testing.T) {
	provider := &failOnNthProvider{failOnCall: 2}

	err := AutoConfigure("example.com", "10.0.0.1", provider)
	if err == nil {
		t.Fatal("expected error when wildcard record creation fails")
	}

	if !strings.Contains(err.Error(), "*.example.com") {
		t.Errorf("expected error to mention wildcard '*.example.com', got: %v", err)
	}

	if len(provider.createCalls) != 2 {
		t.Errorf("expected 2 CreateRecord calls, got %d", len(provider.createCalls))
	}
}

// TestAutoConfigureRecordValues verifies the exact arguments passed to
// CreateRecord for both the root and wildcard records.
func TestAutoConfigureRecordValues(t *testing.T) {
	mock := &mockDNSProvider{}
	domain := "fleet.example.com"
	serverIP := "192.168.1.100"

	err := AutoConfigure(domain, serverIP, mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.createCalls) != 2 {
		t.Fatalf("expected 2 CreateRecord calls, got %d", len(mock.createCalls))
	}

	// Root record.
	root := mock.createCalls[0]
	if root.domain != domain {
		t.Errorf("root: expected domain %q, got %q", domain, root.domain)
	}
	if root.recordType != "A" {
		t.Errorf("root: expected recordType %q, got %q", "A", root.recordType)
	}
	if root.name != domain {
		t.Errorf("root: expected name %q, got %q", domain, root.name)
	}
	if root.value != serverIP {
		t.Errorf("root: expected value %q, got %q", serverIP, root.value)
	}
	if root.ttl != 300 {
		t.Errorf("root: expected TTL 300, got %d", root.ttl)
	}

	// Wildcard record.
	wc := mock.createCalls[1]
	if wc.domain != domain {
		t.Errorf("wildcard: expected domain %q, got %q", domain, wc.domain)
	}
	if wc.recordType != "A" {
		t.Errorf("wildcard: expected recordType %q, got %q", "A", wc.recordType)
	}
	expectedWildcard := "*." + domain
	if wc.name != expectedWildcard {
		t.Errorf("wildcard: expected name %q, got %q", expectedWildcard, wc.name)
	}
	if wc.value != serverIP {
		t.Errorf("wildcard: expected value %q, got %q", serverIP, wc.value)
	}
	if wc.ttl != 300 {
		t.Errorf("wildcard: expected TTL 300, got %d", wc.ttl)
	}
}

// TestAutoConfigureWildcardFormat verifies that the wildcard record name is
// formatted as "*.domain".
func TestAutoConfigureWildcardFormat(t *testing.T) {
	tests := []struct {
		domain           string
		expectedWildcard string
	}{
		{"example.com", "*.example.com"},
		{"sub.example.com", "*.sub.example.com"},
		{"deep.nested.example.com", "*.deep.nested.example.com"},
		{"my-app.io", "*.my-app.io"},
	}

	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			mock := &mockDNSProvider{}
			err := AutoConfigure(tt.domain, "1.2.3.4", mock)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(mock.createCalls) < 2 {
				t.Fatalf("expected at least 2 calls, got %d", len(mock.createCalls))
			}

			wildcardName := mock.createCalls[1].name
			if wildcardName != tt.expectedWildcard {
				t.Errorf("expected wildcard name %q, got %q", tt.expectedWildcard, wildcardName)
			}
		})
	}
}

// TestGetProviderCloudflare verifies that "cloudflare" returns a non-nil
// CloudflareProvider.
func TestGetProviderCloudflare(t *testing.T) {
	provider, err := GetProvider("cloudflare", "valid-api-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider == nil {
		t.Fatal("expected non-nil provider for 'cloudflare'")
	}

	cf, ok := provider.(*CloudflareProvider)
	if !ok {
		t.Fatalf("expected *CloudflareProvider, got %T", provider)
	}
	if cf.apiToken != "valid-api-token" {
		t.Errorf("expected apiToken %q, got %q", "valid-api-token", cf.apiToken)
	}
}

// TestGetProviderUnknown is a table-driven test verifying that various
// invalid provider names return errors.
func TestGetProviderUnknown(t *testing.T) {
	unknownProviders := []struct {
		name string
	}{
		{"aws"},
		{"route53"},
		{"digitalocean"},
		{""},
		{" "},
	}

	for _, tt := range unknownProviders {
		label := tt.name
		if label == "" {
			label = "(empty)"
		} else if strings.TrimSpace(label) == "" {
			label = "(whitespace)"
		}

		t.Run(label, func(t *testing.T) {
			provider, err := GetProvider(tt.name, "some-token")
			if err == nil {
				t.Errorf("expected error for unknown provider %q", tt.name)
			}
			if provider != nil {
				t.Errorf("expected nil provider for unknown name %q, got %T", tt.name, provider)
			}
		})
	}
}

// TestGetProviderCaseSensitive verifies that the provider lookup is
// case-sensitive: "Cloudflare" and "CLOUDFLARE" should return errors.
func TestGetProviderCaseSensitive(t *testing.T) {
	casedNames := []string{"Cloudflare", "CLOUDFLARE", "CloudFlare", "cloudFLARE"}

	for _, name := range casedNames {
		t.Run(name, func(t *testing.T) {
			provider, err := GetProvider(name, "token")
			if err == nil {
				t.Errorf("expected error for case-variant provider name %q", name)
			}
			if provider != nil {
				t.Errorf("expected nil provider for %q, got %T", name, provider)
			}
		})
	}
}

// TestProviderInterfaceCompliance verifies that CloudflareProvider implements
// the Provider interface at compile time.
func TestProviderInterfaceCompliance(t *testing.T) {
	p, err := NewCloudflareProvider("test-token")
	if err != nil {
		t.Fatalf("unexpected error creating CloudflareProvider: %v", err)
	}

	// Compile-time interface satisfaction check.
	var _ Provider = p
}

// TestAutoConfigureWithNilProvider verifies the behavior when a nil provider
// is passed to AutoConfigure. This should panic since the method call on
// a nil interface will cause a nil pointer dereference.
func TestAutoConfigureWithNilProvider(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic when calling AutoConfigure with nil provider")
		}
	}()

	_ = AutoConfigure("example.com", "1.2.3.4", nil)
}

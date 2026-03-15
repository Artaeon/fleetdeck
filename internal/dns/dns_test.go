package dns

import (
	"fmt"
	"testing"
)

// mockDNSProvider records all operations for later inspection.
type mockDNSProvider struct {
	records       []Record
	createCalls   []createCall
	deleteCalls   []deleteCall
	listCalls     []string
	createErr     error
	deleteErr     error
	listErr       error
}

type createCall struct {
	domain     string
	recordType string
	name       string
	value      string
	ttl        int
}

type deleteCall struct {
	domain     string
	recordType string
	name       string
}

func (m *mockDNSProvider) CreateRecord(domain, recordType, name, value string, ttl int) error {
	m.createCalls = append(m.createCalls, createCall{
		domain:     domain,
		recordType: recordType,
		name:       name,
		value:      value,
		ttl:        ttl,
	})
	return m.createErr
}

func (m *mockDNSProvider) DeleteRecord(domain, recordType, name string) error {
	m.deleteCalls = append(m.deleteCalls, deleteCall{
		domain:     domain,
		recordType: recordType,
		name:       name,
	})
	return m.deleteErr
}

func (m *mockDNSProvider) ListRecords(domain string) ([]Record, error) {
	m.listCalls = append(m.listCalls, domain)
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.records, nil
}

func (m *mockDNSProvider) Name() string { return "mock" }

func TestGetProvider(t *testing.T) {
	provider, err := GetProvider("cloudflare", "test-api-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider == nil {
		t.Fatal("expected non-nil provider")
	}
	if provider.Name() != "cloudflare" {
		t.Errorf("expected provider name %q, got %q", "cloudflare", provider.Name())
	}

	// Verify it's the correct concrete type.
	cf, ok := provider.(*CloudflareProvider)
	if !ok {
		t.Fatalf("expected *CloudflareProvider, got %T", provider)
	}
	if cf.apiToken != "test-api-token" {
		t.Errorf("expected apiToken %q, got %q", "test-api-token", cf.apiToken)
	}
	if cf.httpClient == nil {
		t.Error("expected non-nil httpClient")
	}
}

func TestGetProviderEmptyToken(t *testing.T) {
	// Cloudflare provider requires a non-empty token.
	_, err := GetProvider("cloudflare", "")
	if err == nil {
		t.Error("expected error for empty API token")
	}
}

func TestGetProviderInvalid(t *testing.T) {
	invalidNames := []string{
		"invalid",
		"route53",
		"gcloud",
		"digitalocean",
		"Cloudflare", // case sensitive
		"CLOUDFLARE", // case sensitive
		"",
	}

	for _, name := range invalidNames {
		t.Run(name, func(t *testing.T) {
			provider, err := GetProvider(name, "token")

			if err == nil {
				t.Errorf("expected error for invalid provider %q, got nil", name)
			}
			if provider != nil {
				t.Errorf("expected nil provider for invalid name %q, got %T", name, provider)
			}
		})
	}
}

func TestRootDomain(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"example.com", "example.com"},
		{"www.example.com", "example.com"},
		{"app.staging.example.com", "example.com"},
		{"a.b.c.d.example.com", "example.com"},
		{"localhost", "localhost"},
		{"my-app.io", "my-app.io"},
		{"sub.my-app.io", "my-app.io"},
		{"deep.nested.sub.domain.co.uk", "domain.co.uk"},
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

func TestAutoConfigureSuccess(t *testing.T) {
	mock := &mockDNSProvider{}
	domain := "example.com"
	serverIP := "1.2.3.4"

	err := AutoConfigure(domain, serverIP, mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.createCalls) != 2 {
		t.Fatalf("expected 2 CreateRecord calls, got %d", len(mock.createCalls))
	}

	// First call: root A record.
	first := mock.createCalls[0]
	if first.domain != "example.com" {
		t.Errorf("first call: expected domain %q, got %q", "example.com", first.domain)
	}
	if first.recordType != "A" {
		t.Errorf("first call: expected type %q, got %q", "A", first.recordType)
	}
	if first.name != "example.com" {
		t.Errorf("first call: expected name %q, got %q", "example.com", first.name)
	}
	if first.value != "1.2.3.4" {
		t.Errorf("first call: expected value %q, got %q", "1.2.3.4", first.value)
	}
	if first.ttl != 300 {
		t.Errorf("first call: expected TTL 300, got %d", first.ttl)
	}

	// Second call: wildcard A record.
	second := mock.createCalls[1]
	if second.domain != "example.com" {
		t.Errorf("second call: expected domain %q, got %q", "example.com", second.domain)
	}
	if second.recordType != "A" {
		t.Errorf("second call: expected type %q, got %q", "A", second.recordType)
	}
	if second.name != "*.example.com" {
		t.Errorf("second call: expected name %q, got %q", "*.example.com", second.name)
	}
	if second.value != "1.2.3.4" {
		t.Errorf("second call: expected value %q, got %q", "1.2.3.4", second.value)
	}
	if second.ttl != 300 {
		t.Errorf("second call: expected TTL 300, got %d", second.ttl)
	}
}

func TestAutoConfigureRootRecordFailure(t *testing.T) {
	mock := &mockDNSProvider{
		createErr: fmt.Errorf("API error: rate limited"),
	}

	err := AutoConfigure("example.com", "1.2.3.4", mock)
	if err == nil {
		t.Fatal("expected error when root A record creation fails")
	}

	// Should have attempted only the first call before failing.
	if len(mock.createCalls) != 1 {
		t.Errorf("expected 1 CreateRecord call (failed on first), got %d", len(mock.createCalls))
	}
}

func TestAutoConfigureWildcardRecordFailure(t *testing.T) {
	callCount := 0
	mock := &mockDNSProvider{}
	// Override to fail only on second call.
	originalCreateErr := mock.createErr
	mock.createErr = originalCreateErr

	// Use a custom mock that fails on the second call.
	failOnSecond := &failOnNthProvider{failOnCall: 2}

	err := AutoConfigure("example.com", "1.2.3.4", failOnSecond)
	if err == nil {
		t.Fatal("expected error when wildcard record creation fails")
	}
	_ = callCount

	if len(failOnSecond.createCalls) != 2 {
		t.Errorf("expected 2 CreateRecord calls, got %d", len(failOnSecond.createCalls))
	}
}

// failOnNthProvider fails CreateRecord on the Nth call.
type failOnNthProvider struct {
	failOnCall  int
	createCalls []createCall
}

func (f *failOnNthProvider) CreateRecord(domain, recordType, name, value string, ttl int) error {
	f.createCalls = append(f.createCalls, createCall{domain, recordType, name, value, ttl})
	if len(f.createCalls) == f.failOnCall {
		return fmt.Errorf("simulated failure on call %d", f.failOnCall)
	}
	return nil
}

func (f *failOnNthProvider) DeleteRecord(domain, recordType, name string) error { return nil }
func (f *failOnNthProvider) ListRecords(domain string) ([]Record, error)        { return nil, nil }
func (f *failOnNthProvider) Name() string                                       { return "fail-on-nth" }

func TestAutoConfigureSubdomain(t *testing.T) {
	mock := &mockDNSProvider{}
	domain := "app.staging.example.com"
	serverIP := "10.0.0.1"

	err := AutoConfigure(domain, serverIP, mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.createCalls) != 2 {
		t.Fatalf("expected 2 CreateRecord calls, got %d", len(mock.createCalls))
	}

	// Root A record uses the full domain.
	if mock.createCalls[0].name != "app.staging.example.com" {
		t.Errorf("expected root record name %q, got %q", "app.staging.example.com", mock.createCalls[0].name)
	}

	// Wildcard prepends *. to the full domain.
	if mock.createCalls[1].name != "*.app.staging.example.com" {
		t.Errorf("expected wildcard record name %q, got %q", "*.app.staging.example.com", mock.createCalls[1].name)
	}
}

func TestRecordStruct(t *testing.T) {
	record := Record{
		ID:      "abc123",
		Type:    "A",
		Name:    "example.com",
		Value:   "1.2.3.4",
		TTL:     300,
		Proxied: true,
	}

	if record.ID != "abc123" {
		t.Errorf("expected ID %q, got %q", "abc123", record.ID)
	}
	if record.Type != "A" {
		t.Errorf("expected Type %q, got %q", "A", record.Type)
	}
	if record.Name != "example.com" {
		t.Errorf("expected Name %q, got %q", "example.com", record.Name)
	}
	if record.Value != "1.2.3.4" {
		t.Errorf("expected Value %q, got %q", "1.2.3.4", record.Value)
	}
	if record.TTL != 300 {
		t.Errorf("expected TTL 300, got %d", record.TTL)
	}
	if !record.Proxied {
		t.Error("expected Proxied=true")
	}
}

func TestGetProviderErrorMessage(t *testing.T) {
	_, err := GetProvider("nonexistent", "token")
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}

	errMsg := err.Error()
	if errMsg == "" {
		t.Error("expected non-empty error message")
	}
}

func TestNewCloudflareProvider(t *testing.T) {
	p, err := NewCloudflareProvider("my-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
	if p.apiToken != "my-token" {
		t.Errorf("expected apiToken %q, got %q", "my-token", p.apiToken)
	}
	if p.httpClient == nil {
		t.Error("expected non-nil httpClient")
	}
	if p.Name() != "cloudflare" {
		t.Errorf("expected Name() = %q, got %q", "cloudflare", p.Name())
	}
}

func TestNewCloudflareProviderEmptyToken(t *testing.T) {
	p, err := NewCloudflareProvider("")
	if err == nil {
		t.Error("expected error for empty token")
	}
	if p != nil {
		t.Error("expected nil provider for empty token")
	}

	p2, err2 := NewCloudflareProvider("   ")
	if err2 == nil {
		t.Error("expected error for whitespace-only token")
	}
	if p2 != nil {
		t.Error("expected nil provider for whitespace-only token")
	}
}

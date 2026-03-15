package dns

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestCloudflareProvider creates a CloudflareProvider whose httpClient
// points at the given httptest server. Because we are in the same package we
// can set the unexported fields directly.
func newTestCloudflareProvider(serverURL string) *CloudflareProvider {
	return &CloudflareProvider{
		apiToken:   "test-token",
		httpClient: http.DefaultClient,
	}
}

// rewriteTransport rewrites every outgoing request so that
// cloudflareAPIBase is replaced with the test server URL. This lets us
// intercept calls made via the const-based URL without changing production
// code.
type rewriteTransport struct {
	base      http.RoundTripper
	targetURL string // e.g. "http://127.0.0.1:PORT"
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Replace the Cloudflare API base with the test server URL.
	newURL := strings.Replace(req.URL.String(), cloudflareAPIBase, t.targetURL, 1)
	newReq, err := http.NewRequest(req.Method, newURL, req.Body)
	if err != nil {
		return nil, err
	}
	newReq.Header = req.Header
	return t.base.RoundTrip(newReq)
}

// cfProvider returns a CloudflareProvider wired to the given test server.
func cfProvider(serverURL string) *CloudflareProvider {
	return &CloudflareProvider{
		apiToken: "test-token",
		httpClient: &http.Client{
			Transport: &rewriteTransport{
				base:      http.DefaultTransport,
				targetURL: serverURL,
			},
		},
	}
}

// ---------- Test: CreateRecord ----------

func TestCloudflareCreateRecord(t *testing.T) {
	var (
		gotMethod string
		gotPath   string
		gotBody   map[string]any
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Zone lookup
		if strings.Contains(r.URL.Path, "/zones") && r.URL.Query().Get("name") != "" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(cfAPIResponse{
				Success: true,
				Result:  mustMarshal([]cfZone{{ID: "zone-123", Name: "example.com"}}),
			})
			return
		}

		// Create record
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/dns_records") {
			gotMethod = r.Method
			gotPath = r.URL.Path
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &gotBody)

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(cfAPIResponse{
				Success: true,
				Result:  mustMarshal(map[string]string{"id": "rec-1"}),
			})
			return
		}

		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := cfProvider(srv.URL)
	err := p.CreateRecord("example.com", "A", "app.example.com", "1.2.3.4", 300)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", gotMethod)
	}
	if !strings.HasSuffix(gotPath, "/zones/zone-123/dns_records") {
		t.Errorf("unexpected path: %s", gotPath)
	}
	if gotBody["type"] != "A" {
		t.Errorf("expected type=A, got %v", gotBody["type"])
	}
	if gotBody["name"] != "app.example.com" {
		t.Errorf("expected name=app.example.com, got %v", gotBody["name"])
	}
	if gotBody["content"] != "1.2.3.4" {
		t.Errorf("expected content=1.2.3.4, got %v", gotBody["content"])
	}
	// JSON numbers decode as float64
	if ttl, ok := gotBody["ttl"].(float64); !ok || int(ttl) != 300 {
		t.Errorf("expected ttl=300, got %v", gotBody["ttl"])
	}
}

// ---------- Test: DeleteRecord ----------

func TestCloudflareDeleteRecord(t *testing.T) {
	var (
		deleteMethod string
		deletePath   string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Zone lookup
		if r.URL.Query().Get("name") != "" && r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(cfAPIResponse{
				Success: true,
				Result:  mustMarshal([]cfZone{{ID: "zone-abc", Name: "example.com"}}),
			})
			return
		}

		// List records
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/dns_records") {
			json.NewEncoder(w).Encode(cfAPIResponse{
				Success: true,
				Result: mustMarshal([]cfRecord{
					{ID: "rec-1", Type: "A", Name: "example.com", Content: "1.2.3.4", TTL: 300},
					{ID: "rec-2", Type: "CNAME", Name: "www.example.com", Content: "example.com", TTL: 300},
				}),
			})
			return
		}

		// Delete record
		if r.Method == http.MethodDelete {
			deleteMethod = r.Method
			deletePath = r.URL.Path
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(cfAPIResponse{Success: true})
			return
		}

		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := cfProvider(srv.URL)
	err := p.DeleteRecord("example.com", "A", "example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if deleteMethod != http.MethodDelete {
		t.Errorf("expected DELETE, got %s", deleteMethod)
	}
	if !strings.Contains(deletePath, "/zones/zone-abc/dns_records/rec-1") {
		t.Errorf("expected delete path to contain zone-abc and rec-1, got %s", deletePath)
	}
}

func TestCloudflareDeleteRecordNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Zone lookup
		if r.URL.Query().Get("name") != "" {
			json.NewEncoder(w).Encode(cfAPIResponse{
				Success: true,
				Result:  mustMarshal([]cfZone{{ID: "zone-1", Name: "example.com"}}),
			})
			return
		}

		// List records - return empty
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/dns_records") {
			json.NewEncoder(w).Encode(cfAPIResponse{
				Success: true,
				Result:  mustMarshal([]cfRecord{}),
			})
			return
		}
	}))
	defer srv.Close()

	p := cfProvider(srv.URL)
	err := p.DeleteRecord("example.com", "TXT", "nonexistent.example.com")
	if err == nil {
		t.Fatal("expected error when record not found")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

// ---------- Test: ListRecords ----------

func TestCloudflareListRecords(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Zone lookup
		if r.URL.Query().Get("name") != "" {
			json.NewEncoder(w).Encode(cfAPIResponse{
				Success: true,
				Result:  mustMarshal([]cfZone{{ID: "zone-42", Name: "example.com"}}),
			})
			return
		}

		// List records
		if strings.Contains(r.URL.Path, "/dns_records") {
			json.NewEncoder(w).Encode(cfAPIResponse{
				Success: true,
				Result: mustMarshal([]cfRecord{
					{ID: "r1", Type: "A", Name: "example.com", Content: "10.0.0.1", TTL: 300, Proxied: true},
					{ID: "r2", Type: "CNAME", Name: "www.example.com", Content: "example.com", TTL: 600, Proxied: false},
					{ID: "r3", Type: "MX", Name: "example.com", Content: "mail.example.com", TTL: 3600, Proxied: false},
				}),
			})
			return
		}
	}))
	defer srv.Close()

	p := cfProvider(srv.URL)
	records, err := p.ListRecords("example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}

	// Verify first record is correctly mapped.
	if records[0].ID != "r1" {
		t.Errorf("expected ID r1, got %s", records[0].ID)
	}
	if records[0].Type != "A" {
		t.Errorf("expected Type A, got %s", records[0].Type)
	}
	if records[0].Value != "10.0.0.1" {
		t.Errorf("expected Value 10.0.0.1, got %s", records[0].Value)
	}
	if records[0].TTL != 300 {
		t.Errorf("expected TTL 300, got %d", records[0].TTL)
	}
	if !records[0].Proxied {
		t.Error("expected Proxied=true for first record")
	}

	// Verify second record type mapping.
	if records[1].Type != "CNAME" {
		t.Errorf("expected CNAME, got %s", records[1].Type)
	}
	if records[1].Value != "example.com" {
		t.Errorf("expected content example.com, got %s", records[1].Value)
	}
	if records[1].Proxied {
		t.Error("expected Proxied=false for second record")
	}
}

// ---------- Test: FindZoneID ----------

func TestCloudflareFindZoneID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		qName := r.URL.Query().Get("name")
		if qName != "example.com" {
			t.Errorf("expected zone query for example.com, got %s", qName)
		}

		json.NewEncoder(w).Encode(cfAPIResponse{
			Success: true,
			Result:  mustMarshal([]cfZone{{ID: "zone-found-id", Name: "example.com"}}),
		})
	}))
	defer srv.Close()

	p := cfProvider(srv.URL)
	id, err := p.findZoneID("sub.example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "zone-found-id" {
		t.Errorf("expected zone-found-id, got %s", id)
	}
}

// ---------- Test: FindZoneID not found ----------

func TestCloudflareFindZoneIDNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cfAPIResponse{
			Success: true,
			Result:  mustMarshal([]cfZone{}), // empty
		})
	}))
	defer srv.Close()

	p := cfProvider(srv.URL)
	_, err := p.findZoneID("nonexistent.example.com")
	if err == nil {
		t.Fatal("expected error when no zone found")
	}
	if !strings.Contains(err.Error(), "no zone found") {
		t.Errorf("expected 'no zone found' in error, got: %v", err)
	}
}

// ---------- Test: API error response ----------

func TestCloudflareAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Zone lookup succeeds
		if r.URL.Query().Get("name") != "" {
			json.NewEncoder(w).Encode(cfAPIResponse{
				Success: true,
				Result:  mustMarshal([]cfZone{{ID: "z1", Name: "example.com"}}),
			})
			return
		}

		// Create record fails with API errors
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(cfAPIResponse{
			Success: false,
			Errors: []cfAPIError{
				{Code: 9109, Message: "Invalid access token"},
			},
		})
	}))
	defer srv.Close()

	p := cfProvider(srv.URL)
	err := p.CreateRecord("example.com", "A", "example.com", "1.2.3.4", 300)
	if err == nil {
		t.Fatal("expected error on API failure")
	}
	if !strings.Contains(err.Error(), "9109") {
		t.Errorf("expected error to contain code 9109, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Invalid access token") {
		t.Errorf("expected error to contain message, got: %v", err)
	}
}

func TestCloudflareAPIErrorZoneLookup(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cfAPIResponse{
			Success: false,
			Errors: []cfAPIError{
				{Code: 6003, Message: "Invalid request headers"},
				{Code: 6103, Message: "Invalid format for Authorization header"},
			},
		})
	}))
	defer srv.Close()

	p := cfProvider(srv.URL)
	_, err := p.ListRecords("example.com")
	if err == nil {
		t.Fatal("expected error on zone lookup API failure")
	}
	if !strings.Contains(err.Error(), "6003") {
		t.Errorf("expected error to contain code 6003, got: %v", err)
	}
}

// ---------- Test: Empty token ----------

func TestCloudflareEmptyToken(t *testing.T) {
	p, err := NewCloudflareProvider("")
	if err == nil {
		t.Error("expected error for empty token")
	}
	if p != nil {
		t.Error("expected nil provider for empty token")
	}
}

// ---------- Test: Whitespace token ----------

func TestCloudflareWhitespaceToken(t *testing.T) {
	tests := []string{"  ", "\t", " \t ", "\n"}
	for _, tok := range tests {
		p, err := NewCloudflareProvider(tok)
		if err == nil {
			t.Errorf("expected error for whitespace token %q", tok)
		}
		if p != nil {
			t.Errorf("expected nil provider for whitespace token %q", tok)
		}
	}
}

// ---------- Test: rootDomain (table-driven) ----------

func TestRootDomainCases(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"example.com", "example.com"},
		{"sub.example.com", "example.com"},
		{"a.b.c.example.com", "example.com"},
		{"com", "com"},
		{"localhost", "localhost"},
		{"deep.nested.sub.domain.example.com", "example.com"},
		{"my-app.io", "my-app.io"},
		{"staging.my-app.io", "my-app.io"},
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

// ---------- Test: formatCFErrors ----------

func TestFormatCFErrors(t *testing.T) {
	t.Run("zero errors", func(t *testing.T) {
		result := formatCFErrors(nil)
		if result != "" {
			t.Errorf("expected empty string for nil errors, got %q", result)
		}
	})

	t.Run("one error", func(t *testing.T) {
		result := formatCFErrors([]cfAPIError{
			{Code: 1001, Message: "Invalid zone identifier"},
		})
		if result != "[1001] Invalid zone identifier" {
			t.Errorf("unexpected format: %q", result)
		}
	})

	t.Run("two errors", func(t *testing.T) {
		result := formatCFErrors([]cfAPIError{
			{Code: 6003, Message: "Invalid request headers"},
			{Code: 6103, Message: "Invalid format for Authorization header"},
		})
		expected := "[6003] Invalid request headers; [6103] Invalid format for Authorization header"
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		result := formatCFErrors([]cfAPIError{})
		if result != "" {
			t.Errorf("expected empty string for empty slice, got %q", result)
		}
	})
}

// ---------- Test: Name() method ----------

func TestCloudflareName(t *testing.T) {
	p, err := NewCloudflareProvider("valid-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "cloudflare" {
		t.Errorf("expected Name()=%q, got %q", "cloudflare", p.Name())
	}
}

// ---------- Test: Provider interface compliance ----------

func TestCloudflareImplementsProvider(t *testing.T) {
	p, err := NewCloudflareProvider("token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Compile-time check: *CloudflareProvider satisfies the Provider interface.
	var _ Provider = p
}

// ---------- Test: Authorization header ----------

func TestCloudflareAuthorizationHeader(t *testing.T) {
	var gotAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cfAPIResponse{
			Success: true,
			Result:  mustMarshal([]cfZone{{ID: "z1", Name: "example.com"}}),
		})
	}))
	defer srv.Close()

	p := cfProvider(srv.URL)
	p.apiToken = "my-secret-token"
	_, _ = p.findZoneID("example.com")

	if gotAuth != "Bearer my-secret-token" {
		t.Errorf("expected Authorization header %q, got %q", "Bearer my-secret-token", gotAuth)
	}
}

// ---------- Test: Content-Type header ----------

func TestCloudflareContentTypeHeader(t *testing.T) {
	var gotContentType string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			gotContentType = r.Header.Get("Content-Type")
		}
		w.Header().Set("Content-Type", "application/json")
		// Zone lookup
		if r.URL.Query().Get("name") != "" {
			json.NewEncoder(w).Encode(cfAPIResponse{
				Success: true,
				Result:  mustMarshal([]cfZone{{ID: "z1", Name: "example.com"}}),
			})
			return
		}
		// Create record
		json.NewEncoder(w).Encode(cfAPIResponse{
			Success: true,
			Result:  mustMarshal(map[string]string{"id": "r1"}),
		})
	}))
	defer srv.Close()

	p := cfProvider(srv.URL)
	_ = p.CreateRecord("example.com", "A", "example.com", "1.2.3.4", 300)

	if gotContentType != "application/json" {
		t.Errorf("expected Content-Type %q, got %q", "application/json", gotContentType)
	}
}

// ---------- Test: checkResponse with non-JSON error body ----------

func TestCloudflareCheckResponseNonJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Zone lookup succeeds
		if r.URL.Query().Get("name") != "" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(cfAPIResponse{
				Success: true,
				Result:  mustMarshal([]cfZone{{ID: "z1", Name: "example.com"}}),
			})
			return
		}
		// Create record returns plain text error
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("Bad Gateway"))
	}))
	defer srv.Close()

	p := cfProvider(srv.URL)
	err := p.CreateRecord("example.com", "A", "example.com", "1.2.3.4", 300)
	if err == nil {
		t.Fatal("expected error for non-JSON error response")
	}
	if !strings.Contains(err.Error(), "502") || !strings.Contains(err.Error(), "Bad Gateway") {
		t.Errorf("expected HTTP 502 Bad Gateway in error, got: %v", err)
	}
}

// ---------- Test: doRequest with empty token on provider ----------

func TestCloudflareDoRequestEmptyToken(t *testing.T) {
	// Create provider manually with empty token (bypassing constructor).
	p := &CloudflareProvider{
		apiToken:   "",
		httpClient: http.DefaultClient,
	}
	_, err := p.doRequest("GET", "http://localhost/test", nil)
	if err == nil {
		t.Fatal("expected error for empty token in doRequest")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Errorf("expected 'not configured' in error, got: %v", err)
	}
}

// ---------- helpers ----------

func mustMarshal(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return json.RawMessage(data)
}

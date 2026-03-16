package dns

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// hetznerRewriteTransport rewrites every outgoing request so that
// hetznerAPIBase is replaced with the test server URL.
type hetznerRewriteTransport struct {
	base      http.RoundTripper
	targetURL string
}

func (t *hetznerRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	newURL := strings.Replace(req.URL.String(), hetznerAPIBase, t.targetURL, 1)
	newReq, err := http.NewRequest(req.Method, newURL, req.Body)
	if err != nil {
		return nil, err
	}
	newReq.Header = req.Header
	return t.base.RoundTrip(newReq)
}

// hzProvider returns a HetznerProvider wired to the given test server.
func hzProvider(serverURL string) *HetznerProvider {
	return &HetznerProvider{
		apiToken: "test-token",
		httpClient: &http.Client{
			Transport: &hetznerRewriteTransport{
				base:      http.DefaultTransport,
				targetURL: serverURL,
			},
		},
	}
}

// ---------- Test: CreateRecord ----------

func TestHetznerCreateRecord(t *testing.T) {
	var (
		gotMethod string
		gotPath   string
		gotBody   map[string]any
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Zone lookup
		if strings.Contains(r.URL.Path, "/zones") && r.URL.Query().Get("name") != "" {
			json.NewEncoder(w).Encode(hetznerZonesResponse{
				Zones: []hetznerZone{{ID: "zone-123", Name: "example.com"}},
			})
			return
		}

		// Create record
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/records") {
			gotMethod = r.Method
			gotPath = r.URL.Path
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &gotBody)

			json.NewEncoder(w).Encode(hetznerRecordResponse{
				Record: hetznerRecord{ID: "rec-1"},
			})
			return
		}

		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := hzProvider(srv.URL)
	err := p.CreateRecord("example.com", "A", "example.com", "1.2.3.4", 300)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", gotMethod)
	}
	if !strings.HasSuffix(gotPath, "/records") {
		t.Errorf("unexpected path: %s", gotPath)
	}
	if gotBody["type"] != "A" {
		t.Errorf("expected type=A, got %v", gotBody["type"])
	}
	if gotBody["name"] != "@" {
		t.Errorf("expected name=@, got %v", gotBody["name"])
	}
	if gotBody["value"] != "1.2.3.4" {
		t.Errorf("expected value=1.2.3.4, got %v", gotBody["value"])
	}
	if gotBody["zone_id"] != "zone-123" {
		t.Errorf("expected zone_id=zone-123, got %v", gotBody["zone_id"])
	}
	// JSON numbers decode as float64
	if ttl, ok := gotBody["ttl"].(float64); !ok || int(ttl) != 300 {
		t.Errorf("expected ttl=300, got %v", gotBody["ttl"])
	}
}

// ---------- Test: DeleteRecord ----------

func TestHetznerDeleteRecord(t *testing.T) {
	var (
		deleteMethod string
		deletePath   string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Zone lookup
		if r.URL.Query().Get("name") != "" && r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(hetznerZonesResponse{
				Zones: []hetznerZone{{ID: "zone-abc", Name: "example.com"}},
			})
			return
		}

		// List records
		if r.Method == http.MethodGet && r.URL.Query().Get("zone_id") != "" {
			json.NewEncoder(w).Encode(hetznerRecordsResponse{
				Records: []hetznerRecord{
					{ID: "rec-1", Type: "A", Name: "@", Value: "1.2.3.4", TTL: 300},
					{ID: "rec-2", Type: "CNAME", Name: "www", Value: "example.com", TTL: 300},
				},
			})
			return
		}

		// Delete record
		if r.Method == http.MethodDelete {
			deleteMethod = r.Method
			deletePath = r.URL.Path
			w.WriteHeader(http.StatusOK)
			return
		}

		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := hzProvider(srv.URL)
	err := p.DeleteRecord("example.com", "A", "example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if deleteMethod != http.MethodDelete {
		t.Errorf("expected DELETE, got %s", deleteMethod)
	}
	if !strings.Contains(deletePath, "/records/rec-1") {
		t.Errorf("expected delete path to contain rec-1, got %s", deletePath)
	}
}

func TestHetznerDeleteRecordNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Zone lookup
		if r.URL.Query().Get("name") != "" {
			json.NewEncoder(w).Encode(hetznerZonesResponse{
				Zones: []hetznerZone{{ID: "zone-1", Name: "example.com"}},
			})
			return
		}

		// List records - return empty
		if r.Method == http.MethodGet && r.URL.Query().Get("zone_id") != "" {
			json.NewEncoder(w).Encode(hetznerRecordsResponse{
				Records: []hetznerRecord{},
			})
			return
		}
	}))
	defer srv.Close()

	p := hzProvider(srv.URL)
	err := p.DeleteRecord("example.com", "TXT", "nonexistent.example.com")
	if err == nil {
		t.Fatal("expected error when record not found")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

// ---------- Test: ListRecords ----------

func TestHetznerListRecords(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Zone lookup
		if r.URL.Query().Get("name") != "" {
			json.NewEncoder(w).Encode(hetznerZonesResponse{
				Zones: []hetznerZone{{ID: "zone-42", Name: "example.com"}},
			})
			return
		}

		// List records
		if r.URL.Query().Get("zone_id") != "" {
			json.NewEncoder(w).Encode(hetznerRecordsResponse{
				Records: []hetznerRecord{
					{ID: "r1", Type: "A", Name: "@", Value: "10.0.0.1", TTL: 300},
					{ID: "r2", Type: "CNAME", Name: "www", Value: "example.com", TTL: 600},
					{ID: "r3", Type: "MX", Name: "@", Value: "mail.example.com", TTL: 3600},
				},
			})
			return
		}
	}))
	defer srv.Close()

	p := hzProvider(srv.URL)
	records, err := p.ListRecords("example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}

	// Verify first record is correctly mapped (@ → example.com).
	if records[0].ID != "r1" {
		t.Errorf("expected ID r1, got %s", records[0].ID)
	}
	if records[0].Type != "A" {
		t.Errorf("expected Type A, got %s", records[0].Type)
	}
	if records[0].Name != "example.com" {
		t.Errorf("expected Name example.com, got %s", records[0].Name)
	}
	if records[0].Value != "10.0.0.1" {
		t.Errorf("expected Value 10.0.0.1, got %s", records[0].Value)
	}
	if records[0].TTL != 300 {
		t.Errorf("expected TTL 300, got %d", records[0].TTL)
	}

	// Verify second record (www → www.example.com).
	if records[1].Type != "CNAME" {
		t.Errorf("expected CNAME, got %s", records[1].Type)
	}
	if records[1].Name != "www.example.com" {
		t.Errorf("expected Name www.example.com, got %s", records[1].Name)
	}
	if records[1].Value != "example.com" {
		t.Errorf("expected Value example.com, got %s", records[1].Value)
	}
}

// ---------- Test: FindZoneID ----------

func TestHetznerFindZoneID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		qName := r.URL.Query().Get("name")
		if qName != "example.com" {
			t.Errorf("expected zone query for example.com, got %s", qName)
		}

		json.NewEncoder(w).Encode(hetznerZonesResponse{
			Zones: []hetznerZone{{ID: "zone-found-id", Name: "example.com"}},
		})
	}))
	defer srv.Close()

	p := hzProvider(srv.URL)
	id, err := p.findZoneID("sub.example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "zone-found-id" {
		t.Errorf("expected zone-found-id, got %s", id)
	}
}

// ---------- Test: FindZoneID not found ----------

func TestHetznerFindZoneIDNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(hetznerZonesResponse{
			Zones: []hetznerZone{}, // empty
		})
	}))
	defer srv.Close()

	p := hzProvider(srv.URL)
	_, err := p.findZoneID("nonexistent.example.com")
	if err == nil {
		t.Fatal("expected error when no zone found")
	}
	if !strings.Contains(err.Error(), "no zone found") {
		t.Errorf("expected 'no zone found' in error, got: %v", err)
	}
}

// ---------- Test: Empty token ----------

func TestHetznerEmptyToken(t *testing.T) {
	p, err := NewHetznerProvider("")
	if err == nil {
		t.Error("expected error for empty token")
	}
	if p != nil {
		t.Error("expected nil provider for empty token")
	}
}

// ---------- Test: Whitespace token ----------

func TestHetznerWhitespaceToken(t *testing.T) {
	tests := []string{"  ", "\t", " \t ", "\n"}
	for _, tok := range tests {
		p, err := NewHetznerProvider(tok)
		if err == nil {
			t.Errorf("expected error for whitespace token %q", tok)
		}
		if p != nil {
			t.Errorf("expected nil provider for whitespace token %q", tok)
		}
	}
}

// ---------- Test: Name() method ----------

func TestHetznerName(t *testing.T) {
	p, err := NewHetznerProvider("valid-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "hetzner" {
		t.Errorf("expected Name()=%q, got %q", "hetzner", p.Name())
	}
}

// ---------- Test: Provider interface compliance ----------

func TestHetznerImplementsProvider(t *testing.T) {
	p, err := NewHetznerProvider("token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Compile-time check: *HetznerProvider satisfies the Provider interface.
	var _ Provider = p
}

// ---------- Test: Authorization header ----------

func TestHetznerAuthorizationHeader(t *testing.T) {
	var gotAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Auth-API-Token")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(hetznerZonesResponse{
			Zones: []hetznerZone{{ID: "z1", Name: "example.com"}},
		})
	}))
	defer srv.Close()

	p := hzProvider(srv.URL)
	p.apiToken = "my-secret-token"
	_, _ = p.findZoneID("example.com")

	if gotAuth != "my-secret-token" {
		t.Errorf("expected Auth-API-Token header %q, got %q", "my-secret-token", gotAuth)
	}
}

// ---------- Test: Name conversion helpers ----------

func TestHetznerNameConversion(t *testing.T) {
	// hetznerName: full → relative
	tests := []struct {
		fqdn     string
		zone     string
		expected string
	}{
		{"example.com", "example.com", "@"},
		{"www.example.com", "example.com", "www"},
		{"*.example.com", "example.com", "*"},
		{"sub.deep.example.com", "example.com", "sub.deep"},
		{"other.com", "example.com", "other.com"},
	}

	for _, tt := range tests {
		t.Run("hetznerName/"+tt.fqdn, func(t *testing.T) {
			got := hetznerName(tt.fqdn, tt.zone)
			if got != tt.expected {
				t.Errorf("hetznerName(%q, %q) = %q, want %q", tt.fqdn, tt.zone, got, tt.expected)
			}
		})
	}

	// fullName: relative → full
	reverseTests := []struct {
		hName    string
		zone     string
		expected string
	}{
		{"@", "example.com", "example.com"},
		{"www", "example.com", "www.example.com"},
		{"*", "example.com", "*.example.com"},
		{"sub.deep", "example.com", "sub.deep.example.com"},
	}

	for _, tt := range reverseTests {
		t.Run("fullName/"+tt.hName, func(t *testing.T) {
			got := fullName(tt.hName, tt.zone)
			if got != tt.expected {
				t.Errorf("fullName(%q, %q) = %q, want %q", tt.hName, tt.zone, got, tt.expected)
			}
		})
	}
}

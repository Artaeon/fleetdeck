package dns

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// contaboRewriteTransport rewrites every outgoing request so that
// contaboAPIBase is replaced with the test server URL.
type contaboRewriteTransport struct {
	base      http.RoundTripper
	targetURL string
}

func (t *contaboRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	newURL := strings.Replace(req.URL.String(), contaboAPIBase, t.targetURL, 1)
	newReq, err := http.NewRequest(req.Method, newURL, req.Body)
	if err != nil {
		return nil, err
	}
	newReq.Header = req.Header
	return t.base.RoundTrip(newReq)
}

// cbProvider returns a ContaboProvider wired to the given test server.
func cbProvider(serverURL string) *ContaboProvider {
	return &ContaboProvider{
		apiToken: "test-token",
		httpClient: &http.Client{
			Transport: &contaboRewriteTransport{
				base:      http.DefaultTransport,
				targetURL: serverURL,
			},
		},
	}
}

// contaboZonesResponse is a helper to build zone list responses.
func contaboZonesResponse(zones []contaboZone) contaboAPIResponse {
	return contaboAPIResponse{Data: mustMarshal(zones)}
}

// contaboRecordsResponse is a helper to build record list responses.
func contaboRecordsResponse(records []contaboRecord) contaboAPIResponse {
	return contaboAPIResponse{Data: mustMarshal(records)}
}

// ---------- Test: CreateRecord ----------

func TestContaboCreateRecord(t *testing.T) {
	var (
		gotMethod string
		gotPath   string
		gotBody   map[string]any
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Zone lookup
		if r.Method == http.MethodGet && r.URL.Path == "/dns/zones" {
			json.NewEncoder(w).Encode(contaboZonesResponse([]contaboZone{
				{ID: "zone-123", Name: "example.com"},
			}))
			return
		}

		// Create record
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/records") {
			gotMethod = r.Method
			gotPath = r.URL.Path
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &gotBody)

			json.NewEncoder(w).Encode(contaboAPIResponse{
				Data: mustMarshal(map[string]string{"id": "rec-1"}),
			})
			return
		}

		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := cbProvider(srv.URL)
	err := p.CreateRecord("example.com", "A", "app", "1.2.3.4", 300)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", gotMethod)
	}
	if !strings.HasSuffix(gotPath, "/dns/zones/zone-123/records") {
		t.Errorf("unexpected path: %s", gotPath)
	}
	if gotBody["type"] != "A" {
		t.Errorf("expected type=A, got %v", gotBody["type"])
	}
	if gotBody["name"] != "app" {
		t.Errorf("expected name=app, got %v", gotBody["name"])
	}
	if gotBody["value"] != "1.2.3.4" {
		t.Errorf("expected value=1.2.3.4, got %v", gotBody["value"])
	}
	if ttl, ok := gotBody["ttl"].(float64); !ok || int(ttl) != 300 {
		t.Errorf("expected ttl=300, got %v", gotBody["ttl"])
	}
}

// ---------- Test: DeleteRecord ----------

func TestContaboDeleteRecord(t *testing.T) {
	var (
		deleteMethod string
		deletePath   string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Zone lookup
		if r.Method == http.MethodGet && r.URL.Path == "/dns/zones" {
			json.NewEncoder(w).Encode(contaboZonesResponse([]contaboZone{
				{ID: "zone-abc", Name: "example.com"},
			}))
			return
		}

		// List records
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/records") {
			json.NewEncoder(w).Encode(contaboRecordsResponse([]contaboRecord{
				{ID: "rec-1", Type: "A", Name: "@", Value: "1.2.3.4", TTL: 300},
				{ID: "rec-2", Type: "CNAME", Name: "www", Value: "example.com", TTL: 300},
			}))
			return
		}

		// Delete record
		if r.Method == http.MethodDelete {
			deleteMethod = r.Method
			deletePath = r.URL.Path
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(contaboAPIResponse{})
			return
		}

		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := cbProvider(srv.URL)
	err := p.DeleteRecord("example.com", "A", "example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if deleteMethod != http.MethodDelete {
		t.Errorf("expected DELETE, got %s", deleteMethod)
	}
	if !strings.Contains(deletePath, "/dns/zones/zone-abc/records/rec-1") {
		t.Errorf("expected delete path to contain zone-abc and rec-1, got %s", deletePath)
	}
}

// ---------- Test: DeleteRecord not found ----------

func TestContaboDeleteRecordNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Zone lookup
		if r.Method == http.MethodGet && r.URL.Path == "/dns/zones" {
			json.NewEncoder(w).Encode(contaboZonesResponse([]contaboZone{
				{ID: "zone-1", Name: "example.com"},
			}))
			return
		}

		// List records - return empty
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/records") {
			json.NewEncoder(w).Encode(contaboRecordsResponse([]contaboRecord{}))
			return
		}
	}))
	defer srv.Close()

	p := cbProvider(srv.URL)
	err := p.DeleteRecord("example.com", "TXT", "nonexistent")
	if err == nil {
		t.Fatal("expected error when record not found")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

// ---------- Test: ListRecords ----------

func TestContaboListRecords(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Zone lookup
		if r.Method == http.MethodGet && r.URL.Path == "/dns/zones" {
			json.NewEncoder(w).Encode(contaboZonesResponse([]contaboZone{
				{ID: "zone-42", Name: "example.com"},
			}))
			return
		}

		// List records
		if strings.Contains(r.URL.Path, "/records") {
			json.NewEncoder(w).Encode(contaboRecordsResponse([]contaboRecord{
				{ID: "r1", Type: "A", Name: "@", Value: "10.0.0.1", TTL: 300},
				{ID: "r2", Type: "CNAME", Name: "www", Value: "example.com", TTL: 600},
				{ID: "r3", Type: "MX", Name: "@", Value: "mail.example.com", TTL: 3600},
			}))
			return
		}
	}))
	defer srv.Close()

	p := cbProvider(srv.URL)
	records, err := p.ListRecords("example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}

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

	if records[1].Type != "CNAME" {
		t.Errorf("expected CNAME, got %s", records[1].Type)
	}
	if records[1].Value != "example.com" {
		t.Errorf("expected value example.com, got %s", records[1].Value)
	}
}

// ---------- Test: FindZoneID ----------

func TestContaboFindZoneID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(contaboZonesResponse([]contaboZone{
			{ID: "zone-other", Name: "other.com"},
			{ID: "zone-found-id", Name: "example.com"},
		}))
	}))
	defer srv.Close()

	p := cbProvider(srv.URL)
	id, err := p.findZoneID("sub.example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "zone-found-id" {
		t.Errorf("expected zone-found-id, got %s", id)
	}
}

// ---------- Test: FindZoneID not found ----------

func TestContaboFindZoneIDNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(contaboZonesResponse([]contaboZone{}))
	}))
	defer srv.Close()

	p := cbProvider(srv.URL)
	_, err := p.findZoneID("nonexistent.example.com")
	if err == nil {
		t.Fatal("expected error when no zone found")
	}
	if !strings.Contains(err.Error(), "no zone found") {
		t.Errorf("expected 'no zone found' in error, got: %v", err)
	}
}

// ---------- Test: Empty token ----------

func TestContaboEmptyToken(t *testing.T) {
	p, err := NewContaboProvider("")
	if err == nil {
		t.Error("expected error for empty token")
	}
	if p != nil {
		t.Error("expected nil provider for empty token")
	}
}

// ---------- Test: Whitespace token ----------

func TestContaboWhitespaceToken(t *testing.T) {
	tests := []string{"  ", "\t", " \t ", "\n"}
	for _, tok := range tests {
		p, err := NewContaboProvider(tok)
		if err == nil {
			t.Errorf("expected error for whitespace token %q", tok)
		}
		if p != nil {
			t.Errorf("expected nil provider for whitespace token %q", tok)
		}
	}
}

// ---------- Test: Name() method ----------

func TestContaboName(t *testing.T) {
	p, err := NewContaboProvider("valid-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "contabo" {
		t.Errorf("expected Name()=%q, got %q", "contabo", p.Name())
	}
}

// ---------- Test: Provider interface compliance ----------

func TestContaboImplementsProvider(t *testing.T) {
	p, err := NewContaboProvider("token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var _ Provider = p
}

// ---------- Test: Authorization header ----------

func TestContaboAuthorizationHeader(t *testing.T) {
	var gotAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(contaboZonesResponse([]contaboZone{
			{ID: "z1", Name: "example.com"},
		}))
	}))
	defer srv.Close()

	p := cbProvider(srv.URL)
	p.apiToken = "my-secret-token"
	_, _ = p.findZoneID("example.com")

	if gotAuth != "Bearer my-secret-token" {
		t.Errorf("expected Authorization header %q, got %q", "Bearer my-secret-token", gotAuth)
	}
}

// ---------- Test: x-request-id header ----------

func TestContaboRequestID(t *testing.T) {
	var gotRequestID string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequestID = r.Header.Get("x-request-id")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(contaboZonesResponse([]contaboZone{
			{ID: "z1", Name: "example.com"},
		}))
	}))
	defer srv.Close()

	p := cbProvider(srv.URL)
	_, _ = p.findZoneID("example.com")

	if gotRequestID == "" {
		t.Error("expected x-request-id header to be set")
	}
	// UUID v4 format: 8-4-4-4-12 hex characters
	if len(gotRequestID) != 36 {
		t.Errorf("expected UUID-format x-request-id (36 chars), got %q (%d chars)", gotRequestID, len(gotRequestID))
	}
}

// ---------- Test: ListRecords pagination ----------

func TestContaboListRecordsPagination(t *testing.T) {
	var recordPages int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Zone lookup
		if r.Method == http.MethodGet && r.URL.Path == "/dns/zones" {
			json.NewEncoder(w).Encode(contaboZonesResponse([]contaboZone{
				{ID: "zone-pg", Name: "example.com"},
			}))
			return
		}

		// List records with pagination
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/records") {
			recordPages++
			page := r.URL.Query().Get("page")

			switch page {
			case "1", "":
				json.NewEncoder(w).Encode(contaboAPIResponse{
					Data: mustMarshal([]contaboRecord{
						{ID: "r1", Type: "A", Name: "@", Value: "10.0.0.1", TTL: 300},
						{ID: "r2", Type: "CNAME", Name: "www", Value: "example.com", TTL: 600},
					}),
					Pagination: &contaboPagination{
						Size:          2,
						TotalElements: 4,
						TotalPages:    2,
						Page:          1,
					},
				})
			case "2":
				json.NewEncoder(w).Encode(contaboAPIResponse{
					Data: mustMarshal([]contaboRecord{
						{ID: "r3", Type: "MX", Name: "@", Value: "mail.example.com", TTL: 3600},
						{ID: "r4", Type: "TXT", Name: "@", Value: "v=spf1 include:example.com ~all", TTL: 3600},
					}),
					Pagination: &contaboPagination{
						Size:          2,
						TotalElements: 4,
						TotalPages:    2,
						Page:          2,
					},
				})
			default:
				http.Error(w, "unexpected page", http.StatusBadRequest)
			}
			return
		}

		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := cbProvider(srv.URL)
	records, err := p.ListRecords("example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if recordPages != 2 {
		t.Errorf("expected 2 record page requests, got %d", recordPages)
	}

	if len(records) != 4 {
		t.Fatalf("expected 4 records across 2 pages, got %d", len(records))
	}

	if records[0].ID != "r1" || records[0].Type != "A" {
		t.Errorf("unexpected first record: %+v", records[0])
	}
	if records[2].ID != "r3" || records[2].Type != "MX" {
		t.Errorf("unexpected third record: %+v", records[2])
	}
	if records[3].ID != "r4" || records[3].Type != "TXT" {
		t.Errorf("unexpected fourth record: %+v", records[3])
	}
}

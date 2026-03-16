package dns

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const hetznerAPIBase = "https://dns.hetzner.com/api/v1"

// HetznerProvider implements Provider using the Hetzner DNS API.
type HetznerProvider struct {
	apiToken   string
	httpClient *http.Client
}

// NewHetznerProvider returns a HetznerProvider configured with the
// given API token. Returns an error if apiToken is empty.
func NewHetznerProvider(apiToken string) (*HetznerProvider, error) {
	if strings.TrimSpace(apiToken) == "" {
		return nil, fmt.Errorf("hetzner API token must not be empty")
	}
	return &HetznerProvider{
		apiToken: apiToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

func (h *HetznerProvider) Name() string { return "hetzner" }

func (h *HetznerProvider) CreateRecord(domain, recordType, name, value string, ttl int) error {
	zoneID, err := h.findZoneID(domain)
	if err != nil {
		return err
	}

	zoneName := rootDomain(domain)
	body := map[string]any{
		"zone_id": zoneID,
		"type":    recordType,
		"name":    hetznerName(name, zoneName),
		"value":   value,
		"ttl":     ttl,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling record payload: %w", err)
	}

	url := fmt.Sprintf("%s/records", hetznerAPIBase)
	resp, err := h.doRequest("POST", url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return h.checkResponse(resp, "create DNS record")
}

func (h *HetznerProvider) DeleteRecord(domain, recordType, name string) error {
	zoneID, err := h.findZoneID(domain)
	if err != nil {
		return err
	}

	zoneName := rootDomain(domain)
	records, err := h.listZoneRecords(zoneID, zoneName)
	if err != nil {
		return err
	}

	for _, r := range records {
		if r.Type == recordType && r.Name == name {
			url := fmt.Sprintf("%s/records/%s", hetznerAPIBase, r.ID)
			resp, err := h.doRequest("DELETE", url, nil)
			if err != nil {
				return err
			}
			resp.Body.Close()
			return h.checkResponse(resp, "delete DNS record")
		}
	}

	return fmt.Errorf("record %s %s not found in zone %s", recordType, name, domain)
}

func (h *HetznerProvider) ListRecords(domain string) ([]Record, error) {
	zoneID, err := h.findZoneID(domain)
	if err != nil {
		return nil, err
	}
	zoneName := rootDomain(domain)
	return h.listZoneRecords(zoneID, zoneName)
}

// Hetzner API response types.

type hetznerZonesResponse struct {
	Zones []hetznerZone `json:"zones"`
}

type hetznerZone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type hetznerRecordsResponse struct {
	Records []hetznerRecord `json:"records"`
}

type hetznerRecord struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value string `json:"value"`
	TTL   int    `json:"ttl"`
}

type hetznerRecordResponse struct {
	Record hetznerRecord `json:"record"`
}

// findZoneID resolves a domain (e.g. "app.example.com") to its Hetzner
// zone ID by extracting the root domain and querying the zones API.
func (h *HetznerProvider) findZoneID(domain string) (string, error) {
	root := rootDomain(domain)

	url := fmt.Sprintf("%s/zones?name=%s", hetznerAPIBase, root)
	resp, err := h.doRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var zonesResp hetznerZonesResponse
	if err := json.NewDecoder(resp.Body).Decode(&zonesResp); err != nil {
		return "", fmt.Errorf("decoding zone lookup response: %w", err)
	}

	if len(zonesResp.Zones) == 0 {
		return "", fmt.Errorf("no zone found for domain %s", root)
	}

	return zonesResp.Zones[0].ID, nil
}

// listZoneRecords fetches all DNS records for the given zone ID.
func (h *HetznerProvider) listZoneRecords(zoneID, zoneName string) ([]Record, error) {
	url := fmt.Sprintf("%s/records?zone_id=%s", hetznerAPIBase, zoneID)
	resp, err := h.doRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var recordsResp hetznerRecordsResponse
	if err := json.NewDecoder(resp.Body).Decode(&recordsResp); err != nil {
		return nil, fmt.Errorf("decoding records response: %w", err)
	}

	records := make([]Record, len(recordsResp.Records))
	for i, r := range recordsResp.Records {
		records[i] = Record{
			ID:    r.ID,
			Type:  r.Type,
			Name:  fullName(r.Name, zoneName),
			Value: r.Value,
			TTL:   r.TTL,
		}
	}
	return records, nil
}

// doRequest builds and executes an authenticated HTTP request against the
// Hetzner DNS API.
func (h *HetznerProvider) doRequest(method, url string, body io.Reader) (*http.Response, error) {
	if strings.TrimSpace(h.apiToken) == "" {
		return nil, fmt.Errorf("hetzner API token is not configured")
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Auth-API-Token", h.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("hetzner API request failed: %w", err)
	}
	return resp, nil
}

// checkResponse returns a descriptive error if the HTTP response indicates
// a failure.
func (h *HetznerProvider) checkResponse(resp *http.Response, action string) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("%s: HTTP %d: %s", action, resp.StatusCode, strings.TrimSpace(string(body)))
}

// hetznerName converts a fully qualified domain name to the Hetzner relative
// record name. For example:
//   - "example.com" with zone "example.com" → "@"
//   - "www.example.com" with zone "example.com" → "www"
//   - "*.example.com" with zone "example.com" → "*"
func hetznerName(fqdn, zoneName string) string {
	if fqdn == zoneName {
		return "@"
	}
	suffix := "." + zoneName
	if strings.HasSuffix(fqdn, suffix) {
		return strings.TrimSuffix(fqdn, suffix)
	}
	return fqdn
}

// fullName converts a Hetzner relative record name back to a fully qualified
// domain name. For example:
//   - "@" with zone "example.com" → "example.com"
//   - "www" with zone "example.com" → "www.example.com"
func fullName(hName, zoneName string) string {
	if hName == "@" {
		return zoneName
	}
	return hName + "." + zoneName
}

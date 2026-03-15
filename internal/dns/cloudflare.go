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

const cloudflareAPIBase = "https://api.cloudflare.com/client/v4"

// CloudflareProvider implements Provider using the Cloudflare API v4.
type CloudflareProvider struct {
	apiToken   string
	httpClient *http.Client
}

// NewCloudflareProvider returns a CloudflareProvider configured with the
// given API token for Bearer authentication.
func NewCloudflareProvider(apiToken string) *CloudflareProvider {
	return &CloudflareProvider{
		apiToken: apiToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *CloudflareProvider) Name() string { return "cloudflare" }

func (c *CloudflareProvider) CreateRecord(domain, recordType, name, value string, ttl int) error {
	zoneID, err := c.findZoneID(domain)
	if err != nil {
		return err
	}

	body := map[string]any{
		"type":    recordType,
		"name":    name,
		"content": value,
		"ttl":     ttl,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling record payload: %w", err)
	}

	url := fmt.Sprintf("%s/zones/%s/dns_records", cloudflareAPIBase, zoneID)
	resp, err := c.doRequest("POST", url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return c.checkResponse(resp, "create DNS record")
}

func (c *CloudflareProvider) DeleteRecord(domain, recordType, name string) error {
	zoneID, err := c.findZoneID(domain)
	if err != nil {
		return err
	}

	// Find the record ID by listing and matching type+name.
	records, err := c.listZoneRecords(zoneID)
	if err != nil {
		return err
	}

	for _, r := range records {
		if r.Type == recordType && r.Name == name {
			url := fmt.Sprintf("%s/zones/%s/dns_records/%s", cloudflareAPIBase, zoneID, r.ID)
			resp, err := c.doRequest("DELETE", url, nil)
			if err != nil {
				return err
			}
			resp.Body.Close()
			return c.checkResponse(resp, "delete DNS record")
		}
	}

	return fmt.Errorf("record %s %s not found in zone %s", recordType, name, domain)
}

func (c *CloudflareProvider) ListRecords(domain string) ([]Record, error) {
	zoneID, err := c.findZoneID(domain)
	if err != nil {
		return nil, err
	}
	return c.listZoneRecords(zoneID)
}

// cfAPIResponse is the common envelope for all Cloudflare API responses.
type cfAPIResponse struct {
	Success bool            `json:"success"`
	Errors  []cfAPIError    `json:"errors"`
	Result  json.RawMessage `json:"result"`
}

type cfAPIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cfZone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type cfRecord struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
}

// findZoneID resolves a domain (e.g. "app.example.com") to its Cloudflare
// zone ID by extracting the root domain and querying the zones API.
func (c *CloudflareProvider) findZoneID(domain string) (string, error) {
	root := rootDomain(domain)

	url := fmt.Sprintf("%s/zones?name=%s", cloudflareAPIBase, root)
	resp, err := c.doRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var apiResp cfAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return "", fmt.Errorf("decoding zone lookup response: %w", err)
	}
	if !apiResp.Success {
		return "", fmt.Errorf("zone lookup failed: %s", formatCFErrors(apiResp.Errors))
	}

	var zones []cfZone
	if err := json.Unmarshal(apiResp.Result, &zones); err != nil {
		return "", fmt.Errorf("decoding zones: %w", err)
	}
	if len(zones) == 0 {
		return "", fmt.Errorf("no zone found for domain %s", root)
	}

	return zones[0].ID, nil
}

// listZoneRecords fetches all DNS records for the given zone ID.
func (c *CloudflareProvider) listZoneRecords(zoneID string) ([]Record, error) {
	url := fmt.Sprintf("%s/zones/%s/dns_records?per_page=100", cloudflareAPIBase, zoneID)
	resp, err := c.doRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var apiResp cfAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decoding records response: %w", err)
	}
	if !apiResp.Success {
		return nil, fmt.Errorf("listing records failed: %s", formatCFErrors(apiResp.Errors))
	}

	var cfRecords []cfRecord
	if err := json.Unmarshal(apiResp.Result, &cfRecords); err != nil {
		return nil, fmt.Errorf("decoding records: %w", err)
	}

	records := make([]Record, len(cfRecords))
	for i, r := range cfRecords {
		records[i] = Record{
			ID:      r.ID,
			Type:    r.Type,
			Name:    r.Name,
			Value:   r.Content,
			TTL:     r.TTL,
			Proxied: r.Proxied,
		}
	}
	return records, nil
}

// doRequest builds and executes an authenticated HTTP request against the
// Cloudflare API.
func (c *CloudflareProvider) doRequest(method, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cloudflare API request failed: %w", err)
	}
	return resp, nil
}

// checkResponse reads the Cloudflare API response and returns a descriptive
// error if the request was not successful.
func (c *CloudflareProvider) checkResponse(resp *http.Response, action string) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	body, _ := io.ReadAll(resp.Body)

	var apiResp cfAPIResponse
	if err := json.Unmarshal(body, &apiResp); err == nil && len(apiResp.Errors) > 0 {
		return fmt.Errorf("%s: %s", action, formatCFErrors(apiResp.Errors))
	}

	return fmt.Errorf("%s: HTTP %d: %s", action, resp.StatusCode, strings.TrimSpace(string(body)))
}

// rootDomain extracts the registrable domain from a full domain name.
// For example, "app.staging.example.com" becomes "example.com".
func rootDomain(domain string) string {
	parts := strings.Split(domain, ".")
	if len(parts) <= 2 {
		return domain
	}
	return strings.Join(parts[len(parts)-2:], ".")
}

// formatCFErrors joins multiple Cloudflare API errors into a single string.
func formatCFErrors(errors []cfAPIError) string {
	msgs := make([]string, len(errors))
	for i, e := range errors {
		msgs[i] = fmt.Sprintf("[%d] %s", e.Code, e.Message)
	}
	return strings.Join(msgs, "; ")
}

package dns

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

const contaboAPIBase = "https://api.contabo.com/v1"

// ContaboProvider implements Provider using the Contabo API v1.
type ContaboProvider struct {
	apiToken   string
	httpClient *http.Client
}

// NewContaboProvider returns a ContaboProvider configured with the given API
// token for Bearer authentication. Returns an error if apiToken is empty.
func NewContaboProvider(apiToken string) (*ContaboProvider, error) {
	if strings.TrimSpace(apiToken) == "" {
		return nil, fmt.Errorf("contabo API token must not be empty")
	}
	return &ContaboProvider{
		apiToken: apiToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

func (c *ContaboProvider) Name() string { return "contabo" }

func (c *ContaboProvider) CreateRecord(domain, recordType, name, value string, ttl int) error {
	zoneID, err := c.findZoneID(domain)
	if err != nil {
		return err
	}

	zoneName := rootDomain(domain)
	body := map[string]any{
		"type":  recordType,
		"name":  contaboName(name, zoneName),
		"value": value,
		"ttl":   ttl,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling record payload: %w", err)
	}

	url := fmt.Sprintf("%s/dns/zones/%s/records", contaboAPIBase, zoneID)
	resp, err := c.doRequest("POST", url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return c.checkResponse(resp, "create DNS record")
}

func (c *ContaboProvider) DeleteRecord(domain, recordType, name string) error {
	zoneID, err := c.findZoneID(domain)
	if err != nil {
		return err
	}

	zoneName := rootDomain(domain)

	// Find the record ID by listing and matching type+name.
	records, err := c.listZoneRecords(zoneID, zoneName)
	if err != nil {
		return err
	}

	for _, r := range records {
		if r.Type == recordType && r.Name == name {
			url := fmt.Sprintf("%s/dns/zones/%s/records/%s", contaboAPIBase, zoneID, r.ID)
			resp, err := c.doRequest("DELETE", url, nil)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			return c.checkResponse(resp, "delete DNS record")
		}
	}

	return fmt.Errorf("record %s %s not found in zone %s", recordType, name, domain)
}

func (c *ContaboProvider) ListRecords(domain string) ([]Record, error) {
	zoneID, err := c.findZoneID(domain)
	if err != nil {
		return nil, err
	}
	zoneName := rootDomain(domain)
	return c.listZoneRecords(zoneID, zoneName)
}

// contaboAPIResponse is the common envelope for Contabo API responses.
type contaboAPIResponse struct {
	Data       json.RawMessage    `json:"data"`
	Pagination *contaboPagination `json:"_pagination"`
}

type contaboPagination struct {
	Size          int `json:"size"`
	TotalElements int `json:"totalElements"`
	TotalPages    int `json:"totalPages"`
	Page          int `json:"page"`
}

type contaboZone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type contaboRecord struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value string `json:"value"`
	TTL   int    `json:"ttl"`
}

// findZoneID resolves a domain (e.g. "app.example.com") to its Contabo
// zone ID by extracting the root domain and querying the zones API.
// It paginates through all pages until the matching zone is found.
func (c *ContaboProvider) findZoneID(domain string) (string, error) {
	root := rootDomain(domain)

	page := 1
	for {
		url := fmt.Sprintf("%s/dns/zones?page=%d&size=100", contaboAPIBase, page)
		resp, err := c.doRequest("GET", url, nil)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()

		if err := c.checkResponse(resp, "zone lookup"); err != nil {
			return "", err
		}

		var apiResp contaboAPIResponse
		if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
			return "", fmt.Errorf("decoding zone lookup response: %w", err)
		}

		var zones []contaboZone
		if err := json.Unmarshal(apiResp.Data, &zones); err != nil {
			return "", fmt.Errorf("decoding zones: %w", err)
		}

		for _, z := range zones {
			if z.Name == root {
				return z.ID, nil
			}
		}

		if apiResp.Pagination == nil || page >= apiResp.Pagination.TotalPages {
			break
		}
		page++
	}

	return "", fmt.Errorf("no zone found for domain %s", root)
}

// listZoneRecords fetches all DNS records for the given zone ID,
// paginating through all pages to accumulate the complete record set.
func (c *ContaboProvider) listZoneRecords(zoneID, zoneName string) ([]Record, error) {
	var records []Record

	page := 1
	for {
		url := fmt.Sprintf("%s/dns/zones/%s/records?page=%d&size=500", contaboAPIBase, zoneID, page)
		resp, err := c.doRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if err := c.checkResponse(resp, "listing records"); err != nil {
			return nil, err
		}

		var apiResp contaboAPIResponse
		if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
			return nil, fmt.Errorf("decoding records response: %w", err)
		}

		var cbRecords []contaboRecord
		if err := json.Unmarshal(apiResp.Data, &cbRecords); err != nil {
			return nil, fmt.Errorf("decoding records: %w", err)
		}

		for _, r := range cbRecords {
			records = append(records, Record{
				ID:    r.ID,
				Type:  r.Type,
				Name:  contaboFullName(r.Name, zoneName),
				Value: r.Value,
				TTL:   r.TTL,
			})
		}

		if apiResp.Pagination == nil || page >= apiResp.Pagination.TotalPages {
			break
		}
		page++
	}

	return records, nil
}

// doRequest builds and executes an authenticated HTTP request against the
// Contabo API, including the required x-request-id header.
func (c *ContaboProvider) doRequest(method, url string, body io.Reader) (*http.Response, error) {
	if strings.TrimSpace(c.apiToken) == "" {
		return nil, fmt.Errorf("contabo API token is not configured")
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-request-id", uuid.New().String())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("contabo API request failed: %w", err)
	}
	return resp, nil
}

// checkResponse returns an error if the HTTP status code indicates failure.
func (c *ContaboProvider) checkResponse(resp *http.Response, action string) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("%s: HTTP %d: %s", action, resp.StatusCode, strings.TrimSpace(string(body)))
}

// contaboName converts a fully qualified domain name to a Contabo relative
// record name. For example:
//   - "example.com" with zone "example.com" → "@"
//   - "www.example.com" with zone "example.com" → "www"
//   - "*.example.com" with zone "example.com" → "*"
func contaboName(fqdn, zoneName string) string {
	if fqdn == zoneName {
		return "@"
	}
	suffix := "." + zoneName
	if strings.HasSuffix(fqdn, suffix) {
		return strings.TrimSuffix(fqdn, suffix)
	}
	return fqdn
}

// contaboFullName converts a Contabo relative record name back to a fully
// qualified domain name.
func contaboFullName(name, zoneName string) string {
	if name == "@" {
		return zoneName
	}
	return name + "." + zoneName
}

// Package dns provides a pluggable interface for managing DNS records.
package dns

import "fmt"

// Record represents a single DNS record.
type Record struct {
	ID      string `json:"id"`
	Type    string `json:"type"`    // "A", "AAAA", "CNAME"
	Name    string `json:"name"`
	Value   string `json:"value"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
}

// Provider is the interface that DNS providers must implement.
type Provider interface {
	CreateRecord(domain, recordType, name, value string, ttl int) error
	DeleteRecord(domain, recordType, name string) error
	ListRecords(domain string) ([]Record, error)
	Name() string
}

// AutoConfigure creates the standard DNS records for a fleetdeck project:
// an A record for the domain itself and a wildcard *.domain pointing to
// serverIP. This is sufficient for Traefik to route all sub-domains.
func AutoConfigure(domain, serverIP string, provider Provider) error {
	// Root A record (e.g. example.com → 1.2.3.4)
	if err := provider.CreateRecord(domain, "A", domain, serverIP, 300); err != nil {
		return fmt.Errorf("creating A record for %s: %w", domain, err)
	}

	// Wildcard A record (e.g. *.example.com → 1.2.3.4)
	wildcard := "*." + domain
	if err := provider.CreateRecord(domain, "A", wildcard, serverIP, 300); err != nil {
		return fmt.Errorf("creating wildcard A record for %s: %w", wildcard, err)
	}

	return nil
}

// GetProvider returns the DNS provider identified by name, configured with
// the given API token. Returns an error for unknown provider names.
func GetProvider(name string, apiToken string) (Provider, error) {
	switch name {
	case "cloudflare":
		return NewCloudflareProvider(apiToken)
	default:
		return nil, fmt.Errorf("unknown DNS provider: %s", name)
	}
}

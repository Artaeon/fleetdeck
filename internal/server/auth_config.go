package server

import "fmt"

// ValidateAPIToken rejects missing and trivially weak dashboard credentials
// before the listener opens. Operators should generate a random token; length
// validation alone cannot establish entropy.
func ValidateAPIToken(token string) error {
	if len(token) < 32 || len(token) > 1024 {
		return fmt.Errorf("dashboard requires a random API token of 32–1024 characters; set FLEETDECK_API_TOKEN or server.api_token")
	}
	for _, c := range token {
		if c < '!' || c > '~' {
			return fmt.Errorf("dashboard API token must contain only printable ASCII characters without whitespace")
		}
	}
	return nil
}

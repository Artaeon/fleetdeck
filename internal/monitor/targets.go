package monitor

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"
)

// LoadTargets reads an operator-owned list for private health routes and public
// edge probes. Credentials in URLs are rejected because results are persisted.
func LoadTargets(path string, interval, timeout time.Duration) ([]Target, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var entries []struct {
		Name           string `json:"name"`
		URL            string `json:"url"`
		ExpectedStatus int    `json:"expected_status"`
	}
	decoder := json.NewDecoder(io.LimitReader(f, 65537))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&entries); err != nil {
		return nil, fmt.Errorf("invalid monitoring targets JSON")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("unexpected monitoring targets data")
	}
	if len(entries) == 0 || len(entries) > 100 {
		return nil, fmt.Errorf("configure between 1 and 100 monitoring targets")
	}
	seen := map[string]bool{}
	targets := make([]Target, 0, len(entries))
	for _, entry := range entries {
		parsed, err := url.Parse(entry.URL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil || parsed.Fragment != "" || parsed.RawQuery != "" {
			return nil, fmt.Errorf("monitoring URLs must be HTTP(S) without credentials, query strings or fragments")
		}
		if strings.TrimSpace(entry.Name) == "" || len(entry.Name) > 100 || strings.ContainsAny(entry.Name, "\r\n") || seen[entry.Name] {
			return nil, fmt.Errorf("monitoring target names must be unique and nonempty")
		}
		seen[entry.Name] = true
		if entry.ExpectedStatus == 0 {
			entry.ExpectedStatus = 200
		}
		if entry.ExpectedStatus < 100 || entry.ExpectedStatus > 599 {
			return nil, fmt.Errorf("invalid expected HTTP status")
		}
		targets = append(targets, Target{Name: entry.Name, URL: entry.URL, Method: "GET", ExpectedStatus: entry.ExpectedStatus, Interval: interval, Timeout: timeout})
	}
	return targets, nil
}

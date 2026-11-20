package securitywatch

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

// EvidenceDigest identifies changes in actionable findings without exposing private IDs.
func (report Report) EvidenceDigest() (string, error) {
	var evidence []string
	add := func(scope string, value any) error {
		b, err := json.Marshal(struct {
			Scope string `json:"scope"`
			Value any    `json:"value"`
		}{scope, value})
		if err == nil {
			evidence = append(evidence, string(b))
		}
		return err
	}
	for _, c := range []struct {
		name  string
		check Check
	}{{"release", report.Release}, {"host", report.Host}, {"inventory", report.Inventory}} {
		v := c.check
		if (v.Status == "up_to_date" || v.Status == "checked") && !v.RebootRequired {
			continue
		}
		if err := add(c.name, v); err != nil {
			return "", err
		}
	}
	for _, original := range report.Images {
		if original.Status == "checked" && len(original.Findings) == 0 {
			continue
		}
		v := original
		v.Findings = sortedFindings(v.Findings)
		if err := add("image", v); err != nil {
			return "", err
		}
	}
	for _, original := range report.Containers {
		if len(original.Findings) == 0 {
			continue
		}
		v := original
		v.ID = ""
		v.Findings = sortedFindings(v.Findings)
		if err := add("runtime", v); err != nil {
			return "", err
		}
	}
	sort.Strings(evidence)
	b, err := json.Marshal(evidence)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func sortedFindings(in []Finding) []Finding {
	out := append([]Finding(nil), in...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].Severity < out[j].Severity
	})
	return out
}

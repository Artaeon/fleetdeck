package discover

import (
	"regexp"
	"strings"
)

var hostRegex = regexp.MustCompile("`([^`]+)`")

func ExtractDomain(labels map[string]string) string {
	for key, value := range labels {
		if !strings.Contains(key, "traefik.http.routers") || !strings.HasSuffix(key, ".rule") {
			continue
		}
		if strings.Contains(value, "Host(") || strings.Contains(value, "Host(`") {
			matches := hostRegex.FindStringSubmatch(value)
			if len(matches) >= 2 {
				return matches[1]
			}
		}
	}
	return ""
}

func ExtractDomainFromLabelsList(labels []string) string {
	for _, label := range labels {
		parts := strings.SplitN(label, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if !strings.Contains(key, "traefik.http.routers") || !strings.HasSuffix(key, ".rule") {
			continue
		}
		if strings.Contains(value, "Host(") || strings.Contains(value, "Host(`") {
			matches := hostRegex.FindStringSubmatch(value)
			if len(matches) >= 2 {
				return matches[1]
			}
		}
	}
	return ""
}

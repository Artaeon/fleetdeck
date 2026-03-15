package dns

import "strings"

// knownMultiLevelTLDs contains well-known TLDs that consist of two labels.
// Domains under these TLDs need an extra label to form a registrable domain
// (e.g. "example.co.uk" rather than just "co.uk").
var knownMultiLevelTLDs = map[string]bool{
	"co.uk":  true,
	"co.jp":  true,
	"co.kr":  true,
	"co.nz":  true,
	"co.za":  true,
	"co.in":  true,
	"co.il":  true,
	"com.au": true,
	"com.br": true,
	"com.cn": true,
	"com.mx": true,
	"com.sg": true,
	"com.tw": true,
	"com.hk": true,
	"org.uk": true,
	"net.uk": true,
	"ac.uk":  true,
	"me.uk":  true,
	"org.au": true,
	"net.au": true,
	"or.jp":  true,
	"ne.jp":  true,
	"ac.jp":  true,
	"go.jp":  true,
	"org.nz": true,
	"net.nz": true,
	"eu.com": true,
	"us.com": true,
}

// isMultiLevelTLD reports whether the given TLD string (e.g. "co.uk") is a
// known multi-level TLD.
func isMultiLevelTLD(tld string) bool {
	return knownMultiLevelTLDs[strings.ToLower(tld)]
}

// rootDomain extracts the registrable domain from a full domain name.
// It handles multi-level TLDs such as ".co.uk":
//
//	"app.example.co.uk" -> "example.co.uk"
//	"app.example.com"   -> "example.com"
//	"example.com"       -> "example.com"
func rootDomain(domain string) string {
	parts := strings.Split(domain, ".")
	n := len(parts)

	if n <= 2 {
		return domain
	}

	// Check whether the last two labels form a multi-level TLD.
	lastTwo := strings.Join(parts[n-2:], ".")
	if isMultiLevelTLD(lastTwo) {
		// Need at least 3 parts to have a registrable domain under a
		// multi-level TLD (e.g. "example.co.uk").
		if n < 3 {
			return domain
		}
		return strings.Join(parts[n-3:], ".")
	}

	return strings.Join(parts[n-2:], ".")
}

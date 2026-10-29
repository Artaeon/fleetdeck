package fleet

import (
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"os"
	"time"
)

// Agent exposes exactly one GET route and only a pre-sanitized file. It needs
// no FleetDeck database, Docker socket, SSH key, shell or administrator token.
func Agent(node, path string, allowed map[[32]byte]bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 || len(r.TLS.PeerCertificates) == 0 || !allowed[sha256.Sum256(r.TLS.PeerCertificates[0].Raw)] {
			http.Error(w, "unauthorized", 401)
			return
		}
		if r.URL.Path != "/v1/snapshot" || r.URL.RawQuery != "" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			http.Error(w, "read only", 405)
			return
		}
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0027 != 0 || info.Size() > MaxBytes {
			http.Error(w, "snapshot unavailable", 503)
			return
		}
		f, err := os.Open(path)
		if err != nil {
			http.Error(w, "snapshot unavailable", 503)
			return
		}
		defer f.Close()
		s, err := Decode(f, node, time.Now())
		if err != nil {
			http.Error(w, "snapshot unavailable or stale", 503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(s)
	})
}

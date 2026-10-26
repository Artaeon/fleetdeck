package server

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// The monitoring credential is accepted only on GET /metrics. It never creates
// browser sessions or grants access to project data or administrative actions.
func (s *Server) requireMetricsAuth(next http.HandlerFunc) http.HandlerFunc {
	admin := s.requireAuth(next)
	return func(w http.ResponseWriter, r *http.Request) {
		if s.metricsToken != "" && ValidateDashboardTokens(s.apiToken, s.metricsToken) == nil &&
			strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") &&
			subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")), []byte(s.metricsToken)) == 1 {
			if !safeTransport(r) {
				writeError(w, http.StatusForbidden, "HTTPS or direct loopback access is required")
				return
			}
			next(w, r)
			return
		}
		admin(w, r)
	}
}

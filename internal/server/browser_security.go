package server

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

func peerIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func requestHTTPS(r *http.Request) bool {
	return r.TLS != nil || (isTrustedProxy(peerIP(r)) && r.Header.Get("X-Forwarded-Proto") == "https")
}

// Cleartext is permitted only for direct loopback access, including an SSH
// tunnel. Forwarding headers must never downgrade a proxied session cookie.
func directLoopback(r *http.Request) bool {
	peer := net.ParseIP(peerIP(r))
	if peer == nil || !peer.IsLoopback() {
		return false
	}
	for _, header := range []string{"Forwarded", "X-Forwarded-For", "X-Forwarded-Proto", "X-Forwarded-Host"} {
		if r.Header.Get(header) != "" {
			return false
		}
	}
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	ip := net.ParseIP(host)
	return strings.EqualFold(host, "localhost") || (ip != nil && ip.IsLoopback())
}

func safeTransport(r *http.Request) bool {
	return requestHTTPS(r) || directLoopback(r)
}

func sameOrigin(r *http.Request) bool {
	origin, err := url.Parse(r.Header.Get("Origin"))
	if err != nil || origin.User != nil || origin.Host == "" || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" {
		return false
	}
	scheme := "http"
	if requestHTTPS(r) {
		scheme = "https"
	}
	return origin.Scheme == scheme && strings.EqualFold(origin.Host, r.Host)
}

func safeCookieRequest(r *http.Request) bool {
	if !safeTransport(r) {
		return false
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return sameOrigin(r)
	}
}

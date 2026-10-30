package server

import (
	"context"
	"net"
	"net/http"
)

// listenTCP keeps management listeners on ordinary TCP regardless of Go's
// defaults or GODEBUG. FleetDeck does not require Multipath TCP.
func listenTCP(address string) (net.Listener, error) {
	var config net.ListenConfig
	config.SetMultipathTCP(false)
	return config.Listen(context.Background(), "tcp", address)
}

// ServeHTTP serves the configured HTTP or HTTPS endpoint without enabling MPTCP.
// ServeTLS preserves the server's TLS configuration, including client identity
// checks and HTTP/2 setup, just as ListenAndServeTLS does.
func ServeHTTP(s *http.Server) error {
	address := s.Addr
	if address == "" {
		address = ":http"
		if s.TLSConfig != nil {
			address = ":https"
		}
	}
	listener, err := listenTCP(address)
	if err != nil {
		return err
	}
	defer listener.Close()
	if s.TLSConfig != nil {
		return s.ServeTLS(listener, "", "")
	}
	return s.Serve(listener)
}

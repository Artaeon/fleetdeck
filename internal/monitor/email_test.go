package monitor

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSMTPRequiresVerifiedTLSBeforeAuthentication(t *testing.T) {
	seed := httptest.NewTLSServer(nil)
	certificate := seed.TLS.Certificates[0]
	seed.Close()
	parsed, _ := x509.ParseCertificate(certificate.Certificate[0])
	roots := x509.NewCertPool()
	roots.AddCert(parsed)
	for _, mode := range []string{"downgrade", "untrusted", "verified"} {
		t.Run(mode, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			done := make(chan bool, 1)
			go func() {
				authenticated := false
				defer func() { done <- authenticated }()
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				conn.SetDeadline(time.Now().Add(5 * time.Second))
				reader := bufio.NewReader(conn)
				fmt.Fprint(conn, "220 test ESMTP\r\n")
				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						return
					}
					switch {
					case strings.HasPrefix(line, "EHLO"):
						if mode == "downgrade" {
							fmt.Fprint(conn, "250-test\r\n250 AUTH PLAIN\r\n")
						} else {
							fmt.Fprint(conn, "250-test\r\n250-STARTTLS\r\n250 AUTH PLAIN\r\n")
						}
					case strings.HasPrefix(line, "STARTTLS"):
						fmt.Fprint(conn, "220 start TLS\r\n")
						secure := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12})
						if secure.Handshake() != nil {
							return
						}
						conn = secure
						reader = bufio.NewReader(conn)
					case strings.HasPrefix(line, "AUTH"):
						authenticated = true
						fmt.Fprint(conn, "235 authenticated\r\n")
					case strings.HasPrefix(line, "MAIL"), strings.HasPrefix(line, "RCPT"):
						fmt.Fprint(conn, "250 ok\r\n")
					case strings.HasPrefix(line, "DATA"):
						fmt.Fprint(conn, "354 send\r\n")
						for {
							line, err := reader.ReadString('\n')
							if err != nil {
								return
							}
							if line == ".\r\n" {
								break
							}
						}
						fmt.Fprint(conn, "250 accepted\r\n")
					case strings.HasPrefix(line, "QUIT"):
						fmt.Fprint(conn, "221 bye\r\n")
						return
					default:
						return
					}
				}
			}()
			_, port, _ := net.SplitHostPort(listener.Addr().String())
			number, _ := strconv.Atoi(port)
			provider := &EmailProvider{Host: "127.0.0.1", Port: number, Username: "test", Password: "test", From: "sender@example.com", To: []string{"recipient@example.com"}}
			alert := Alert{Level: "info", Title: "Delivery test", Timestamp: time.Now()}
			if mode == "verified" {
				err = provider.send(alert, &tls.Config{RootCAs: roots, ServerName: "127.0.0.1", MinVersion: tls.VersionTLS12})
			} else {
				err = provider.Send(alert)
			}
			if (err == nil) != (mode == "verified") {
				t.Fatalf("unexpected delivery result: %v", err)
			}
			if authenticated := <-done; authenticated != (mode == "verified") {
				t.Fatal("credentials sent before verified TLS")
			}
		})
	}
}
func TestPrivateSMTPConfigAndHeaderValidation(t *testing.T) {
	provider := EmailProvider{Host: "smtp.example.com", Port: 587, Username: "sender", Password: "private", From: "sender@example.com", To: []string{"recipient@example.com"}}
	data, _ := json.Marshal(provider)
	file := filepath.Join(t.TempDir(), "smtp.json")
	os.WriteFile(file, data, 0644)
	if _, err := LoadEmailProvider(file, nil); err == nil {
		t.Fatal("readable secret accepted")
	}
	os.Chmod(file, 0600)
	if _, err := LoadEmailProvider(file, nil); err != nil {
		t.Fatal(err)
	}
	provider.To = []string{"victim@example.com\r\nBcc: attacker@example.com"}
	if provider.validate() == nil {
		t.Fatal("header injection accepted")
	}
	provider.To = []string{"recipient@example.com"}
	if provider.Send(Alert{Title: "Hello\r\nBcc: attacker@example.com"}) == nil {
		t.Fatal("subject injection accepted")
	}
	os.WriteFile(file, append(data, []byte(" {}")...), 0600)
	if _, err := LoadEmailProvider(file, nil); err == nil {
		t.Fatal("trailing configuration accepted")
	}
}

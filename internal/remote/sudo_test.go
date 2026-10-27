package remote

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"golang.org/x/crypto/ssh"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSudoCommandsAndTransfersUseVerifiedPrivilege(t *testing.T) {
	for _, allowed := range []bool{false, true} {
		t.Run(map[bool]string{false: "denied", true: "allowed"}[allowed], func(t *testing.T) {
			_, key, _ := ed25519.GenerateKey(rand.Reader)
			signer, _ := ssh.NewSignerFromKey(key)
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			commands := make(chan string, 16)
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				config := &ssh.ServerConfig{NoClientAuth: true}
				config.AddHostKey(signer)
				server, channels, requests, err := ssh.NewServerConn(conn, config)
				if err != nil {
					return
				}
				defer server.Close()
				go ssh.DiscardRequests(requests)
				for channel := range channels {
					stream, requests, err := channel.Accept()
					if err != nil {
						return
					}
					go func() {
						defer stream.Close()
						for req := range requests {
							if req.Type != "exec" {
								req.Reply(false, nil)
								continue
							}
							var data struct{ Command string }
							ssh.Unmarshal(req.Payload, &data)
							commands <- data.Command
							req.Reply(true, nil)
							status := uint32(0)
							if data.Command == "sudo -n id -u" {
								if allowed {
									io.WriteString(stream, "0\n")
								} else {
									status = 1
								}
							} else if strings.Contains(data.Command, "cat >") {
								io.Copy(io.Discard, stream)
							} else {
								io.WriteString(stream, "payload")
							}
							payload := make([]byte, 4)
							binary.BigEndian.PutUint32(payload, status)
							stream.SendRequest("exit-status", false, payload)
							return
						}
					}()
				}
			}()
			conn, err := ssh.Dial("tcp", listener.Addr().String(), &ssh.ClientConfig{User: "operator", HostKeyCallback: ssh.FixedHostKey(signer.PublicKey())})
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			client := &Client{User: "operator", conn: conn}
			err = client.EnableSudo()
			if (err == nil) != allowed {
				t.Fatalf("sudo verification: %v", err)
			}
			if got := <-commands; got != "sudo -n id -u" {
				t.Fatal(got)
			}
			command := "printf '%s' 'literal;$(not-a-command)'"
			expected := command
			if allowed {
				expected = "sudo -n sh -c " + shellQuote(command)
			}
			if _, err := client.Run(command); err != nil {
				t.Fatal(err)
			}
			if got := <-commands; got != expected {
				t.Fatalf("command lost quoting: %s", got)
			}
			if !allowed {
				return
			}
			if err := client.UploadBytes([]byte("secret"), "/private/file with 'quote", 0600); err != nil {
				t.Fatal(err)
			}
			if got := <-commands; !strings.HasPrefix(got, "sudo -n sh -c ") || !strings.Contains(got, "chmod 600") {
				t.Fatal(got)
			}
			destination := filepath.Join(t.TempDir(), "download")
			if err := client.Download("/private/file", destination); err != nil {
				t.Fatal(err)
			}
			if got := <-commands; got != client.command("cat '/private/file'") {
				t.Fatal(got)
			}
			content, _ := os.ReadFile(destination)
			if string(content) != "payload" {
				t.Fatal("download changed")
			}
			var out bytes.Buffer
			if err := client.RunStream(command, &out, io.Discard); err != nil {
				t.Fatal(err)
			}
			if got := <-commands; got != expected {
				t.Fatal(got)
			}
		})
	}
}

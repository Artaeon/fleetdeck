package remote

import (
	"crypto/ed25519"
	"crypto/rand"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAgentAuthenticationAndIdentitySelection(t *testing.T) {
	// A real agent protocol and SSH handshake prove signing without exporting a key.
	dir, err := os.MkdirTemp("", "fd-agent-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	socket := filepath.Join(dir, "a")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_, private, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := ssh.NewSignerFromKey(private)
	keyring := agent.NewKeyring()
	if err := keyring.Add(agent.AddedKey{PrivateKey: private}); err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := listener.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); _ = agent.ServeAgent(keyring, c) }()
		}
	}()
	t.Setenv("SSH_AUTH_SOCK", socket)
	t.Setenv("HOME", dir)
	os.Mkdir(filepath.Join(dir, ".ssh"), 0700)
	for _, tc := range []struct {
		name, fingerprint string
		allowed           bool
	}{
		{"matching", ssh.FingerprintSHA256(signer.PublicKey()), true},
		{"wrong", "SHA256:wrong-identity", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("FLEETDECK_SSH_KEY_FINGERPRINT", tc.fingerprint)
			server, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer server.Close()
			host, port, _ := net.SplitHostPort(server.Addr().String())
			config := &ssh.ServerConfig{PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
				if string(key.Marshal()) != string(signer.PublicKey().Marshal()) {
					return nil, os.ErrPermission
				}
				return nil, nil
			}}
			config.AddHostKey(signer)
			line := knownhosts.Line([]string{knownhosts.Normalize(server.Addr().String())}, signer.PublicKey()) + "\n"
			os.WriteFile(filepath.Join(dir, ".ssh", "known_hosts"), []byte(line), 0600)
			done := make(chan struct{})
			go func() {
				defer close(done)
				c, err := server.Accept()
				if err != nil {
					return
				}
				defer c.Close()
				c.SetDeadline(time.Now().Add(10 * time.Second))
				sc, _, _, err := ssh.NewServerConn(c, config)
				if err == nil {
					sc.Close()
				}
			}()
			client, err := NewClient(host, port, "operator", nil, nil)
			if tc.allowed && err != nil {
				t.Fatal(err)
			}
			if !tc.allowed && err == nil {
				t.Fatal("unselected key authenticated")
			}
			if client != nil {
				client.Close()
			}
			<-done
		})
	}
}

func TestAgentUnavailableFailsClosed(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	if _, cleanup, err := authentication(nil, nil); err == nil {
		cleanup()
		t.Fatal("missing agent accepted")
	}
	t.Setenv("SSH_AUTH_SOCK", filepath.Join(t.TempDir(), "missing"))
	if _, cleanup, err := authentication(nil, nil); err == nil {
		cleanup()
		t.Fatal("missing socket accepted")
	}
	// An explicitly supplied malformed key must not fall back to the agent.
	if _, cleanup, err := authentication([]byte("invalid"), nil); err == nil {
		cleanup()
		t.Fatal("bad explicit key accepted")
	}
}

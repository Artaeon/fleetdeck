package remote

import (
	"crypto/ed25519"
	"encoding/pem"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func TestTOFURejectsRevokedHostKey(t *testing.T) {
	// Deterministic test material, only used by this loopback test server.
	private := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(private, "test-only-not-a-deployment-key")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("SSH_AUTH_SOCK", "")
	if err := os.Mkdir(filepath.Join(dir, ".ssh"), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".ssh", "known_hosts")
	entry := "@revoked * " + string(ssh.MarshalAuthorizedKey(signer.PublicKey()))
	if err := os.WriteFile(path, []byte(entry), 0600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := listener.(*net.TCPListener).SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	callback, err := knownhosts.New(path)
	if err != nil {
		t.Fatal(err)
	}
	var revoked *knownhosts.RevokedError
	if err := callback(listener.Addr().String(), listener.Addr(), signer.PublicKey()); !errors.As(err, &revoked) {
		t.Fatal("fixture did not revoke the server host key")
	}
	cfg := &ssh.ServerConfig{NoClientAuth: true}
	cfg.AddHostKey(signer)
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		server, _, _, err := ssh.NewServerConn(conn, cfg)
		if err == nil {
			server.Close()
		}
	}()
	client, err := NewClientTOFU(host, port, "test-operator", pem.EncodeToMemory(block), nil)
	if client != nil {
		client.Close()
	}
	<-done
	if err == nil {
		t.Fatal("TOFU accepted an explicitly revoked host key")
	}
	if !errors.As(err, &revoked) {
		t.Fatalf("revocation must remain the cause of the connection failure: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != entry {
		t.Error("revoked key was appended as a newly trusted host")
	}
}

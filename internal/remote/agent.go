package remote

import (
	"fmt"
	"net"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// authentication keeps private keys inside the local agent when no key file was
// explicitly supplied. No agent socket is forwarded to the server.
func authentication(privateKey, passphrase []byte) ([]ssh.AuthMethod, func(), error) {
	noop := func() {}
	if len(privateKey) > 0 {
		signer, err := ParsePrivateKey(privateKey, passphrase)
		if err != nil {
			return nil, noop, err
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, noop, nil
	}
	socket := os.Getenv("SSH_AUTH_SOCK")
	if socket == "" {
		return nil, noop, fmt.Errorf("no SSH key or agent: configure SSH_AUTH_SOCK (for example the 1Password SSH agent) or supply --key")
	}
	conn, err := net.DialTimeout("unix", socket, 5*time.Second)
	if err != nil {
		return nil, noop, fmt.Errorf("connecting to local SSH agent: %w", err)
	}
	// Bound user approval waits as well as a stalled or inaccessible agent.
	if err := conn.SetDeadline(time.Now().Add(2 * time.Minute)); err != nil {
		conn.Close()
		return nil, noop, err
	}
	cleanup := func() { conn.Close() }
	client := agent.NewClient(conn)
	fingerprint := os.Getenv("FLEETDECK_SSH_KEY_FINGERPRINT")
	callback := func() ([]ssh.Signer, error) {
		signers, err := client.Signers()
		if err != nil {
			return nil, fmt.Errorf("reading SSH agent identities: %w", err)
		}
		var allowed []ssh.Signer
		for _, signer := range signers {
			if fingerprint == "" || ssh.FingerprintSHA256(signer.PublicKey()) == fingerprint {
				allowed = append(allowed, signer)
			}
		}
		if len(allowed) == 0 {
			return nil, fmt.Errorf("SSH agent has no matching identity; unlock it and check FLEETDECK_SSH_KEY_FINGERPRINT")
		}
		return allowed, nil
	}
	return []ssh.AuthMethod{ssh.PublicKeysCallback(callback)}, cleanup, nil
}

package remote

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type Client struct {
	Host string
	Port string
	User string
	conn *ssh.Client
}

func ParsePrivateKey(data []byte) (ssh.Signer, error) {
	signer, err := ssh.ParsePrivateKey(data)
	if err != nil {
		// Detect encrypted keys and provide a helpful error message.
		if strings.Contains(err.Error(), "encrypted") || strings.Contains(err.Error(), "passphrase") {
			return nil, fmt.Errorf("private key is encrypted with a passphrase, which is not supported; use an unencrypted key or an SSH agent: %w", err)
		}
		return nil, fmt.Errorf("parsing private key: %w", err)
	}
	return signer, nil
}

// NewClient creates an SSH client using known_hosts-based host key verification.
// The known_hosts file is read from ~/.ssh/known_hosts. If the file does not
// exist, an error is returned; use NewClientInsecure to skip host key checking.
func NewClient(host, port, user string, privateKey []byte) (*Client, error) {
	signer, err := ParsePrivateKey(privateKey)
	if err != nil {
		return nil, err
	}

	home := os.Getenv("HOME")
	if home == "" {
		return nil, fmt.Errorf("HOME environment variable not set; cannot locate known_hosts (use NewClientTOFU for trust-on-first-use verification)")
	}
	knownHostsPath := filepath.Join(home, ".ssh", "known_hosts")
	hostKeyCallback, err := knownhosts.New(knownHostsPath)
	if err != nil {
		return nil, fmt.Errorf("reading known_hosts file %s: %w (use NewClientTOFU for trust-on-first-use verification)", knownHostsPath, err)
	}

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: hostKeyCallback,
	}

	return dialSSH(host, port, user, config)
}

// NewClientTOFU creates an SSH client using Trust On First Use host key
// verification. If the host is already in ~/.ssh/known_hosts, its key is
// verified normally. If the host is not yet known, the key is accepted and
// appended to known_hosts for future verification. This mirrors the behavior
// of ssh -o StrictHostKeyChecking=accept-new.
func NewClientTOFU(host, port, user string, privateKey []byte) (*Client, error) {
	signer, err := ParsePrivateKey(privateKey)
	if err != nil {
		return nil, err
	}

	home := os.Getenv("HOME")
	if home == "" {
		return nil, fmt.Errorf("HOME environment variable not set; cannot locate known_hosts")
	}

	sshDir := filepath.Join(home, ".ssh")
	knownHostsPath := filepath.Join(sshDir, "known_hosts")

	// Ensure ~/.ssh directory exists.
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return nil, fmt.Errorf("creating ssh directory %s: %w", sshDir, err)
	}

	// Try to load existing known_hosts; ignore errors if the file doesn't exist yet.
	var existingCallback ssh.HostKeyCallback
	if _, err := os.Stat(knownHostsPath); err == nil {
		cb, err := knownhosts.New(knownHostsPath)
		if err != nil {
			return nil, fmt.Errorf("reading known_hosts file %s: %w", knownHostsPath, err)
		}
		existingCallback = cb
	}

	callback := func(hostname string, addr net.Addr, key ssh.PublicKey) error {
		// If we have an existing known_hosts, check it first.
		if existingCallback != nil {
			err := existingCallback(hostname, addr, key)
			if err == nil {
				return nil // Host key matched.
			}
			// If the error is a key mismatch, reject it.
			if _, ok := err.(*knownhosts.KeyError); ok {
				ke := err.(*knownhosts.KeyError)
				if len(ke.Want) > 0 {
					return fmt.Errorf("host key mismatch for %s: %w", hostname, err)
				}
			}
			// Host not found in known_hosts; fall through to accept and save.
		}

		// Accept the key and append it to known_hosts.
		line := knownhosts.Line([]string{knownhosts.Normalize(hostname)}, key)
		f, err := os.OpenFile(knownHostsPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return fmt.Errorf("opening known_hosts for writing: %w", err)
		}
		defer f.Close()

		if _, err := fmt.Fprintln(f, line); err != nil {
			return fmt.Errorf("writing to known_hosts: %w", err)
		}
		return nil
	}

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: callback,
	}

	return dialSSH(host, port, user, config)
}

// NewClientInsecure creates an SSH client using Trust On First Use host key
// verification. Deprecated: use NewClientTOFU directly.
func NewClientInsecure(host, port, user string, privateKey []byte) (*Client, error) {
	return NewClientTOFU(host, port, user, privateKey)
}

func dialSSH(host, port, user string, config *ssh.ClientConfig) (*Client, error) {
	addr := net.JoinHostPort(host, port)
	conn, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", addr, err)
	}

	return &Client{
		Host: host,
		Port: port,
		User: user,
		conn: conn,
	}, nil
}

func (c *Client) Run(cmd string) (string, error) {
	stdout, stderr, err := c.RunWithStderr(cmd)
	if err != nil {
		if stderr != "" {
			return "", fmt.Errorf("%w: %s", err, stderr)
		}
		return "", err
	}
	return stdout, nil
}

func (c *Client) RunWithStderr(cmd string) (stdout, stderr string, err error) {
	session, err := c.conn.NewSession()
	if err != nil {
		return "", "", fmt.Errorf("creating session: %w", err)
	}
	defer session.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	session.Stdout = &stdoutBuf
	session.Stderr = &stderrBuf

	if err := session.Run(cmd); err != nil {
		return stdoutBuf.String(), stderrBuf.String(), err
	}
	return stdoutBuf.String(), stderrBuf.String(), nil
}

func (c *Client) RunStream(cmd string, stdout, stderr io.Writer) error {
	session, err := c.conn.NewSession()
	if err != nil {
		return fmt.Errorf("creating session: %w", err)
	}
	defer session.Close()

	session.Stdout = stdout
	session.Stderr = stderr

	if err := session.Run(cmd); err != nil {
		return err
	}
	return nil
}

func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *Client) IsConnected() bool {
	if c.conn == nil {
		return false
	}
	_, _, err := c.conn.SendRequest("keepalive@openssh.com", true, nil)
	return err == nil
}

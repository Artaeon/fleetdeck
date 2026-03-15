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
		return nil, fmt.Errorf("HOME environment variable not set; cannot locate known_hosts (use NewClientInsecure to skip host key verification)")
	}
	knownHostsPath := filepath.Join(home, ".ssh", "known_hosts")
	hostKeyCallback, err := knownhosts.New(knownHostsPath)
	if err != nil {
		return nil, fmt.Errorf("reading known_hosts file %s: %w (use NewClientInsecure to skip host key verification)", knownHostsPath, err)
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

// NewClientInsecure creates an SSH client that skips host key verification.
// This should only be used when the caller explicitly opts into insecure mode,
// such as in trusted or ephemeral environments.
func NewClientInsecure(host, port, user string, privateKey []byte) (*Client, error) {
	signer, err := ParsePrivateKey(privateKey)
	if err != nil {
		return nil, err
	}

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	return dialSSH(host, port, user, config)
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

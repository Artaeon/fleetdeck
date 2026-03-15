package remote

import (
	"bytes"
	"fmt"
	"io"
	"net"

	"golang.org/x/crypto/ssh"
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
		return nil, fmt.Errorf("parsing private key: %w", err)
	}
	return signer, nil
}

func NewClient(host, port, user string, privateKey []byte) (*Client, error) {
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

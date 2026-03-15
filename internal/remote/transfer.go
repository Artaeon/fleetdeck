package remote

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (c *Client) Upload(localPath, remotePath string) error {
	data, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("reading local file: %w", err)
	}

	info, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("stat local file: %w", err)
	}

	return c.UploadBytes(data, remotePath, info.Mode())
}

func (c *Client) UploadBytes(data []byte, remotePath string, mode os.FileMode) error {
	session, err := c.conn.NewSession()
	if err != nil {
		return fmt.Errorf("creating session: %w", err)
	}
	defer session.Close()

	dir := filepath.Dir(remotePath)
	cmd := fmt.Sprintf("mkdir -p %s && cat > %s && chmod %o %s",
		shellQuote(dir), shellQuote(remotePath), mode.Perm(), shellQuote(remotePath))

	session.Stdin = bytes.NewReader(data)
	var stderr bytes.Buffer
	session.Stderr = &stderr

	if err := session.Run(cmd); err != nil {
		return fmt.Errorf("uploading to %s: %s: %w", remotePath, strings.TrimSpace(stderr.String()), err)
	}
	return nil
}

func (c *Client) Download(remotePath, localPath string) error {
	session, err := c.conn.NewSession()
	if err != nil {
		return fmt.Errorf("creating session: %w", err)
	}
	defer session.Close()

	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return fmt.Errorf("creating local directory: %w", err)
	}

	f, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("creating local file: %w", err)
	}
	defer f.Close()

	var stderr bytes.Buffer
	session.Stdout = f
	session.Stderr = &stderr

	cmd := fmt.Sprintf("cat %s", shellQuote(remotePath))
	if err := session.Run(cmd); err != nil {
		os.Remove(localPath)
		return fmt.Errorf("downloading %s: %s: %w", remotePath, strings.TrimSpace(stderr.String()), err)
	}
	return nil
}

func (c *Client) UploadDir(localDir, remoteDir string) error {
	return filepath.Walk(localDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(localDir, path)
		if err != nil {
			return fmt.Errorf("computing relative path: %w", err)
		}
		remotePath := filepath.Join(remoteDir, rel)

		if info.IsDir() {
			_, mkdirErr := c.Run(fmt.Sprintf("mkdir -p %s", shellQuote(remotePath)))
			return mkdirErr
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}
		return c.UploadBytes(data, remotePath, info.Mode())
	})
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
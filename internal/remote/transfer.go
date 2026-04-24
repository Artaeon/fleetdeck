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

	return c.UploadBytes(data, remotePath, hardenSensitiveMode(remotePath, info.Mode()))
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

// skipDirs contains directory names that should never be uploaded to the
// server. These are build artifacts, dependency caches, and VCS metadata
// that would waste bandwidth and are rebuilt on the server.
var skipDirs = map[string]bool{
	"node_modules":   true,
	".next":          true,
	".nuxt":          true,
	".git":           true,
	".svn":           true,
	"dist":           true,
	"build":          true,
	"vendor":         true,
	"__pycache__":    true,
	".venv":          true,
	"venv":           true,
	".tox":           true,
	"target":         true, // Rust, Java
	".gradle":        true,
	".cache":         true,
	".parcel-cache":  true,
	".turbo":         true,
	".vercel":        true,
	".output":        true,
	"coverage":       true,
	".nyc_output":    true,
	".pytest_cache":  true,
	".mypy_cache":    true,
}

func (c *Client) UploadDir(localDir, remoteDir string) error {
	fileCount := 0
	return filepath.Walk(localDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip build artifacts and dependency directories.
		if info.IsDir() && skipDirs[info.Name()] {
			return filepath.SkipDir
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

		fileCount++
		if fileCount%50 == 0 {
			fmt.Printf("  uploaded %d files...\n", fileCount)
		}

		return c.UploadBytes(data, remotePath, hardenSensitiveMode(remotePath, info.Mode()))
	})
}

// hardenSensitiveMode narrows the upload permissions for files that are
// commonly treated as secrets regardless of how permissive they happen
// to be on the operator's laptop. A .env file sitting at 0644 locally
// should not land on the server 0644 just because chmod was never run.
//
// We match on the basename rather than the full path so the rule
// triggers consistently whether the upload comes from UploadDir (walking
// a project tree) or a direct Upload call.
func hardenSensitiveMode(remotePath string, mode os.FileMode) os.FileMode {
	base := strings.ToLower(filepath.Base(remotePath))
	// Exact .env names (covers .env, .env.local, .env.production, …).
	if base == ".env" || strings.HasPrefix(base, ".env.") {
		return 0600
	}
	// Private key material — match common extensions operators put in
	// project directories when rolling their own TLS or signing keys.
	sensitiveExt := []string{".pem", ".key", ".p12", ".pfx", ".jks"}
	for _, ext := range sensitiveExt {
		if strings.HasSuffix(base, ext) {
			return 0600
		}
	}
	return mode.Perm()
}

// shellQuote wraps s in single quotes for safe use in a shell command.
// Single quotes prevent all interpretation except for the quote character
// itself, which is handled by ending the quoted string, adding an escaped
// quote, and re-opening the quoted string. Null bytes are stripped because
// they cannot be represented in shell arguments and could truncate the string.
func shellQuote(s string) string {
	// Remove null bytes — they cannot appear in shell arguments and could
	// cause the argument to be silently truncated.
	s = strings.ReplaceAll(s, "\x00", "")
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
//go:build integration

package remote

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sshTestConfig(t *testing.T) (host, port, user string, key []byte) {
	t.Helper()

	host = os.Getenv("SSH_TEST_HOST")
	port = os.Getenv("SSH_TEST_PORT")
	user = os.Getenv("SSH_TEST_USER")
	keyPath := os.Getenv("SSH_TEST_KEY")

	if host == "" || port == "" || user == "" || keyPath == "" {
		t.Skip("skipping SSH integration test: SSH_TEST_HOST, SSH_TEST_PORT, SSH_TEST_USER, and SSH_TEST_KEY must be set")
	}

	var err error
	key, err = os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("reading SSH key %s: %v", keyPath, err)
	}
	return
}

func TestIntegrationSSHConnect(t *testing.T) {
	host, port, user, key := sshTestConfig(t)

	client, err := NewClientTOFU(host, port, user, key)
	if err != nil {
		t.Fatalf("NewClientTOFU failed: %v", err)
	}
	defer client.Close()

	out, err := client.Run("echo hello")
	if err != nil {
		t.Fatalf("Run(echo hello) failed: %v", err)
	}
	if strings.TrimSpace(out) != "hello" {
		t.Errorf("expected %q, got %q", "hello", strings.TrimSpace(out))
	}
}

func TestIntegrationSSHRunCommand(t *testing.T) {
	host, port, user, key := sshTestConfig(t)

	client, err := NewClientTOFU(host, port, user, key)
	if err != nil {
		t.Fatalf("NewClientTOFU failed: %v", err)
	}
	defer client.Close()

	out, err := client.Run("uname -s")
	if err != nil {
		t.Fatalf("Run(uname -s) failed: %v", err)
	}
	if strings.TrimSpace(out) != "Linux" {
		t.Errorf("expected %q, got %q", "Linux", strings.TrimSpace(out))
	}
}

func TestIntegrationSSHUploadDownload(t *testing.T) {
	host, port, user, key := sshTestConfig(t)

	client, err := NewClientTOFU(host, port, user, key)
	if err != nil {
		t.Fatalf("NewClientTOFU failed: %v", err)
	}
	defer client.Close()

	content := "fleetdeck integration test data"
	localDir := t.TempDir()
	localFile := filepath.Join(localDir, "upload.txt")
	if err := os.WriteFile(localFile, []byte(content), 0644); err != nil {
		t.Fatalf("writing local file: %v", err)
	}

	remotePath := "/tmp/fleetdeck_test_upload.txt"
	defer client.Run("rm -f " + remotePath)

	if err := client.Upload(localFile, remotePath); err != nil {
		t.Fatalf("Upload failed: %v", err)
	}

	downloadFile := filepath.Join(localDir, "download.txt")
	if err := client.Download(remotePath, downloadFile); err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	got, err := os.ReadFile(downloadFile)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}
	if string(got) != content {
		t.Errorf("downloaded content = %q, want %q", string(got), content)
	}
}

func TestIntegrationSSHUploadDir(t *testing.T) {
	host, port, user, key := sshTestConfig(t)

	client, err := NewClientTOFU(host, port, user, key)
	if err != nil {
		t.Fatalf("NewClientTOFU failed: %v", err)
	}
	defer client.Close()

	localDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(localDir, "subdir"), 0755); err != nil {
		t.Fatalf("creating subdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(localDir, "a.txt"), []byte("file a"), 0644); err != nil {
		t.Fatalf("writing a.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(localDir, "subdir", "b.txt"), []byte("file b"), 0644); err != nil {
		t.Fatalf("writing subdir/b.txt: %v", err)
	}

	remoteDir := "/tmp/fleetdeck_test_dir"
	defer client.Run("rm -rf " + remoteDir)

	if err := client.UploadDir(localDir, remoteDir); err != nil {
		t.Fatalf("UploadDir failed: %v", err)
	}

	out, err := client.Run("cat " + remoteDir + "/a.txt")
	if err != nil {
		t.Fatalf("reading remote a.txt: %v", err)
	}
	if strings.TrimSpace(out) != "file a" {
		t.Errorf("remote a.txt = %q, want %q", strings.TrimSpace(out), "file a")
	}

	out, err = client.Run("cat " + remoteDir + "/subdir/b.txt")
	if err != nil {
		t.Fatalf("reading remote subdir/b.txt: %v", err)
	}
	if strings.TrimSpace(out) != "file b" {
		t.Errorf("remote subdir/b.txt = %q, want %q", strings.TrimSpace(out), "file b")
	}
}

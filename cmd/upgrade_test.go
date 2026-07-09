package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestChecksumFor(t *testing.T) {
	manifest := "abc123  fleetdeck-linux-amd64\n" +
		"def456  fleetdeck-linux-arm64\n" +
		"999000  fleetdeck_1.0.0_linux_amd64.tar.gz\n"

	got, err := checksumFor(manifest, "fleetdeck-linux-arm64")
	if err != nil {
		t.Fatalf("checksumFor: %v", err)
	}
	if got != "def456" {
		t.Errorf("expected def456, got %s", got)
	}

	if _, err := checksumFor(manifest, "fleetdeck-darwin-amd64"); err == nil {
		t.Error("expected error for a filename not in the manifest")
	}
}

func TestSha256File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "blob")
	content := []byte("hello fleetdeck")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	sum := sha256.Sum256(content)
	want := hex.EncodeToString(sum[:])

	got, err := sha256File(path)
	if err != nil {
		t.Fatalf("sha256File: %v", err)
	}
	if got != want {
		t.Errorf("expected %s, got %s", want, got)
	}
}

func TestVerifyChecksumMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fleetdeck-linux-amd64")
	if err := os.WriteFile(path, []byte("real binary bytes"), 0644); err != nil {
		t.Fatal(err)
	}

	// A manifest whose recorded hash does not match the file must be rejected.
	// Point at a local file:// URL so no network is needed.
	manifestPath := filepath.Join(dir, "checksums.txt")
	if err := os.WriteFile(manifestPath, []byte("0000000000000000  fleetdeck-linux-amd64\n"), 0644); err != nil {
		t.Fatal(err)
	}

	err := verifyChecksum(path, "fleetdeck-linux-amd64", "file://"+manifestPath)
	if err == nil {
		t.Fatal("expected a checksum mismatch error, got nil")
	}
}

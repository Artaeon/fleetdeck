package cmd

import (
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func TestLocalFileMD5(t *testing.T) {
	dir := t.TempDir()

	// Test with a real file
	content := []byte("hello world")
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, content, 0644)

	got := localFileMD5(path)
	want := fmt.Sprintf("%x", md5.Sum(content))
	if got != want {
		t.Errorf("localFileMD5() = %q, want %q", got, want)
	}

	// Test with nonexistent file
	got = localFileMD5(filepath.Join(dir, "nonexistent"))
	if got != "" {
		t.Errorf("localFileMD5(nonexistent) = %q, want empty", got)
	}

	// Test with empty file
	emptyPath := filepath.Join(dir, "empty")
	os.WriteFile(emptyPath, []byte{}, 0644)
	got = localFileMD5(emptyPath)
	want = fmt.Sprintf("%x", md5.Sum([]byte{}))
	if got != want {
		t.Errorf("localFileMD5(empty) = %q, want %q", got, want)
	}
}

func TestLocalFileMD5_Consistency(t *testing.T) {
	dir := t.TempDir()
	content := []byte("Dockerfile content here\nFROM golang:1.23\n")
	path := filepath.Join(dir, "Dockerfile")
	os.WriteFile(path, content, 0644)

	// Should return same hash on multiple calls
	hash1 := localFileMD5(path)
	hash2 := localFileMD5(path)
	if hash1 != hash2 {
		t.Errorf("localFileMD5 not consistent: %q != %q", hash1, hash2)
	}

	// Different content should produce different hash
	path2 := filepath.Join(dir, "Dockerfile2")
	os.WriteFile(path2, []byte("FROM alpine:3.20\n"), 0644)
	hash3 := localFileMD5(path2)
	if hash1 == hash3 {
		t.Error("different files should produce different hashes")
	}
}

func TestUpdateCmdRegistered(t *testing.T) {
	// Verify the update command is registered on rootCmd
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "update <project>" {
			found = true
			break
		}
	}
	if !found {
		t.Error("update command not registered on rootCmd")
	}
}

func TestUpdateCmdFlags(t *testing.T) {
	flags := []string{
		"server", "dir", "port", "key", "passphrase",
		"insecure", "rebuild", "pull", "restart-only",
		"service", "no-cache", "pre-deploy", "post-deploy",
	}

	for _, name := range flags {
		if updateCmd.Flags().Lookup(name) == nil {
			t.Errorf("update command missing flag %q", name)
		}
	}
}

func TestUpdateCmdRequiresProjectName(t *testing.T) {
	// Verify the command requires exactly 1 arg
	if updateCmd.Args == nil {
		t.Error("update command should have Args validation")
	}
	// cobra.ExactArgs(1) should reject 0 args
	err := updateCmd.Args(updateCmd, []string{})
	if err == nil {
		t.Error("expected error when no project name provided")
	}
}

// Suppress unused import warning
var _ = cobra.ExactArgs

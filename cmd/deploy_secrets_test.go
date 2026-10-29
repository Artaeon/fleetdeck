package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPotentialSecretStopsBeforeSSHOrKeyAccess(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte("services: {app: {image: 'example/app:1'}}\n"), 0600)
	h := sha256.Sum256([]byte("synthetic FleetDeck deployment check"))
	os.WriteFile(filepath.Join(dir, "source.txt"), []byte("ghp_"+hex.EncodeToString(h[:])[:36]), 0600)
	// Nil profile and an unregistered target would fail much later. The source
	// gate must fail before resolving a server, reading keys or opening a socket.
	command := &cobra.Command{}
	command.SetContext(context.Background())
	err := deployRemote(command, dir, "demo", "demo.example.test", "unregistered-target", nil, "basic", 0, "", "", false, false)
	if err == nil || !strings.Contains(err.Error(), "potential secrets detected") {
		t.Fatalf("wrong failure: %v", err)
	}
}

package cmd

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

// Version, Commit, and BuildDate are normally injected at build time via
// -ldflags (see Makefile and .goreleaser.yaml). When the binary is built
// without those flags — for example via `go install` — we fall back to
// Go's embedded build info so the version command still reports something
// meaningful instead of the literal string "dev".
var (
	Version   = ""
	Commit    = ""
	BuildDate = ""
)

func init() {
	if Version == "" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
			Version = info.Main.Version
		}
	}
	if Commit == "" || BuildDate == "" {
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, s := range info.Settings {
				switch s.Key {
				case "vcs.revision":
					if Commit == "" && s.Value != "" {
						Commit = s.Value
					}
				case "vcs.time":
					if BuildDate == "" && s.Value != "" {
						BuildDate = s.Value
					}
				}
			}
		}
	}
	if Version == "" {
		Version = "dev"
	}
}

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade FleetDeck to the latest version",
	RunE: func(cmd *cobra.Command, args []string) error {
		ui.Info("Current version: %s", Version)
		ui.Info("Checking for updates...")

		arch := runtime.GOARCH
		platform := runtime.GOOS

		binName := fmt.Sprintf("fleetdeck-%s-%s", platform, arch)
		baseURL := "https://github.com/fleetdeck/fleetdeck/releases/latest/download"
		downloadURL := baseURL + "/" + binName

		ui.Info("Downloading from %s...", downloadURL)

		execPath, err := os.Executable()
		if err != nil {
			return fmt.Errorf("finding current binary: %w", err)
		}

		tmpPath := execPath + ".new"
		dlCmd := exec.Command("curl", "-fsSL", "-o", tmpPath, downloadURL)
		dlCmd.Stdout = os.Stdout
		dlCmd.Stderr = os.Stderr
		if err := dlCmd.Run(); err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("downloading update: %w", err)
		}

		// Verify the download against the published checksums before trusting it
		// enough to overwrite the running binary. Without this, a MITM, DNS
		// hijack, or a tampered release asset could hand us an arbitrary binary
		// to execute — typically as root on a deploy host.
		ui.Info("Verifying checksum...")
		if err := verifyChecksum(tmpPath, binName, baseURL+"/checksums.txt"); err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("checksum verification failed: %w", err)
		}
		ui.Success("Checksum verified")

		if err := os.Chmod(tmpPath, 0755); err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("setting permissions: %w", err)
		}

		if err := os.Rename(tmpPath, execPath); err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("replacing binary: %w", err)
		}

		ui.Success("FleetDeck upgraded successfully!")
		ui.Info("Restart any running FleetDeck processes to use the new version.")
		return nil
	},
}

// verifyChecksum downloads the release checksums manifest, looks up the SHA256
// recorded for binName, and confirms the file at path hashes to that value.
// It returns an error if the manifest can't be fetched, has no entry for
// binName, or the hashes don't match.
func verifyChecksum(path, binName, checksumURL string) error {
	manifest, err := exec.Command("curl", "-fsSL", checksumURL).Output()
	if err != nil {
		return fmt.Errorf("downloading %s: %w", checksumURL, err)
	}
	want, err := checksumFor(string(manifest), binName)
	if err != nil {
		return err
	}
	got, err := sha256File(path)
	if err != nil {
		return fmt.Errorf("hashing downloaded binary: %w", err)
	}
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", binName, want, got)
	}
	return nil
}

// checksumFor parses a `sha256  filename` checksums manifest (as produced by
// goreleaser) and returns the checksum recorded for filename.
func checksumFor(manifest, filename string) (string, error) {
	scanner := bufio.NewScanner(strings.NewReader(manifest))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && fields[1] == filename {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("no checksum entry for %q in checksums.txt", filename)
}

// sha256File returns the lowercase hex SHA256 of the file at path.
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of FleetDeck",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("fleetdeck %s (%s/%s)\n", Version, runtime.GOOS, runtime.GOARCH)
		if Commit != "" {
			commit := Commit
			if len(commit) > 12 {
				commit = commit[:12]
			}
			fmt.Printf("  commit: %s\n", commit)
		}
		if BuildDate != "" {
			fmt.Printf("  built:  %s\n", BuildDate)
		}
	},
}

func init() {
	rootCmd.AddCommand(upgradeCmd)
	rootCmd.AddCommand(versionCmd)
}

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"

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

		downloadURL := fmt.Sprintf(
			"https://github.com/fleetdeck/fleetdeck/releases/latest/download/fleetdeck-%s-%s",
			platform, arch,
		)

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

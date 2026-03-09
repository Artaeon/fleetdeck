package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

var Version = "dev"

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
	},
}

func init() {
	rootCmd.AddCommand(upgradeCmd)
	rootCmd.AddCommand(versionCmd)
}

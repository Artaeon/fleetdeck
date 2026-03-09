package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize FleetDeck on a fresh server",
	Long: `Sets up the server for FleetDeck:
- Verifies Docker, Docker Compose, Git, and gh CLI are installed
- Creates the base directory (/opt/fleetdeck)
- Sets up Traefik if not present
- Initializes the SQLite database`,
	RunE: func(cmd *cobra.Command, args []string) error {
		totalSteps := 5

		// Step 1: Check required tools
		ui.Step(1, totalSteps, "Checking required tools...")
		tools := []string{"docker", "git", "gh"}
		for _, tool := range tools {
			if _, err := exec.LookPath(tool); err != nil {
				return fmt.Errorf("%s is not installed. Please install it first", tool)
			}
			ui.Success("%s found", tool)
		}

		// Check Docker Compose (v2 plugin)
		composeCheck := exec.Command("docker", "compose", "version")
		if err := composeCheck.Run(); err != nil {
			return fmt.Errorf("docker compose plugin is not installed")
		}
		ui.Success("docker compose found")

		// Step 2: Create base directory
		ui.Step(2, totalSteps, "Creating base directory %s...", cfg.Server.BasePath)
		if err := os.MkdirAll(cfg.Server.BasePath, 0755); err != nil {
			return fmt.Errorf("creating base directory: %w", err)
		}
		ui.Success("Base directory ready")

		// Step 3: Initialize database
		ui.Step(3, totalSteps, "Initializing database...")
		database = nil // force re-open
		openDB()
		ui.Success("Database initialized at %s", cfg.DBPath())

		// Step 4: Check/setup Traefik network
		ui.Step(4, totalSteps, "Checking Traefik network...")
		networkCheck := exec.Command("docker", "network", "inspect", cfg.Traefik.Network)
		if err := networkCheck.Run(); err != nil {
			ui.Info("Creating Traefik network %s...", cfg.Traefik.Network)
			createNet := exec.Command("docker", "network", "create", cfg.Traefik.Network)
			if out, err := createNet.CombinedOutput(); err != nil {
				return fmt.Errorf("creating Traefik network: %s: %w", string(out), err)
			}
			ui.Success("Traefik network created")
		} else {
			ui.Success("Traefik network exists")
		}

		// Step 5: Check if Traefik is running
		ui.Step(5, totalSteps, "Checking Traefik status...")
		traefikCheck := exec.Command("docker", "ps", "--filter", "name=traefik", "--format", "{{.Names}}")
		out, err := traefikCheck.Output()
		if err != nil || len(out) == 0 {
			ui.Warn("Traefik is not running. You'll need to start Traefik separately.")
			ui.Info("Example: docker run -d --name traefik --network %s \\", cfg.Traefik.Network)
			ui.Info("  -p 80:80 -p 443:443 \\")
			ui.Info("  -v /var/run/docker.sock:/var/run/docker.sock \\")
			ui.Info("  traefik:v3.0 --providers.docker --entrypoints.websecure.address=:443 \\")
			ui.Info("  --certificatesresolvers.myresolver.acme.tlschallenge=true \\")
			ui.Info("  --certificatesresolvers.myresolver.acme.email=you@example.com \\")
			ui.Info("  --certificatesresolvers.myresolver.acme.storage=/acme.json")
		} else {
			ui.Success("Traefik is running")
		}

		// Save default config if none exists
		if cfgFile == "" {
			if _, err := os.Stat("/etc/fleetdeck/config.toml"); os.IsNotExist(err) {
				if err := cfg.Save(""); err != nil {
					ui.Warn("Could not save default config: %v", err)
				} else {
					ui.Success("Default config saved to /etc/fleetdeck/config.toml")
				}
			}
		}

		fmt.Println()
		ui.Success("FleetDeck initialized! Run 'fleetdeck create <name>' to create your first project.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}

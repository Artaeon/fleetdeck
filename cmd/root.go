package cmd

import (
	"os"

	"github.com/fleetdeck/fleetdeck/internal/audit"
	"github.com/fleetdeck/fleetdeck/internal/config"
	"github.com/fleetdeck/fleetdeck/internal/crypto"
	"github.com/fleetdeck/fleetdeck/internal/db"
	"github.com/fleetdeck/fleetdeck/internal/templates"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

var (
	cfgFile string
	cfg     *config.Config
	database *db.DB
)

var rootCmd = &cobra.Command{
	Use:   "fleetdeck",
	Short: "Lightweight self-hosted deployment platform",
	Long: `FleetDeck is a CLI-first deployment platform for developers who run
multiple Docker projects on a single server with Traefik.

It automates Linux user creation, SSH key generation, GitHub repo setup,
Docker Compose configuration, and CI/CD workflow generation.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		var err error
		cfg, err = config.Load(cfgFile)
		if err != nil {
			ui.Error("Failed to load config: %v", err)
			os.Exit(1)
		}

		// Load custom templates from disk
		if err := templates.LoadCustomTemplates(cfg.Server.BasePath); err != nil {
			ui.Warn("Could not load custom templates: %v", err)
		}

		// Initialize audit logging
		if cfg.Audit.Enabled {
			if err := audit.Init(cfg.Audit.LogPath); err != nil {
				ui.Warn("Could not initialize audit log: %v", err)
			}
		}
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: /etc/fleetdeck/config.toml)")
}

func Execute() error {
	defer func() {
		audit.Close()
		if database != nil {
			database.Close()
		}
	}()
	return rootCmd.Execute()
}

func openDB() *db.DB {
	if database != nil {
		return database
	}
	var err error
	database, err = db.Open(cfg.DBPath())
	if err != nil {
		ui.Error("Failed to open database: %v", err)
		os.Exit(1)
	}

	// Configure secret encryption if an encryption key is set
	if cfg.Server.EncryptionKey != "" {
		key := crypto.DeriveKeyFromPassphrase(cfg.Server.EncryptionKey)
		database.SetEncryptionKey(key)
	}

	return database
}

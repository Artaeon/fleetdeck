package cmd

import (
	"os"

	"github.com/fleetdeck/fleetdeck/internal/audit"
	"github.com/fleetdeck/fleetdeck/internal/config"
	"github.com/fleetdeck/fleetdeck/internal/crypto"
	"github.com/fleetdeck/fleetdeck/internal/db"
	_ "github.com/fleetdeck/fleetdeck/internal/profiles" // Register deployment profiles
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
	Short: "One-click deployment platform for self-hosted applications",
	Long: `FleetDeck is a smart, CLI-first deployment platform that takes your
application from code to production with a single command.

Features:
  - Auto-detect app type and recommend deployment profile
  - One-command deploy: local or remote via SSH
  - Server provisioning: Docker, Traefik, firewall, SSL
  - Deployment profiles: bare, server, saas, static, worker, fullstack
  - Zero-downtime deploys: basic, blue/green, rolling strategies
  - Health monitoring with webhook, Slack, and email alerts
  - DNS management (Cloudflare) with auto-configuration
  - Environment management: staging, production, preview
  - Backups, rollback, audit logging, and web dashboard`,
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

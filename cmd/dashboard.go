package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/server"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Start the web dashboard",
	Long:  "Starts the FleetDeck web dashboard for visual project management.",
	RunE: func(cmd *cobra.Command, args []string) error {
		addr, _ := cmd.Flags().GetString("addr")
		if err := server.ValidateAPIToken(cfg.Server.APIToken); err != nil {
			return err
		}

		d := openDB()
		srv := server.New(cfg, d, addr)

		// Graceful shutdown
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		go func() {
			<-ctx.Done()
			ui.Info("Shutting down dashboard...")
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := srv.Shutdown(shutdownCtx); err != nil {
				ui.Warn("Forced shutdown: %v", err)
			}
		}()

		fmt.Println()
		ui.Success("FleetDeck Dashboard started")
		ui.Info("Listening on http://%s", addr)
		ui.Info("Authentication enabled (Bearer token / cookie)")
		if cfg.Server.WebhookSecret == "" {
			ui.Warn("No webhook_secret configured — GitHub webhooks are disabled")
		}
		ui.Info("Press Ctrl+C to stop")
		fmt.Println()

		return srv.Start()
	},
}

func init() {
	dashboardCmd.Flags().String("addr", "127.0.0.1:8420", "Listen address (host:port)")
	rootCmd.AddCommand(dashboardCmd)
}

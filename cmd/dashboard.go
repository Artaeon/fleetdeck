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
		if err := server.ValidateDashboardAuth(cfg.Server); err != nil {
			return err
		}
		cert, _ := cmd.Flags().GetString("tls-cert")
		key, _ := cmd.Flags().GetString("tls-key")
		ca, _ := cmd.Flags().GetString("client-ca")
		clients, _ := cmd.Flags().GetStringArray("allow-client-cert")
		options := server.ListenerOptions{CertFile: cert, KeyFile: key, ClientCAFile: ca, AllowedClients: clients}
		if _, err := options.TLSConfig(addr); err != nil {
			return err
		}

		d := openDB()
		srv := server.New(cfg, d, addr)
		defer srv.Shutdown(context.Background())
		if err := srv.ConfigureListener(options); err != nil {
			return err
		}

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
		ui.Info("Starting FleetDeck Dashboard")
		scheme := "http"
		if cert != "" {
			scheme = "https"
			ui.Info("Device certificate required for every connection")
		}
		ui.Info("Listen address: %s://%s", scheme, addr)
		if cfg.Server.OIDC.Enabled {
			ui.Info("Authentication: Authelia with required MFA and admin group; token login disabled")
		} else {
			ui.Info("Authentication enabled (Bearer token / cookie)")
		}
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
	dashboardCmd.Flags().String("tls-cert", "", "Dashboard server certificate PEM file")
	dashboardCmd.Flags().String("tls-key", "", "Dashboard server private key PEM file")
	dashboardCmd.Flags().String("client-ca", "", "CA bundle for dashboard client certificates")
	dashboardCmd.Flags().StringArray("allow-client-cert", nil, "Authorized device leaf certificate PEM file (repeat for each device)")
	rootCmd.AddCommand(dashboardCmd)
}

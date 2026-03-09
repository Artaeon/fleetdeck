package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

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

		d := openDB()
		srv := server.New(cfg, d, addr)

		// Graceful shutdown
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		go func() {
			<-ctx.Done()
			ui.Info("Shutting down dashboard...")
			srv.Shutdown(context.Background())
		}()

		fmt.Println()
		ui.Success("FleetDeck Dashboard started")
		ui.Info("Listening on http://%s", addr)
		ui.Info("Press Ctrl+C to stop")
		fmt.Println()

		return srv.Start()
	},
}

func init() {
	dashboardCmd.Flags().String("addr", ":8420", "Listen address (host:port)")
	rootCmd.AddCommand(dashboardCmd)
}

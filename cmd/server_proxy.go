package cmd

import (
	"github.com/fleetdeck/fleetdeck/internal/edge"
	"github.com/spf13/cobra"
)

func init() {
	serverCmd.AddCommand(&cobra.Command{
		Use:   "reconnect-proxy",
		Short: "Reconnect the trusted reverse proxy after recreating its container",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return edge.Reconnect(edge.LocalRunner{Context: cmd.Context()})
		},
	})
}

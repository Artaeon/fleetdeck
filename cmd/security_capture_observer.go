package cmd

import (
	"github.com/fleetdeck/fleetdeck/internal/securityreport"
	"github.com/spf13/cobra"
	"time"
)

func init() {
	var node, path string
	c := &cobra.Command{Use: "capture-observer", Short: "Record security counts from an authenticated observer snapshot on stdin", Args: cobra.NoArgs, PersistentPreRun: func(*cobra.Command, []string) {}, RunE: func(cmd *cobra.Command, _ []string) error {
		return securityreport.CaptureObserver(path, node, cmd.InOrStdin(), time.Now())
	}}
	c.Flags().StringVar(&node, "node-id", "", "Expected node identity")
	c.Flags().StringVar(&path, "output", "", "Private per-node observation history")
	securityCmd.AddCommand(c)
}

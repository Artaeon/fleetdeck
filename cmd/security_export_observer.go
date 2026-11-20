package cmd

import (
	"fmt"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/fleetexport"
	"github.com/spf13/cobra"
)

func init() {
	var node, state, output string
	c := &cobra.Command{
		Use: "export-observer", Short: "Write aggregate watcher counts for an independent reporting host", Args: cobra.NoArgs,
		PersistentPreRun: func(*cobra.Command, []string) {},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if node == "" || state == "" || output == "" {
				return fmt.Errorf("node ID, recorded watcher state and private output path are required")
			}
			s, err := fleetexport.ReportSnapshot(node, state, time.Now())
			if err != nil {
				return err
			}
			return fleetexport.WriteSnapshot(output, s)
		},
	}
	c.Flags().StringVar(&node, "node-id", "", "Enrolled source identifier")
	c.Flags().StringVar(&state, "security-state", "", "Private security watcher state")
	c.Flags().StringVar(&output, "output", "", "Atomic aggregate snapshot in a protected directory")
	securityCmd.AddCommand(c)
}

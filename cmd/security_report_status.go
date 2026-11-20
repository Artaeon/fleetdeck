package cmd

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/securityreport"
	"github.com/spf13/cobra"
)

func newSecurityReportStatusCmd() *cobra.Command {
	var path string
	var age time.Duration
	c := &cobra.Command{Use: "report-status", Short: "Check the last reporting run and outstanding deliveries", Args: cobra.NoArgs, PersistentPreRun: func(*cobra.Command, []string) {}, RunE: func(cmd *cobra.Command, _ []string) error {
		status, err := securityreport.Status(path, time.Now(), age)
		if err != nil {
			return err
		}
		if err = json.NewEncoder(cmd.OutOrStdout()).Encode(status); err != nil {
			return err
		}
		if status.Status != "current" {
			return fmt.Errorf("calendar report is %s", status.Status)
		}
		return nil
	}}
	c.Flags().StringVar(&path, "delivery-state", "", "Private daily or weekly checkpoint")
	c.Flags().DurationVar(&age, "max-age", 2*time.Hour, "Maximum age of the reporting job heartbeat")
	return c
}
func init() { securityCmd.AddCommand(newSecurityReportStatusCmd()) }

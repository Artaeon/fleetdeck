package cmd

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/fleet"
	"github.com/fleetdeck/fleetdeck/internal/securityreport"
	"github.com/spf13/cobra"
)

func newExportReportStatusCmd() *cobra.Command {
	var daily, weekly string
	c := &cobra.Command{Use: "export-report-status", Short: "Export only the two evaluation times and pending counts", Args: cobra.NoArgs,
		PersistentPreRun: func(*cobra.Command, []string) {},
		RunE: func(cmd *cobra.Command, _ []string) error {
			now := time.Now().UTC()
			var out fleet.ReportRuns
			read := func(path, edition string, result *fleet.ReportRun) error {
				status, err := securityreport.Status(path, now, 2*time.Hour)
				if err != nil || status.Edition != edition || status.LastEvaluation.IsZero() {
					return fmt.Errorf("cannot project the %s reporting checkpoint", edition)
				}
				*result = fleet.ReportRun{LastEvaluation: status.LastEvaluation, Pending: status.Pending}
				return nil
			}
			if err := read(daily, "daily", &out.Daily); err != nil {
				return err
			}
			if err := read(weekly, "weekly", &out.Weekly); err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(out)
		}}
	c.Flags().StringVar(&daily, "daily-state", "", "Private daily report checkpoint")
	c.Flags().StringVar(&weekly, "weekly-state", "", "Private weekly report checkpoint")
	return c
}

func init() { securityCmd.AddCommand(newExportReportStatusCmd()) }

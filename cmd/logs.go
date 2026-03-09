package cmd

import (
	"os"

	"github.com/fleetdeck/fleetdeck/internal/project"
	"github.com/spf13/cobra"
)

var logsCmd = &cobra.Command{
	Use:   "logs <name>",
	Short: "View project logs",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		d := openDB()
		p, err := d.GetProject(args[0])
		if err != nil {
			return err
		}

		service, _ := cmd.Flags().GetString("service")
		tail, _ := cmd.Flags().GetInt("tail")
		follow, _ := cmd.Flags().GetBool("follow")

		logCmd := project.ComposeLogs(p.ProjectPath, service, tail, follow)
		logCmd.Stdout = os.Stdout
		logCmd.Stderr = os.Stderr
		return logCmd.Run()
	},
}

func init() {
	logsCmd.Flags().StringP("service", "s", "", "Specific service to show logs for")
	logsCmd.Flags().IntP("tail", "t", 100, "Number of lines to show")
	logsCmd.Flags().BoolP("follow", "f", false, "Follow log output")

	rootCmd.AddCommand(logsCmd)
}

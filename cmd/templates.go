package cmd

import (
	"github.com/fleetdeck/fleetdeck/internal/templates"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

var templatesCmd = &cobra.Command{
	Use:     "templates",
	Aliases: []string{"tpl"},
	Short:   "List available project templates",
	Run: func(cmd *cobra.Command, args []string) {
		tmplList := templates.List()

		headers := []string{"NAME", "DESCRIPTION"}
		var rows [][]string
		for _, t := range tmplList {
			rows = append(rows, []string{t.Name, t.Description})
		}

		ui.Table(headers, rows)
	},
}

func init() {
	rootCmd.AddCommand(templatesCmd)
}

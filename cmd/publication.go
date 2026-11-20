package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/fleetdeck/fleetdeck/internal/publication"
	"github.com/spf13/cobra"
)

func init() {
	group := &cobra.Command{Use: "publish", Short: "Use a project-specific static publication identity", PersistentPreRun: func(*cobra.Command, []string) {}}
	load := func() (publication.Catalog, error) {
		base, err := os.UserConfigDir()
		if err != nil {
			return publication.Catalog{}, err
		}
		return publication.Load(filepath.Join(base, "fleetdeck", "publication.toml"))
	}
	group.AddCommand(&cobra.Command{Use: "targets", Short: "List enrolled hosts and project publication grants", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		cfg, err := load()
		if err != nil {
			return err
		}
		return json.NewEncoder(c.OutOrStdout()).Encode(cfg.List())
	}})
	for _, action := range []string{"status", "verify", "apply"} {
		use := action + " PROJECT"
		n := 1
		if action != "status" {
			use += " ARCHIVE"
			n = 2
		}
		c := &cobra.Command{Use: use, Short: map[string]string{"status": "Read the active project release", "verify": "Verify a candidate on its enrolled host", "apply": "Publish a verified static project artifact"}[action], Args: cobra.ExactArgs(n), RunE: func(c *cobra.Command, args []string) error {
			cfg, err := load()
			if err != nil {
				return err
			}
			var path, prev string
			if len(args) == 2 {
				path = args[1]
				prev, _ = c.Flags().GetString("expected")
			}
			r, err := cfg.Run(c.Context(), args[0], action, path, prev)
			if err != nil {
				return err
			}
			return json.NewEncoder(c.OutOrStdout()).Encode(r)
		}}
		if action != "status" {
			c.Flags().String("expected", "", "Require this exact currently active release")
		}
		group.AddCommand(c)
	}
	rootCmd.AddCommand(group)
}

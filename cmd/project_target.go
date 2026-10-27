package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/fleetdeck/fleetdeck/internal/profiles"
	"github.com/fleetdeck/fleetdeck/internal/project"
	"github.com/fleetdeck/fleetdeck/internal/projectconfig"
	"github.com/spf13/cobra"
)

var projectCmd = &cobra.Command{Use: "project", Short: "Remember private deployment targets for local projects"}
var projectLinkCmd = &cobra.Command{Use: "link [directory]", Short: "Save a named server and deployment settings outside the repository", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}
	target := projectconfig.Target{}
	target.Server, _ = cmd.Flags().GetString("server")
	target.Name, _ = cmd.Flags().GetString("name")
	target.Domain, _ = cmd.Flags().GetString("domain")
	target.Profile, _ = cmd.Flags().GetString("profile")
	target.ServerConfig, _ = cmd.Flags().GetString("server-config")
	if _, err := openDB().GetServer(target.Server); err != nil {
		return fmt.Errorf("register the named server with 'fleetdeck server add' first")
	}
	if err := validateProjectTarget(target); err != nil {
		return err
	}
	replace, _ := cmd.Flags().GetBool("replace")
	path, err := projectconfig.Save(dir, target, replace)
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Private project target:", path)
	fmt.Fprintln(cmd.OutOrStdout(), "Next: fleetdeck deploy", dir)
	return nil
}}
var projectShowCmd = &cobra.Command{Use: "show [directory]", Short: "Show the saved target for this local project", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}
	target, err := projectconfig.Load(dir)
	if err != nil {
		return err
	}
	if target == nil {
		return fmt.Errorf("no saved target; use 'fleetdeck project link'")
	}
	return json.NewEncoder(cmd.OutOrStdout()).Encode(target)
}}

func validateProjectTarget(t projectconfig.Target) error {
	if err := project.ValidateName(t.Name); err != nil {
		return err
	}
	if err := validateDomain(t.Domain); err != nil {
		return err
	}
	if t.Profile != "" {
		if _, err := profiles.Get(t.Profile); err != nil {
			return err
		}
	}
	if t.Server == "" || !filepath.IsAbs(t.ServerConfig) || strings.ContainsAny(t.ServerConfig, "\x00\r\n") {
		return fmt.Errorf("a named server and absolute server-config path are required")
	}
	return nil
}
func applyProjectTarget(cmd *cobra.Command, dir string) error {
	target, err := projectconfig.Load(dir)
	if err != nil {
		return err
	}
	if target == nil {
		return nil
	}
	if err = validateProjectTarget(*target); err != nil {
		return err
	}
	for flag, value := range map[string]string{"server": target.Server, "name": target.Name, "domain": target.Domain, "profile": target.Profile, "server-config": target.ServerConfig} {
		if !cmd.Flags().Changed(flag) && value != "" {
			if err = cmd.Flags().Set(flag, value); err != nil {
				return err
			}
		}
	}
	return nil
}
func init() {
	projectLinkCmd.Flags().String("server", "", "Registered server name")
	projectLinkCmd.Flags().String("name", "", "Stable project name on the server")
	projectLinkCmd.Flags().String("domain", "", "Application hostname")
	projectLinkCmd.Flags().String("profile", "", "Optional deployment profile")
	projectLinkCmd.Flags().String("server-config", "/etc/fleetdeck/config.toml", "Operator-owned configuration on the server")
	projectLinkCmd.Flags().Bool("replace", false, "Explicitly replace an existing local target")
	projectCmd.AddCommand(projectLinkCmd, projectShowCmd)
	rootCmd.AddCommand(projectCmd)
}

package cmd

import (
	"fmt"

	"github.com/fleetdeck/fleetdeck/internal/environments"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Environment management (staging, production, preview)",
	Long: `Manage multiple deployment environments per project.

Each environment gets its own Docker Compose stack, domain prefix,
and configuration. Use 'promote' to push staging to production.`,
}

var envCreateCmd = &cobra.Command{
	Use:   "create <project> <environment>",
	Short: "Create a new environment",
	Long: `Creates a new environment for a project.

Examples:
  fleetdeck env create myapp staging --domain staging.myapp.com
  fleetdeck env create myapp preview --domain preview.myapp.com --branch feature/new-ui`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectName := args[0]
		envName := args[1]

		domain, _ := cmd.Flags().GetString("domain")
		branch, _ := cmd.Flags().GetString("branch")

		if domain == "" {
			// Auto-generate domain: envName.originalDomain
			d := openDB()
			proj, err := d.GetProject(projectName)
			if err != nil {
				return fmt.Errorf("project %q not found: %w", projectName, err)
			}
			domain = envName + "." + proj.Domain
		}

		mgr := environments.NewManager(cfg.Server.BasePath)
		env, err := mgr.Create(projectName, envName, domain, branch)
		if err != nil {
			return fmt.Errorf("creating environment: %w", err)
		}

		fmt.Println()
		ui.Success("Environment %s created for %s", ui.Bold(envName), ui.Bold(projectName))
		ui.Info("Domain: %s", env.Domain)
		ui.Info("Path: %s", mgr.GetEnvPath(projectName, envName))
		if branch != "" {
			ui.Info("Branch: %s", branch)
		}
		fmt.Println()
		ui.Info("Start with: cd %s && docker compose up -d", mgr.GetEnvPath(projectName, envName))
		fmt.Println()

		return nil
	},
}

var envListCmd = &cobra.Command{
	Use:   "list <project>",
	Short: "List environments for a project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectName := args[0]

		mgr := environments.NewManager(cfg.Server.BasePath)
		envs, err := mgr.List(projectName)
		if err != nil {
			return err
		}

		if len(envs) == 0 {
			ui.Info("No environments found for %s", projectName)
			ui.Info("Create one with: fleetdeck env create %s staging", projectName)
			return nil
		}

		fmt.Println()
		headers := []string{"Environment", "Domain", "Branch", "Status", "Created"}
		var rows [][]string
		for _, env := range envs {
			rows = append(rows, []string{
				env.Name,
				env.Domain,
				env.Branch,
				ui.StatusColor(env.Status),
				env.CreatedAt.Format("2006-01-02 15:04"),
			})
		}
		ui.Table(headers, rows)
		fmt.Println()

		return nil
	},
}

var envPromoteCmd = &cobra.Command{
	Use:   "promote <project> <from> <to>",
	Short: "Promote one environment to another",
	Long: `Promotes configuration and images from one environment to another.

Example:
  fleetdeck env promote myapp staging production`,
	Args: cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectName := args[0]
		fromEnv := args[1]
		toEnv := args[2]

		mgr := environments.NewManager(cfg.Server.BasePath)
		if err := mgr.Promote(projectName, fromEnv, toEnv); err != nil {
			return fmt.Errorf("promoting %s to %s: %w", fromEnv, toEnv, err)
		}

		fmt.Println()
		ui.Success("Promoted %s to %s for %s", ui.Bold(fromEnv), ui.Bold(toEnv), ui.Bold(projectName))
		fmt.Println()

		return nil
	},
}

var envDeleteCmd = &cobra.Command{
	Use:   "delete <project> <environment>",
	Short: "Delete an environment",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectName := args[0]
		envName := args[1]

		mgr := environments.NewManager(cfg.Server.BasePath)
		if err := mgr.Delete(projectName, envName); err != nil {
			return fmt.Errorf("deleting environment: %w", err)
		}

		ui.Success("Environment %s deleted from %s", envName, projectName)
		return nil
	},
}

func init() {
	envCreateCmd.Flags().String("domain", "", "Domain for the environment (auto-generated if not set)")
	envCreateCmd.Flags().String("branch", "", "Git branch for this environment")

	envCmd.AddCommand(envCreateCmd)
	envCmd.AddCommand(envListCmd)
	envCmd.AddCommand(envPromoteCmd)
	envCmd.AddCommand(envDeleteCmd)
	rootCmd.AddCommand(envCmd)
}

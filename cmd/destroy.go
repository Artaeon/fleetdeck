package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/fleetdeck/fleetdeck/internal/project"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

var destroyCmd = &cobra.Command{
	Use:   "destroy <name>",
	Short: "Destroy a project (removes user, containers, optionally data)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		keepData, _ := cmd.Flags().GetBool("keep-data")
		keepRepo, _ := cmd.Flags().GetBool("keep-repo")
		force, _ := cmd.Flags().GetBool("force")

		d := openDB()
		p, err := d.GetProject(name)
		if err != nil {
			return err
		}

		if !force {
			fmt.Printf("Are you sure you want to destroy project %s? [y/N] ", ui.Bold(name))
			reader := bufio.NewReader(os.Stdin)
			answer, _ := reader.ReadString('\n')
			answer = strings.TrimSpace(strings.ToLower(answer))
			if answer != "y" && answer != "yes" {
				ui.Info("Aborted")
				return nil
			}
		}

		// Auto-snapshot before destruction
		autoSnapshot(name, "destroy")

		totalSteps := 4

		// Step 1: Stop containers
		ui.Step(1, totalSteps, "Stopping containers...")
		_ = project.ComposeDown(p.ProjectPath)
		ui.Success("Containers stopped")

		// Step 2: Delete GitHub repo if needed
		if p.GitHubRepo != "" && !keepRepo {
			ui.Step(2, totalSteps, "Deleting GitHub repository...")
			if err := project.DeleteGitHubRepo(p.GitHubRepo); err != nil {
				ui.Warn("Could not delete GitHub repo: %v", err)
			} else {
				ui.Success("GitHub repository deleted")
			}
		} else {
			ui.Step(2, totalSteps, "Keeping GitHub repository")
		}

		// Step 3: Remove data/user
		if !keepData {
			ui.Step(3, totalSteps, "Removing project data and user...")
			if err := os.RemoveAll(p.ProjectPath); err != nil {
				ui.Warn("Could not remove project directory: %v", err)
			}
			if err := project.DeleteLinuxUser(name); err != nil {
				ui.Warn("Could not delete Linux user: %v", err)
			}
			ui.Success("Project data and user removed")
		} else {
			ui.Step(3, totalSteps, "Keeping project data")
		}

		// Step 4: Remove from database
		ui.Step(4, totalSteps, "Removing from database...")
		if err := d.DeleteProject(name); err != nil {
			ui.Warn("Could not remove from database: %v", err)
		}
		ui.Success("Removed from database")

		fmt.Println()
		ui.Success("Project %s destroyed", ui.Bold(name))
		return nil
	},
}

func init() {
	destroyCmd.Flags().Bool("keep-data", false, "Keep project data on disk")
	destroyCmd.Flags().Bool("keep-repo", false, "Keep GitHub repository")
	destroyCmd.Flags().BoolP("force", "f", false, "Skip confirmation prompt")

	rootCmd.AddCommand(destroyCmd)
}

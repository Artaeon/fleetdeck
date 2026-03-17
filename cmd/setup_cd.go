package cmd

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/fleetdeck/fleetdeck/internal/audit"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

var setupCDCmd = &cobra.Command{
	Use:   "setup-cd <project-name>",
	Short: "Configure CI/CD auto-deploy for a project",
	Long: `Registers a GitHub repository for automatic deployments via webhooks.

Optionally configure branch-to-environment mappings so that pushes to
different branches deploy to different environments.

Examples:
  fleetdeck setup-cd myapp --repo myorg/myapp
  fleetdeck setup-cd myapp --repo myorg/myapp --branch-map main:production,develop:staging
  fleetdeck setup-cd myapp --repo myorg/myapp --webhook-url https://fleet.example.com/api/webhook/github`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectName := args[0]
		repo, _ := cmd.Flags().GetString("repo")
		branchMap, _ := cmd.Flags().GetString("branch-map")
		webhookURL, _ := cmd.Flags().GetString("webhook-url")
		createHook, _ := cmd.Flags().GetBool("create-webhook")

		if repo == "" {
			return fmt.Errorf("--repo is required (e.g. myorg/myapp)")
		}

		d := openDB()
		p, err := d.GetProject(projectName)
		if err != nil {
			return fmt.Errorf("project %q not found: %w", projectName, err)
		}

		// Update GitHub repo
		p.GitHubRepo = repo

		// Parse and store branch mappings
		if branchMap != "" {
			mappings := parseBranchMap(branchMap)
			data, err := json.Marshal(mappings)
			if err != nil {
				return fmt.Errorf("encoding branch mappings: %w", err)
			}
			p.BranchMappings = string(data)
			ui.Info("Branch mappings:")
			for branch, env := range mappings {
				ui.Info("  %s -> %s", branch, env)
			}
		}

		if err := d.UpdateProject(p); err != nil {
			return fmt.Errorf("updating project: %w", err)
		}
		ui.Success("Configured CI/CD for %s (repo: %s)", ui.Bold(projectName), repo)

		// Create GitHub webhook if requested
		if createHook && webhookURL != "" {
			ui.Info("Creating GitHub webhook...")
			if err := createGitHubWebhook(repo, webhookURL, cfg.Server.WebhookSecret); err != nil {
				ui.Warn("Could not create webhook: %v", err)
				ui.Info("Create it manually at https://github.com/%s/settings/hooks", repo)
			} else {
				ui.Success("GitHub webhook created")
			}
		} else if webhookURL != "" {
			ui.Info("Webhook URL: %s", webhookURL)
			ui.Info("Add this URL to https://github.com/%s/settings/hooks", repo)
			ui.Info("Set content type to application/json and configure the webhook secret.")
		}

		audit.Log("setup-cd", projectName, fmt.Sprintf("repo=%s", repo), true)
		return nil
	},
}

func parseBranchMap(s string) map[string]string {
	result := make(map[string]string)
	for _, pair := range strings.Split(s, ",") {
		parts := strings.SplitN(strings.TrimSpace(pair), ":", 2)
		if len(parts) == 2 {
			result[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return result
}

func createGitHubWebhook(repo, webhookURL, secret string) error {
	config := map[string]interface{}{
		"url":          webhookURL,
		"content_type": "json",
		"secret":       secret,
	}
	payload := map[string]interface{}{
		"name":   "web",
		"active": true,
		"events": []string{"push"},
		"config": config,
	}
	data, _ := json.Marshal(payload)

	cmd := exec.Command("gh", "api", fmt.Sprintf("repos/%s/hooks", repo),
		"--method", "POST",
		"--input", "-")
	cmd.Stdin = strings.NewReader(string(data))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func init() {
	setupCDCmd.Flags().String("repo", "", "GitHub repository (owner/repo)")
	setupCDCmd.Flags().String("branch-map", "", "Branch-to-environment mappings (e.g. main:production,develop:staging)")
	setupCDCmd.Flags().String("webhook-url", "", "Webhook URL for GitHub to call")
	setupCDCmd.Flags().Bool("create-webhook", false, "Create the GitHub webhook via gh CLI")

	rootCmd.AddCommand(setupCDCmd)
}

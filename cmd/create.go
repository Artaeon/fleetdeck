package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/fleetdeck/fleetdeck/internal/db"
	"github.com/fleetdeck/fleetdeck/internal/project"
	"github.com/fleetdeck/fleetdeck/internal/templates"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

var createCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new project",
	Long: `Creates a new project with:
- Linux user with minimal rights
- SSH keypair for CI/CD
- GitHub repository with deploy secrets
- Docker Compose configuration with Traefik labels
- Generated .env file with secrets`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		domain, _ := cmd.Flags().GetString("domain")
		githubOrg, _ := cmd.Flags().GetString("github-org")
		templateName, _ := cmd.Flags().GetString("template")
		skipGithub, _ := cmd.Flags().GetBool("skip-github")

		if domain == "" {
			return fmt.Errorf("--domain is required")
		}

		if githubOrg == "" && !skipGithub {
			githubOrg = cfg.GitHub.DefaultOrg
		}

		tmpl, err := templates.Get(templateName)
		if err != nil {
			return err
		}

		data := templates.TemplateData{
			Name:            name,
			Domain:          domain,
			PostgresVersion: cfg.Defaults.PostgresVersion,
		}

		projectPath := cfg.ProjectPath(name)
		linuxUser := project.LinuxUserName(name)
		totalSteps := 8
		if skipGithub {
			totalSteps = 5
		}

		// Step 1: Create Linux user
		ui.Step(1, totalSteps, "Creating Linux user %s...", linuxUser)
		if err := project.CreateLinuxUser(name, projectPath); err != nil {
			return fmt.Errorf("creating Linux user: %w", err)
		}
		ui.Success("User %s created", linuxUser)

		// Step 2: Create project directory structure
		ui.Step(2, totalSteps, "Setting up project at %s...", projectPath)
		if err := project.ScaffoldProject(projectPath, tmpl, data); err != nil {
			return fmt.Errorf("scaffolding project: %w", err)
		}
		ui.Success("Project files created")

		// Step 3: Generate .env
		ui.Step(3, totalSteps, "Generating environment file...")
		if err := project.GenerateEnvFile(projectPath, tmpl, data); err != nil {
			return fmt.Errorf("generating .env: %w", err)
		}
		ui.Success("Environment file generated")

		// Step 4: Generate SSH keypair
		ui.Step(4, totalSteps, "Generating SSH keypair...")
		privKeyPath, pubKey, err := project.GenerateSSHKeypair(projectPath)
		if err != nil {
			return fmt.Errorf("generating SSH keys: %w", err)
		}
		ui.Success("SSH keypair generated")

		// Step 5: Set up authorized_keys and fix ownership
		ui.Step(5, totalSteps, "Setting up SSH access...")
		if err := project.SetupAuthorizedKeys(projectPath, pubKey); err != nil {
			return fmt.Errorf("setting up authorized_keys: %w", err)
		}
		if err := project.ChownProjectDir(name, projectPath); err != nil {
			return fmt.Errorf("setting ownership: %w", err)
		}
		ui.Success("SSH access configured")

		repoFullName := ""
		if !skipGithub {
			// Step 6: Create GitHub repo
			ui.Step(6, totalSteps, "Creating GitHub repository...")
			repoURL, err := project.CreateGitHubRepo(githubOrg, name, true)
			if err != nil {
				return fmt.Errorf("creating GitHub repo: %w", err)
			}
			if githubOrg != "" {
				repoFullName = githubOrg + "/" + name
			} else {
				repoFullName = name
			}
			ui.Success("Repository created: %s", repoURL)

			// Step 7: Set GitHub secrets
			ui.Step(7, totalSteps, "Setting GitHub secrets...")
			serverIP, err := project.GetServerIP()
			if err != nil {
				return fmt.Errorf("getting server IP: %w", err)
			}
			privKeyData, err := os.ReadFile(privKeyPath)
			if err != nil {
				return fmt.Errorf("reading private key: %w", err)
			}

			secrets := map[string]string{
				"DEPLOY_HOST":    serverIP,
				"DEPLOY_USER":    linuxUser,
				"SSH_PRIVATE_KEY": string(privKeyData),
			}
			for key, value := range secrets {
				if err := project.SetGitHubSecret(repoFullName, key, value); err != nil {
					return fmt.Errorf("setting secret %s: %w", key, err)
				}
			}
			ui.Success("GitHub secrets configured")

			// Step 8: Push initial code
			ui.Step(8, totalSteps, "Pushing initial code...")
			gitURL := fmt.Sprintf("git@github.com:%s.git", repoFullName)
			if err := project.InitAndPushRepo(projectPath, gitURL); err != nil {
				ui.Warn("Could not push initial code: %v", err)
				ui.Info("You can push manually later")
			} else {
				ui.Success("Initial code pushed")
			}
		}

		// Store in database
		d := openDB()
		proj := &db.Project{
			Name:        name,
			Domain:      domain,
			GitHubRepo:  repoFullName,
			LinuxUser:   linuxUser,
			ProjectPath: projectPath,
			Template:    templateName,
			Status:      "created",
		}
		if err := d.CreateProject(proj); err != nil {
			ui.Warn("Could not save to database: %v", err)
		}

		fmt.Println()
		ui.Success("Project %s created!", ui.Bold(name))
		fmt.Println()
		ui.Info("DNS Setup:")
		fmt.Printf("  Add an A record for %s pointing to your server IP\n", domain)
		if !strings.Contains(domain, "*") {
			fmt.Printf("  %s → <your-server-ip>\n", domain)
		}
		fmt.Println()
		ui.Info("Project path: %s", projectPath)
		ui.Info("Linux user: %s", linuxUser)
		if repoFullName != "" {
			ui.Info("GitHub repo: %s", repoFullName)
		}
		fmt.Println()
		ui.Info("To start: fleetdeck start %s", name)
		ui.Info("To view logs: fleetdeck logs %s", name)

		return nil
	},
}

func init() {
	createCmd.Flags().String("domain", "", "Domain for the project (required)")
	createCmd.Flags().String("github-org", "", "GitHub organization")
	createCmd.Flags().String("template", "node", "Project template (node, python, go, static, nextjs, nestjs, custom)")
	createCmd.Flags().Bool("skip-github", false, "Skip GitHub repo creation")

	_ = createCmd.MarkFlagRequired("domain")

	rootCmd.AddCommand(createCmd)
}


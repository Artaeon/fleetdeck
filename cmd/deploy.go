package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/audit"
	"github.com/fleetdeck/fleetdeck/internal/deploy"
	"github.com/fleetdeck/fleetdeck/internal/detect"
	"github.com/fleetdeck/fleetdeck/internal/profiles"
	"github.com/fleetdeck/fleetdeck/internal/project"
	"github.com/fleetdeck/fleetdeck/internal/remote"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

var deployCmd = &cobra.Command{
	Use:   "deploy [directory]",
	Short: "Deploy an application (local or remote)",
	Long: `One-command deployment that handles everything:

  1. Detects your app type (or uses --profile)
  2. Connects to server via SSH (or deploys locally)
  3. Creates project structure, user, SSH keys
  4. Generates optimized Docker Compose from profile
  5. Builds and deploys with chosen strategy
  6. Sets up GitHub Actions CI/CD

Examples:
  fleetdeck deploy ./my-app --domain app.example.com
  fleetdeck deploy ./my-app --server root@1.2.3.4 --domain app.example.com --profile saas
  fleetdeck deploy . --strategy bluegreen --domain app.example.com`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := "."
		if len(args) > 0 {
			dir = args[0]
		}

		absDir, err := filepath.Abs(dir)
		if err != nil {
			return err
		}

		info, err := os.Stat(absDir)
		if err != nil || !info.IsDir() {
			return fmt.Errorf("%s is not a valid directory", dir)
		}

		domain, _ := cmd.Flags().GetString("domain")
		server, _ := cmd.Flags().GetString("server")
		profileName, _ := cmd.Flags().GetString("profile")
		strategyName, _ := cmd.Flags().GetString("strategy")
		name, _ := cmd.Flags().GetString("name")
		timeout, _ := cmd.Flags().GetDuration("timeout")

		if domain == "" {
			return fmt.Errorf("--domain is required")
		}

		// Ensure strategy defaults to "basic" even if explicitly set to empty.
		if strategyName == "" {
			strategyName = "basic"
		}

		// Step 1: Detect app type
		ui.Step(1, 5, "Analyzing application...")
		result, err := detect.Detect(absDir)
		if err != nil {
			return fmt.Errorf("detection failed: %w", err)
		}
		ui.Success("Detected: %s %s", result.Language, result.Framework)
		if result.Port > 0 {
			ui.Info("Detected application port: %d", result.Port)
		}

		// Use detected profile if none specified
		if profileName == "" {
			profileName = result.Profile
			ui.Info("Auto-selected profile: %s", profileName)
		}

		prof, err := profiles.Get(profileName)
		if err != nil {
			return err
		}

		// Derive project name from directory if not specified
		if name == "" {
			name = filepath.Base(absDir)
		}

		// Validate project name to prevent shell injection and invalid paths
		if err := project.ValidateName(name); err != nil {
			return fmt.Errorf("invalid project name %q: %w", name, err)
		}

		// Step 2: Remote or local?
		if server != "" {
			return deployRemote(cmd, absDir, name, domain, server, prof, strategyName, timeout)
		}

		return deployLocal(cmd, absDir, name, domain, prof, strategyName, timeout)
	},
}

func deployLocal(cmd *cobra.Command, dir, name, domain string, prof *profiles.Profile, strategyName string, timeout time.Duration) error {
	projectPath := cfg.ProjectPath(name)

	// Acquire per-project lock to prevent concurrent deployments.
	lock, err := deploy.AcquireLock(projectPath)
	if err != nil {
		return fmt.Errorf("acquiring deploy lock: %w", err)
	}
	defer lock.Release()

	// Step 3: Deploy
	ui.Step(3, 5, "Deploying with %s strategy...", strategyName)

	strategy, err := deploy.GetStrategy(strategyName)
	if err != nil {
		return err
	}

	healthURL := fmt.Sprintf("https://%s", domain)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	opts := deploy.DeployOptions{
		ProjectPath:    projectPath,
		ProjectName:    name,
		ComposeFile:    filepath.Join(projectPath, "docker-compose.yml"),
		HealthCheckURL: healthURL,
		Timeout:        timeout,
	}

	result, err := strategy.Deploy(ctx, opts)
	if err != nil {
		ui.Error("Deployment failed: %v", err)
		audit.Log("deploy", name, fmt.Sprintf("strategy=%s failed: %v", strategyName, err), false)
		return err
	}

	// Step 4: Verify
	ui.Step(4, 5, "Verifying deployment...")
	if result.Success {
		ui.Success("Deployment successful (took %s)", result.Duration.Round(time.Second))
	} else {
		ui.Error("Deployment verification failed")
	}

	// Step 5: Summary
	ui.Step(5, 5, "Deployment complete")
	fmt.Println()
	ui.Success("Application deployed at https://%s", domain)
	ui.Info("Profile: %s", prof.Name)
	ui.Info("Strategy: %s", strategyName)
	fmt.Println()

	audit.Log("deploy", name, fmt.Sprintf("strategy=%s profile=%s domain=%s", strategyName, prof.Name, domain), true)
	return nil
}

func deployRemote(cmd *cobra.Command, dir, name, domain, server string, prof *profiles.Profile, strategyName string, timeout time.Duration) error {
	// Acquire per-project lock to prevent concurrent deployments.
	lock, err := deploy.AcquireLock(dir)
	if err != nil {
		return fmt.Errorf("acquiring deploy lock: %w", err)
	}
	defer lock.Release()

	host, user := parseTarget(server)
	port, _ := cmd.Flags().GetString("port")
	keyFile, _ := cmd.Flags().GetString("key")

	// Read SSH key
	var keyData []byte
	if keyFile != "" {
		var err error
		keyData, err = os.ReadFile(keyFile)
		if err != nil {
			return fmt.Errorf("reading SSH key: %w", err)
		}
	} else {
		for _, path := range []string{
			os.ExpandEnv("$HOME/.ssh/id_ed25519"),
			os.ExpandEnv("$HOME/.ssh/id_rsa"),
		} {
			data, err := os.ReadFile(path)
			if err == nil {
				keyData = data
				break
			}
		}
		if keyData == nil {
			return fmt.Errorf("no SSH key found; use --key to specify one")
		}
	}

	// Step 2: Connect
	insecure, _ := cmd.Flags().GetBool("insecure")
	ui.Step(2, 5, "Connecting to %s@%s...", user, host)
	var (
		client   *remote.Client
		connErr  error
	)
	if insecure {
		ui.Warn("Using Trust On First Use SSH host key verification (--insecure)")
		client, connErr = remote.NewClientTOFU(host, port, user, keyData)
	} else {
		client, connErr = remote.NewClient(host, port, user, keyData)
	}
	if connErr != nil {
		return fmt.Errorf("SSH connection failed: %w", connErr)
	}
	defer client.Close()
	ui.Success("Connected to %s", host)

	// Step 3: Upload project
	ui.Step(3, 5, "Uploading project to server...")
	remotePath := "/opt/fleetdeck/" + name
	quotedPath := "'/opt/fleetdeck/" + name + "'"
	if _, err := client.Run("mkdir -p " + quotedPath); err != nil {
		return fmt.Errorf("creating remote directory: %w", err)
	}

	if err := client.UploadDir(dir, remotePath); err != nil {
		return fmt.Errorf("uploading project: %w", err)
	}
	ui.Success("Project uploaded to %s", remotePath)

	// Step 4: Build and deploy
	ui.Step(4, 5, "Building and deploying on server...")
	deployCmd := "cd " + quotedPath + " && docker compose build && docker compose up -d"
	output, err := client.Run(deployCmd)
	if err != nil {
		ui.Error("Remote deployment failed: %s", output)
		return fmt.Errorf("remote deployment: %w", err)
	}
	ui.Success("Application deployed on server")

	// Step 5: Summary
	ui.Step(5, 5, "Deployment complete")
	fmt.Println()
	ui.Success("Application deployed at https://%s", domain)
	ui.Info("Server: %s@%s", user, host)
	ui.Info("Path: %s", remotePath)
	ui.Info("Profile: %s", prof.Name)
	fmt.Println()

	audit.Log("deploy.remote", name, fmt.Sprintf("server=%s profile=%s domain=%s", server, prof.Name, domain), true)
	return nil
}

func init() {
	deployCmd.Flags().String("domain", "", "Domain for the application (required)")
	deployCmd.Flags().String("server", "", "Remote server (user@host) for remote deployment")
	deployCmd.Flags().String("port", "22", "SSH port for remote deployment")
	deployCmd.Flags().String("key", "", "Path to SSH private key for remote deployment")
	deployCmd.Flags().String("profile", "", "Deployment profile (auto-detected if not set)")
	deployCmd.Flags().String("strategy", "basic", "Deployment strategy (basic, bluegreen, rolling)")
	deployCmd.Flags().String("name", "", "Project name (defaults to directory name)")
	deployCmd.Flags().Duration("timeout", 5*time.Minute, "Deployment timeout")
	deployCmd.Flags().Bool("insecure", false, "Skip SSH host key verification for remote deploys")

	rootCmd.AddCommand(deployCmd)
}

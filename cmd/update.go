package cmd

import (
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fleetdeck/fleetdeck/internal/audit"
	"github.com/fleetdeck/fleetdeck/internal/db"
	"github.com/fleetdeck/fleetdeck/internal/project"
	"github.com/fleetdeck/fleetdeck/internal/remote"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update <project>",
	Short: "Update a deployed application on the server",
	Long: `Lightweight update for already-deployed applications. Unlike deploy,
this command skips detection, profile resolution, and file generation.
It syncs changed files, rebuilds only if needed, and restarts services.

If the Dockerfile changed, containers are automatically rebuilt.
Use --restart-only to skip file sync entirely.

Examples:
  fleetdeck update myapp --server root@1.2.3.4
  fleetdeck update myapp --server prod
  fleetdeck update myapp --server prod --rebuild
  fleetdeck update myapp --server prod --restart-only
  fleetdeck update myapp --server prod --service app
  fleetdeck update myapp --server prod --pull`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectName := args[0]
		if err := project.ValidateName(projectName); err != nil {
			return fmt.Errorf("invalid project name %q: %w", projectName, err)
		}

		dir, _ := cmd.Flags().GetString("dir")
		server, _ := cmd.Flags().GetString("server")
		rebuild, _ := cmd.Flags().GetBool("rebuild")
		pull, _ := cmd.Flags().GetBool("pull")
		restartOnly, _ := cmd.Flags().GetBool("restart-only")
		service, _ := cmd.Flags().GetString("service")
		noCache, _ := cmd.Flags().GetBool("no-cache")
		preDeployHook, _ := cmd.Flags().GetString("pre-deploy")
		postDeployHook, _ := cmd.Flags().GetString("post-deploy")

		// Only open DB when we actually need it (dir or server not provided)
		needsDB := dir == "" || server == ""
		var d *db.DB
		if needsDB {
			d = openDB()
		}

		// Resolve local source directory
		if dir == "" && d != nil {
			if proj, err := d.GetProject(projectName); err == nil && proj.ProjectPath != "" {
				dir = proj.ProjectPath
			}
		}
		if dir == "" && !restartOnly {
			return fmt.Errorf("could not find local directory for %q; use --dir or --restart-only", projectName)
		}
		if dir != "" {
			absDir, err := filepath.Abs(dir)
			if err != nil {
				return err
			}
			info, err := os.Stat(absDir)
			if err != nil || !info.IsDir() {
				return fmt.Errorf("%s is not a valid directory", dir)
			}
			dir = absDir
		}

		// Resolve server from DB if not specified
		if server == "" && d != nil {
			if proj, err := d.GetProject(projectName); err == nil && proj.ServerID != "" {
				if s, err := d.GetServerByID(proj.ServerID); err == nil {
					server = s.Name
				}
			}
		}
		if server == "" {
			return fmt.Errorf("--server is required (or register the project with a server)")
		}

		port, _ := cmd.Flags().GetString("port")
		keyFile, _ := cmd.Flags().GetString("key")
		passphrase, _ := cmd.Flags().GetString("passphrase")
		if envPass := os.Getenv("FLEETDECK_SSH_PASSPHRASE"); envPass != "" {
			passphrase = envPass
		}

		// Resolve server: registered name or user@host
		var host, user string
		if !strings.Contains(server, "@") && !strings.Contains(server, ".") && !strings.Contains(server, ":") {
			if d == nil {
				d = openDB()
			}
			s, err := d.GetServer(server)
			if err != nil {
				return fmt.Errorf("server %q not found; use 'fleetdeck server add' to register it or specify user@host", server)
			}
			host = s.Host
			user = s.User
			port = s.Port
			keyFile = s.KeyPath
			ui.Info("Using registered server %s (%s@%s)", s.Name, user, host)
		} else {
			host, user = parseTarget(server)
		}

		// Read SSH key
		var keyData []byte
		if keyFile != "" {
			var err error
			keyData, err = os.ReadFile(keyFile)
			if err != nil {
				return fmt.Errorf("reading SSH key: %w", err)
			}
		} else {
			keyData = findSSHKey(host)
			if keyData == nil {
				return fmt.Errorf("no SSH key found; use --key to specify one")
			}
		}

		// Connect
		insecure, _ := cmd.Flags().GetBool("insecure")
		step := 1
		// totalSteps: connect + restart = 2, +1 for sync, +1 for rebuild
		totalSteps := 2
		if !restartOnly {
			totalSteps++ // sync step
		}
		if rebuild {
			totalSteps++ // rebuild step (known upfront)
		}

		ui.Step(step, totalSteps, "Connecting to %s@%s...", user, host)
		var passphraseBytes []byte
		if passphrase != "" {
			passphraseBytes = []byte(passphrase)
		}
		var client *remote.Client
		var connErr error
		if insecure {
			client, connErr = remote.NewClientTOFU(host, port, user, keyData, passphraseBytes)
		} else {
			client, connErr = remote.NewClient(host, port, user, keyData, passphraseBytes)
		}
		if connErr != nil {
			return fmt.Errorf("SSH connection failed: %w", connErr)
		}
		defer client.Close()
		ui.Success("Connected to %s", host)
		step++

		remotePath := "/opt/fleetdeck/" + projectName
		quotedPath := "'/opt/fleetdeck/" + projectName + "'"

		// Verify project exists on server
		if _, err := client.Run("test -d " + quotedPath); err != nil {
			return fmt.Errorf("project %q not found on server at %s; run 'fleetdeck deploy' first", projectName, remotePath)
		}

		needsRebuild := rebuild
		action := "restart"

		// Sync files
		if !restartOnly {
			ui.Step(step, totalSteps, "Syncing files to server...")

			// Get remote Dockerfile hash before upload
			var remoteDockerfileHash string
			out, err := client.Run("md5sum " + quotedPath + "/Dockerfile 2>/dev/null | awk '{print $1}'")
			if err == nil {
				remoteDockerfileHash = strings.TrimSpace(out)
			}

			// Get remote docker-compose.yml hash before upload
			var remoteComposeHash string
			out, err = client.Run("md5sum " + quotedPath + "/docker-compose.yml 2>/dev/null | awk '{print $1}'")
			if err == nil {
				remoteComposeHash = strings.TrimSpace(out)
			}

			// Upload
			if err := client.UploadDir(dir, remotePath); err != nil {
				return fmt.Errorf("syncing files: %w", err)
			}
			ui.Success("Files synced")

			// Check if Dockerfile changed
			localDockerfileHash := localFileMD5(filepath.Join(dir, "Dockerfile"))
			if localDockerfileHash != "" && localDockerfileHash != remoteDockerfileHash {
				ui.Info("Dockerfile changed, rebuild required")
				needsRebuild = true
				totalSteps++ // add build step (detected at runtime)
			}

			// Check if docker-compose.yml changed
			localComposeHash := localFileMD5(filepath.Join(dir, "docker-compose.yml"))
			if localComposeHash != "" && localComposeHash != remoteComposeHash {
				ui.Info("docker-compose.yml changed")
				action = "recreate"
			}

			step++
		}

		// Build if needed
		if needsRebuild {
			ui.Step(step, totalSteps, "Rebuilding containers...")
			buildCmd := "cd " + quotedPath + " && docker compose build"
			if noCache {
				buildCmd += " --no-cache"
			}
			if pull {
				buildCmd += " --pull"
			}
			if service != "" {
				buildCmd += " " + shellQuote(service)
			}
			output, err := client.Run(buildCmd)
			if err != nil {
				ui.Error("Build failed: %s", output)
				return fmt.Errorf("build failed: %w", err)
			}
			ui.Success("Build complete")
			action = "rebuild"
			step++
		} else if pull {
			ui.Info("Pulling latest images...")
			pullCmd := "cd " + quotedPath + " && docker compose pull"
			if service != "" {
				pullCmd += " " + shellQuote(service)
			}
			client.Run(pullCmd)
		}

		// Pre-deploy hook
		if preDeployHook != "" {
			ui.Info("Running pre-deploy hook...")
			svc := service
			if svc == "" {
				svc = "app"
			}
			hookCmd := "cd " + quotedPath + " && docker compose exec -T " + shellQuote(svc) + " sh -c " + shellQuote(preDeployHook)
			output, err := client.Run(hookCmd)
			if err != nil {
				ui.Error("Pre-deploy hook failed: %s", output)
				return fmt.Errorf("pre-deploy hook failed: %w", err)
			}
			ui.Success("Pre-deploy hook completed")
		}

		// Restart
		ui.Step(step, totalSteps, "Restarting services...")
		var restartCmd string
		if needsRebuild || action == "recreate" {
			if service != "" {
				restartCmd = "cd " + quotedPath + " && docker compose up -d --no-deps " + shellQuote(service)
			} else {
				restartCmd = "cd " + quotedPath + " && docker compose up -d"
			}
		} else {
			if service != "" {
				restartCmd = "cd " + quotedPath + " && docker compose restart " + shellQuote(service)
			} else {
				restartCmd = "cd " + quotedPath + " && docker compose restart"
			}
		}

		output, err := client.Run(restartCmd)
		if err != nil {
			ui.Error("Restart failed: %s", output)
			return fmt.Errorf("restart failed: %w", err)
		}
		ui.Success("Services restarted")

		// Post-deploy hook
		if postDeployHook != "" {
			ui.Info("Running post-deploy hook...")
			svc := service
			if svc == "" {
				svc = "app"
			}
			hookCmd := "cd " + quotedPath + " && docker compose exec -T " + shellQuote(svc) + " sh -c " + shellQuote(postDeployHook)
			output, err := client.Run(hookCmd)
			if err != nil {
				ui.Error("Post-deploy hook failed: %s", output)
				return fmt.Errorf("post-deploy hook failed: %w", err)
			}
			ui.Success("Post-deploy hook completed")
		}

		// Summary
		fmt.Println()
		ui.Success("Update complete for %s", projectName)
		ui.Info("Server: %s@%s", user, host)
		ui.Info("Path: %s", remotePath)
		switch {
		case restartOnly:
			ui.Info("Action: restart only")
		case needsRebuild:
			ui.Info("Action: sync + rebuild + restart")
		default:
			ui.Info("Action: sync + restart")
		}
		if service != "" {
			ui.Info("Service: %s", service)
		}
		fmt.Println()

		audit.Log("update", projectName, fmt.Sprintf("server=%s rebuild=%v restart_only=%v service=%s", server, needsRebuild, restartOnly, service), true)
		return nil
	},
}

// localFileMD5 returns the hex MD5 of a file, or empty string if unreadable.
func localFileMD5(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%x", md5.Sum(data))
}

func init() {
	updateCmd.Flags().String("server", "", "Remote server (user@host or registered name)")
	updateCmd.Flags().String("dir", "", "Local source directory (overrides DB lookup)")
	updateCmd.Flags().String("port", "22", "SSH port")
	updateCmd.Flags().String("key", "", "Path to SSH private key")
	updateCmd.Flags().String("passphrase", "", "Passphrase for encrypted SSH private key")
	updateCmd.Flags().Bool("insecure", false, "Skip SSH host key verification")
	updateCmd.Flags().Bool("rebuild", false, "Force rebuild even if Dockerfile unchanged")
	updateCmd.Flags().Bool("pull", false, "Pull latest base images")
	updateCmd.Flags().Bool("restart-only", false, "Just restart containers, don't sync files")
	updateCmd.Flags().String("service", "", "Target a specific service (default: all)")
	updateCmd.Flags().Bool("no-cache", false, "Pass --no-cache to docker compose build")
	updateCmd.Flags().String("pre-deploy", "", "Command to run before restart")
	updateCmd.Flags().String("post-deploy", "", "Command to run after restart")

	rootCmd.AddCommand(updateCmd)
}

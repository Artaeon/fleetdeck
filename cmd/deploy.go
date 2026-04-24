package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/audit"
	"github.com/fleetdeck/fleetdeck/internal/backup"
	"github.com/fleetdeck/fleetdeck/internal/deploy"
	"github.com/fleetdeck/fleetdeck/internal/detect"
	"github.com/fleetdeck/fleetdeck/internal/generate"
	"github.com/fleetdeck/fleetdeck/internal/migrate"
	"github.com/fleetdeck/fleetdeck/internal/profiles"
	"github.com/fleetdeck/fleetdeck/internal/project"
	"github.com/fleetdeck/fleetdeck/internal/remote"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

// deployWatchOptions bundles the post-deploy watchdog flags so they can
// be shared between the local and remote deploy paths without a
// five-arg signature addition per path.
type deployWatchOptions struct {
	watch        time.Duration
	rollback     bool
	rollbackMode string // "full" | "files" — see runPostDeployWatchdog
	interval     time.Duration
	threshold    int
	expectedCode int
}

// readWatchOptions extracts the --watch family of flags from the deploy
// command. Zero values disable the post-deploy observation entirely so
// callers that don't ask for a watchdog see no behavior change.
func readWatchOptions(cmd *cobra.Command) deployWatchOptions {
	watch, _ := cmd.Flags().GetDuration("watch")
	rollback, _ := cmd.Flags().GetBool("watch-rollback")
	rollbackMode, _ := cmd.Flags().GetString("watch-rollback-mode")
	interval, _ := cmd.Flags().GetDuration("watch-interval")
	threshold, _ := cmd.Flags().GetInt("watch-threshold")
	expected, _ := cmd.Flags().GetInt("watch-status")
	return deployWatchOptions{
		watch:        watch,
		rollback:     rollback,
		rollbackMode: rollbackMode,
		interval:     interval,
		threshold:    threshold,
		expectedCode: expected,
	}
}

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
  fleetdeck deploy ./my-app --server prod --domain app.example.com  # use registered server name
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
		preDeployHook, _ := cmd.Flags().GetString("pre-deploy")
		postDeployHook, _ := cmd.Flags().GetString("post-deploy")
		noCache, _ := cmd.Flags().GetBool("no-cache")
		fresh, _ := cmd.Flags().GetBool("fresh")

		if domain == "" {
			return fmt.Errorf("--domain is required")
		}
		if err := validateDomain(domain); err != nil {
			return err
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

		// Show detection warnings
		if warnings, ok := getWarnings(result); ok {
			for _, w := range warnings {
				ui.Warn(w)
			}
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

		// Auto-generate missing deployment files
		generated := generateMissingFiles(absDir, name, domain, result)
		for _, g := range generated {
			ui.Success("Generated %s", g)
		}

		migrateCommand, _ := cmd.Flags().GetString("migrate")
		if migrateCommand != "" && server != "" {
			// Remote migration would need to tunnel docker compose exec
			// over the SSH client; the local migrate package is not yet
			// wired that way. Fail loudly rather than silently skipping.
			return fmt.Errorf("--migrate is not yet supported for remote deploys; SSH to the server and run 'fleetdeck migrate run %s --command %q' after deploy", name, migrateCommand)
		}

		// Step 2: Remote or local?
		if server != "" {
			if err := deployRemote(cmd, absDir, name, domain, server, prof, strategyName, timeout, preDeployHook, postDeployHook, noCache, fresh); err != nil {
				return err
			}
		} else {
			if err := deployLocal(cmd, absDir, name, domain, prof, strategyName, timeout, preDeployHook, postDeployHook, noCache); err != nil {
				return err
			}
			// Local --migrate runs AFTER the new containers are up but
			// BEFORE we declare the deploy done. A failing migration
			// aborts the RunE, which means the watchdog below is not
			// reached — operator sees the migration error and the
			// pre-migration snapshot is available for immediate rollback.
			if migrateCommand != "" {
				if err := runDeployMigration(cmd, name, migrateCommand); err != nil {
					return err
				}
			}
		}

		// Post-deploy watchdog. Runs only when --watch > 0. If the watchdog
		// declares the deploy unhealthy and --watch-rollback was set, the
		// pre-deploy snapshot is restored here before this RunE returns.
		return runPostDeployWatchdog(cmd.Context(), name, domain, readWatchOptions(cmd))
	},
}

// runDeployMigration executes the operator's migration command as part
// of the deploy, reusing the full internal/migrate pipeline (snapshot +
// tracked run). Extracted so local deploys can opt in without scattering
// migrate-related flag plumbing through the existing deployLocal body.
func runDeployMigration(cmd *cobra.Command, projectName, command string) error {
	d := openDB()
	p, err := d.GetProject(projectName)
	if err != nil {
		return fmt.Errorf("loading project for --migrate: %w", err)
	}
	ui.Info("Running deploy migration: %s", command)
	runner := migrate.New(cfg, d)
	res, err := runner.Run(cmd.Context(), p, migrate.Options{
		Command: command,
		Timeout: 10 * time.Minute,
	})
	if err != nil {
		if res != nil && res.Output != "" {
			fmt.Println(res.Output)
		}
		audit.Log("deploy.migrate", projectName, err.Error(), false)
		if res != nil && res.SnapshotID != "" {
			ui.Warn("Pre-migration snapshot: %s — restore with 'fleetdeck migrate rollback %s'",
				shortID(res.SnapshotID), projectName)
		}
		return err
	}
	audit.Log("deploy.migrate", projectName,
		fmt.Sprintf("id=%s snapshot=%s duration=%s",
			shortID(res.MigrationID), shortID(res.SnapshotID), res.Duration.Round(time.Millisecond)),
		true)
	ui.Success("Migration succeeded in %s", res.Duration.Round(time.Millisecond))
	return nil
}

func deployLocal(cmd *cobra.Command, dir, name, domain string, prof *profiles.Profile, strategyName string, timeout time.Duration, preDeployHook, postDeployHook string, noCache bool) error {
	projectPath := cfg.ProjectPath(name)

	// Acquire per-project lock to prevent concurrent deployments.
	lock, err := deploy.AcquireLock(projectPath)
	if err != nil {
		return fmt.Errorf("acquiring deploy lock: %w", err)
	}
	defer lock.Release()

	// Pre-deploy snapshot to protect against failed deployments
	autoSnapshot(name, "deploy")

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
		PreDeployHook:  preDeployHook,
		PostDeployHook: postDeployHook,
		NoCache:        noCache,
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

func deployRemote(cmd *cobra.Command, dir, name, domain, server string, prof *profiles.Profile, strategyName string, timeout time.Duration, preDeployHook, postDeployHook string, noCache, fresh bool) error {
	// Acquire per-project lock using a canonical path based on project name,
	// so concurrent deploys from different directories are serialized on this machine.
	lockDir := filepath.Join(os.TempDir(), "fleetdeck-locks", name)
	if err := os.MkdirAll(lockDir, 0755); err != nil {
		return fmt.Errorf("creating lock directory: %w", err)
	}
	lock, err := deploy.AcquireLock(lockDir)
	if err != nil {
		return fmt.Errorf("acquiring deploy lock: %w (another deploy for %q may be running)", err, name)
	}
	defer lock.Release()

	// Pre-deploy snapshot to protect against failed deployments
	autoSnapshot(name, "deploy")

	port, _ := cmd.Flags().GetString("port")
	keyFile, _ := cmd.Flags().GetString("key")
	passphrase, _ := cmd.Flags().GetString("passphrase")
	if envPass := os.Getenv("FLEETDECK_SSH_PASSPHRASE"); envPass != "" {
		passphrase = envPass
	}

	// Resolve server: either a registered name or user@host
	var host, user string
	if !strings.Contains(server, "@") && !strings.Contains(server, ".") && !strings.Contains(server, ":") {
		// No @ or dots — must be a registered server name
		d := openDB()
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

	// Step 2: Connect
	insecure, _ := cmd.Flags().GetBool("insecure")
	ui.Step(2, 5, "Connecting to %s@%s...", user, host)
	var (
		client   *remote.Client
		connErr  error
	)
	var passphraseBytes []byte
	if passphrase != "" {
		passphraseBytes = []byte(passphrase)
	}
	if insecure {
		ui.Warn("Using Trust On First Use SSH host key verification (--insecure)")
		client, connErr = remote.NewClientTOFU(host, port, user, keyData, passphraseBytes)
	} else {
		client, connErr = remote.NewClient(host, port, user, keyData, passphraseBytes)
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

	// Step 3b: Auto-generate missing env files
	envReqs, _ := detect.DetectEnvFiles(dir)
	for _, req := range envReqs {
		if req.Exists {
			continue
		}
		remoteEnvPath := remotePath + "/" + req.Required
		if req.Example != "" {
			ui.Info("Generating %s from %s...", req.Required, filepath.Base(req.Example))
			tmpFile := filepath.Join(os.TempDir(), "fleetdeck-env-"+filepath.Base(req.Required))
			if err := detect.GenerateEnvFromExample(req.Example, tmpFile); err != nil {
				ui.Warn("Could not generate %s: %v", req.Required, err)
				continue
			}
			defer os.Remove(tmpFile)
			if err := client.Upload(tmpFile, remoteEnvPath); err != nil {
				ui.Warn("Could not upload %s: %v", req.Required, err)
			} else {
				ui.Success("Generated %s with secrets", req.Required)
			}
		} else {
			ui.Warn("Missing %s (no example file found to generate from)", req.Required)
			// Create an empty file so docker compose doesn't fail
			if err := client.UploadBytes([]byte("# Auto-generated by FleetDeck\n"), remoteEnvPath, 0600); err != nil {
				ui.Warn("Could not create empty %s: %v", req.Required, err)
			}
		}
	}

	// Step 4: Build and deploy
	ui.Step(4, 5, "Building and deploying on server (%s strategy)...", strategyName)

	// Fresh deploy: tear down existing containers and volumes
	if fresh {
		ui.Info("Fresh deploy: removing existing containers and volumes...")
		freshCmd := "cd " + quotedPath + " && docker compose down -v"
		if output, err := client.Run(freshCmd); err != nil {
			ui.Warn("Fresh teardown returned an error (may be first deploy): %s", output)
		}
	}

	// Build first (common to all strategies)
	buildCmd := "cd " + quotedPath + " && docker compose build"
	if noCache {
		buildCmd += " --no-cache"
	}
	output, err := client.Run(buildCmd)
	if err != nil {
		ui.Error("Remote build failed: %s", output)
		return fmt.Errorf("remote build: %w", err)
	}

	// Execute strategy-specific deploy
	var remoteDeployCmd string
	switch strategyName {
	case "bluegreen":
		// Bring up the new set under a temporary compose project name,
		// then poll its 'app' service for readiness before tearing down
		// the old set. If readiness never converges, abort the swap and
		// leave the old containers serving traffic.
		//
		// Previously this path was `up -d && sleep 5 && swap`, which
		// (a) treated a fixed sleep as a health check, and (b) tore down
		// the old project before verifying anything — leaving a window
		// with no containers at all if the new image booted slowly.
		//
		// The readiness check runs `docker compose -p <new> ps` and looks
		// for a container whose health is "healthy" OR whose state is
		// "running" without a healthcheck defined. 60 attempts * 2 s =
		// 120 s ceiling.
		newProject := name + "-new"
		remoteDeployCmd = fmt.Sprintf(
			"set -e\n"+
				"cd %s\n"+
				"docker compose -p %s up -d --pull always\n"+
				"echo 'Waiting for new containers to become healthy...'\n"+
				"ready=0\n"+
				"for i in $(seq 1 60); do\n"+
				"  status=$(docker compose -p %s ps --format '{{.Health}}|{{.State}}' 2>/dev/null || true)\n"+
				"  if [ -z \"$status\" ]; then\n"+
				"    sleep 2; continue\n"+
				"  fi\n"+
				"  if echo \"$status\" | grep -q 'unhealthy'; then\n"+
				"    echo 'A new container is unhealthy — aborting swap.'\n"+
				"    docker compose -p %s logs --tail=50 || true\n"+
				"    docker compose -p %s down --remove-orphans || true\n"+
				"    exit 11\n"+
				"  fi\n"+
				"  bad=$(echo \"$status\" | awk -F'|' '{print $2}' | grep -v -E '^(running|healthy)$' || true)\n"+
				"  healthy=$(echo \"$status\" | awk -F'|' '$1==\"healthy\" || ($1==\"\" && $2==\"running\") {print}')\n"+
				"  total=$(echo \"$status\" | wc -l)\n"+
				"  ok=$(echo \"$healthy\" | wc -l)\n"+
				"  if [ -z \"$bad\" ] && [ \"$total\" = \"$ok\" ]; then\n"+
				"    ready=1; break\n"+
				"  fi\n"+
				"  sleep 2\n"+
				"done\n"+
				"if [ \"$ready\" != 1 ]; then\n"+
				"  echo 'New containers did not become healthy within 120s — aborting swap.'\n"+
				"  docker compose -p %s logs --tail=50 || true\n"+
				"  docker compose -p %s down --remove-orphans || true\n"+
				"  exit 12\n"+
				"fi\n"+
				"# Cutover: stop old, promote new to primary project name.\n"+
				"docker compose down\n"+
				"docker compose -p %s down --remove-orphans\n"+
				"docker compose up -d --pull always\n",
			quotedPath, newProject, newProject, newProject, newProject, newProject, newProject, newProject,
		)
	case "rolling":
		// Pull first, then restart each service one at a time
		remoteDeployCmd = fmt.Sprintf(
			"cd %s && docker compose pull && "+
				"for svc in $(docker compose config --services); do "+
				"docker compose up -d --no-deps $svc; "+
				"sleep 2; "+
				"done",
			quotedPath,
		)
	default: // "basic"
		remoteDeployCmd = "cd " + quotedPath + " && docker compose up -d --pull always"
	}

	// Run pre-deploy hook (after build, before deploy — containers must exist)
	if preDeployHook != "" {
		ui.Info("Running pre-deploy hook...")
		hookCmd := "cd " + quotedPath + " && docker compose exec -T app sh -c " + shellQuote(preDeployHook)
		output, err := client.Run(hookCmd)
		if err != nil {
			ui.Error("Pre-deploy hook failed: %s", output)
			return fmt.Errorf("pre-deploy hook failed: %w", err)
		}
		ui.Success("Pre-deploy hook completed")
	}

	output, err = client.Run(remoteDeployCmd)
	if err != nil {
		ui.Error("Remote deployment failed: %s", output)
		return fmt.Errorf("remote deployment: %w", err)
	}

	// Run post-deploy hook
	if postDeployHook != "" {
		ui.Info("Running post-deploy hook...")
		hookCmd := "cd " + quotedPath + " && docker compose exec -T app sh -c " + shellQuote(postDeployHook)
		output, err := client.Run(hookCmd)
		if err != nil {
			ui.Error("Post-deploy hook failed: %s", output)
			return fmt.Errorf("post-deploy hook failed: %w", err)
		}
		ui.Success("Post-deploy hook completed")
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

// generateMissingFiles creates Dockerfile, docker-compose.yml, and .dockerignore
// in the project directory if they don't already exist. Returns a list of generated filenames.
func generateMissingFiles(dir, name, domain string, result *detect.Result) []string {
	var generated []string

	// Generate Dockerfile if missing
	if !fileExistsInDir(dir, "Dockerfile") {
		content := generate.Dockerfile(result)
		if content != "" {
			if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(content), 0644); err == nil {
				generated = append(generated, "Dockerfile")
			}
		}
	}

	// Generate docker-compose.yml if missing
	if !fileExistsInDir(dir, "docker-compose.yml") {
		port := result.Port
		if port == 0 {
			port = 3000
		}
		content := generate.Compose(generate.ComposeOptions{
			ProjectName: name,
			Domain:      domain,
			Port:        port,
			HasDB:       result.HasDB,
			AppType:     result.AppType,
		})
		if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(content), 0644); err == nil {
			generated = append(generated, "docker-compose.yml")
		}
	}

	// Generate .dockerignore if missing
	if !fileExistsInDir(dir, ".dockerignore") {
		content := generate.Dockerignore(result.AppType)
		if content != "" {
			if err := os.WriteFile(filepath.Join(dir, ".dockerignore"), []byte(content), 0644); err == nil {
				generated = append(generated, ".dockerignore")
			}
		}
	}

	return generated
}

// getWarnings extracts warnings from a detect result.
func getWarnings(result *detect.Result) ([]string, bool) {
	type warnable interface {
		GetWarnings() []string
	}
	// Use type assertion if Warnings field exists on result
	// For now, we check via the field directly
	if len(result.Warnings) > 0 {
		return result.Warnings, true
	}
	return nil, false
}

func fileExistsInDir(dir, name string) bool {
	info, err := os.Stat(filepath.Join(dir, name))
	return err == nil && !info.IsDir()
}

func init() {
	deployCmd.Flags().String("domain", "", "Domain for the application (required)")
	deployCmd.Flags().String("server", "", "Remote server (user@host or registered name) for remote deployment")
	deployCmd.Flags().String("port", "22", "SSH port for remote deployment")
	deployCmd.Flags().String("key", "", "Path to SSH private key for remote deployment")
	deployCmd.Flags().String("profile", "", "Deployment profile (auto-detected if not set)")
	deployCmd.Flags().String("strategy", "basic", "Deployment strategy (basic, bluegreen, rolling)")
	deployCmd.Flags().String("name", "", "Project name (defaults to directory name)")
	deployCmd.Flags().Duration("timeout", 5*time.Minute, "Deployment timeout")
	deployCmd.Flags().Bool("insecure", false, "Skip SSH host key verification for remote deploys")
	deployCmd.Flags().String("passphrase", "", "Passphrase for encrypted SSH private key")
	deployCmd.Flags().String("pre-deploy", "", "Command to run before deploy (e.g. \"npm run migrate\")")
	deployCmd.Flags().String("post-deploy", "", "Command to run after deploy (e.g. \"npm run seed\")")
	deployCmd.Flags().Bool("no-cache", false, "Pass --no-cache to docker compose build for clean rebuilds")
	deployCmd.Flags().Bool("fresh", false, "Remove existing containers and volumes before deploying (docker compose down -v)")
	deployCmd.Flags().String("migrate", "", "Run this command inside the app container after deploy (auto-snapshots DB first). Local deploys only.")
	deployCmd.Flags().Duration("watch", 0, "After successful deploy, poll the domain for this long to verify it stays healthy (0 disables)")
	deployCmd.Flags().Bool("watch-rollback", false, "If --watch detects an unhealthy deploy, automatically restore the pre-deploy snapshot")
	deployCmd.Flags().String("watch-rollback-mode", "full", "Auto-rollback scope: 'full' (config+volumes+DB) or 'files' (config only, preserves user data written during watch window)")
	deployCmd.Flags().Duration("watch-interval", 10*time.Second, "How often to probe during --watch")
	deployCmd.Flags().Int("watch-threshold", 3, "Consecutive failed probes before --watch declares the deploy bad")
	deployCmd.Flags().Int("watch-status", 200, "Expected HTTP status code during --watch probes")

	rootCmd.AddCommand(deployCmd)
}

// runPostDeployWatchdog observes the deployed domain for opts.watch. If it
// declares the deploy unhealthy and opts.rollback is set, the most recent
// pre-deploy snapshot is restored to undo the broken deployment. Returns
// an error only when the rollback itself fails — an unhealthy verdict
// without rollback is reported to the operator but does not error.
func runPostDeployWatchdog(ctx context.Context, projectName, domain string, opts deployWatchOptions) error {
	if opts.watch <= 0 {
		return nil
	}
	ui.Info("Watching https://%s for %s (threshold: %d consecutive failures)...",
		domain, opts.watch.Round(time.Second), opts.threshold)

	watchCtx, cancel := context.WithTimeout(ctx, opts.watch+30*time.Second)
	defer cancel()

	res := deploy.Observe(watchCtx, deploy.WatchdogConfig{
		URL:              fmt.Sprintf("https://%s", domain),
		Duration:         opts.watch,
		Interval:         opts.interval,
		FailureThreshold: opts.threshold,
		ExpectedStatus:   opts.expectedCode,
	})

	if res.Healthy {
		ui.Success("Post-deploy watch: %d probes, no threshold breach", res.Probes)
		audit.Log("deploy.watch", projectName, fmt.Sprintf("healthy probes=%d", res.Probes), true)
		return nil
	}

	ui.Error("Post-deploy watch: %d consecutive failures (last status: %d, last error: %q)",
		res.ConsecutiveFailures, res.LastStatus, res.LastError)
	audit.Log("deploy.watch", projectName, fmt.Sprintf("unhealthy status=%d err=%q", res.LastStatus, res.LastError), false)

	if !opts.rollback {
		ui.Warn("Run 'fleetdeck rollback %s' to restore the pre-deploy snapshot.", projectName)
		return nil
	}

	ui.Info("Auto-rollback (mode=%s): restoring pre-deploy snapshot of %s...", opts.rollbackMode, projectName)
	return rollbackToPreDeploySnapshot(projectName, opts.rollbackMode)
}

// rollbackToPreDeploySnapshot finds the most recent backup whose trigger
// is "pre-deploy" and restores it, respecting the scope requested by
// mode:
//
//	"full"  — restore config files + volumes + database dumps. The safe
//	          default for stateless apps and landing pages where a
//	          silent rollback is cheaper than any data that accrued in
//	          the N-minute watch window.
//	"files" — restore only config files (docker-compose.yml, .env,
//	          Dockerfile). Volumes and database dumps are LEFT ALONE so
//	          user data written during the watch window is preserved.
//	          Use this for stateful apps (mealtime etc.) where data
//	          loss from the watch window is worse than running the old
//	          code against the new schema for a few minutes.
//
// Returns an error if no such snapshot exists or the restore fails.
func rollbackToPreDeploySnapshot(projectName, mode string) error {
	if mode == "" {
		mode = "full"
	}

	d := openDB()
	p, err := d.GetProject(projectName)
	if err != nil {
		return fmt.Errorf("loading project: %w", err)
	}
	backups, err := d.ListBackupRecords(p.ID, 0)
	if err != nil {
		return fmt.Errorf("listing backups: %w", err)
	}

	var restoreOpts backup.RestoreOptions
	switch mode {
	case "full":
		restoreOpts = backup.RestoreOptions{} // default = restore everything
	case "files":
		restoreOpts = backup.RestoreOptions{FilesOnly: true}
	default:
		return fmt.Errorf("invalid --watch-rollback-mode %q (want full|files)", mode)
	}

	for _, b := range backups {
		if b.Trigger != "pre-deploy" {
			continue
		}
		if err := backup.RestoreBackup(b.Path, p.ProjectPath, restoreOpts); err != nil {
			audit.Log("deploy.watch.rollback", projectName, err.Error(), false)
			return fmt.Errorf("restoring snapshot %s: %w", b.ID[:minInt(12, len(b.ID))], err)
		}
		if err := d.UpdateProjectStatus(p.Name, "running"); err != nil {
			ui.Warn("Could not update project status: %v", err)
		}
		audit.Log("deploy.watch.rollback", projectName,
			fmt.Sprintf("restored=%s mode=%s", b.ID[:minInt(12, len(b.ID))], mode), true)
		ui.Success("Rolled back to pre-deploy snapshot %s (mode=%s)", b.ID[:minInt(12, len(b.ID))], mode)
		return nil
	}
	return fmt.Errorf("no pre-deploy snapshot found to roll back to")
}

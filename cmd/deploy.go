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
	"github.com/fleetdeck/fleetdeck/internal/sourcedeploy"
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
	Long: `Deploy a reviewed Docker Compose application locally or over SSH.

Remote source deployments require compatible FleetDeck on the server. They
preserve existing secrets, serialize updates on the server, verify a snapshot
before replacing source, and wait for Compose readiness. Stateful source updates
may stop application writers during the backup/build window. Database rollback
is an explicit operator decision, never an automatic response to a failed probe.

Save a private target once:
  fleetdeck project link . --server production --name demo --domain demo.example.com
  fleetdeck deploy .

Seed the first .env explicitly when needed:
  fleetdeck deploy . --env-file /private/path/production.env

For immutable releases and reviewed migrations, see 'fleetdeck release'.`,
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

		if err := applyProjectTarget(cmd, absDir); err != nil {
			return err
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

		if server != "" {
			if fresh || strategyName != "basic" || preDeployHook != "" || postDeployHook != "" {
				return fmt.Errorf("safe source deployments currently require basic strategy without --fresh or shell hooks; use bounded release jobs for reviewed migration/recovery workflows")
			}
			if readWatchOptions(cmd).watch > 0 || readWatchOptions(cmd).rollback {
				return fmt.Errorf("remote source deployments use server-side Compose readiness; client watchdog rollback cannot restore a remote snapshot")
			}
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
		if server != "" {
			return nil
		}
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
	} else if os.Getenv("SSH_AUTH_SOCK") == "" {
		keyData = findSSHKey(host)
		if keyData == nil {
			return fmt.Errorf("no SSH key found; use --key to specify one")
		}
	}

	// Step 2: Connect
	insecure, _ := cmd.Flags().GetBool("insecure")
	ui.Step(2, 5, "Connecting to %s@%s...", user, host)
	var (
		client  *remote.Client
		connErr error
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
	if err := client.EnableSudo(); err != nil {
		return err
	}
	ui.Success("Connected to %s", host)

	version := remote.ReadVersion(client)
	if version.SourceDeployProtocol != sourcedeploy.Protocol {
		return fmt.Errorf("server does not support safe source deployment protocol %s; update FleetDeck on the server first (no files uploaded)", sourcedeploy.Protocol)
	}
	serverConfig, _ := cmd.Flags().GetString("server-config")
	if !filepath.IsAbs(serverConfig) || strings.ContainsAny(serverConfig, "\x00\r\n") {
		return fmt.Errorf("server-config must be an absolute path")
	}
	envFile, _ := cmd.Flags().GetString("env-file")
	// Materialize privately before starting SSH so local errors cannot deliver a
	// truncated archive that looks like a complete deployment to the receiver.
	archive, err := os.CreateTemp("", "fleetdeck-source-*.tar")
	if err != nil {
		return err
	}
	defer os.Remove(archive.Name())
	defer archive.Close()
	if err = sourcedeploy.ArchiveWithEnv(dir, archive, envFile); err != nil {
		return err
	}
	if _, err = archive.Seek(0, 0); err != nil {
		return err
	}
	args := []string{"fleetdeck", "--config", serverConfig, "deploy", "receive", "--name", name, "--domain", domain, "--profile", prof.Name, "--timeout", timeout.String()}
	if noCache {
		args = append(args, "--no-cache")
	}
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = shellQuote(arg)
	}
	ui.Info("Server will preserve secrets and verify a backup before replacing existing source")
	ctx, cancel := context.WithTimeout(cmd.Context(), timeout+time.Minute)
	defer cancel()
	if err = client.RunInput(ctx, strings.Join(quoted, " "), archive, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
		return fmt.Errorf("server-side deployment failed; inspect the server snapshot and reported readiness before retrying: %w", err)
	}
	audit.Log("deploy.remote", name, "server="+server+" protocol="+sourcedeploy.Protocol, true)
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
	deployCmd.Flags().String("env-file", "", "Explicit private .env seed for a new remote project; existing server values always win")
	deployCmd.Flags().String("domain", "", "Domain for the application (required)")
	deployCmd.Flags().String("server-config", "/etc/fleetdeck/config.toml", "Operator-owned FleetDeck configuration on the remote server")
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

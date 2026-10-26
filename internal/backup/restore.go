package backup

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/project"
	"github.com/fleetdeck/fleetdeck/internal/ui"
)

type RestoreOptions struct {
	FilesOnly   bool
	VolumesOnly bool
	DBOnly      bool
	NoStart     bool
}

func RestoreBackup(backupPath, projectPath string, opts RestoreOptions) error {
	manifest, err := ReadManifest(backupPath)
	if err != nil {
		return fmt.Errorf("reading manifest: %w", err)
	}

	// Verify backup integrity before restoring
	ui.Info("Verifying backup integrity before restore...")
	results := verifyManifest(backupPath, manifest)
	if HasFailures(results) {
		total, ok, failed, missing := CountResults(results)
		for _, r := range results {
			if r.Status != VerifyOK {
				ui.Error("  %s: %v", r.Component.Name, r.Error)
			}
		}
		return fmt.Errorf("backup verification failed: %d/%d OK, %d failed, %d missing — aborting restore",
			ok, total, failed, missing)
	}
	ui.Success("Backup integrity verified (%d components OK)", len(results))
	fmt.Println()

	restoreAll := !opts.FilesOnly && !opts.VolumesOnly && !opts.DBOnly

	source, err := os.OpenRoot(backupPath)
	if err != nil {
		return err
	}
	defer source.Close()
	destination, err := os.OpenRoot(projectPath)
	if err != nil {
		return err
	}
	defer destination.Close()
	// Preflight every selected configuration target before stopping containers
	// or writing any file, including targets later in the manifest.
	if restoreAll || opts.FilesOnly {
		for _, comp := range manifest.Components {
			if comp.Type == "config" {
				if err := checkPath(destination, comp.Name, false, true); err != nil {
					return fmt.Errorf("unsafe configuration destination: %w", err)
				}
			}
		}
	}

	totalSteps := 0
	if restoreAll || opts.FilesOnly {
		totalSteps++
	}
	if restoreAll || opts.VolumesOnly {
		totalSteps++
	}
	if restoreAll || opts.DBOnly {
		totalSteps++
	}
	if !opts.NoStart {
		totalSteps++
	}
	// +1 for stop
	totalSteps++

	step := 0

	// Component restore failures are collected rather than ignored: previously
	// every per-component failure was warned and skipped, then RestoreBackup
	// returned nil, so a restore that failed to import the DB or extract a
	// volume reported success and the app was started against partial/empty
	// data. We now abort (and do NOT start the project) if anything failed.
	var failures []string

	// Never replace data while a failed stop may have left writers running.
	step++
	ui.Step(step, totalSteps, "Stopping project containers...")
	if err := project.ComposeDown(projectPath); err != nil {
		return fmt.Errorf("stopping project before restore: %w; no backup data restored", err)
	} else {
		ui.Success("Containers stopped")
	}

	// Restore config files
	if restoreAll || opts.FilesOnly {
		step++
		ui.Step(step, totalSteps, "Restoring configuration files...")
		configCount := 0
		for _, comp := range manifest.Components {
			if comp.Type != "config" {
				continue
			}
			if err := restoreConfigFile(source, destination, comp); err != nil {
				ui.Warn("Could not restore %s: %v", comp.Name, err)
				failures = append(failures, fmt.Sprintf("config %s: %v", comp.Name, err))
				continue
			}
			configCount++
		}
		ui.Success("Restored %d configuration files", configCount)
	}

	// Restore volumes
	if restoreAll || opts.VolumesOnly {
		step++
		ui.Step(step, totalSteps, "Restoring volumes...")
		volCount := 0
		for _, comp := range manifest.Components {
			if comp.Type != "volume" {
				continue
			}
			archivePath := filepath.Join(backupPath, comp.Path)

			if strings.HasPrefix(filepath.Base(archivePath), "namedvol_") {
				// Named volume — restore via docker
				volName := extractNamedVolumeName(comp.Name)
				if volName != "" {
					if err := restoreNamedVolume(archivePath, volName); err != nil {
						ui.Warn("Could not restore named volume %s: %v", volName, err)
						failures = append(failures, fmt.Sprintf("volume %s: %v", volName, err))
						continue
					}
				}
			} else {
				// Bind mount — extract to project path
				if err := restoreProjectBindMount(archivePath, projectPath, comp.RestorePath); err != nil {
					ui.Warn("Could not restore volume %s: %v", comp.Name, err)
					failures = append(failures, fmt.Sprintf("volume %s: %v", comp.Name, err))
					continue
				}
			}
			volCount++
		}
		ui.Success("Restored %d volumes", volCount)
	}

	// Restore databases
	if restoreAll || opts.DBOnly {
		step++
		ui.Step(step, totalSteps, "Restoring databases...")
		dbCount := 0
		for _, comp := range manifest.Components {
			if comp.Type != "database" {
				continue
			}
			dumpPath := filepath.Join(backupPath, comp.Path)

			// Need to start just the database container first
			if err := startDBContainer(projectPath, comp.Name); err != nil {
				ui.Warn("Could not start database container: %v", err)
				failures = append(failures, fmt.Sprintf("database %s (start): %v", comp.Name, err))
				continue
			}

			if err := restoreDatabase(dumpPath, projectPath, comp.Name); err != nil {
				ui.Warn("Could not restore database %s: %v", comp.Name, err)
				failures = append(failures, fmt.Sprintf("database %s: %v", comp.Name, err))
				continue
			}
			dbCount++
		}
		if dbCount > 0 {
			ui.Success("Restored %d databases", dbCount)
		}
	}

	// If any component failed to restore, do NOT start the project — running it
	// against a partially-restored state (e.g. an empty database) is worse than
	// leaving it stopped — and report the failure so the operator knows the
	// restore is not trustworthy.
	if len(failures) > 0 {
		ui.Error("Restore incomplete — %d component(s) failed:", len(failures))
		for _, f := range failures {
			ui.Error("  - %s", f)
		}
		return fmt.Errorf("restore incomplete: %d component(s) failed; project not started to avoid running against partially-restored data", len(failures))
	}

	// Start the project
	if !opts.NoStart {
		step++
		ui.Step(step, totalSteps, "Starting project...")
		if err := project.ComposeUp(projectPath); err != nil {
			return fmt.Errorf("starting project: %w", err)
		}
		ui.Success("Project started")
	}

	return nil
}

func restoreNamedVolume(archivePath, volumeName string) error {
	archiveDir := filepath.Dir(archivePath)
	archiveFile := filepath.Base(archivePath)

	// Extract into a staging dir FIRST so a failed or corrupt archive leaves the
	// existing volume data untouched. The previous implementation ran
	// `rm -rf /data/* && tar xzf`, which wiped the volume *before* extracting —
	// if the extract then failed (corrupt archive, out of space) the volume was
	// left empty and the data was gone. `set -e` aborts before the destructive
	// clear on any earlier failure; the clear uses `find -mindepth 1` so it also
	// removes dotfiles, giving a clean replace rather than a merge.
	script := "set -e; " +
		"rm -rf /data/.restore.tmp; mkdir -p /data/.restore.tmp; " +
		"tar xzf /backup/" + shellQuote(archiveFile) + " -C /data/.restore.tmp; " +
		"find /data -mindepth 1 -maxdepth 1 -not -name .restore.tmp -exec rm -rf {} +; " +
		"cp -a /data/.restore.tmp/. /data/; " +
		"rm -rf /data/.restore.tmp"

	cmd := exec.Command("docker", "run", "--rm",
		"-v", volumeName+":/data",
		"-v", archiveDir+":/backup:ro",
		"alpine",
		"sh", "-c", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("restore: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func restoreBindMount(archivePath, projectPath string) error {
	cmd := exec.Command("tar", "xzf", archivePath, "-C", projectPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tar extract: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// startDBContainer brings up the named DB service via docker compose and
// waits for the container to start accepting exec calls. Returns an error
// on timeout or startup failure so the caller doesn't silently proceed to
// import a dump into a container that never came up.
func startDBContainer(projectPath, componentName string) error {
	// Extract service name from component name like "postgres (PostgreSQL)"
	parts := strings.Fields(componentName)
	if len(parts) == 0 {
		return fmt.Errorf("invalid DB component name %q", componentName)
	}
	serviceName := parts[0]

	cmd := exec.Command("docker", "compose", "up", "-d", serviceName)
	cmd.Dir = projectPath
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("starting %s: %s: %w", serviceName, strings.TrimSpace(string(out)), err)
	}

	// Poll up to 60 s with a 2 s backoff. Previously the loop had no sleep,
	// which meant 30 failed docker-exec attempts completed in a few ms and
	// the function returned nil unconditionally — so a DB that never started
	// would silently fail the dump import a moment later with a confusing
	// error about the database being unreachable.
	const (
		attempts = 30
		backoff  = 2 * time.Second
	)
	for i := 0; i < attempts; i++ {
		wait := exec.Command("docker", "compose", "exec", "-T", serviceName, "true")
		wait.Dir = projectPath
		if err := wait.Run(); err == nil {
			return nil
		}
		time.Sleep(backoff)
	}
	return fmt.Errorf("timed out waiting for %s to start (%d attempts, %s each)", serviceName, attempts, backoff)
}

func restoreDatabase(dumpPath, projectPath, componentName string) error {
	serviceName := strings.Fields(componentName)[0]
	envVars := loadEnvFile(projectPath)

	composePath := filepath.Join(projectPath, "docker-compose.yml")

	if strings.Contains(componentName, "PostgreSQL") {
		user := envVars["POSTGRES_USER"]
		dbName := envVars["POSTGRES_DB"]
		if user == "" {
			user = "postgres"
		}
		if dbName == "" {
			dbName = user
		}

		// Use shellQuote to prevent injection from env file values
		cmd := exec.Command("bash", "-c",
			"gunzip -c "+shellQuote(dumpPath)+" | docker compose -f "+shellQuote(composePath)+" exec -T "+shellQuote(serviceName)+" psql -U "+shellQuote(user)+" "+shellQuote(dbName))
		cmd.Dir = projectPath
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("psql restore: %s: %w", strings.TrimSpace(string(out)), err)
		}
	} else if strings.Contains(componentName, "MySQL") {
		password := envVars["MYSQL_ROOT_PASSWORD"]
		dbName := envVars["MYSQL_DATABASE"]
		if dbName == "" {
			dbName = envVars["MYSQL_DB"]
		}

		// Pass the password through MYSQL_PWD instead of '-p<password>' on
		// argv. The CLI form is visible via `ps aux` to any local user for
		// the duration of the restore; the env form is not.
		mysqlCmd := "docker compose -f " + shellQuote(composePath) + " exec -T"
		if password != "" {
			mysqlCmd += " -e MYSQL_PWD"
		}
		mysqlCmd += " " + shellQuote(serviceName) + " mysql -u root " + shellQuote(dbName)

		cmd := exec.Command("bash", "-c",
			"gunzip -c "+shellQuote(dumpPath)+" | "+mysqlCmd)
		cmd.Dir = projectPath
		if password != "" {
			cmd.Env = append(os.Environ(), "MYSQL_PWD="+password)
		}
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("mysql restore: %s: %w", strings.TrimSpace(string(out)), err)
		}
	}

	return nil
}

func extractNamedVolumeName(name string) string {
	// Name format: "volname (named volume)"
	parts := strings.Split(name, " ")
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

func ReadManifest(backupPath string) (*Manifest, error) {
	root, err := os.OpenRoot(backupPath)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if err := checkPath(root, "manifest.json", false, false); err != nil {
		return nil, err
	}
	data, err := root.ReadFile("manifest.json")
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}
	return &m, nil
}

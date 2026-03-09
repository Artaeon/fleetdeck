package backup

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
	manifestPath := filepath.Join(backupPath, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("reading manifest: %w", err)
	}

	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("parsing manifest: %w", err)
	}

	// Verify backup integrity before restoring
	ui.Info("Verifying backup integrity before restore...")
	results, err := VerifyBackup(backupPath)
	if err != nil {
		return fmt.Errorf("pre-restore verification failed: %w", err)
	}
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

	// Stop running containers first
	step++
	ui.Step(step, totalSteps, "Stopping project containers...")
	_ = project.ComposeDown(projectPath)
	ui.Success("Containers stopped")

	// Restore config files
	if restoreAll || opts.FilesOnly {
		step++
		ui.Step(step, totalSteps, "Restoring configuration files...")
		configCount := 0
		for _, comp := range manifest.Components {
			if comp.Type != "config" {
				continue
			}
			// Validate paths to prevent path traversal
			if strings.Contains(comp.Path, "..") || strings.Contains(comp.Name, "..") {
				ui.Warn("Skipping suspicious path: %s", comp.Path)
				continue
			}
			src := filepath.Join(backupPath, comp.Path)
			dst := filepath.Join(projectPath, comp.Name)

			if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
				ui.Warn("Could not create dir for %s: %v", comp.Name, err)
				continue
			}

			if _, _, err := copyFileWithChecksum(src, dst); err != nil {
				ui.Warn("Could not restore %s: %v", comp.Name, err)
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
			if strings.Contains(comp.Path, "..") || strings.Contains(comp.Name, "..") {
				ui.Warn("Skipping suspicious path: %s", comp.Path)
				continue
			}
			archivePath := filepath.Join(backupPath, comp.Path)

			if strings.HasPrefix(filepath.Base(archivePath), "namedvol_") {
				// Named volume — restore via docker
				volName := extractNamedVolumeName(comp.Name)
				if volName != "" {
					if err := restoreNamedVolume(archivePath, volName); err != nil {
						ui.Warn("Could not restore named volume %s: %v", volName, err)
						continue
					}
				}
			} else {
				// Bind mount — extract to project path
				if err := restoreBindMount(archivePath, projectPath); err != nil {
					ui.Warn("Could not restore volume %s: %v", comp.Name, err)
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
				continue
			}

			if err := restoreDatabase(dumpPath, projectPath, comp.Name); err != nil {
				ui.Warn("Could not restore database %s: %v", comp.Name, err)
				continue
			}
			dbCount++
		}
		if dbCount > 0 {
			ui.Success("Restored %d databases", dbCount)
		}
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

	cmd := exec.Command("docker", "run", "--rm",
		"-v", volumeName+":/data",
		"-v", archiveDir+":/backup:ro",
		"alpine",
		"sh", "-c", "rm -rf /data/* && tar xzf /backup/"+shellQuote(archiveFile)+" -C /data")
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

func startDBContainer(projectPath, componentName string) error {
	// Extract service name from component name like "postgres (PostgreSQL)"
	serviceName := strings.Fields(componentName)[0]

	cmd := exec.Command("docker", "compose", "up", "-d", serviceName)
	cmd.Dir = projectPath
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("starting %s: %s: %w", serviceName, strings.TrimSpace(string(out)), err)
	}

	// Wait for it to be healthy
	waitCmd := exec.Command("docker", "compose", "exec", serviceName, "true")
	waitCmd.Dir = projectPath
	for i := 0; i < 30; i++ {
		if err := waitCmd.Run(); err == nil {
			return nil
		}
		waitCmd = exec.Command("docker", "compose", "exec", serviceName, "true")
		waitCmd.Dir = projectPath
	}
	return nil
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

		passArg := ""
		if password != "" {
			passArg = "-p" + password
		}

		mysqlArgs := []string{"docker", "compose", "-f", composePath, "exec", "-T", serviceName, "mysql", "-u", "root"}
		if passArg != "" {
			mysqlArgs = append(mysqlArgs, passArg)
		}
		mysqlArgs = append(mysqlArgs, dbName)
		cmd := exec.Command("bash", "-c",
			"gunzip -c "+shellQuote(dumpPath)+" | "+shellQuote(mysqlArgs...))
		cmd.Dir = projectPath
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
	data, err := os.ReadFile(filepath.Join(backupPath, "manifest.json"))
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

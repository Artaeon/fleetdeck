package cmd

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show server overview",
	RunE: func(cmd *cobra.Command, args []string) error {
		d := openDB()

		fmt.Println(ui.Bold("Server Overview"))
		fmt.Println()

		// CPU info
		fmt.Printf("  CPUs:        %d cores\n", runtime.NumCPU())

		// Memory info
		if out, err := exec.Command("free", "-h", "--si").Output(); err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				if strings.HasPrefix(line, "Mem:") {
					fields := strings.Fields(line)
					if len(fields) >= 3 {
						fmt.Printf("  Memory:      %s used / %s total\n", fields[2], fields[1])
					}
				}
			}
		}

		// Disk info
		if out, err := exec.Command("df", "-h", cfg.Server.BasePath).Output(); err == nil {
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			if len(lines) >= 2 {
				fields := strings.Fields(lines[1])
				if len(fields) >= 5 {
					fmt.Printf("  Disk:        %s used / %s total (%s)\n", fields[2], fields[1], fields[4])
				}
			}
		}

		// Project stats
		projects, err := d.ListProjects()
		if err != nil {
			return err
		}

		running := 0
		stopped := 0
		totalContainers := 0
		for _, p := range projects {
			switch p.Status {
			case "running":
				running++
			case "stopped":
				stopped++
			}
			_, total := countContainersForProject(p.ProjectPath)
			totalContainers += total
		}

		fmt.Println()
		fmt.Printf("  Projects:    %d running, %d stopped\n", running, stopped)
		fmt.Printf("  Containers:  %d total\n", totalContainers)

		// Traefik status
		traefikCheck := exec.Command("docker", "ps", "--filter", "name=traefik", "--format", "{{.Status}}")
		if out, err := traefikCheck.Output(); err == nil && len(strings.TrimSpace(string(out))) > 0 {
			fmt.Printf("  Traefik:     %s\n", ui.StatusColor("running"))
		} else {
			fmt.Printf("  Traefik:     %s\n", ui.StatusColor("stopped"))
		}

		return nil
	},
}

func countContainersForProject(path string) (int, int) {
	cmd := exec.Command("docker", "compose", "ps", "--format", "json")
	cmd.Dir = path
	out, err := cmd.Output()
	if err != nil {
		return 0, 0
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	total := 0
	running := 0
	for _, line := range lines {
		if line == "" {
			continue
		}
		total++
		if strings.Contains(line, `"running"`) {
			running++
		}
	}
	return running, total
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

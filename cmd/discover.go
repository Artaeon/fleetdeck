package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/fleetdeck/fleetdeck/internal/db"
	"github.com/fleetdeck/fleetdeck/internal/discover"
	"github.com/fleetdeck/fleetdeck/internal/project"
	"github.com/fleetdeck/fleetdeck/internal/ui"
	"github.com/spf13/cobra"
)

var discoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Discover existing Docker Compose projects on this server",
	Long: `Scans the server for Docker Compose projects, running containers,
and Traefik routes. Displays what was found so you can selectively import
projects into FleetDeck.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		d := openDB()

		searchPaths := cfg.Discovery.SearchPaths
		extra, _ := cmd.Flags().GetStringSlice("search-path")
		if len(extra) > 0 {
			searchPaths = append(searchPaths, extra...)
		}

		// Temporarily override search paths
		origPaths := cfg.Discovery.SearchPaths
		cfg.Discovery.SearchPaths = searchPaths
		defer func() { cfg.Discovery.SearchPaths = origPaths }()

		ui.Info("Scanning for Docker Compose projects...")
		projects, err := discover.DiscoverAll(cfg, d)
		if err != nil {
			return fmt.Errorf("discovery failed: %w", err)
		}

		showAll, _ := cmd.Flags().GetBool("all")
		asJSON, _ := cmd.Flags().GetBool("json")

		if !showAll {
			var filtered []discover.DiscoveredProject
			for _, p := range projects {
				if !p.AlreadyManaged {
					filtered = append(filtered, p)
				}
			}
			projects = filtered
		}

		if len(projects) == 0 {
			ui.Info("No unmanaged projects found")
			if !showAll {
				ui.Info("Use --all to include already-managed projects")
			}
			return nil
		}

		if asJSON {
			data, _ := json.MarshalIndent(projects, "", "  ")
			fmt.Println(string(data))
			return nil
		}

		headers := []string{"#", "NAME", "PATH", "DOMAIN", "USER", "CONTAINERS", "STATUS", "MANAGED"}
		var rows [][]string
		for i, p := range projects {
			status := "stopped"
			if p.Running {
				status = "running"
			}
			managed := "no"
			if p.AlreadyManaged {
				managed = "yes (" + p.ManagedName + ")"
			}
			containers := fmt.Sprintf("%d/%d", p.RunningCount, p.ContainerCount)

			rows = append(rows, []string{
				fmt.Sprintf("%d", i+1),
				p.Name,
				truncPath(p.Dir, 35),
				p.Domain,
				p.LinuxUser,
				containers,
				ui.StatusColor(status),
				managed,
			})
		}

		ui.Table(headers, rows)
		fmt.Printf("\nFound %d project(s)\n", len(projects))
		return nil
	},
}

var discoverImportCmd = &cobra.Command{
	Use:   "import",
	Short: "Import discovered projects into FleetDeck",
	RunE: func(cmd *cobra.Command, args []string) error {
		d := openDB()

		searchPaths := cfg.Discovery.SearchPaths
		extra, _ := cmd.Flags().GetStringSlice("search-path")
		if len(extra) > 0 {
			searchPaths = append(searchPaths, extra...)
		}

		origPaths := cfg.Discovery.SearchPaths
		cfg.Discovery.SearchPaths = searchPaths
		defer func() { cfg.Discovery.SearchPaths = origPaths }()

		ui.Info("Scanning for Docker Compose projects...")
		projects, err := discover.DiscoverAll(cfg, d)
		if err != nil {
			return err
		}

		// Filter to only unmanaged
		var unmanaged []discover.DiscoveredProject
		for _, p := range projects {
			if !p.AlreadyManaged {
				unmanaged = append(unmanaged, p)
			}
		}

		if len(unmanaged) == 0 {
			ui.Success("All discovered projects are already managed by FleetDeck")
			return nil
		}

		importAll, _ := cmd.Flags().GetBool("all")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		// Display found projects
		for i, p := range unmanaged {
			status := "stopped"
			if p.Running {
				status = "running"
			}
			fmt.Printf("  %d. %s (%s) — %s, %d containers [%s]\n",
				i+1, ui.Bold(p.Name), p.Dir, p.Domain, p.ContainerCount, status)
		}
		fmt.Println()

		var selected []int
		if importAll {
			for i := range unmanaged {
				selected = append(selected, i)
			}
		} else {
			fmt.Print("Enter project numbers to import (comma-separated, or 'all'): ")
			reader := bufio.NewReader(os.Stdin)
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)

			if input == "" {
				ui.Info("Aborted")
				return nil
			}

			if input == "all" {
				for i := range unmanaged {
					selected = append(selected, i)
				}
			} else {
				for _, part := range strings.Split(input, ",") {
					part = strings.TrimSpace(part)
					var idx int
					if _, err := fmt.Sscanf(part, "%d", &idx); err == nil && idx >= 1 && idx <= len(unmanaged) {
						selected = append(selected, idx-1)
					}
				}
			}
		}

		if len(selected) == 0 {
			ui.Info("No projects selected")
			return nil
		}

		if dryRun {
			ui.Info("Dry run — would import %d project(s):", len(selected))
			for _, idx := range selected {
				p := unmanaged[idx]
				fmt.Printf("  - %s (%s)\n", p.Name, p.Dir)
			}
			return nil
		}

		// Import selected projects
		imported := 0
		for _, idx := range selected {
			p := unmanaged[idx]
			linuxUser := p.LinuxUser
			if linuxUser == "" {
				linuxUser = project.LinuxUserName(p.Name)
			}

			status := "stopped"
			if p.Running {
				status = "running"
			}

			domain := p.Domain
			if domain == "" {
				domain = p.Name + ".local"
			}

			proj := &db.Project{
				Name:        p.Name,
				Domain:      domain,
				LinuxUser:   linuxUser,
				ProjectPath: p.Dir,
				Template:    "custom",
				Status:      status,
				Source:      "discovered",
			}

			if err := d.CreateProject(proj); err != nil {
				ui.Warn("Could not import %s: %v", p.Name, err)
				continue
			}

			ui.Success("Imported %s (%s)", p.Name, p.Dir)
			imported++
		}

		fmt.Println()
		ui.Success("Imported %d project(s)", imported)
		return nil
	},
}

func truncPath(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}
	return "..." + path[len(path)-maxLen+3:]
}

func init() {
	discoverCmd.Flags().StringSlice("search-path", nil, "Additional paths to search")
	discoverCmd.Flags().Bool("all", false, "Include already-managed projects")
	discoverCmd.Flags().Bool("json", false, "Output as JSON")

	discoverImportCmd.Flags().StringSlice("search-path", nil, "Additional paths to search")
	discoverImportCmd.Flags().Bool("all", false, "Import all discovered projects without prompting")
	discoverImportCmd.Flags().Bool("dry-run", false, "Show what would be imported without doing it")

	discoverCmd.AddCommand(discoverImportCmd)
	rootCmd.AddCommand(discoverCmd)
}

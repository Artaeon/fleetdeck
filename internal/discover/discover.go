package discover

import (
	"path/filepath"
	"strings"

	"github.com/fleetdeck/fleetdeck/internal/config"
	"github.com/fleetdeck/fleetdeck/internal/db"
)

type DiscoveredProject struct {
	Name           string
	Dir            string
	Domain         string
	LinuxUser      string
	Services       []DiscoveredService
	Running        bool
	ContainerCount int
	RunningCount   int
	AlreadyManaged bool
	ManagedName    string // name in FleetDeck DB if already managed
	HasDB          bool
	DBType         string
}

type DiscoveredService struct {
	Name        string
	Image       string
	ContainerID string
	State       string
}

func DiscoverAll(cfg *config.Config, database *db.DB) ([]DiscoveredProject, error) {
	// Get existing managed projects
	existingPaths, err := database.ListProjectPaths()
	if err != nil {
		existingPaths = make(map[string]string)
	}

	// Scan for compose files
	composeProjects, err := ScanComposeFiles(cfg.Discovery.SearchPaths, cfg.Discovery.ExcludePaths)
	if err != nil {
		composeProjects = nil
	}

	// Scan running containers
	containerGroups, err := ScanRunningContainers()
	if err != nil {
		containerGroups = nil
	}

	// Merge results
	return mergeResults(composeProjects, containerGroups, existingPaths), nil
}

func mergeResults(composeProjects []ComposeProject, containerGroups map[string][]ContainerInfo, existingPaths map[string]string) []DiscoveredProject {
	projectMap := make(map[string]*DiscoveredProject)

	// Start from compose projects found on disk
	for _, cp := range composeProjects {
		absDir, _ := filepath.Abs(cp.Dir)
		dp := &DiscoveredProject{
			Name:   cp.Name,
			Dir:    absDir,
			Domain: cp.Domain,
			HasDB:  cp.HasDB,
			DBType: cp.DBType,
		}

		// Detect owner
		owner, err := DetectProjectOwner(absDir)
		if err == nil {
			dp.LinuxUser = owner
		}

		// Check if already managed
		if managedName, exists := existingPaths[absDir]; exists {
			dp.AlreadyManaged = true
			dp.ManagedName = managedName
		}

		projectMap[absDir] = dp
	}

	// Enrich with container info
	for dir, containers := range containerGroups {
		if dir == "__standalone__" {
			continue
		}

		absDir, _ := filepath.Abs(dir)
		dp, exists := projectMap[absDir]
		if !exists {
			// Found containers but no compose file on disk
			name := filepath.Base(absDir)
			if len(containers) > 0 && containers[0].ComposeProject != "" {
				name = containers[0].ComposeProject
			}
			dp = &DiscoveredProject{
				Name: name,
				Dir:  absDir,
			}
			if managedName, managed := existingPaths[absDir]; managed {
				dp.AlreadyManaged = true
				dp.ManagedName = managedName
			}
			projectMap[absDir] = dp
		}

		for _, c := range containers {
			svc := DiscoveredService{
				Name:        c.Service,
				Image:       c.Image,
				ContainerID: c.ID,
				State:       c.State,
			}
			dp.Services = append(dp.Services, svc)
			dp.ContainerCount++
			if c.State == "running" {
				dp.RunningCount++
				dp.Running = true
			}

			// Try to extract domain from container labels
			if dp.Domain == "" {
				dp.Domain = ExtractDomain(c.Labels)
			}

			// Detect owner from container working dir
			if dp.LinuxUser == "" {
				owner, err := DetectProjectOwner(absDir)
				if err == nil {
					dp.LinuxUser = owner
				}
			}

			// Detect DB from image
			image := strings.ToLower(c.Image)
			if strings.HasPrefix(image, "postgres") {
				dp.HasDB = true
				dp.DBType = "postgres"
			} else if strings.HasPrefix(image, "mysql") || strings.HasPrefix(image, "mariadb") {
				dp.HasDB = true
				dp.DBType = "mysql"
			}
		}
	}

	// Convert map to slice
	var results []DiscoveredProject
	for _, dp := range projectMap {
		results = append(results, *dp)
	}
	return results
}

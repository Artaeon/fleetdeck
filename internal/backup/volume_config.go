package backup

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
)

type composeMount struct {
	Type     string `json:"type"`
	Source   string `json:"source"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"read_only"`
}

type volumeConfig struct {
	Services map[string]struct {
		Volumes []composeMount `json:"volumes"`
	} `json:"services"`
	Volumes map[string]struct {
		Name string `json:"name"`
	} `json:"volumes"`
}

func resolveVolumeConfig(projectPath string) (*volumeConfig, error) {
	cmd := exec.Command("docker", "compose", "config", "--format", "json")
	cmd.Dir = projectPath
	data, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("resolving Compose volume configuration: %w", err)
	}
	var config volumeConfig
	// Never log this output: the complete Compose model can contain credentials.
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing Compose volume configuration: %w", err)
	}
	if config.Services == nil {
		return nil, fmt.Errorf("Compose volume configuration has no services")
	}
	return &config, nil
}

func volumeServiceNames(config *volumeConfig) []string {
	names := make([]string, 0, len(config.Services))
	for name := range config.Services {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

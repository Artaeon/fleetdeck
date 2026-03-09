package discover

import (
	"encoding/json"
	"os/exec"
	"strings"
)

type ContainerInfo struct {
	ID         string
	Name       string
	Image      string
	State      string
	Status     string
	Labels     map[string]string
	ProjectDir string // from com.docker.compose.project.working_dir
	ComposeProject string // from com.docker.compose.project
	Service    string // from com.docker.compose.service
}

func ScanRunningContainers() (map[string][]ContainerInfo, error) {
	cmd := exec.Command("docker", "ps", "-a", "--format", "json", "--no-trunc")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	groups := make(map[string][]ContainerInfo)

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}

		var raw struct {
			ID     string `json:"ID"`
			Names  string `json:"Names"`
			Image  string `json:"Image"`
			State  string `json:"State"`
			Status string `json:"Status"`
			Labels string `json:"Labels"`
		}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}

		labels := parseLabels(raw.Labels)

		ci := ContainerInfo{
			ID:             raw.ID,
			Name:           raw.Names,
			Image:          raw.Image,
			State:          raw.State,
			Status:         raw.Status,
			Labels:         labels,
			ProjectDir:     labels["com.docker.compose.project.working_dir"],
			ComposeProject: labels["com.docker.compose.project"],
			Service:        labels["com.docker.compose.service"],
		}

		key := ci.ProjectDir
		if key == "" {
			key = "__standalone__"
		}
		groups[key] = append(groups[key], ci)
	}

	return groups, nil
}

func parseLabels(labelStr string) map[string]string {
	labels := make(map[string]string)
	for _, pair := range strings.Split(labelStr, ",") {
		pair = strings.TrimSpace(pair)
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 {
			labels[parts[0]] = parts[1]
		}
	}
	return labels
}

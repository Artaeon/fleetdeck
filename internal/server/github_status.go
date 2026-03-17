package server

import (
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"
)

// createGitHubDeployment creates a GitHub deployment via the gh CLI and returns the deployment ID.
func createGitHubDeployment(repo, ref, environment string) (int64, error) {
	payload := map[string]interface{}{
		"ref":               ref,
		"environment":       environment,
		"auto_merge":        false,
		"required_contexts": []string{},
	}
	data, _ := json.Marshal(payload)

	cmd := exec.Command("gh", "api", fmt.Sprintf("repos/%s/deployments", repo),
		"--method", "POST",
		"--input", "-")
	cmd.Stdin = strings.NewReader(string(data))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("creating deployment: %s: %w", strings.TrimSpace(string(out)), err)
	}

	var resp struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return 0, fmt.Errorf("parsing deployment response: %w", err)
	}
	return resp.ID, nil
}

// updateGitHubDeploymentStatus updates the status of a GitHub deployment.
// Valid states: pending, in_progress, success, failure, error
func updateGitHubDeploymentStatus(repo string, deploymentID int64, state, description string) error {
	payload := map[string]interface{}{
		"state":       state,
		"description": description,
	}
	data, _ := json.Marshal(payload)

	cmd := exec.Command("gh", "api",
		fmt.Sprintf("repos/%s/deployments/%d/statuses", repo, deploymentID),
		"--method", "POST",
		"--input", "-")
	cmd.Stdin = strings.NewReader(string(data))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("updating deployment status: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// reportDeploymentStatus is a helper that creates a GitHub deployment and reports
// status changes. It's designed to be called at the start of a deployment, and the
// returned function should be called with the final status.
func (s *Server) reportDeploymentStatus(repo, ref, environment string) func(success bool) {
	if repo == "" {
		return func(bool) {} // no-op if no repo configured
	}

	deployID, err := createGitHubDeployment(repo, ref, environment)
	if err != nil {
		log.Printf("github: failed to create deployment for %s: %v", repo, err)
		return func(bool) {}
	}

	// Set initial status to in_progress
	if err := updateGitHubDeploymentStatus(repo, deployID, "in_progress", "Deployment started"); err != nil {
		log.Printf("github: failed to update deployment status: %v", err)
	}

	return func(success bool) {
		state := "failure"
		desc := "Deployment failed"
		if success {
			state = "success"
			desc = "Deployment succeeded"
		}
		if err := updateGitHubDeploymentStatus(repo, deployID, state, desc); err != nil {
			log.Printf("github: failed to update final deployment status: %v", err)
		}
	}
}

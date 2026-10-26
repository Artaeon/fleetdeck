// Package edge connects a trusted reverse proxy to separate project networks.
// It does not grant untrusted Compose files safe access to a rootful daemon.
package edge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type Runner interface{ Run(string) (string, error) }
type LocalRunner struct{ Context context.Context }

func (r LocalRunner) Run(command string) (string, error) {
	ctx := r.Context
	if ctx == nil {
		ctx = context.Background()
	}
	out, err := exec.CommandContext(ctx, "sh", "-c", command).CombinedOutput()
	return string(out), err
}

var networkName = regexp.MustCompile(`^fleetdeck-[a-z0-9][a-z0-9-]{0,62}-edge$`)
var containerID = regexp.MustCompile(`^[a-f0-9]{64}$`)

// Prepare is a no-op for operator Compose files without FleetDeck's extension.
// All selected configuration remains trusted administrator input.
func Prepare(dir, composeFile string, runner Runner) error {
	paths := []string{composeFile}
	if composeFile == "" {
		paths = []string{"compose.yaml", "compose.yml", "docker-compose.yml", "docker-compose.yaml"}
	}
	for _, path := range paths {
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, path)
		}
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("reading edge configuration: %w", err)
		}
		if !bytes.Contains(data, []byte("x-fleetdeck:")) {
			return nil
		}
		var cfg struct {
			FleetDeck struct {
				Edge string `yaml:"edge-network"`
			} `yaml:"x-fleetdeck"`
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("invalid edge configuration: %w", err)
		}
		if cfg.FleetDeck.Edge == "" {
			return nil
		}
		return Ensure(runner, cfg.FleetDeck.Edge)
	}
	return nil
}

type network struct {
	Driver     string
	Labels     map[string]string
	Containers map[string]json.RawMessage
}

func inspect(runner Runner, name string) (*network, error) {
	out, err := runner.Run("docker network inspect '" + name + "'")
	if err != nil {
		return nil, fmt.Errorf("inspecting project network: %w", err)
	}
	var networks []network
	if err := json.Unmarshal([]byte(out), &networks); err != nil || len(networks) != 1 {
		return nil, fmt.Errorf("invalid network inspection response")
	}
	n := &networks[0]
	if n.Driver != "bridge" || n.Labels["io.fleetdeck.edge"] != "true" {
		return nil, fmt.Errorf("network %s is not a FleetDeck edge bridge", name)
	}
	return n, nil
}

// Ensure refuses ambiguous proxies and never adopts an unlabelled network.
func Ensure(runner Runner, name string) error {
	if !networkName.MatchString(name) {
		return fmt.Errorf("invalid project edge network name")
	}
	proxies, err := runner.Run("docker ps --no-trunc -q --filter label=io.fleetdeck.proxy=true")
	if err != nil {
		return fmt.Errorf("finding reverse proxy: %w", err)
	}
	ids := strings.Fields(proxies)
	if len(ids) != 1 || !containerID.MatchString(ids[0]) {
		return fmt.Errorf("expected one running FleetDeck proxy; run server setup or label the trusted Traefik service io.fleetdeck.proxy=true")
	}
	// List first so a daemon/permission failure cannot be mistaken for absence.
	existing, err := runner.Run("docker network ls --format '{{.Name}}'")
	if err != nil {
		return fmt.Errorf("listing networks: %w", err)
	}
	found := false
	for _, candidate := range strings.Fields(existing) {
		if candidate == name {
			found = true
		}
	}
	if !found {
		if _, err := runner.Run("docker network create --driver bridge --label io.fleetdeck.edge=true '" + name + "'"); err != nil {
			// A concurrent deployment may have created it. Verify ownership below.
			if _, checkErr := inspect(runner, name); checkErr != nil {
				return fmt.Errorf("creating edge network: %w", err)
			}
		}
	}
	n, err := inspect(runner, name)
	if err != nil {
		return err
	}
	if _, ok := n.Containers[ids[0]]; ok {
		return nil
	}
	if _, err := runner.Run("docker network connect '" + name + "' '" + ids[0] + "'"); err != nil {
		n, checkErr := inspect(runner, name)
		if checkErr == nil {
			if _, ok := n.Containers[ids[0]]; ok {
				return nil
			}
		}
		return fmt.Errorf("connecting reverse proxy to project network: %w", err)
	}
	return nil
}

// Reconnect restores attachments after FleetDeck recreates its reverse proxy.
func Reconnect(runner Runner) error {
	names, err := runner.Run("docker network ls --filter label=io.fleetdeck.edge=true --format '{{.Name}}'")
	if err != nil {
		return fmt.Errorf("listing project edge networks: %w", err)
	}
	for _, name := range strings.Fields(names) {
		if err := Ensure(runner, name); err != nil {
			return err
		}
	}
	return nil
}

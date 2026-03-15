package bootstrap

import "fmt"

// CommandRunner abstracts command execution so bootstrap steps work over SSH
// or locally.
type CommandRunner interface {
	Run(cmd string) (string, error)
}

// Config holds the parameters needed to bootstrap a fresh server.
type Config struct {
	Host           string
	Port           int
	User           string
	PrivateKey     string
	Domain         string
	Email          string // ACME / Let's Encrypt contact
	SwapSizeGB     int
	TraefikNetwork string
}

// Result tracks what the bootstrap process accomplished.
type Result struct {
	DockerInstalled    bool
	TraefikConfigured  bool
	FirewallConfigured bool
	SwapCreated        bool
	Errors             []string
}

// Bootstrap provisions a fresh server by running each step in order. Every
// step is idempotent and safe to re-run.
func Bootstrap(cfg Config, runner CommandRunner) (*Result, error) {
	res := &Result{}

	// 1. System preparation
	if err := prepareSystem(runner); err != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("prepare system: %v", err))
		return res, fmt.Errorf("preparing system: %w", err)
	}

	if err := setTimezone(runner); err != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("set timezone: %v", err))
		// Non-fatal, continue.
	}

	if err := configureSSH(runner); err != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("configure ssh: %v", err))
		// Non-fatal, continue.
	}

	// 2. Swap
	if cfg.SwapSizeGB > 0 {
		if err := configureSwap(runner, cfg.SwapSizeGB); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("configure swap: %v", err))
		} else {
			res.SwapCreated = true
		}
	}

	// 3. Docker
	if err := installDocker(runner); err != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("install docker: %v", err))
		return res, fmt.Errorf("installing docker: %w", err)
	}
	if err := verifyDocker(runner); err != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("verify docker: %v", err))
		return res, fmt.Errorf("verifying docker: %w", err)
	}
	res.DockerInstalled = true

	// 4. Firewall
	if err := configureFirewall(runner); err != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("configure firewall: %v", err))
	} else {
		res.FirewallConfigured = true
	}

	// 5. Traefik
	network := cfg.TraefikNetwork
	if network == "" {
		network = "traefik"
	}
	if err := setupTraefik(runner, cfg.Domain, cfg.Email, network); err != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("setup traefik: %v", err))
		return res, fmt.Errorf("setting up traefik: %w", err)
	}
	res.TraefikConfigured = true

	return res, nil
}

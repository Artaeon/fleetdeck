//go:build integration

// Run only on a disposable systemd Docker host. Installs the real guard twice.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/fleetdeck/fleetdeck/internal/bootstrap"
)

type runner struct{}

func (runner) Run(command string) (string, error) {
	out, err := exec.Command("sh", "-c", command).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%w: %s", err, out)
	}
	return string(out), nil
}
func dockerPID() string {
	out, err := exec.Command("systemctl", "show", "--property=MainPID", "--value", "docker.service").Output()
	if err != nil || strings.TrimSpace(string(out)) == "0" {
		panic("Docker must already be running")
	}
	return strings.TrimSpace(string(out))
}
func main() {
	if os.Geteuid() != 0 || os.Getenv("FLEETDECK_INTEGRATION") != "true" {
		panic("requires root and FLEETDECK_INTEGRATION=true on a disposable Docker host")
	}
	pid := dockerPID()
	for i := 0; i < 2; i++ {
		if err := bootstrap.ConfigureDockerFirewall(runner{}); err != nil {
			panic(err)
		}
		if after := dockerPID(); after != pid {
			panic("firewall installation restarted Docker")
		}
	}
	fmt.Println("Firewall installation and reload preserve the running Docker daemon.")
}

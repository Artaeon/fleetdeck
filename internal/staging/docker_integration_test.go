package staging

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// Run with FLEETDECK_STAGING_DOCKER_TEST=1 and pre-pulled official test images.
// Uses a disposable network/proxy; it does not touch any existing projects.
func TestDockerPreviewGate(t *testing.T) {
	if os.Getenv("FLEETDECK_STAGING_DOCKER_TEST") != "1" {
		t.Skip("opt-in Docker integration")
	}
	docker := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("docker", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("Docker fixture step failed: %v", err)
		}
		return strings.TrimSpace(string(out))
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	o := options()
	o.Name = "fd-gate-" + suffix
	o.Image = "docker.io/library/node@sha256:d32cdf619f63fe0471182d08996dd516c6275bb5fd31ae06e55a570bd9e1ad43"
	dir := filepath.Join(t.TempDir(), "preview")
	password := []byte("synthetic-preview-password-only")
	if err := Create(dir, o, password); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "compose.yaml")
	data, _ := os.ReadFile(file)
	var compose map[string]any
	if err := yaml.Unmarshal(data, &compose); err != nil {
		t.Fatal(err)
	}
	web := compose["services"].(map[string]any)["web"].(map[string]any)
	// Application fixture, no production source or data. Use the default local
	// certificate rather than requesting an ACME certificate for a test domain.
	web["command"] = []string{"node", "-e", `require('http').createServer((q,s)=>{s.end(q.headers.authorization?'leaked-auth':'gated-nonroot-'+process.getuid())}).listen(3000,'0.0.0.0')`}
	delete(web["labels"].(map[string]any), "traefik.http.routers."+o.Name+"-web.tls.certresolver")
	data, _ = yaml.Marshal(compose)
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	// Missing credentials must stop before a container is created.
	private := filepath.Join(dir, ".env")
	saved := filepath.Join(dir, "saved.env")
	if err := os.Rename(private, saved); err != nil {
		t.Fatal(err)
	}
	missing := exec.Command("docker", "compose", "-f", file, "config", "--quiet")
	missing.Dir = dir
	if missing.Run() == nil {
		t.Fatal("missing credential accepted")
	}
	if err := os.Rename(saved, private); err != nil {
		t.Fatal(err)
	}
	edge := "fleetdeck-" + o.Name + "-edge"
	docker("network", "create", "--label", "io.fleetdeck.edge=true", edge)
	t.Cleanup(func() { exec.Command("docker", "network", "rm", edge).Run() })
	proxy := o.Name + "-proxy"
	docker("run", "-d", "--name", proxy, "--network", edge, "-p", "127.0.0.1::443", "-v", "/var/run/docker.sock:/var/run/docker.sock:ro", "traefik@sha256:1336e85aaed293bd63d005086e0dd013b04078b26aaeb1c18b26a2f928af7e65", "--providers.docker=true", "--providers.docker.exposedbydefault=false", "--entrypoints.websecure.address=:443")
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", proxy).Run() })
	docker("compose", "-f", file, "up", "-d", "--pull", "never")
	t.Cleanup(func() { exec.Command("docker", "compose", "-f", file, "down", "--volumes").Run() })
	endpoint := "https://" + docker("port", proxy, "443/tcp") + "/api/private"
	client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}} // disposable local certificate
	probe := func(user, pass string) int {
		request, _ := http.NewRequest("GET", endpoint, nil)
		request.Host = o.Domain
		if user != "" {
			request.SetBasicAuth(user, pass)
		}
		response, err := client.Do(request)
		if err != nil {
			return 0
		}
		defer response.Body.Close()
		if response.StatusCode == 200 {
			b := make([]byte, 128)
			n, _ := response.Body.Read(b)
			if string(b[:n]) != "gated-nonroot-10001" {
				t.Fatalf("gate forwarded credentials or application ran as root")
			}
		}
		return response.StatusCode
	}
	deadline := time.Now().Add(30 * time.Second)
	for probe("", "") != 401 {
		if time.Now().After(deadline) {
			t.Fatal("preview gate did not become ready")
		}
		time.Sleep(time.Second)
	}
	if got := probe(o.Username, "wrong"); got != 401 {
		t.Fatalf("wrong password status=%d", got)
	}
	if got := probe(o.Username, string(password)); got != 200 {
		t.Fatalf("valid password status=%d", got)
	}
}

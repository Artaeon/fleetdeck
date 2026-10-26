package profiles

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestSaaSMailboxHasNoPublicExposure(t *testing.T) {
	p, err := Get("saas")
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := Render(p.Compose, ProfileData{Name: "example", Domain: "example.com", Port: 3000, PostgresVersion: "16"})
	if err != nil {
		t.Fatal(err)
	}
	var stack struct {
		Services map[string]struct {
			Ports    []string `yaml:"ports"`
			Networks []string `yaml:"networks"`
			Labels   []string `yaml:"labels"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(rendered), &stack); err != nil {
		t.Fatal(err)
	}
	mailbox, exists := stack.Services["mailpit"]
	if !exists {
		t.Fatal("private mailbox missing")
	}
	if len(mailbox.Ports) != 0 {
		t.Fatal("mailbox publishes a host port")
	}
	if len(mailbox.Networks) != 1 || mailbox.Networks[0] != "example-internal" {
		t.Fatal("mailbox joins a public/shared network")
	}
	if len(mailbox.Labels) != 1 || mailbox.Labels[0] != "traefik.enable=false" {
		t.Fatal("mailbox must explicitly disable proxy exposure")
	}
}

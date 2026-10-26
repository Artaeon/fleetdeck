package edge_test

import (
	"github.com/fleetdeck/fleetdeck/internal/generate"
	"github.com/fleetdeck/fleetdeck/internal/profiles"
	"github.com/fleetdeck/fleetdeck/internal/templates"
	"gopkg.in/yaml.v3"
	"slices"
	"testing"
)

func TestGeneratedServicesRestrictRuntimePrivileges(t *testing.T) {
	var stacks []string
	for _, profile := range profiles.List() {
		data := profiles.ProfileData{Name: "fixture", Domain: "example.com", Port: 3000, PostgresVersion: "17"}
		profile.ApplyDefaults(&data)
		out, err := profiles.Render(profile.Compose, data)
		if err != nil {
			t.Fatal(err)
		}
		stacks = append(stacks, out)
	}
	for _, template := range templates.List() {
		out, err := templates.Render(template.Compose, templates.TemplateData{Name: "fixture", Domain: "example.com", PostgresVersion: "17"})
		if err != nil {
			t.Fatal(err)
		}
		stacks = append(stacks, out)
	}
	stacks = append(stacks, generate.Compose(generate.ComposeOptions{ProjectName: "fixture", Domain: "example.com", Port: 3000}))
	for _, stack := range stacks {
		var cfg struct {
			Services map[string]struct {
				SecurityOpt []string `yaml:"security_opt"`
				CapDrop     []string `yaml:"cap_drop"`
				Pids        int      `yaml:"pids_limit"`
			}
		}
		if err := yaml.Unmarshal([]byte(stack), &cfg); err != nil {
			t.Fatal(err)
		}
		for name, service := range cfg.Services {
			if !slices.Contains(service.SecurityOpt, "no-new-privileges:true") || !slices.Contains(service.CapDrop, "NET_RAW") || service.Pids != 512 {
				t.Errorf("service %s lacks runtime protections", name)
			}
		}
	}
}

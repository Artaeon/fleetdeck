package edge_test

import (
	"strings"
	"testing"

	"github.com/fleetdeck/fleetdeck/internal/generate"
	"github.com/fleetdeck/fleetdeck/internal/profiles"
	"github.com/fleetdeck/fleetdeck/internal/templates"
	"gopkg.in/yaml.v3"
)

func checkEdge(t *testing.T, content string) {
	t.Helper()
	var config struct {
		Edge struct {
			Network string `yaml:"edge-network"`
		} `yaml:"x-fleetdeck"`
		Networks map[string]struct {
			Name     string
			External bool
			Internal bool
		}
		Services map[string]struct {
			Networks []string
			Labels   []string
		}
	}
	if err := yaml.Unmarshal([]byte(content), &config); err != nil {
		t.Fatal(err)
	}
	want := "fleetdeck-fixture-edge"
	if config.Edge.Network != want || config.Networks["default"].Name != want || !config.Networks["default"].External {
		t.Fatal("missing separate managed edge", content)
	}
	if strings.Contains(content, "traefik_default") {
		t.Fatal("shared project network remains")
	}
	for name, network := range config.Networks {
		if name != "default" && !network.Internal {
			t.Errorf("backend network %s allows external traffic", name)
		}
	}
	for name, service := range config.Services {
		routed := false
		for _, label := range service.Labels {
			if label == "traefik.enable=true" {
				routed = true
			}
		}
		if routed {
			if !strings.Contains(strings.Join(service.Labels, "\n"), "traefik.docker.network="+want) {
				t.Errorf("%s proxy may select wrong network", name)
			}
		}
		if name == "postgres" || name == "redis" || name == "mailpit" {
			for _, network := range service.Networks {
				if network == "default" {
					t.Errorf("%s exposed to proxy network", name)
				}
			}
		}
	}
}
func TestAllGeneratedWebProjectsUseSeparateEdges(t *testing.T) {
	for _, profile := range profiles.List() {
		if profile.Name == "worker" {
			continue
		}
		t.Run("profile/"+profile.Name, func(t *testing.T) {
			data := profiles.ProfileData{Name: "fixture", Domain: "example.com", Port: 3000, PostgresVersion: "17"}
			profile.ApplyDefaults(&data)
			out, err := profiles.Render(profile.Compose, data)
			if err != nil {
				t.Fatal(err)
			}
			checkEdge(t, out)
		})
	}
	for _, template := range templates.List() {
		t.Run("template/"+template.Name, func(t *testing.T) {
			out, err := templates.Render(template.Compose, templates.TemplateData{Name: "fixture", Domain: "example.com", PostgresVersion: "17"})
			if err != nil {
				t.Fatal(err)
			}
			checkEdge(t, out)
		})
	}
	checkEdge(t, generate.Compose(generate.ComposeOptions{ProjectName: "fixture", Domain: "example.com", Port: 3000}))
}

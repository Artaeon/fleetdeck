package profiles

import (
	"bytes"
	"fmt"
	"text/template"
)

// Profile defines a deployment profile with composable services.
type Profile struct {
	Name        string
	Description string
	Services    []Service
	Compose     string
	EnvTemplate string
	Nginx       string // Optional nginx config for static/fullstack
}

// Service represents a single service within a profile.
type Service struct {
	Name        string
	Image       string
	Description string
	Required    bool
}

// ProfileData holds variables for rendering profile templates.
type ProfileData struct {
	Name            string
	Domain          string
	Port            int
	PostgresVersion string
	RedisVersion    string
	AppType         string
	Framework       string
	CPULimit        string // e.g. "1.0" (cores)
	MemoryLimit     string // e.g. "512M"
}

var registry = map[string]*Profile{}

// Register adds a profile to the registry.
func Register(p *Profile) {
	registry[p.Name] = p
}

// Get returns a profile by name.
func Get(name string) (*Profile, error) {
	p, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("profile %q not found (available: bare, server, saas, static, worker, fullstack)", name)
	}
	return p, nil
}

// List returns all registered profiles.
func List() []*Profile {
	order := []string{"bare", "server", "saas", "static", "worker", "fullstack"}
	var result []*Profile
	for _, name := range order {
		if p, ok := registry[name]; ok {
			result = append(result, p)
		}
	}
	// Append any profiles not in the predefined order
	for name, p := range registry {
		found := false
		for _, o := range order {
			if name == o {
				found = true
				break
			}
		}
		if !found {
			result = append(result, p)
		}
	}
	return result
}

// Render renders a profile's compose template with the given data.
func Render(tmpl string, data ProfileData) (string, error) {
	t, err := template.New("").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parsing profile template: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("rendering profile template: %w", err)
	}
	return buf.String(), nil
}

// RenderEnv renders a profile's environment template.
func RenderEnv(tmpl string, data ProfileData) (string, error) {
	return Render(tmpl, data)
}

// ServiceNames returns the list of service names for a profile.
func (p *Profile) ServiceNames() []string {
	var names []string
	for _, s := range p.Services {
		names = append(names, s.Name)
	}
	return names
}

// HasService checks if a profile includes a specific service.
func (p *Profile) HasService(name string) bool {
	for _, s := range p.Services {
		if s.Name == name {
			return true
		}
	}
	return false
}

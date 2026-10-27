package releasejob

import (
	"fmt"
	"strings"

	"github.com/fleetdeck/fleetdeck/internal/config"
)

// ReceiverPolicy checks the fixed operator-selected target before opening the
// registry or invoking an executor. It deliberately supports staging only.
func ReceiverPolicy(cfg *config.Config, target string) (config.ReleaseTargetConfig, error) {
	if cfg == nil || !cfg.Release.Enabled || !identifierPattern.MatchString(target) {
		return config.ReleaseTargetConfig{}, fmt.Errorf("CI release receiver is not configured")
	}
	p, ok := cfg.Release.Targets[target]
	if !ok || p.Environment != "staging" || len(p.ImageRepositories) == 0 || len(p.ImageRepositories) > 32 {
		return p, fmt.Errorf("CI receiver requires a staging target with an image repository allowlist")
	}
	for component, repository := range p.ImageRepositories {
		if !identifierPattern.MatchString(component) || !imagePattern.MatchString(repository+"@sha256:"+strings.Repeat("0", 64)) {
			return p, fmt.Errorf("invalid CI component repository allowlist")
		}
	}
	return p, nil
}

func ValidateReceived(request Request, target string, policy config.ReleaseTargetConfig) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if request.Target.ID != target || request.Target.Environment != "staging" ||
		request.Target.Profile != policy.Profile || request.HealthProfile != policy.HealthProfile {
		return fmt.Errorf("request does not match the fixed CI target")
	}
	for _, image := range request.Images {
		repository, ok := policy.ImageRepositories[image.Component]
		if !ok || strings.Split(image.Reference, "@")[0] != repository {
			return fmt.Errorf("image component or repository is not allowed for this CI identity")
		}
	}
	return nil
}

package releasejob

import (
	"github.com/fleetdeck/fleetdeck/internal/config"
	"strings"
	"testing"
)

func TestReceiverEnforcesTargetAndExactImageRepository(t *testing.T) {
	request, err := Decode(strings.NewReader(validJob))
	if err != nil {
		t.Fatal(err)
	}
	policy := config.ReleaseTargetConfig{Environment: "staging", Profile: "standard", HealthProfile: "http-standard", ImageRepositories: map[string]string{"api": "registry.example.com/example/api"}}
	cfg := &config.Config{Release: config.ReleaseConfig{Enabled: true, Targets: map[string]config.ReleaseTargetConfig{"target-example": policy}}}
	if _, err := ReceiverPolicy(cfg, "target-example"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateReceived(request, "target-example", policy); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*Request){
		func(r *Request) { r.Target.ID = "another-target" },
		func(r *Request) { configureProductionRequest(r) },
		func(r *Request) { r.Images[0].Component = "database" },
		func(r *Request) {
			r.Images[0].Reference = strings.Replace(r.Images[0].Reference, "/api@", "/api-malicious@", 1)
		},
		func(r *Request) { r.HealthProfile = "other" },
		func(r *Request) { r.Target.Profile = "other" },
	} {
		r, _ := Decode(strings.NewReader(validJob))
		change(&r)
		if err := ValidateReceived(r, "target-example", policy); err == nil {
			t.Fatal("unauthorized request accepted")
		}
	}
	for _, repositories := range []map[string]string{nil, {}, {"api": "registry.example.com/example/api:latest"}, {"api": "registry.example.com/example/api@sha256:" + strings.Repeat("0", 64)}} {
		p := policy
		p.ImageRepositories = repositories
		cfg.Release.Targets["target-example"] = p
		if _, err := ReceiverPolicy(cfg, "target-example"); err == nil {
			t.Fatal("invalid policy accepted")
		}
	}
	policy.Environment = "production"
	cfg.Release.Targets["target-example"] = policy
	if _, err := ReceiverPolicy(cfg, "target-example"); err == nil {
		t.Fatal("production CI accepted")
	}
}

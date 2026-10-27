package cmd

import (
	"bytes"
	"context"
	"github.com/fleetdeck/fleetdeck/internal/config"
	"strings"
	"testing"
)

func TestReceiveRefusesRemoteCommandsBeforeOpeningDatabase(t *testing.T) {
	releaseReceiveCmd.SetContext(context.Background())
	old := cfg
	defer func() { cfg = old }()
	cfg = &config.Config{Release: config.ReleaseConfig{Enabled: true, Targets: map[string]config.ReleaseTargetConfig{"target-example": {Environment: "staging", Profile: "standard", HealthProfile: "http-standard", ImageRepositories: map[string]string{"api": "registry.example.com/example/api"}}}}}
	releaseReceiveCmd.Flags().Set("target", "target-example")
	defer releaseReceiveCmd.Flags().Set("target", "")
	for _, original := range []string{"", "sh", "apply; id", "status --target production", "scp -t file", "sftp", "apply\nstatus"} {
		t.Setenv("SSH_ORIGINAL_COMMAND", original)
		if err := releaseReceiveCmd.RunE(releaseReceiveCmd, nil); err == nil {
			t.Fatalf("accepted %q", original)
		}
	}
	t.Setenv("SSH_ORIGINAL_COMMAND", "apply")
	releaseReceiveCmd.SetIn(strings.NewReader(strings.Replace(validReleaseJob, `"target-example"`, `"other-target"`, 1)))
	releaseReceiveCmd.SetOut(&bytes.Buffer{})
	defer releaseReceiveCmd.SetIn(nil)
	defer releaseReceiveCmd.SetOut(nil)
	if err := releaseReceiveCmd.RunE(releaseReceiveCmd, nil); err == nil {
		t.Fatal("accepted another target")
	}
}

package sourcedeploy

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/fleetdeck/fleetdeck/internal/backup"
)

func TestQuiesceKeepsExplicitDatabaseRunningForLogicalDump(t *testing.T) {
	var m model
	if err := json.Unmarshal([]byte(`{"services":{"db":{"image":"sha256:fixture","labels":{"io.fleetdeck.backup.database":"postgres"},"volumes":[{"type":"volume","source":"data"}]},"app":{"image":"example/app:1"}}}`), &m); err != nil {
		t.Fatal(err)
	}
	stopped := false
	e := &Engine{Runner: runnerFunc(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if contains(args, "stop") {
			if !strings.HasSuffix(strings.Join(args, " "), "stop --timeout 30 app") {
				t.Fatalf("stopped database before logical dump: %v", args)
			}
			stopped = true
			return nil, nil
		}
		if stopped {
			return []byte("db\n"), nil
		}
		return []byte("db\napp\n"), nil
	})}
	result, err := e.quiesce(context.Background(), t.TempDir(), "demo", "compose.yaml", m)
	if err != nil || len(result) != 1 || result[0] != "app" {
		t.Fatal(result, err)
	}
	db := m.Services["db"]
	db.Labels[backup.DatabaseLabel] = "unsupported"
	m.Services["db"] = db
	if _, err = e.quiesce(context.Background(), t.TempDir(), "demo", "compose.yaml", m); err == nil {
		t.Fatal("invalid database engine accepted")
	}
}

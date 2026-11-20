package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/fleet"
)

func TestExportOnlyTwoReportCheckpoints(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	for _, edition := range []string{"daily", "weekly"} {
		data, err := json.Marshal(map[string]any{
			"version": 3, "evaluated_at": now, "edition": edition, "schedule": "private-schedule",
			"labels": "private-operator-label", "sent": map[string]time.Time{"telegram": now},
			"pending": map[string]any{},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, edition), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	run := func(daily, weekly string) ([]byte, error) {
		var out bytes.Buffer
		cmd := newExportReportStatusCmd()
		cmd.SetOut(&out)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SilenceUsage = true
		cmd.SetArgs([]string{"--daily-state", filepath.Join(dir, daily), "--weekly-state", filepath.Join(dir, weekly)})
		err := cmd.Execute()
		return out.Bytes(), err
	}
	body, err := run("daily", "weekly")
	if err != nil {
		t.Fatal(err)
	}
	got, err := fleet.DecodeReportRuns(bytes.NewReader(body), now)
	want := fleet.ReportRun{LastEvaluation: now, Pending: 0}
	if err != nil || !reflect.DeepEqual(got, fleet.ReportRuns{Daily: want, Weekly: want}) {
		t.Fatalf("projection: %v %v", got, err)
	}
	if _, err := run("weekly", "daily"); err == nil {
		t.Fatal("accepted swapped checkpoint editions")
	}
}

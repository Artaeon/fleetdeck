package monitor

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMonitoringTargetValidation(t *testing.T) {
	file := filepath.Join(t.TempDir(), "targets.json")
	cases := []struct {
		body  string
		valid bool
	}{
		{`[{"name":"private-api","url":"http://127.0.0.1:8080/health"},{"name":"preview-gate","url":"https://preview.example.com","expected_status":401}]`, true},
		{`[{"name":"secret","url":"https://user:secret@example.com"}]`, false},
		{`[{"name":"secret","url":"https://example.com?key=secret"}]`, false},
		{`[{"name":"duplicate","url":"https://example.com"},{"name":"duplicate","url":"https://example.com"}]`, false},
		{`[{"name":"bad","url":"file:///etc/passwd"}]`, false}, {`[]`, false},
	}
	for _, tc := range cases {
		os.WriteFile(file, []byte(tc.body), 0600)
		targets, err := LoadTargets(file, time.Minute, 5*time.Second)
		if (err == nil) != tc.valid {
			t.Fatalf("validation: %v", err)
		}
		if tc.valid && (len(targets) != 2 || targets[1].ExpectedStatus != 401) {
			t.Fatal("lost target policy")
		}
	}
}

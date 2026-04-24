package remote

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemotePath(t *testing.T) {
	cases := []struct {
		target string
		id     string
		want   string
	}{
		{"b2:bucket", "abc123", "b2:bucket/abc123"},
		{"b2:bucket/", "abc123", "b2:bucket/abc123"},
		{"r2:", "abc123", "r2:abc123"},
		{"r2:backups/prod", "xyz", "r2:backups/prod/xyz"},
	}
	for _, c := range cases {
		r := NewRclone(c.target)
		got := r.remotePath(c.id)
		if got != c.want {
			t.Errorf("remotePath(%q, %q) = %q, want %q", c.target, c.id, got, c.want)
		}
	}
}

// TestListParsesRcloneLsjson drives List() against a fake rclone binary
// that emits a single JSON array — the real rclone output format — and
// confirms we return the directory names of every IsDir entry.
func TestListParsesRcloneLsjson(t *testing.T) {
	fakeRclone := writeFakeRclone(t, map[string]fakeResponse{
		"lsjson": {
			stdout: mustMarshalJSON(t, []map[string]any{
				{"Path": "abc123", "Name": "abc123", "Size": -1, "IsDir": true},
				{"Path": "manifest.json", "Name": "manifest.json", "Size": 120, "IsDir": false},
				{"Path": "def456", "Name": "def456", "Size": -1, "IsDir": true},
			}),
		},
	})

	r := &Rclone{Target: "b2:bucket", Binary: fakeRclone}
	ids, err := r.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if strings.Join(ids, ",") != "abc123,def456" {
		t.Errorf("expected [abc123 def456], got %v", ids)
	}
}

// TestListHandlesMissingTarget verifies that an "empty remote" — which
// rclone reports as a non-zero exit with "directory not found" in stderr —
// is treated as an empty listing rather than an error.
func TestListHandlesMissingTarget(t *testing.T) {
	fakeRclone := writeFakeRclone(t, map[string]fakeResponse{
		"lsjson": {stderr: "directory not found", exitCode: 3},
	})

	r := &Rclone{Target: "b2:bucket", Binary: fakeRclone}
	ids, err := r.List(context.Background())
	if err != nil {
		t.Fatalf("List should swallow 'not found', got: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected empty list, got %v", ids)
	}
}

// TestDeleteIsIdempotent verifies that purging an already-missing remote
// directory returns nil, so retention enforcement can call Delete
// repeatedly without flooding logs with spurious errors.
func TestDeleteIsIdempotent(t *testing.T) {
	fakeRclone := writeFakeRclone(t, map[string]fakeResponse{
		"purge": {stderr: "directory not found", exitCode: 3},
	})

	r := &Rclone{Target: "b2:bucket", Binary: fakeRclone}
	if err := r.Delete(context.Background(), "abc123"); err != nil {
		t.Errorf("Delete should swallow 'not found', got: %v", err)
	}
}

// --- fake rclone scaffolding ---

type fakeResponse struct {
	stdout   string
	stderr   string
	exitCode int
}

// writeFakeRclone writes a shell script that dispatches on the first
// argument (the rclone subcommand). Returns the absolute path to the
// script, which can be slotted into Rclone.Binary.
func writeFakeRclone(t *testing.T, responses map[string]fakeResponse) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "rclone")

	var sb strings.Builder
	sb.WriteString("#!/bin/sh\n")
	sb.WriteString("case \"$1\" in\n")
	for sub, resp := range responses {
		sb.WriteString("  " + sub + ")\n")
		if resp.stdout != "" {
			sb.WriteString("    cat <<'FLEETDECK_EOF'\n")
			sb.WriteString(resp.stdout)
			sb.WriteString("\nFLEETDECK_EOF\n")
		}
		if resp.stderr != "" {
			sb.WriteString("    echo '" + resp.stderr + "' >&2\n")
		}
		exit := resp.exitCode
		sb.WriteString("    exit ")
		if exit == 0 {
			sb.WriteString("0")
		} else {
			sb.WriteString("3")
		}
		sb.WriteString("\n    ;;\n")
	}
	sb.WriteString("  *)\n    echo 'unexpected subcommand: '\"$1\" >&2\n    exit 2\n    ;;\nesac\n")

	if err := os.WriteFile(path, []byte(sb.String()), 0755); err != nil {
		t.Fatalf("writing fake rclone: %v", err)
	}
	return path
}

func mustMarshalJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

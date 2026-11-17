package cmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecurityReportDoesNotInitializeHostConfiguration(t *testing.T) {
	c := newSecurityReportCmd()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"--input", "node-a=" + filepath.Join(t.TempDir(), "missing.json"), "--language", "de"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "ungeklärte") || !strings.Contains(out.String(), "Nicht erfasst") {
		t.Fatal(out.String())
	}
}

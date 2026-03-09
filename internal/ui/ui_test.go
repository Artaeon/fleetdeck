package ui

import (
	"strings"
	"testing"
)

func TestBold(t *testing.T) {
	result := Bold("hello")
	if !strings.Contains(result, "hello") {
		t.Error("Bold should contain the original text")
	}
	if !strings.Contains(result, colorBold) {
		t.Error("Bold should contain bold escape code")
	}
	if !strings.Contains(result, colorReset) {
		t.Error("Bold should contain reset escape code")
	}
}

func TestStatusColor(t *testing.T) {
	tests := []struct {
		status   string
		contains string
	}{
		{"running", colorGreen},
		{"stopped", colorRed},
		{"error", colorRed},
		{"created", colorYellow},
		{"pending", colorYellow},
		{"deploying", colorYellow},
		{"unknown", "unknown"}, // no color wrapping
	}

	for _, tt := range tests {
		result := StatusColor(tt.status)
		if !strings.Contains(result, tt.contains) {
			t.Errorf("StatusColor(%q) should contain %q", tt.status, tt.contains)
		}
		if !strings.Contains(result, tt.status) {
			t.Errorf("StatusColor(%q) should contain the status text", tt.status)
		}
	}
}

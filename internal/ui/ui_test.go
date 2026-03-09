package ui

import (
	"bytes"
	"io"
	"os"
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

func TestBoldEmptyString(t *testing.T) {
	result := Bold("")
	if result != colorBold+colorReset {
		t.Errorf("Bold(\"\") = %q, expected %q", result, colorBold+colorReset)
	}
}

func TestBoldSpecialChars(t *testing.T) {
	input := "hello <world> & \"friends\""
	result := Bold(input)
	if !strings.Contains(result, input) {
		t.Errorf("Bold should preserve special characters, got %q", result)
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

func TestStatusColorUnknownReturnsPlain(t *testing.T) {
	result := StatusColor("something-else")
	if result != "something-else" {
		t.Errorf("expected plain string for unknown status, got %q", result)
	}
	// Should NOT contain any escape codes
	if strings.Contains(result, "\033[") {
		t.Error("unknown status should not contain ANSI escape codes")
	}
}

func TestStatusColorEmptyString(t *testing.T) {
	result := StatusColor("")
	if result != "" {
		t.Errorf("expected empty string for empty status, got %q", result)
	}
}

// captureStdout captures stdout output from a function call.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("creating pipe: %v", err)
	}
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

// captureStderr captures stderr output from a function call.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("creating pipe: %v", err)
	}
	os.Stderr = w

	fn()

	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestInfo(t *testing.T) {
	output := captureStdout(t, func() {
		Info("hello %s", "world")
	})
	if !strings.Contains(output, "hello world") {
		t.Errorf("Info output should contain formatted message, got %q", output)
	}
	if !strings.Contains(output, colorBlue) {
		t.Errorf("Info output should contain blue color code, got %q", output)
	}
	if !strings.HasSuffix(output, "\n") {
		t.Error("Info output should end with newline")
	}
}

func TestInfoNoArgs(t *testing.T) {
	output := captureStdout(t, func() {
		Info("simple message")
	})
	if !strings.Contains(output, "simple message") {
		t.Errorf("Info output should contain message, got %q", output)
	}
}

func TestWarn(t *testing.T) {
	output := captureStdout(t, func() {
		Warn("caution: %d items", 42)
	})
	if !strings.Contains(output, "caution: 42 items") {
		t.Errorf("Warn output should contain formatted message, got %q", output)
	}
	if !strings.Contains(output, colorYellow) {
		t.Errorf("Warn output should contain yellow color code, got %q", output)
	}
	if !strings.HasSuffix(output, "\n") {
		t.Error("Warn output should end with newline")
	}
}

func TestWarnNoArgs(t *testing.T) {
	output := captureStdout(t, func() {
		Warn("be careful")
	})
	if !strings.Contains(output, "be careful") {
		t.Errorf("Warn output should contain message, got %q", output)
	}
}

func TestError(t *testing.T) {
	output := captureStderr(t, func() {
		Error("failed: %s", "connection refused")
	})
	if !strings.Contains(output, "failed: connection refused") {
		t.Errorf("Error output should contain formatted message, got %q", output)
	}
	if !strings.Contains(output, colorRed) {
		t.Errorf("Error output should contain red color code, got %q", output)
	}
	if !strings.HasSuffix(output, "\n") {
		t.Error("Error output should end with newline")
	}
}

func TestErrorNoArgs(t *testing.T) {
	output := captureStderr(t, func() {
		Error("something went wrong")
	})
	if !strings.Contains(output, "something went wrong") {
		t.Errorf("Error output should contain message, got %q", output)
	}
}

func TestErrorWritesToStderr(t *testing.T) {
	// Verify Error does NOT write to stdout
	stdoutOutput := captureStdout(t, func() {
		// We need to also capture stderr to avoid it printing during test
		captureStderr(t, func() {
			Error("stderr only")
		})
	})
	if strings.Contains(stdoutOutput, "stderr only") {
		t.Error("Error should write to stderr, not stdout")
	}
}

func TestSuccess(t *testing.T) {
	output := captureStdout(t, func() {
		Success("deployed %s to %s", "v1.0", "production")
	})
	if !strings.Contains(output, "deployed v1.0 to production") {
		t.Errorf("Success output should contain formatted message, got %q", output)
	}
	if !strings.Contains(output, colorGreen) {
		t.Errorf("Success output should contain green color code, got %q", output)
	}
	if !strings.HasSuffix(output, "\n") {
		t.Error("Success output should end with newline")
	}
}

func TestSuccessNoArgs(t *testing.T) {
	output := captureStdout(t, func() {
		Success("done")
	})
	if !strings.Contains(output, "done") {
		t.Errorf("Success output should contain message, got %q", output)
	}
}

func TestStep(t *testing.T) {
	output := captureStdout(t, func() {
		Step(1, 5, "installing %s", "dependencies")
	})
	if !strings.Contains(output, "installing dependencies") {
		t.Errorf("Step output should contain formatted message, got %q", output)
	}
	if !strings.Contains(output, "[1/5]") {
		t.Errorf("Step output should contain step counter, got %q", output)
	}
	if !strings.Contains(output, colorCyan) {
		t.Errorf("Step output should contain cyan color code, got %q", output)
	}
	if !strings.HasSuffix(output, "\n") {
		t.Error("Step output should end with newline")
	}
}

func TestStepNoArgs(t *testing.T) {
	output := captureStdout(t, func() {
		Step(3, 3, "final step")
	})
	if !strings.Contains(output, "[3/3]") {
		t.Errorf("Step output should contain step counter, got %q", output)
	}
	if !strings.Contains(output, "final step") {
		t.Errorf("Step output should contain message, got %q", output)
	}
}

func TestStepVariousNumbers(t *testing.T) {
	output := captureStdout(t, func() {
		Step(10, 100, "processing")
	})
	if !strings.Contains(output, "[10/100]") {
		t.Errorf("Step output should contain correct step numbers, got %q", output)
	}
}

func TestTable(t *testing.T) {
	output := captureStdout(t, func() {
		Table(
			[]string{"NAME", "STATUS", "URL"},
			[][]string{
				{"myapp", "running", "https://myapp.com"},
				{"api", "stopped", "https://api.com"},
			},
		)
	})

	if !strings.Contains(output, "NAME") {
		t.Error("Table output should contain headers")
	}
	if !strings.Contains(output, "STATUS") {
		t.Error("Table output should contain headers")
	}
	if !strings.Contains(output, "myapp") {
		t.Error("Table output should contain row data")
	}
	if !strings.Contains(output, "api") {
		t.Error("Table output should contain row data")
	}
	if !strings.Contains(output, colorBold) {
		t.Error("Table headers should be bold")
	}
}

func TestTableEmptyRows(t *testing.T) {
	output := captureStdout(t, func() {
		Table(
			[]string{"HEADER1", "HEADER2"},
			[][]string{},
		)
	})

	if !strings.Contains(output, "HEADER1") {
		t.Error("Table should still print headers with no rows")
	}
}

func TestTableSingleRow(t *testing.T) {
	output := captureStdout(t, func() {
		Table(
			[]string{"COL"},
			[][]string{{"value"}},
		)
	})

	if !strings.Contains(output, "COL") {
		t.Error("Table should contain header")
	}
	if !strings.Contains(output, "value") {
		t.Error("Table should contain row value")
	}
}

func TestTableManyColumns(t *testing.T) {
	output := captureStdout(t, func() {
		Table(
			[]string{"A", "B", "C", "D", "E"},
			[][]string{
				{"1", "2", "3", "4", "5"},
			},
		)
	})

	for _, h := range []string{"A", "B", "C", "D", "E"} {
		if !strings.Contains(output, h) {
			t.Errorf("Table should contain header %q", h)
		}
	}
	for _, v := range []string{"1", "2", "3", "4", "5"} {
		if !strings.Contains(output, v) {
			t.Errorf("Table should contain value %q", v)
		}
	}
}

func TestInfoContainsArrowPrefix(t *testing.T) {
	output := captureStdout(t, func() {
		Info("test")
	})
	// The arrow character is the prefix
	if !strings.Contains(output, "→") {
		t.Errorf("Info should have arrow prefix, got %q", output)
	}
}

func TestWarnContainsExclamationPrefix(t *testing.T) {
	output := captureStdout(t, func() {
		Warn("test")
	})
	if !strings.Contains(output, "!") {
		t.Errorf("Warn should have exclamation prefix, got %q", output)
	}
}

func TestSuccessContainsCheckmark(t *testing.T) {
	output := captureStdout(t, func() {
		Success("test")
	})
	if !strings.Contains(output, "✓") {
		t.Errorf("Success should have checkmark prefix, got %q", output)
	}
}

func TestErrorContainsCross(t *testing.T) {
	output := captureStderr(t, func() {
		Error("test")
	})
	if !strings.Contains(output, "✗") {
		t.Errorf("Error should have cross prefix, got %q", output)
	}
}

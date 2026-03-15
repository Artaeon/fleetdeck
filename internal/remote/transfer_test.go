package remote

import (
	"strings"
	"testing"
)

// simulateShellUnquoteForTransfer reverses the shellQuote transformation.
// shellQuote produces: '<content with ' replaced by '"'"'>'
// To reverse: strip outer quotes, then replace '"'"' with '.
func simulateShellUnquoteForTransfer(quoted string) string {
	if len(quoted) < 2 || quoted[0] != '\'' || quoted[len(quoted)-1] != '\'' {
		return quoted
	}
	inner := quoted[1 : len(quoted)-1]
	return strings.ReplaceAll(inner, "'\"'\"'", "'")
}

func TestShellQuoteRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "simple word", input: "hello"},
		{name: "with spaces", input: "hello world"},
		{name: "with single quote", input: "it's"},
		{name: "with double quotes", input: `say "hello"`},
		{name: "mixed quotes", input: `it's "complex"`},
		{name: "consecutive single quotes", input: "'''"},
		{name: "empty string", input: ""},
		{name: "only spaces", input: "   "},
		{name: "tab character", input: "a\tb"},
		{name: "backslash", input: `back\slash`},
		{name: "dollar sign", input: "$HOME"},
		{name: "backtick", input: "`cmd`"},
		{name: "semicolon", input: "a;b"},
		{name: "pipe", input: "a|b"},
		{name: "ampersand", input: "a&b"},
		{name: "parentheses", input: "(sub)"},
		{name: "braces", input: "{a,b}"},
		{name: "angle brackets", input: "<in>out>"},
		{name: "exclamation", input: "!bang"},
		{name: "hash", input: "#comment"},
		{name: "asterisk", input: "*.txt"},
		{name: "question mark", input: "file?.log"},
		{name: "square brackets", input: "[abc]"},
		{name: "tilde", input: "~root"},
		{name: "equals sign", input: "key=value"},
		{name: "percent", input: "100%"},
		{name: "at sign", input: "user@host"},
		{name: "caret", input: "^start"},
		{name: "plus", input: "a+b"},
		{name: "comma", input: "a,b,c"},
		{name: "colon", input: "host:port"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quoted := shellQuote(tt.input)
			unquoted := simulateShellUnquoteForTransfer(quoted)
			if unquoted != tt.input {
				t.Errorf("round-trip failed: input=%q, quoted=%q, unquoted=%q", tt.input, quoted, unquoted)
			}
		})
	}
}

func TestShellQuoteWithFilePaths(t *testing.T) {
	paths := []struct {
		name string
		path string
	}{
		{name: "standard path", path: "/opt/fleetdeck/my-app"},
		{name: "path with spaces", path: "/home/user/project with spaces"},
		{name: "path with single quote", path: "/tmp/test'file"},
		{name: "node_modules cache", path: "/opt/app/node_modules/.cache"},
		{name: "hidden directory", path: "/home/user/.config/fleetdeck/config.toml"},
		{name: "deeply nested", path: "/srv/data/v2/projects/my-app/deployments/2024/01"},
		{name: "hyphenated segments", path: "/opt/fleet-deck/my-web-app/docker-compose.yml"},
		{name: "dotfiles", path: "/home/user/.ssh/authorized_keys"},
		{name: "path with parens", path: "/tmp/build (copy)/output"},
		{name: "path with at sign", path: "/home/deploy@prod/app"},
		{name: "path with hash", path: "/tmp/build#123/output"},
		{name: "relative-looking path", path: "../../../etc/passwd"},
		{name: "path with double dots", path: "/opt/app/../other/file"},
		{name: "Windows-style path", path: "C:\\Users\\admin\\project"},
	}

	for _, tt := range paths {
		t.Run(tt.name, func(t *testing.T) {
			quoted := shellQuote(tt.path)

			// Must be properly quoted.
			if !strings.HasPrefix(quoted, "'") || !strings.HasSuffix(quoted, "'") {
				t.Errorf("shellQuote(%q) not properly quoted: %s", tt.path, quoted)
			}

			// Round-trip must preserve the path.
			unquoted := simulateShellUnquoteForTransfer(quoted)
			if unquoted != tt.path {
				t.Errorf("round-trip failed for path %q: got %q", tt.path, unquoted)
			}
		})
	}
}

func TestShellQuoteWithURLs(t *testing.T) {
	urls := []struct {
		name string
		url  string
	}{
		{name: "simple URL", url: "https://example.com"},
		{name: "URL with query params", url: "https://example.com?foo=bar&baz=qux"},
		{name: "URL with fragment", url: "https://example.com/page#section"},
		{name: "URL with port", url: "https://example.com:8080/path"},
		{name: "URL with auth", url: "https://user:pass@example.com/path"},
		{name: "URL with special chars", url: "https://example.com/path?q=hello+world&lang=en"},
		{name: "URL with percent encoding", url: "https://example.com/path%20with%20spaces"},
		{name: "URL with brackets", url: "https://example.com/api/v1/users[0]"},
		{name: "Git SSH URL", url: "git@github.com:org/repo.git"},
		{name: "Docker registry URL", url: "registry.example.com:5000/my-image:latest"},
	}

	for _, tt := range urls {
		t.Run(tt.name, func(t *testing.T) {
			quoted := shellQuote(tt.url)
			unquoted := simulateShellUnquoteForTransfer(quoted)
			if unquoted != tt.url {
				t.Errorf("round-trip failed for URL %q: got %q", tt.url, unquoted)
			}
		})
	}
}

func TestShellQuoteMultiline(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "single newline", input: "line1\nline2"},
		{name: "multiple newlines", input: "line1\nline2\nline3\nline4"},
		{name: "trailing newline", input: "content\n"},
		{name: "leading newline", input: "\ncontent"},
		{name: "only newlines", input: "\n\n\n"},
		{name: "mixed newlines and quotes", input: "line1\nit's line2\nline3"},
		{name: "CRLF", input: "line1\r\nline2"},
		{name: "carriage return only", input: "line1\rline2"},
		{name: "newline with shell commands", input: "echo hello\nrm -rf /\necho done"},
		{name: "multiline script", input: "#!/bin/bash\nset -e\necho 'deploy'\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quoted := shellQuote(tt.input)

			// Must start and end with single quotes.
			if !strings.HasPrefix(quoted, "'") || !strings.HasSuffix(quoted, "'") {
				t.Errorf("shellQuote(%q) not properly quoted: %s", tt.input, quoted)
			}

			// Round-trip must preserve content including newlines.
			unquoted := simulateShellUnquoteForTransfer(quoted)
			if unquoted != tt.input {
				t.Errorf("round-trip failed for multiline input: got %q, want %q", unquoted, tt.input)
			}
		})
	}
}

func TestShellQuoteUnicode(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "accented characters", input: "cafe\u0301"},
		{name: "CJK characters", input: "\u4f60\u597d\u4e16\u754c"},
		{name: "Cyrillic", input: "\u041f\u0440\u0438\u0432\u0435\u0442"},
		{name: "Arabic", input: "\u0645\u0631\u062d\u0628\u0627"},
		{name: "emoji", input: "deploy \U0001f680"},
		{name: "multiple emoji", input: "\u2705 \u274c \u26a0\ufe0f"},
		{name: "mixed ASCII and Unicode", input: "project-\u00fc\u00e4\u00f6-name"},
		{name: "mathematical symbols", input: "\u2200x \u2208 S: x > 0"},
		{name: "zero-width joiner", input: "test\u200dvalue"},
		{name: "right-to-left mark", input: "hello\u200fworld"},
		{name: "fullwidth characters", input: "\uff21\uff22\uff23"},
		{name: "combining diacriticals", input: "n\u0303o\u0308"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quoted := shellQuote(tt.input)

			if !strings.HasPrefix(quoted, "'") || !strings.HasSuffix(quoted, "'") {
				t.Errorf("shellQuote(%q) not properly quoted: %s", tt.input, quoted)
			}

			unquoted := simulateShellUnquoteForTransfer(quoted)
			if unquoted != tt.input {
				t.Errorf("round-trip failed for Unicode input %q: got %q", tt.input, unquoted)
			}
		})
	}
}

func TestShellQuoteMaxLength(t *testing.T) {
	lengths := []int{100, 1000, 10000}

	for _, n := range lengths {
		// Test with a simple repeated character.
		input := strings.Repeat("a", n)
		quoted := shellQuote(input)
		if len(quoted) != n+2 { // n chars + 2 quotes
			t.Errorf("shellQuote(repeat('a', %d)) length = %d, want %d", n, len(quoted), n+2)
		}
		unquoted := simulateShellUnquoteForTransfer(quoted)
		if unquoted != input {
			t.Errorf("round-trip failed for %d-char string", n)
		}
	}

	// Test with a string full of single quotes (worst case for expansion).
	input := strings.Repeat("'", 10000)
	quoted := shellQuote(input)
	unquoted := simulateShellUnquoteForTransfer(quoted)
	if unquoted != input {
		t.Errorf("round-trip failed for 10000 single quotes: got length %d", len(unquoted))
	}

	// Verify the quoted length is correct: each ' becomes '"'"' (5 chars)
	// plus the outer two quotes.
	// Inner: 10000 single quotes, each replaced by '"'"' = 10000 * 5 = 50000
	// Plus outer quotes: 50000 + 2 = 50002
	expectedLen := 10000*5 + 2
	if len(quoted) != expectedLen {
		t.Errorf("shellQuote(10000 quotes) length = %d, want %d", len(quoted), expectedLen)
	}
}

func TestShellQuoteAllASCII(t *testing.T) {
	// Test every printable ASCII character (32-126) individually.
	for c := byte(32); c <= 126; c++ {
		input := string([]byte{c})
		t.Run("ascii_"+string(c), func(t *testing.T) {
			quoted := shellQuote(input)

			// Must be properly quoted.
			if !strings.HasPrefix(quoted, "'") || !strings.HasSuffix(quoted, "'") {
				t.Errorf("shellQuote(%q) not properly quoted: %q", input, quoted)
			}

			// Round-trip must preserve the character.
			unquoted := simulateShellUnquoteForTransfer(quoted)
			if unquoted != input {
				t.Errorf("round-trip failed for ASCII %d (%q): got %q", c, input, unquoted)
			}
		})
	}
}

func TestShellQuoteControlCharacters(t *testing.T) {
	// Test control characters (except null byte which is stripped).
	controlChars := []struct {
		name  string
		input string
	}{
		{name: "bell", input: "\x07"},
		{name: "backspace", input: "\x08"},
		{name: "form feed", input: "\x0c"},
		{name: "vertical tab", input: "\x0b"},
		{name: "escape", input: "\x1b"},
		{name: "delete", input: "\x7f"},
		{name: "mixed control and text", input: "before\x07after"},
	}

	for _, tt := range controlChars {
		t.Run(tt.name, func(t *testing.T) {
			quoted := shellQuote(tt.input)

			// Must be properly quoted.
			if !strings.HasPrefix(quoted, "'") || !strings.HasSuffix(quoted, "'") {
				t.Errorf("shellQuote(%q) not properly quoted: %q", tt.input, quoted)
			}

			// Round-trip must preserve the character.
			unquoted := simulateShellUnquoteForTransfer(quoted)
			if unquoted != tt.input {
				t.Errorf("round-trip failed for %q: got %q", tt.input, unquoted)
			}
		})
	}
}

func TestShellQuoteNullByteInPaths(t *testing.T) {
	// Null bytes in file paths should be stripped.
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "null in middle of path",
			input:    "/opt/app\x00/file",
			expected: "'/opt/app/file'",
		},
		{
			name:     "null at end of path",
			input:    "/opt/app/file\x00",
			expected: "'/opt/app/file'",
		},
		{
			name:     "null at start of path",
			input:    "\x00/opt/app",
			expected: "'/opt/app'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quoted := shellQuote(tt.input)
			if quoted != tt.expected {
				t.Errorf("shellQuote(%q) = %q, want %q", tt.input, quoted, tt.expected)
			}
			if strings.Contains(quoted, "\x00") {
				t.Errorf("quoted result should not contain null bytes")
			}
		})
	}
}

func TestShellQuoteIdempotent(t *testing.T) {
	// Quoting an already-quoted string should produce a valid double-quoted result.
	// This is NOT about producing the original, but about correctness: the result
	// should be a valid single-quoted string that evaluates to the already-quoted string.
	inputs := []string{"hello", "it's", "/path/to/file", "$HOME"}

	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			quoted1 := shellQuote(input)
			quoted2 := shellQuote(quoted1)

			// Double-unquoting should give us the original.
			unquoted2 := simulateShellUnquoteForTransfer(quoted2)
			unquoted1 := simulateShellUnquoteForTransfer(unquoted2)
			if unquoted1 != input {
				t.Errorf("double round-trip failed: input=%q, got=%q", input, unquoted1)
			}
		})
	}
}

package remote

import (
	"strings"
	"testing"
)

func TestShellQuoteInjectionAttempts(t *testing.T) {
	// Each payload represents a common shell injection vector.
	// After quoting, the result must be safe to embed in a shell command.
	injectionPayloads := []struct {
		name    string
		payload string
	}{
		{name: "semicolon command chain", payload: "test; rm -rf /"},
		{name: "command substitution dollar", payload: "$(whoami)"},
		{name: "command substitution backtick", payload: "`id`"},
		{name: "newline injection", payload: "test\nmalicious"},
		{name: "SQL-style injection", payload: "'; DROP TABLE;--"},
		{name: "variable expansion", payload: "${HOME}"},
		{name: "pipe to command", payload: "test|cat /etc/passwd"},
		{name: "redirect output", payload: "test > /tmp/owned"},
		{name: "logical AND chain", payload: "test && malicious"},
		{name: "logical OR chain", payload: "test || malicious"},
		{name: "subshell", payload: "$(cat /etc/shadow)"},
		{name: "background execution", payload: "test & malicious"},
		{name: "heredoc attempt", payload: "test << EOF"},
		{name: "process substitution", payload: "test <(cat /etc/passwd)"},
		{name: "glob expansion", payload: "test *"},
		{name: "tilde expansion", payload: "~root/.ssh/id_rsa"},
		{name: "ANSI escape", payload: "test\x1b[31mred"},
		{name: "brace expansion", payload: "{a,b,c}"},
		{name: "double ampersand with rm", payload: "x && rm -rf /"},
		{name: "nested substitution", payload: "$(echo $(whoami))"},
	}

	for _, tt := range injectionPayloads {
		t.Run(tt.name, func(t *testing.T) {
			quoted := shellQuote(tt.payload)

			// The quoted result must start and end with a single quote.
			if !strings.HasPrefix(quoted, "'") {
				t.Errorf("shellQuote(%q) should start with single quote, got: %s", tt.payload, quoted)
			}
			if !strings.HasSuffix(quoted, "'") {
				t.Errorf("shellQuote(%q) should end with single quote, got: %s", tt.payload, quoted)
			}

			// The result must not contain any unescaped single quotes.
			// The safe pattern is: end quote, escaped quote, start quote: '"'"'
			// Strip the outer quotes and check that every single quote
			// appears as part of the '"'"' escape sequence.
			inner := quoted[1 : len(quoted)-1]
			assertNoUnescapedSingleQuotes(t, tt.payload, inner)

			// The quoted string must not be empty (even for special payloads).
			if len(quoted) < 2 {
				t.Errorf("shellQuote(%q) produced too-short result: %s", tt.payload, quoted)
			}
		})
	}
}

// assertNoUnescapedSingleQuotes verifies that the inner content of a quoted
// string has no bare single quotes -- they must appear only as part of the
// '"'"' escape sequence (which within the inner portion shows up as: '\"'\"' ).
func assertNoUnescapedSingleQuotes(t *testing.T, original, inner string) {
	t.Helper()

	// After stripping the well-known escape pattern, there should be no
	// remaining single quotes.
	cleaned := strings.ReplaceAll(inner, "'\"'\"'", "")
	if strings.Contains(cleaned, "'") {
		t.Errorf("shellQuote(%q) contains unescaped single quote in inner content: %s", original, inner)
	}
}

func TestShellQuotePreservesContent(t *testing.T) {
	// The purpose of shell quoting is that when the shell interprets the
	// quoted string, it produces the original value. We can't run a real
	// shell here, but we can verify the round-trip by simulating the
	// shell's unquoting: remove outer quotes and undo the escape sequence.
	testStrings := []string{
		"simple",
		"hello world",
		"/path/to/file",
		"file with spaces and (parens)",
		"it's a test",
		"double''quotes",
		"mixed 'quotes' and \"double quotes\"",
		"path/with/slashes",
		"special chars: @#%^&*",
		"unicode: \u00e9\u00e8\u00ea",
		"empty-not-really",
		"tab\there",
		"newline\nhere",
	}

	for _, s := range testStrings {
		t.Run(s, func(t *testing.T) {
			quoted := shellQuote(s)
			unquoted := simulateShellUnquote(quoted)
			if unquoted != s {
				t.Errorf("round-trip failed: shellQuote(%q) = %q, simulated unquote = %q", s, quoted, unquoted)
			}
		})
	}
}

// simulateShellUnquote reverses the shellQuote transformation.
// shellQuote produces: '<content with ' replaced by '"'"'>'
// So to reverse: strip outer quotes, then replace '"'"' with '.
func simulateShellUnquote(quoted string) string {
	if len(quoted) < 2 || quoted[0] != '\'' || quoted[len(quoted)-1] != '\'' {
		return quoted // not properly quoted
	}
	inner := quoted[1 : len(quoted)-1]
	return strings.ReplaceAll(inner, "'\"'\"'", "'")
}

func TestShellQuoteNullBytes(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "null byte in middle", input: "before\x00after"},
		{name: "null byte at start", input: "\x00start"},
		{name: "null byte at end", input: "end\x00"},
		{name: "multiple null bytes", input: "a\x00b\x00c"},
		{name: "only null bytes", input: "\x00\x00\x00"},
		{name: "null byte with injection", input: "safe\x00; rm -rf /"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quoted := shellQuote(tt.input)

			// Null bytes must not appear in the output.
			if strings.Contains(quoted, "\x00") {
				t.Errorf("shellQuote(%q) should not contain null bytes, got: %q", tt.input, quoted)
			}

			// The result should still be a valid quoted string.
			if !strings.HasPrefix(quoted, "'") || !strings.HasSuffix(quoted, "'") {
				t.Errorf("shellQuote(%q) should be properly quoted, got: %s", tt.input, quoted)
			}
		})
	}
}

func TestShellQuoteNullBytesStripped(t *testing.T) {
	// Verify that null bytes are truly stripped, not replaced with something else.
	input := "hello\x00world"
	quoted := shellQuote(input)
	expected := "'helloworld'"
	if quoted != expected {
		t.Errorf("shellQuote(%q) = %q, want %q", input, quoted, expected)
	}
}

func TestShellQuoteEmptyString(t *testing.T) {
	result := shellQuote("")
	if result != "''" {
		t.Errorf("shellQuote(\"\") = %q, want \"''\"", result)
	}

	// Verify it's exactly two single-quote characters.
	if len(result) != 2 {
		t.Errorf("shellQuote(\"\") length = %d, want 2", len(result))
	}
}

func TestShellQuoteSingleCharacters(t *testing.T) {
	// Test each dangerous character individually.
	dangerousChars := []string{
		";", "|", "&", "$", "`", "(", ")", "{", "}", "<", ">",
		"!", "~", "#", "*", "?", "[", "]", "\\", "\n", "\t", "\r",
	}

	for _, ch := range dangerousChars {
		t.Run("char_"+strings.ReplaceAll(ch, "\n", "\\n"), func(t *testing.T) {
			quoted := shellQuote(ch)
			if !strings.HasPrefix(quoted, "'") || !strings.HasSuffix(quoted, "'") {
				t.Errorf("shellQuote(%q) not properly quoted: %s", ch, quoted)
			}
		})
	}
}

func TestShellQuoteSingleQuoteAlone(t *testing.T) {
	// A single quote character is the hardest case for single-quote quoting.
	quoted := shellQuote("'")
	expected := "''\"'\"''"
	if quoted != expected {
		t.Errorf("shellQuote(\"'\") = %q, want %q", quoted, expected)
	}

	// Verify round-trip.
	unquoted := simulateShellUnquote(quoted)
	if unquoted != "'" {
		t.Errorf("round-trip of single quote failed: got %q", unquoted)
	}
}

func TestShellQuoteVeryLongString(t *testing.T) {
	// Ensure no issues with long strings.
	long := strings.Repeat("a", 10000)
	quoted := shellQuote(long)
	if !strings.HasPrefix(quoted, "'") || !strings.HasSuffix(quoted, "'") {
		t.Error("long string not properly quoted")
	}
	if len(quoted) != 10002 { // 10000 + 2 quotes
		t.Errorf("expected length 10002, got %d", len(quoted))
	}
}

func TestShellQuoteConsecutiveSingleQuotes(t *testing.T) {
	input := "'''"
	quoted := shellQuote(input)

	// Verify no unescaped quotes in the inner portion.
	inner := quoted[1 : len(quoted)-1]
	cleaned := strings.ReplaceAll(inner, "'\"'\"'", "")
	if strings.Contains(cleaned, "'") {
		t.Errorf("consecutive single quotes not properly escaped: %s", quoted)
	}

	// Verify round-trip.
	unquoted := simulateShellUnquote(quoted)
	if unquoted != input {
		t.Errorf("round-trip failed for consecutive quotes: got %q, want %q", unquoted, input)
	}
}

func TestNewClientRequiresHOME(t *testing.T) {
	// Generate a valid key so we get past key parsing and reach the HOME check.
	keyData := generateTestED25519Key(t)

	// Unset HOME and verify NewClient returns an error mentioning HOME.
	t.Setenv("HOME", "")

	_, err := NewClient("127.0.0.1", "22", "testuser", keyData)
	if err == nil {
		t.Fatal("NewClient() should return error when HOME is empty")
	}
	if !strings.Contains(err.Error(), "HOME") {
		t.Errorf("error should mention HOME, got: %v", err)
	}
	if !strings.Contains(err.Error(), "known_hosts") {
		t.Errorf("error should mention known_hosts, got: %v", err)
	}
}

func TestNewClientRequiresHOMENotSet(t *testing.T) {
	// Also test with HOME completely absent from environment.
	keyData := generateTestED25519Key(t)

	// t.Setenv("HOME", "") sets it to empty string; this tests that case.
	t.Setenv("HOME", "")

	client, err := NewClient("example.com", "22", "deploy", keyData)
	if err == nil {
		t.Fatal("NewClient() should fail when HOME is not set")
		if client != nil {
			client.Close()
		}
	}
}

func TestNewClientInvalidKeyBeforeHOMECheck(t *testing.T) {
	// With invalid key data, the error should be about key parsing,
	// not about HOME -- verifying error ordering.
	t.Setenv("HOME", "")

	_, err := NewClient("127.0.0.1", "22", "user", []byte("not a key"))
	if err == nil {
		t.Fatal("NewClient() should fail with invalid key")
	}
	// The error should be about key parsing, not HOME.
	if strings.Contains(err.Error(), "HOME") {
		t.Error("with invalid key, error should be about key parsing, not HOME")
	}
}

func TestShellQuoteNoInterpolation(t *testing.T) {
	// Verify that shell metacharacters are not interpreted.
	// The quoted string should literally contain the metacharacters.
	tests := []struct {
		name  string
		input string
	}{
		{name: "dollar HOME", input: "$HOME"},
		{name: "dollar USER", input: "$USER"},
		{name: "dollar braces", input: "${PATH}"},
		{name: "backtick uname", input: "`uname -a`"},
		{name: "arithmetic", input: "$((1+1))"},
		{name: "double dollar", input: "$$"},
		{name: "exclamation", input: "!previous"},
		{name: "glob star", input: "*.txt"},
		{name: "glob question", input: "file?.txt"},
		{name: "bracket glob", input: "[abc]"},
		{name: "tilde", input: "~root"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quoted := shellQuote(tt.input)

			// The literal input characters should appear somewhere in the
			// quoted output (between the quotes).
			inner := quoted[1 : len(quoted)-1]
			cleanedInner := strings.ReplaceAll(inner, "'\"'\"'", "'")
			if cleanedInner != tt.input {
				t.Errorf("shellQuote(%q) does not preserve content; inner = %q", tt.input, cleanedInner)
			}
		})
	}
}

package remote

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"testing"

	"golang.org/x/crypto/ssh"
)

// generateTestED25519Key creates a valid, unencrypted ED25519 private key in
// OpenSSH PEM format suitable for use with ssh.ParsePrivateKey.
func generateTestED25519Key(t *testing.T) []byte {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generating ed25519 key: %v", err)
	}

	// Marshal into OpenSSH format (the only format ssh.ParsePrivateKey
	// accepts for ED25519 keys).
	pemBlock, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatalf("marshalling private key: %v", err)
	}

	return pem.EncodeToMemory(pemBlock)
}

func TestParsePrivateKey(t *testing.T) {
	keyData := generateTestED25519Key(t)

	signer, err := ParsePrivateKey(keyData)
	if err != nil {
		t.Fatalf("ParsePrivateKey() returned unexpected error: %v", err)
	}
	if signer == nil {
		t.Fatal("ParsePrivateKey() returned nil signer")
	}

	// The public key should be available and non-nil.
	pub := signer.PublicKey()
	if pub == nil {
		t.Fatal("signer.PublicKey() returned nil")
	}
	if pub.Type() != "ssh-ed25519" {
		t.Errorf("expected key type ssh-ed25519, got %s", pub.Type())
	}
}

func TestParsePrivateKeyInvalid(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "empty input",
			data: []byte{},
		},
		{
			name: "random garbage bytes",
			data: []byte("this is not a valid private key at all"),
		},
		{
			name: "truncated PEM header",
			data: []byte("-----BEGIN OPENSSH PRIVATE KEY-----\n"),
		},
		{
			name: "valid PEM structure but invalid key data",
			data: pem.EncodeToMemory(&pem.Block{
				Type:  "OPENSSH PRIVATE KEY",
				Bytes: []byte("not a real key payload"),
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signer, err := ParsePrivateKey(tt.data)
			if err == nil {
				t.Error("ParsePrivateKey() should return an error for invalid input")
			}
			if signer != nil {
				t.Error("ParsePrivateKey() should return nil signer on error")
			}
		})
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple string",
			input:    "hello",
			expected: "'hello'",
		},
		{
			name:     "string with spaces",
			input:    "hello world",
			expected: "'hello world'",
		},
		{
			name:     "string with single quotes",
			input:    "it's a test",
			expected: "'it'\"'\"'s a test'",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "''",
		},
		{
			name:     "string with special shell characters",
			input:    "hello; rm -rf /",
			expected: "'hello; rm -rf /'",
		},
		{
			name:     "string with dollar sign",
			input:    "$HOME/path",
			expected: "'$HOME/path'",
		},
		{
			name:     "string with backticks",
			input:    "`whoami`",
			expected: "'`whoami`'",
		},
		{
			name:     "string with double quotes",
			input:    `say "hello"`,
			expected: `'say "hello"'`,
		},
		{
			name:     "string with multiple single quotes",
			input:    "a'b'c",
			expected: "'a'\"'\"'b'\"'\"'c'",
		},
		{
			name:     "string with newline",
			input:    "line1\nline2",
			expected: "'line1\nline2'",
		},
		{
			name:     "string with tab",
			input:    "col1\tcol2",
			expected: "'col1\tcol2'",
		},
		{
			name:     "path with spaces",
			input:    "/home/user/my project/file.txt",
			expected: "'/home/user/my project/file.txt'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shellQuote(tt.input)
			if got != tt.expected {
				t.Errorf("shellQuote(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

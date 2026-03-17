package cmd

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/fleetdeck/fleetdeck/internal/ui"
)

// findSSHKey attempts to locate an SSH private key for the given host.
// It checks (in order):
//  1. ~/.ssh/config for a matching Host entry with IdentityFile
//  2. Common key filenames: id_ed25519, id_ecdsa, id_rsa
//  3. Any key-like files in ~/.ssh/ (files without .pub extension that look like keys)
func findSSHKey(host string) []byte {
	sshDir := os.ExpandEnv("$HOME/.ssh")

	// 1. Check ~/.ssh/config for host-specific IdentityFile
	if keyPath := findKeyInSSHConfig(sshDir, host); keyPath != "" {
		data, err := os.ReadFile(keyPath)
		if err == nil {
			ui.Info("Using SSH key from ~/.ssh/config: %s", keyPath)
			return data
		}
	}

	// 2. Try common key filenames
	commonKeys := []string{
		"id_ed25519",
		"id_ecdsa",
		"id_rsa",
	}
	for _, name := range commonKeys {
		path := filepath.Join(sshDir, name)
		data, err := os.ReadFile(path)
		if err == nil {
			return data
		}
	}

	// 3. Scan ~/.ssh/ for any private key files
	entries, err := os.ReadDir(sshDir)
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Skip public keys, known_hosts, config, and authorized_keys
		if strings.HasSuffix(name, ".pub") ||
			name == "known_hosts" ||
			name == "known_hosts.old" ||
			name == "config" ||
			name == "authorized_keys" {
			continue
		}
		path := filepath.Join(sshDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		// Check if file looks like a private key
		if isPrivateKey(data) {
			ui.Info("Using SSH key: %s", path)
			return data
		}
	}

	return nil
}

// findKeyInSSHConfig parses ~/.ssh/config and returns the IdentityFile
// for the first matching Host entry.
func findKeyInSSHConfig(sshDir, host string) string {
	configPath := filepath.Join(sshDir, "config")
	f, err := os.Open(configPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	inMatchingHost := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Split on first whitespace (space or tab)
		idx := strings.IndexAny(line, " \t")
		if idx < 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:idx]))
		value := strings.TrimSpace(line[idx+1:])

		if key == "host" {
			// Check if any of the host patterns match
			inMatchingHost = false
			for _, pattern := range strings.Fields(value) {
				if matchHostPattern(pattern, host) {
					inMatchingHost = true
					break
				}
			}
			continue
		}

		if inMatchingHost && key == "identityfile" {
			// Expand ~ to home directory
			if strings.HasPrefix(value, "~/") {
				value = filepath.Join(os.Getenv("HOME"), value[2:])
			} else if strings.HasPrefix(value, "$HOME/") {
				value = os.ExpandEnv(value)
			}
			// Only return if the file exists
			if _, err := os.Stat(value); err == nil {
				return value
			}
		}
	}

	return ""
}

// matchHostPattern matches an SSH config Host pattern against a hostname.
// Supports * as a wildcard.
func matchHostPattern(pattern, host string) bool {
	if pattern == "*" {
		return true
	}
	if pattern == host {
		return true
	}
	// Simple wildcard matching: *.example.com
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[1:] // .example.com
		return strings.HasSuffix(host, suffix)
	}
	return false
}

// isPrivateKey checks if data looks like an SSH or PEM private key.
func isPrivateKey(data []byte) bool {
	s := string(data)
	return strings.HasPrefix(s, "-----BEGIN") && strings.Contains(s, "PRIVATE KEY")
}

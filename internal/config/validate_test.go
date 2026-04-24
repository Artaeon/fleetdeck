package config

import (
	"strings"
	"testing"
)

// TestValidateAcceptsDefaults is the table-stakes test: the default
// config that every `fleetdeck init` produces must pass validation.
// Regression guard against future tightening that accidentally
// rejects the defaults.
func TestValidateAcceptsDefaults(t *testing.T) {
	if err := DefaultConfig().Validate(); err != nil {
		t.Fatalf("default config should pass validation, got: %v", err)
	}
}

// TestValidateRejectsShortEncryptionKey pins the security floor.
// A 6-char FLEETDECK_ENCRYPTION_KEY would survive PBKDF2 but is
// brute-forceable offline — we'd rather fail at startup than let
// the operator deploy with a weak key and discover the hard way.
func TestValidateRejectsShortEncryptionKey(t *testing.T) {
	c := DefaultConfig()
	c.Server.EncryptionKey = "short" // 5 chars — well below the floor
	err := c.Validate()
	if err == nil {
		t.Fatal("expected validation error for 5-char encryption key")
	}
	if !strings.Contains(err.Error(), "FLEETDECK_ENCRYPTION_KEY") {
		t.Errorf("error should mention the env var, got: %v", err)
	}
	// Hint must point to a remediation — operators should not need
	// to dig through source to figure out what to do.
	if !strings.Contains(err.Error(), "openssl rand") {
		t.Errorf("error should suggest a command to generate a key, got: %v", err)
	}
}

// TestValidateAllowsEmptyEncryptionKey keeps the local-dev path
// working. Encryption is opt-in; an unset key means secret columns
// are stored plain and should not be rejected as misconfiguration.
func TestValidateAllowsEmptyEncryptionKey(t *testing.T) {
	c := DefaultConfig()
	c.Server.EncryptionKey = ""
	if err := c.Validate(); err != nil {
		t.Errorf("empty encryption key should pass validation (opt-in), got: %v", err)
	}
}

// TestValidateRejectsBadStrategy is the 'catch a typo in config.toml
// at startup' case. Without this, the typo surfaces at the first
// deploy as a confusing 'unknown strategy' — weeks after the edit
// that introduced it.
func TestValidateRejectsBadStrategy(t *testing.T) {
	c := DefaultConfig()
	c.Deploy.Strategy = "blue-green" // not the correct spelling
	err := c.Validate()
	if err == nil {
		t.Fatal("expected validation error for unknown strategy")
	}
	if !strings.Contains(err.Error(), "bluegreen") {
		t.Errorf("error should suggest the correct values, got: %v", err)
	}
}

// TestValidateRejectsBadProfile covers the parallel case for the
// default deployment profile.
func TestValidateRejectsBadProfile(t *testing.T) {
	c := DefaultConfig()
	c.Deploy.DefaultProfile = "microservices" // not a real profile
	if err := c.Validate(); err == nil {
		t.Fatal("expected validation error for unknown profile")
	}
}

// TestValidateRejectsNegativeConcurrency pins the channel-creation
// guard — a negative MaxConcurrentDeploys would panic make(chan, n).
func TestValidateRejectsNegativeConcurrency(t *testing.T) {
	c := DefaultConfig()
	c.Server.MaxConcurrentDeploys = -1
	if err := c.Validate(); err == nil {
		t.Fatal("expected validation error for negative MaxConcurrentDeploys")
	}
}

// TestValidateRejectsNegativeRetention catches typos in retention
// fields that would otherwise delete every backup ('max=-5').
func TestValidateRejectsNegativeRetention(t *testing.T) {
	c := DefaultConfig()
	c.Backup.MaxManualBackups = -3
	if err := c.Validate(); err == nil {
		t.Fatal("expected validation error for negative retention count")
	}
}

// TestValidateRejectsBadDNSProvider catches 'provider = "route53"'
// pointing at a provider that isn't implemented. Fails at startup
// rather than at first DNS call.
func TestValidateRejectsBadDNSProvider(t *testing.T) {
	c := DefaultConfig()
	c.DNS.Provider = "route53"
	if err := c.Validate(); err == nil {
		t.Fatal("expected validation error for unimplemented DNS provider")
	}
}

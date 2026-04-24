package remote

import (
	"os"
	"testing"
)

// TestHardenSensitiveMode pins the narrowing behaviour of the upload
// mode filter. The function should tighten to 0600 for anything that
// looks like a credential and leave other files' modes untouched.
func TestHardenSensitiveMode(t *testing.T) {
	cases := []struct {
		path     string
		inMode   os.FileMode
		wantMode os.FileMode
	}{
		// .env family — always force 0600.
		{"/opt/app/.env", 0644, 0600},
		{"/opt/app/.env.production", 0644, 0600},
		{"/opt/app/.ENV", 0644, 0600}, // case-insensitive on basename
		// Key / cert extensions.
		{"/opt/app/secrets/signing.pem", 0644, 0600},
		{"/opt/app/secrets/tls.key", 0600, 0600},
		{"/opt/app/bundle.p12", 0644, 0600},
		{"/opt/app/android.jks", 0664, 0600},
		// Ordinary files pass through unchanged.
		{"/opt/app/docker-compose.yml", 0644, 0644},
		{"/opt/app/Dockerfile", 0755, 0755},
		{"/opt/app/public/index.html", 0644, 0644},
		// Files that merely contain 'env' in the name are NOT narrowed —
		// avoid a false-positive landslide like 'environment.md' becoming
		// 0600 and breaking the web server that serves it.
		{"/opt/app/environment-docs.md", 0644, 0644},
	}
	for _, c := range cases {
		got := hardenSensitiveMode(c.path, c.inMode)
		if got != c.wantMode {
			t.Errorf("hardenSensitiveMode(%q, %o) = %o, want %o", c.path, c.inMode, got, c.wantMode)
		}
	}
}

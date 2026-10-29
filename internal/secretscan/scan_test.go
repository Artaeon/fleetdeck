package secretscan

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func token() string {
	s := sha256.Sum256([]byte("synthetic FleetDeck scanner test, never a real credential"))
	return "ghp_" + hex.EncodeToString(s[:])[:36]
}

func bundle(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := tar.NewWriter(&b)
	for name, body := range files {
		if err := w.WriteHeader(&tar.Header{Name: name, Size: int64(len(body)), Mode: 0600, Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestRealRulesBlockSecretsWithoutReturningValues(t *testing.T) {
	t.Setenv("GITLEAKS_CONFIG_TOML", "invalid attacker configuration")
	secret := token()
	r := Archive(context.Background(), bytes.NewReader(bundle(t, map[string]string{
		"credentials.png": "token = \"" + secret + "\" // gitleaks:allow\n",
		".gitleaks.toml":  "invalid project scanner configuration",
		".gitleaksignore": "credentials.png:github-pat:1\n",
	})))
	if r.Status != "blocked" || r.RequireClean() == nil || len(r.Findings) == 0 {
		t.Fatalf("secret not blocked: %+v", r)
	}
	got := false
	for _, f := range r.Findings {
		if f.Path == "credentials.png" && f.Line == 1 && f.Rule == "github-pat" {
			got = true
		}
	}
	if !got {
		t.Fatalf("missing metadata: %+v", r.Findings)
	}
	encoded, _ := json.Marshal(r)
	for _, forbidden := range []string{secret, "Match", "Secret", "Fragment", "token ="} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Fatal("report exposed source content")
		}
	}
}

func TestSeedIsNotScannedAndOrdinarySourcePasses(t *testing.T) {
	r := Archive(context.Background(), bytes.NewReader(bundle(t, map[string]string{".env": "TOKEN=" + token(), "app.js": "console.log('hello');"})))
	if r.RequireClean() != nil || r.Files != 1 {
		t.Fatalf("unexpected result: %+v", r)
	}
}

func TestEncodedCredentialIsDetectedWithoutReturningEncodedValue(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("token=" + token()))
	r := Archive(context.Background(), bytes.NewReader(bundle(t, map[string]string{"app.txt": encoded})))
	if r.Status != "blocked" {
		t.Fatalf("encoded secret not detected: %+v", r)
	}
	data, _ := json.Marshal(r)
	if bytes.Contains(data, []byte(encoded)) || bytes.Contains(data, []byte(token())) {
		t.Fatal("report exposed credential")
	}
}

func TestFailureNeverPasses(t *testing.T) {
	for name, body := range map[string]string{"nested.zip": "plain text", "renamed.txt": string([]byte{'P', 'K', 3, 4}), "../outside": "hello"} {
		t.Run(name, func(t *testing.T) {
			r := Archive(context.Background(), bytes.NewReader(bundle(t, map[string]string{name: body})))
			if r.Status != "unknown" || r.RequireClean() == nil {
				t.Fatalf("unsafe pass: %+v", r)
			}
		})
	}
	for _, b := range [][]byte{nil, []byte("not a tar"), bundle(t, map[string]string{"huge.txt": strings.Repeat("a", int(MaxFileBytes+1))})} {
		r := Archive(context.Background(), bytes.NewReader(b))
		if r.Status != "unknown" || r.RequireClean() == nil {
			t.Fatalf("unsafe pass: %+v", r)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := Archive(ctx, bytes.NewReader(bundle(t, map[string]string{"safe.txt": "hello"})))
	if r.Code != "cancelled" || r.RequireClean() == nil {
		t.Fatalf("cancelled check passed: %+v", r)
	}
}

func TestReceiverRejectsLinksAndMissingSource(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "private.txt")
	os.WriteFile(outside, []byte(token()), 0600)
	os.Symlink(outside, filepath.Join(dir, "link.txt"))
	for _, name := range []string{"link.txt", "missing.txt", "../private.txt"} {
		r := Files(context.Background(), dir, []string{name})
		if r.Status != "unknown" || r.RequireClean() == nil {
			t.Fatalf("unsafe pass: %+v", r)
		}
	}
	os.WriteFile(filepath.Join(dir, "source.txt"), []byte(token()), 0600)
	r := Files(context.Background(), dir, []string{"source.txt"})
	if r.Status != "blocked" {
		t.Fatalf("receiver did not detect secret: %+v", r)
	}
}

func TestParallelScans(t *testing.T) {
	for i := 0; i < 4; i++ {
		t.Run("scan", func(t *testing.T) {
			t.Parallel()
			r := Archive(context.Background(), bytes.NewReader(bundle(t, map[string]string{"safe.txt": "ordinary source"})))
			if r.RequireClean() != nil {
				t.Fatalf("check failed: %+v", r)
			}
		})
	}
}

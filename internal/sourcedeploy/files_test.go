package sourcedeploy

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func put(t *testing.T, dir, name, value string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(value), 0600); err != nil {
		t.Fatal(err)
	}
}
func TestArchiveExcludesSecretsAndData(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "compose.yaml", "services:\n  app:\n    image: example/app:1\n    env_file: config/production.conf\n    volumes: ['./data:/data']\nsecrets:\n  signing:\n    file: config/signing.txt\n")
	for _, name := range []string{".env", ".env.production", ".git/config", ".ssh/id_ed25519", "data/db.sqlite", "config/production.conf", "config/signing.txt", "tls.pem"} {
		put(t, dir, name, "private-sentinel")
	}
	put(t, dir, "app.txt", "reviewed code")
	var data bytes.Buffer
	if err := Archive(dir, &data); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data.Bytes(), []byte("private-sentinel")) {
		t.Fatal("private source data uploaded")
	}
	out := t.TempDir()
	names, err := Extract(&data, out)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 {
		t.Fatal(names)
	}
}
func TestArchiveRejectsSourceSymlink(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "compose.yaml", "services: {app: {image: example/app:1}}")
	os.Symlink("/etc/passwd", filepath.Join(dir, "link"))
	if err := Archive(dir, io.Discard); err == nil {
		t.Fatal("followed source link")
	}
}
func TestExtractionRejectsEscapeAndSpecialEntries(t *testing.T) {
	for _, h := range []*tar.Header{{Name: "../escape", Typeflag: tar.TypeReg}, {Name: "/absolute", Typeflag: tar.TypeReg}, {Name: "link", Typeflag: tar.TypeSymlink, Linkname: "/etc"}, {Name: ".git/config", Typeflag: tar.TypeReg}, {Name: ".fleetdeck.lock", Typeflag: tar.TypeReg}, {Name: "a\\b", Typeflag: tar.TypeReg}} {
		var b bytes.Buffer
		tw := tar.NewWriter(&b)
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		tw.Close()
		if _, err := Extract(&b, t.TempDir()); err == nil {
			t.Fatalf("accepted %#v", h)
		}
	}
}
func TestExplicitInitialEnvIsPrivate(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "compose.yaml", "services: {app: {image: example/app:1}}")
	env := filepath.Join(t.TempDir(), "production.env")
	os.WriteFile(env, []byte("VALUE=fixture\n"), 0600)
	var b bytes.Buffer
	if err := ArchiveWithEnv(dir, &b, env); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if _, err := Extract(&b, out); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(out, ".env"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal(info, err)
	}
}
func TestDynamicPrivateReferencesFailClosed(t *testing.T) {
	for _, model := range []string{"services: {app: {image: example/app:1, env_file: '${SECRET_PATH}'}}", "services: {app: {image: example/app:1, volumes: ['/srv/shared:/data']}}", "include: other.yml\nservices: {app: {image: example/app:1}}"} {
		dir := t.TempDir()
		put(t, dir, "compose.yaml", model)
		if _, _, err := Protected(dir); err == nil {
			t.Fatal("accepted unsupported protection model")
		}
	}
}

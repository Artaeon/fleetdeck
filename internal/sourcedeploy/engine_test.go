package sourcedeploy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/backup"
	"github.com/fleetdeck/fleetdeck/internal/config"
	"github.com/fleetdeck/fleetdeck/internal/db"
	"github.com/fleetdeck/fleetdeck/internal/deploy"
)

type fixtureRunner struct {
	t              *testing.T
	path           string
	events         []string
	failBuild      bool
	failReady      bool
	expectedSecret string
}

func (r *fixtureRunner) Run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	if contains(args, "config") {
		m := model{Name: "demo", Services: map[string]serviceModel{"app": {Image: "example/app:1"}}}
		return json.Marshal(m)
	}
	if contains(args, "build") {
		r.events = append(r.events, "build")
		if r.expectedSecret != "" {
			secret, err := os.ReadFile(filepath.Join(dir, ".env"))
			if err != nil || string(secret) != r.expectedSecret {
				r.t.Fatal("server secret was not preserved in staging", err)
			}
		}
		if r.failBuild {
			return nil, fmt.Errorf("fixture build failure")
		}
		return nil, nil
	}
	if contains(args, "pull") {
		return nil, nil
	}
	if contains(args, "ps") {
		return nil, nil
	}
	if contains(args, "up") {
		r.events = append(r.events, "up")
		if !contains(args, "--wait") || !contains(args, "--no-build") {
			r.t.Fatal("no readiness gate")
		}
		if r.failReady {
			return nil, fmt.Errorf("fixture unhealthy")
		}
		return nil, nil
	}
	return nil, fmt.Errorf("unexpected command %v", args)
}
func fixture(t *testing.T) (*Engine, *fixtureRunner, string, string) {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig()
	cfg.Server.BasePath = filepath.Join(base, "projects")
	cfg.Backup.BasePath = filepath.Join(base, "backups")
	os.MkdirAll(cfg.Server.BasePath, 0700)
	d, err := db.Open(filepath.Join(base, "registry.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	path := filepath.Join(cfg.Server.BasePath, "demo")
	os.MkdirAll(path, 0750)
	put(t, path, "compose.yaml", "services: {app: {image: 'example/app:1', env_file: '.env'}}\n")
	put(t, path, ".env", "SECRET=server-value\n")
	put(t, path, "app.txt", "old source")
	p := &db.Project{Name: "demo", Domain: "demo.example.test", ProjectPath: path, LinuxUser: "fleetdeck-demo", Status: "running"}
	if err = d.CreateProject(p); err != nil {
		t.Fatal(err)
	}
	e := New(cfg, d)
	runner := &fixtureRunner{t: t, path: path, expectedSecret: "SECRET=server-value\n"}
	e.Runner = runner
	e.PrepareEdge = func(context.Context, string, string) error { return nil }
	e.Backup = func(c *config.Config, d *db.DB, p *db.Project) (*db.BackupRecord, error) {
		runner.events = append(runner.events, "backup")
		old, _ := os.ReadFile(filepath.Join(path, "app.txt"))
		if string(old) != "old source" {
			t.Fatal("changed project before backup")
		}
		dir := filepath.Join(c.Backup.BasePath, "fixture-snapshot")
		os.MkdirAll(dir, 0700)
		components, err := backup.BackupConfigFiles(path, dir)
		if err != nil {
			return nil, err
		}
		m := backup.Manifest{Version: "1", ProjectName: p.Name, ProjectPath: p.ProjectPath, Components: components}
		data, _ := json.Marshal(m)
		os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0600)
		return &db.BackupRecord{ID: "fixture-snapshot", ProjectID: p.ID, Path: dir}, nil
	}
	src := t.TempDir()
	put(t, src, "compose.yaml", "services: {app: {image: 'example/app:1', env_file: '.env'}}\n")
	put(t, src, "app.txt", "new source")
	put(t, src, ".env", "SECRET=local-development\n")
	return e, runner, path, src
}
func archive(t *testing.T, src string) *bytes.Buffer {
	t.Helper()
	var b bytes.Buffer
	if err := ArchiveWithEnv(src, &b, filepath.Join(src, ".env")); err != nil {
		t.Fatal(err)
	}
	return &b
}
func opts() Options { return Options{Name: "demo", Domain: "demo.example.test", Timeout: time.Minute} }
func TestServerBackupPrecedesUpdateAndExistingSecretsWin(t *testing.T) {
	e, r, path, src := fixture(t)
	result, err := e.Apply(context.Background(), opts(), archive(t, src), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Healthy || result.BackupID != "fixture-snapshot" {
		t.Fatal(result)
	}
	if strings.Join(r.events, ",") != "backup,build,up" {
		t.Fatal(r.events)
	}
	secret, _ := os.ReadFile(filepath.Join(path, ".env"))
	if string(secret) != "SECRET=server-value\n" {
		t.Fatal("replaced existing server secret")
	}
	code, _ := os.ReadFile(filepath.Join(path, "app.txt"))
	if string(code) != "new source" {
		t.Fatal("source not applied")
	}
	before, _ := os.ReadFile(filepath.Join(e.Config.Backup.BasePath, "fixture-snapshot/config/app.txt"))
	if string(before) != "old source" {
		t.Fatal("old source absent from snapshot")
	}
	checks, err := backup.VerifyBackup(filepath.Join(e.Config.Backup.BasePath, "fixture-snapshot"))
	if err != nil || backup.HasFailures(checks) {
		t.Fatal("snapshot not verifiable", err)
	}
}
func TestFailedBackupOrBuildNeverReplacesLiveSource(t *testing.T) {
	for _, failure := range []string{"backup", "build", "integrity"} {
		t.Run(failure, func(t *testing.T) {
			e, r, path, src := fixture(t)
			if failure == "backup" {
				e.Backup = func(*config.Config, *db.DB, *db.Project) (*db.BackupRecord, error) {
					return nil, fmt.Errorf("fixture backup failure")
				}
			}
			if failure == "build" {
				r.failBuild = true
			}
			if failure == "integrity" {
				good := e.Backup
				e.Backup = func(c *config.Config, d *db.DB, p *db.Project) (*db.BackupRecord, error) {
					record, err := good(c, d, p)
					if err == nil {
						os.WriteFile(filepath.Join(record.Path, "config/.env"), []byte("corrupt"), 0600)
					}
					return record, err
				}
			}
			if _, err := e.Apply(context.Background(), opts(), archive(t, src), io.Discard); err == nil {
				t.Fatal("accepted failed safety gate")
			}
			code, _ := os.ReadFile(filepath.Join(path, "app.txt"))
			if string(code) != "old source" || contains(r.events, "up") {
				t.Fatal("mutated or started project after failed gate", r.events)
			}
		})
	}
}
func TestReceiverHonorsServerLockAndRejectsSourceSymlinks(t *testing.T) {
	e, _, path, src := fixture(t)
	lock, err := deploy.AcquireLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.Apply(context.Background(), opts(), archive(t, src), io.Discard); err == nil {
		t.Fatal("ignored server lock")
	}
	lock.Release()
	outside := filepath.Join(t.TempDir(), "outside")
	os.WriteFile(outside, []byte("unchanged"), 0600)
	os.Remove(filepath.Join(path, "app.txt"))
	os.Symlink(outside, filepath.Join(path, "app.txt"))
	if _, err = e.Apply(context.Background(), opts(), archive(t, src), io.Discard); err == nil {
		t.Fatal("accepted destination symlink")
	}
	data, _ := os.ReadFile(outside)
	if string(data) != "unchanged" {
		t.Fatal("followed destination link")
	}
}
func TestInitialDeploymentRegistersAccountAndSeedsOnlyExplicitEnv(t *testing.T) {
	e, r, path, src := fixture(t)
	e.DB.DeleteProject("demo")
	os.RemoveAll(path)
	created := false
	e.CreateUser = func(name, path string) error { created = true; return os.MkdirAll(path, 0750) }
	r.expectedSecret = "SECRET=local-development\n"
	e.Backup = func(*config.Config, *db.DB, *db.Project) (*db.BackupRecord, error) {
		t.Fatal("initial empty project should not pretend to have a backup")
		return nil, nil
	}
	result, err := e.Apply(context.Background(), opts(), archive(t, src), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !created || !result.Created || !result.Healthy {
		t.Fatal(result)
	}
	p, err := e.DB.GetProject("demo")
	if err != nil || p.LinuxUser != "fleetdeck-demo" || p.Status != "running" {
		t.Fatal(p, err)
	}
	env, _ := os.ReadFile(filepath.Join(path, ".env"))
	if string(env) != r.expectedSecret {
		t.Fatal("explicit initial environment lost")
	}
}
func TestFailedReadinessIsNotReportedHealthy(t *testing.T) {
	e, r, _, src := fixture(t)
	r.failReady = true
	result, err := e.Apply(context.Background(), opts(), archive(t, src), io.Discard)
	if err == nil || result.Healthy || result.BackupID == "" {
		t.Fatal(result, err)
	}
	p, _ := e.DB.GetProject("demo")
	if p.Status != "failed" {
		t.Fatal(p.Status)
	}
}
func TestBuildIgnoresPrivateReferencesAfterUserNegations(t *testing.T) {
	stage := t.TempDir()
	put(t, stage, "api/Dockerfile", "FROM scratch\n")
	put(t, stage, "api/.dockerignore", "*.txt\n!private.conf\n")
	m := model{Services: map[string]serviceModel{"app": {}}}
	s := m.Services["app"]
	s.Build = &struct {
		Context    string `json:"context"`
		Dockerfile string `json:"dockerfile"`
	}{Context: filepath.Join(stage, "api"), Dockerfile: "Dockerfile"}
	m.Services["app"] = s
	files, err := protectBuildContexts(stage, m, []string{"api/private.conf", "data"})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatal(files)
	}
	data, _ := os.ReadFile(filepath.Join(stage, files[0]))
	if !strings.Contains(string(data), "!private.conf\n") || !strings.HasSuffix(string(data), "/private.conf\n") {
		t.Fatal("lost existing rules or private exclusion", string(data))
	}
}

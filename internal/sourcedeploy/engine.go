package sourcedeploy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/backup"
	"github.com/fleetdeck/fleetdeck/internal/config"
	"github.com/fleetdeck/fleetdeck/internal/db"
	"github.com/fleetdeck/fleetdeck/internal/deploy"
	"github.com/fleetdeck/fleetdeck/internal/edge"
	"github.com/fleetdeck/fleetdeck/internal/project"
)

const Protocol = "1"

type Options struct {
	Name, Domain, Profile string
	Timeout               time.Duration
	NoCache               bool
}
type Result struct {
	Project  string `json:"project"`
	BackupID string `json:"backup_id,omitempty"`
	Created  bool   `json:"created"`
	Healthy  bool   `json:"healthy"`
}
type Runner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}
type LocalRunner struct{}
type boundedOutput struct{ bytes.Buffer }

func (b *boundedOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 16<<20 {
		return 0, fmt.Errorf("command output exceeds limit")
	}
	return b.Buffer.Write(p)
}
func (LocalRunner) Run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	c := exec.CommandContext(ctx, "docker", args...)
	c.Dir = dir
	// The legacy builder ignores Dockerfile-specific secret exclusions.
	c.Env = append(os.Environ(), "DOCKER_BUILDKIT=1")
	var out boundedOutput
	c.Stdout = &out
	c.Stderr = io.Discard
	if err := c.Run(); err != nil {
		return nil, fmt.Errorf("Docker command failed; private command output suppressed: %w", err)
	}
	return out.Bytes(), nil
}

type Engine struct {
	Config      *config.Config
	DB          *db.DB
	Runner      Runner
	CreateUser  func(string, string) error
	Backup      func(*config.Config, *db.DB, *db.Project) (*db.BackupRecord, error)
	PrepareEdge func(context.Context, string, string) error
}

func New(c *config.Config, d *db.DB) *Engine {
	return &Engine{Config: c, DB: d, Runner: LocalRunner{}, CreateUser: project.CreateLinuxUser,
		Backup: func(c *config.Config, d *db.DB, p *db.Project) (*db.BackupRecord, error) {
			return backup.CreateBackup(c, d, p, "snapshot", "source-deploy", backup.Options{})
		},
		PrepareEdge: func(ctx context.Context, dir, file string) error {
			return edge.Prepare(dir, file, edge.LocalRunner{Context: ctx})
		},
	}
}

type serviceModel struct {
	Image string `json:"image"`
	Build *struct {
		Context    string `json:"context"`
		Dockerfile string `json:"dockerfile"`
	} `json:"build"`
	Volumes []struct {
		Type     string `json:"type"`
		Source   string `json:"source"`
		ReadOnly bool   `json:"read_only"`
	} `json:"volumes"`
	Privileged  bool              `json:"privileged"`
	NetworkMode string            `json:"network_mode"`
	PID         string            `json:"pid"`
	IPC         string            `json:"ipc"`
	Devices     []json.RawMessage `json:"devices"`
	CapAdd      []string          `json:"cap_add"`
	SecurityOpt []string          `json:"security_opt"`
}
type model struct {
	Name     string                  `json:"name"`
	Services map[string]serviceModel `json:"services"`
}

func (e *Engine) compose(ctx context.Context, dir, name, file string, args ...string) ([]byte, error) {
	base := []string{"compose", "--project-name", name, "--file", file}
	return e.Runner.Run(ctx, dir, append(base, args...)...)
}
func (e *Engine) model(ctx context.Context, dir, name, file string) (model, error) {
	var m model
	data, err := e.compose(ctx, dir, name, file, "config", "--format", "json")
	if err != nil {
		return m, fmt.Errorf("Compose validation failed; provide required secrets on the server or use --env-file for the initial .env")
	}
	if json.Unmarshal(data, &m) != nil || len(m.Services) == 0 || m.Name != name {
		return m, fmt.Errorf("invalid resolved Compose project")
	}
	return m, nil
}
func validateRuntime(m model, stage string) error {
	for name, s := range m.Services {
		if s.Privileged || s.NetworkMode == "host" || s.PID == "host" || s.IPC == "host" || len(s.Devices) > 0 || len(s.CapAdd) > 0 {
			return fmt.Errorf("service %s requests privileged host access; use a separately reviewed deployment", name)
		}
		for _, option := range s.SecurityOpt {
			if strings.Contains(option, "unconfined") {
				return fmt.Errorf("service %s disables runtime confinement", name)
			}
		}
		for _, v := range s.Volumes {
			if v.Type == "bind" {
				rel, err := filepath.Rel(stage, v.Source)
				if err != nil || !filepath.IsLocal(rel) || rel == "." {
					return fmt.Errorf("service %s bind mount escapes its project", name)
				}
				if filepath.Base(v.Source) == "docker.sock" {
					return fmt.Errorf("service %s requests Docker socket access", name)
				}
			}
		}
		if s.Build != nil {
			rel, err := filepath.Rel(stage, s.Build.Context)
			if err != nil || !filepath.IsLocal(rel) {
				return fmt.Errorf("service %s build context escapes its project", name)
			}
		}
	}
	return nil
}

func (e *Engine) Apply(ctx context.Context, o Options, input io.Reader, progress io.Writer) (Result, error) {
	result := Result{Project: o.Name}
	if err := project.ValidateName(o.Name); err != nil {
		return result, err
	}
	if o.Timeout <= 0 || o.Timeout > time.Hour {
		return result, fmt.Errorf("timeout must be between zero and one hour")
	}
	ctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()
	base, err := filepath.Abs(e.Config.Server.BasePath)
	if err != nil {
		return result, err
	}
	if err = os.MkdirAll(base, 0750); err != nil {
		return result, err
	}
	resolved, err := filepath.EvalSymlinks(base)
	if err != nil || resolved != base {
		return result, fmt.Errorf("server base path must not contain symlinks")
	}
	lockDir := filepath.Join(base, ".source-locks", o.Name)
	if err = os.MkdirAll(lockDir, 0700); err != nil {
		return result, err
	}
	admission, err := deploy.AcquireLock(lockDir)
	if err != nil {
		return result, err
	}
	defer admission.Release()
	path := filepath.Join(base, o.Name)
	projects, err := e.DB.ListProjects()
	if err != nil {
		return result, err
	}
	var p *db.Project
	for _, candidate := range projects {
		if candidate.Name == o.Name {
			p = candidate
			break
		}
	}
	if p == nil {
		if _, err = os.Lstat(path); !os.IsNotExist(err) {
			return result, fmt.Errorf("unregistered target path already exists; import and review it first")
		}
		if len(project.LinuxUserName(o.Name)) > 32 {
			return result, fmt.Errorf("new project names must fit a Linux account (22 characters maximum)")
		}
		if err = e.CreateUser(o.Name, path); err != nil {
			return result, err
		}
		if err = os.MkdirAll(path, 0750); err != nil {
			return result, err
		}
		p = &db.Project{Name: o.Name, Domain: o.Domain, ProjectPath: path, LinuxUser: project.LinuxUserName(o.Name), Template: o.Profile, Source: "created"}
		if err = e.DB.CreateProject(p); err != nil {
			return result, err
		}
		result.Created = true
	} else if filepath.Clean(p.ProjectPath) != path || p.Domain != o.Domain {
		return result, fmt.Errorf("saved server project path or domain differs; review the existing project before deploying")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return result, fmt.Errorf("project path must be a real directory")
	}
	lock, err := deploy.AcquireLock(path)
	if err != nil {
		return result, err
	}
	defer lock.Release()
	stageRoot, err := os.MkdirTemp(base, ".source-stage-*")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(stageRoot)
	stage := filepath.Join(stageRoot, "source")
	if err = os.Mkdir(stage, 0755); err != nil {
		return result, err
	}
	if err = os.Chmod(stage, 0755); err != nil {
		return result, err
	}
	files, err := Extract(input, stage)
	if err != nil {
		return result, err
	}
	compose, private, err := Protected(stage)
	if err != nil {
		return result, err
	}
	for _, override := range []string{"compose.override.yml", "compose.override.yaml", "docker-compose.override.yml", "docker-compose.override.yaml"} {
		if _, err = os.Stat(filepath.Join(stage, override)); err == nil {
			return result, fmt.Errorf("merge Compose overrides into the single reviewed source file first")
		}
		if _, err = os.Stat(filepath.Join(path, override)); err == nil {
			return result, fmt.Errorf("existing Compose overrides require an explicit migration")
		}
	}
	if err = validateComposeIdentity(stage, compose, o.Name); err != nil {
		return result, err
	}
	oldCompose, oldPrivate, oldErr := Protected(path)
	existing := oldErr == nil
	if !existing && !result.Created {
		// A first attempt may have created the account before discovering missing
		// secrets. Only an otherwise empty registered project can be retried here.
		entries, readErr := os.ReadDir(path)
		if readErr != nil {
			return result, readErr
		}
		for _, entry := range entries {
			if entry.Name() != ".fleetdeck.lock" && !sensitive(entry.Name()) {
				return result, fmt.Errorf("existing project has no readable Compose model; refusing an unprotected update")
			}
		}
	}
	if existing {
		if err = validateComposeIdentity(path, oldCompose, o.Name); err != nil {
			return result, err
		}
	}
	if existing && oldCompose != compose {
		return result, fmt.Errorf("keep the existing Compose filename when updating")
	}
	private = append(private, oldPrivate...)
	destination, err := os.OpenRoot(path)
	if err != nil {
		return result, err
	}
	defer destination.Close()
	candidate, err := os.OpenRoot(stage)
	if err != nil {
		return result, err
	}
	defer candidate.Close()
	var updates []string
	for _, name := range files {
		if protected(name, private) {
			if name == ".env" {
				if _, err = destination.Lstat(name); os.IsNotExist(err) {
					updates = append(updates, name)
					continue
				} else if err != nil {
					return result, err
				}
			}
			if err = candidate.Remove(name); err != nil {
				return result, err
			}
			continue
		}
		updates = append(updates, name)
	}
	// Compose env files, secret/config references, and the interpolation .env are
	// copied privately into staging. Persistent data directories are never copied.
	for _, name := range append(private, ".env") {
		info, err := destination.Lstat(name)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return result, err
		}
		if info.IsDir() {
			continue
		}
		if err = copyRootFile(destination, candidate, name, 0600); err != nil {
			return result, fmt.Errorf("preserving private project configuration: %w", err)
		}
	}
	if err = validateComposeEnvironment(stage); err != nil {
		return result, err
	}
	m, err := e.model(ctx, stage, o.Name, compose)
	if err != nil {
		return result, err
	}
	if err = validateRuntime(m, stage); err != nil {
		return result, err
	}
	for _, service := range m.Services {
		if service.Build != nil {
			if _, err = e.Runner.Run(ctx, stage, "buildx", "version"); err != nil {
				return result, fmt.Errorf("source builds require the Docker Buildx plugin and BuildKit")
			}
			break
		}
	}
	ignores, err := protectBuildContexts(stage, m, private)
	if err != nil {
		return result, err
	}
	for _, name := range ignores {
		if !contains(updates, name) {
			updates = append(updates, name)
		}
	}
	sort.Strings(updates)
	for _, name := range updates {
		if err = noLinks(destination, name); err != nil {
			return result, err
		}
		if info, statErr := destination.Lstat(name); statErr == nil && !info.Mode().IsRegular() {
			return result, fmt.Errorf("source destination is not a regular file")
		}
	}
	var record *db.BackupRecord
	var stopped []string
	restart := func() error {
		if len(stopped) == 0 {
			return nil
		}
		recovery, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		_, err := e.compose(recovery, path, o.Name, compose, append([]string{"start"}, stopped...)...)
		if err == nil {
			stopped = nil
		}
		return err
	}
	defer func() {
		if len(stopped) > 0 {
			if err := restart(); err != nil {
				fmt.Fprintln(progress, "Stopped services could not be restarted; operator recovery is required.")
			}
		}
	}()
	if existing {
		old, err := e.model(ctx, path, o.Name, compose)
		if err != nil {
			return result, err
		}
		stopped, err = e.quiesce(ctx, path, o.Name, compose, old)
		if err != nil {
			return result, err
		}
		fmt.Fprintln(progress, "Creating server-side configuration, database and volume snapshot...")
		record, err = e.Backup(e.Config, e.DB, p)
		if err != nil {
			return result, fmt.Errorf("backup failed; no source files replaced: %w", err)
		}
		if err = backup.AddSourceFiles(path, record.Path, append(updates, private...)); err != nil {
			return result, fmt.Errorf("source snapshot failed: %w", err)
		}
		checks, err := backup.VerifyBackup(record.Path)
		if err != nil || len(checks) == 0 || backup.HasFailures(checks) {
			return result, fmt.Errorf("backup verification failed; no source files replaced")
		}
		result.BackupID = record.ID
		fmt.Fprintln(progress, "Verified server snapshot:", record.ID)
		if err = e.saveRuntime(ctx, path, o.Name, compose, record); err != nil {
			return result, err
		}
	}
	fmt.Fprintln(progress, "Building reviewed source with server secrets excluded from build contexts...")
	if _, err = e.compose(ctx, stage, o.Name, compose, "pull", "--ignore-buildable"); err != nil {
		return result, fmt.Errorf("image preparation failed; existing source retained: %w", err)
	}
	build := []string{"build"}
	if o.NoCache {
		build = append(build, "--no-cache")
	}
	if _, err = e.compose(ctx, stage, o.Name, compose, build...); err != nil {
		return result, fmt.Errorf("build failed; existing source retained: %w", err)
	}
	// Backup and build have succeeded. Apply regular files only, preserving all
	// existing private files and all persistent mounts. No volume deletion occurs.
	var written []string
	for _, name := range updates {
		if err = copyRootFile(candidate, destination, name, 0); err != nil {
			if record != nil {
				err = errors.Join(err, restoreSource(destination, record.Path, written))
			}
			return result, fmt.Errorf("source replacement failed: %w", err)
		}
		written = append(written, name)
	}
	if err = e.PrepareEdge(ctx, path, compose); err != nil {
		if record != nil {
			err = errors.Join(err, restoreSource(destination, record.Path, written))
		}
		return result, err
	}
	fmt.Fprintln(progress, "Starting services and waiting for Compose readiness...")
	remaining := int(time.Until(time.Now().Add(o.Timeout)).Seconds())
	if deadline, ok := ctx.Deadline(); ok {
		remaining = int(time.Until(deadline).Seconds())
	}
	if remaining < 1 {
		remaining = 1
	}
	_, err = e.compose(ctx, path, o.Name, compose, "up", "--detach", "--no-build", "--pull", "never", "--wait", "--wait-timeout", fmt.Sprint(remaining))
	stopped = nil // up may have replaced containers; never blindly start old writers now.
	if err != nil {
		_ = e.DB.UpdateProjectStatus(o.Name, "failed")
		return result, fmt.Errorf("readiness failed; snapshot %s retained; review migrations before recovery: %w", result.BackupID, err)
	}
	if err = e.DB.UpdateProjectStatus(o.Name, "running"); err != nil {
		return result, err
	}
	result.Healthy = true
	return result, nil
}

func noLinks(root *os.Root, name string) error {
	if !safeName(name) {
		return fmt.Errorf("invalid project-relative path")
	}
	parts := strings.Split(name, "/")
	for i := range parts {
		info, err := root.Lstat(strings.Join(parts[:i+1], "/"))
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("project source contains a symlink")
		}
		if i < len(parts)-1 && !info.IsDir() {
			return fmt.Errorf("source parent is not a directory")
		}
	}
	return nil
}
func copyRootFile(from, to *os.Root, name string, mode os.FileMode) error {
	if err := noLinks(from, name); err != nil {
		return err
	}
	if err := noLinks(to, name); err != nil {
		return err
	}
	in, err := from.Open(name)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("source is not a regular file")
	}
	if mode == 0 {
		mode = info.Mode().Perm() & 0777
	}
	parts := strings.Split(filepath.ToSlash(filepath.Dir(name)), "/")
	for i := range parts {
		parent := strings.Join(parts[:i+1], "/")
		if parent == "." {
			continue
		}
		if _, err = to.Lstat(parent); os.IsNotExist(err) {
			if err = to.Mkdir(parent, 0755); err != nil {
				return err
			}
			if err = to.Chmod(parent, 0755); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}
	temp := filepath.Join(filepath.Dir(name), ".fleetdeck-copy-"+filepath.Base(name))
	out, err := to.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer to.Remove(temp)
	if err = out.Chmod(mode); err != nil {
		out.Close()
		return err
	}
	_, err = io.Copy(out, in)
	if err == nil {
		err = out.Sync()
	}
	closeErr := out.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return to.Rename(temp, name)
}
func restoreSource(to *os.Root, backupPath string, names []string) error {
	root, err := os.OpenRoot(filepath.Join(backupPath, "config"))
	if err != nil {
		return err
	}
	defer root.Close()
	var failures []error
	for _, name := range names {
		if _, err = root.Lstat(name); os.IsNotExist(err) {
			err = to.Remove(name)
			if os.IsNotExist(err) {
				err = nil
			}
		} else if err == nil {
			err = copyRootFile(root, to, name, 0)
		}
		if err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

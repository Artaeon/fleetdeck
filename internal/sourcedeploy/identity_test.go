package sourcedeploy

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDifferentComposeIdentityCannotTouchLiveProject(t *testing.T) {
	for _, location := range []string{"incoming", "existing"} {
		t.Run(location, func(t *testing.T) {
			e, r, path, src := fixture(t)
			target := src
			if location == "existing" {
				target = path
			}
			put(t, target, "compose.yaml", "name: other-project\nservices: {app: {image: 'example/app:1'}}\n")
			_, err := e.Apply(context.Background(), opts(), archive(t, src), io.Discard)
			if err == nil || !strings.Contains(err.Error(), "Compose name") {
				t.Fatalf("unexpected result: %v", err)
			}
			if len(r.events) != 0 {
				t.Fatal("changed runtime before refusing project mismatch", r.events)
			}
		})
	}
}
func TestComposeOverridesCannotRedirectBackup(t *testing.T) {
	for _, env := range []string{"COMPOSE_FILE=other.yaml\n", "export COMPOSE_PROJECT_NAME=other\n"} {
		e, r, path, src := fixture(t)
		put(t, path, ".env", env)
		_, err := e.Apply(context.Background(), opts(), archive(t, src), io.Discard)
		if err == nil || !strings.Contains(err.Error(), "COMPOSE_") || len(r.events) != 0 {
			t.Fatalf("override not refused: %v %v", err, r.events)
		}
	}
	t.Setenv("COMPOSE_PROJECT_NAME", "other")
	if err := validateComposeEnvironment(t.TempDir()); err == nil {
		t.Fatal("inherited override accepted")
	}
}
func TestMissingBuildxIsRefusedBeforeBackup(t *testing.T) {
	e, r, _, src := fixture(t)
	e.Runner = runnerFunc(func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		if contains(args, "config") {
			return []byte(`{"name":"demo","services":{"app":{"build":{"context":"` + dir + `"}}}}`), nil
		}
		return r.Run(ctx, dir, args...)
	})
	_, err := e.Apply(context.Background(), opts(), archive(t, src), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "Buildx") || len(r.events) != 0 {
		t.Fatalf("legacy builder accepted: %v %v", err, r.events)
	}
}
func TestLocalRunnerRequiresBuildKit(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "docker"), []byte("#!/bin/sh\nprintf '%s' \"$DOCKER_BUILDKIT\"\n"), 0700)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DOCKER_BUILDKIT", "0")
	out, err := (LocalRunner{}).Run(context.Background(), dir, "build")
	if err != nil || string(out) != "1" {
		t.Fatalf("legacy build allowed: %q %v", out, err)
	}
}

type runnerFunc func(context.Context, string, ...string) ([]byte, error)

func (f runnerFunc) Run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	return f(ctx, dir, args...)
}

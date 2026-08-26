package releasejob

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultReleasePreflightRuntimeIsRootOwnedAndOutsideProjectState(t *testing.T) {
	runtime := defaultReleasePreflightRuntime()
	if runtime.directory != defaultReleasePreflightRuntimeDirectory || runtime.ownerUID != 0 {
		t.Fatalf("default runtime = %+v", runtime)
	}
	if !filepath.IsAbs(runtime.directory) || !strings.HasPrefix(runtime.directory, "/run/") {
		t.Fatalf("default runtime directory is not isolated system runtime state: %q", runtime.directory)
	}
}

func TestReleasePreflightRuntimeCreatesPrivateDirectoryOutsideProject(t *testing.T) {
	projectPath := resolvedTemporaryDirectory(t)
	runtimeParent := resolvedTemporaryDirectory(t)
	runtime := releasePreflightRuntime{
		directory: filepath.Join(runtimeParent, "release-preflight"),
		ownerUID:  uint32(os.Geteuid()),
	}

	directory, err := runtime.prepare(projectPath)
	if err != nil {
		t.Fatal(err)
	}
	if directory != runtime.directory || pathInside(projectPath, directory) || pathInside(directory, projectPath) {
		t.Fatalf("runtime directory is not isolated: %q", directory)
	}
	info, err := os.Lstat(directory)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() || info.Mode().Perm() != 0700 {
		t.Fatalf("runtime directory mode = %v", info.Mode())
	}
}

func TestReleasePreflightRuntimeRejectsUnsafePaths(t *testing.T) {
	projectPath := resolvedTemporaryDirectory(t)
	ownerUID := uint32(os.Geteuid())

	t.Run("relative", func(t *testing.T) {
		runtime := releasePreflightRuntime{directory: "relative/runtime", ownerUID: ownerUID}
		if _, err := runtime.prepare(projectPath); err == nil || !strings.Contains(err.Error(), "absolute") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("inside project", func(t *testing.T) {
		directory := filepath.Join(projectPath, ".fleetdeck", "release-preflight")
		runtime := releasePreflightRuntime{directory: directory, ownerUID: ownerUID}
		if _, err := runtime.prepare(projectPath); err == nil || !strings.Contains(err.Error(), "isolated") {
			t.Fatalf("error = %v", err)
		}
		if _, err := os.Stat(directory); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("unsafe in-project runtime directory was created: %v", err)
		}
	})

	t.Run("project inside runtime", func(t *testing.T) {
		directory := filepath.Dir(projectPath)
		runtime := releasePreflightRuntime{directory: directory, ownerUID: ownerUID}
		if _, err := runtime.prepare(projectPath); err == nil || !strings.Contains(err.Error(), "isolated") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("symlink", func(t *testing.T) {
		parent := resolvedTemporaryDirectory(t)
		target := filepath.Join(parent, "target")
		if err := os.Mkdir(target, 0700); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(parent, "runtime-link")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		runtime := releasePreflightRuntime{directory: link, ownerUID: ownerUID}
		if _, err := runtime.prepare(projectPath); err == nil || !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("permissions", func(t *testing.T) {
		parent := resolvedTemporaryDirectory(t)
		directory := filepath.Join(parent, "release-preflight")
		if err := os.Mkdir(directory, 0755); err != nil {
			t.Fatal(err)
		}
		runtime := releasePreflightRuntime{directory: directory, ownerUID: ownerUID}
		if _, err := runtime.prepare(projectPath); err == nil || !strings.Contains(err.Error(), "mode 0700") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("owner", func(t *testing.T) {
		parent := resolvedTemporaryDirectory(t)
		directory := filepath.Join(parent, "release-preflight")
		if err := os.Mkdir(directory, 0700); err != nil {
			t.Fatal(err)
		}
		unexpectedOwner := ownerUID + 1
		if unexpectedOwner == ownerUID {
			unexpectedOwner = ownerUID - 1
		}
		runtime := releasePreflightRuntime{directory: directory, ownerUID: unexpectedOwner}
		if _, err := runtime.prepare(projectPath); err == nil || !strings.Contains(err.Error(), "owner") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("writable ancestor", func(t *testing.T) {
		parent := resolvedTemporaryDirectory(t)
		if err := os.Chmod(parent, 0777); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(parent, 0700)
		directory := filepath.Join(parent, "release-preflight")
		runtime := releasePreflightRuntime{directory: directory, ownerUID: ownerUID}
		if _, err := runtime.prepare(projectPath); err == nil || !strings.Contains(err.Error(), "writable non-sticky") {
			t.Fatalf("error = %v", err)
		}
		if _, err := os.Stat(directory); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("runtime directory was created below an unsafe ancestor: %v", err)
		}
	})

	t.Run("non-directory component", func(t *testing.T) {
		parent := resolvedTemporaryDirectory(t)
		component := filepath.Join(parent, "not-a-directory")
		if err := os.WriteFile(component, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
		runtime := releasePreflightRuntime{
			directory: filepath.Join(component, "release-preflight"),
			ownerUID:  ownerUID,
		}
		if _, err := runtime.prepare(projectPath); err == nil || !strings.Contains(err.Error(), "non-directory") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestComposeRuntimeRejectsUnsafePreflightRuntimeBeforeDocker(t *testing.T) {
	runtime, request, commands, projectPath := composeRuntimeHarness(t)
	resolvedProject, err := filepath.EvalSymlinks(projectPath)
	if err != nil {
		t.Fatal(err)
	}
	runtime.preflightRuntime.directory = filepath.Join(resolvedProject, ".fleetdeck", "release-preflight")

	if _, err := runtime.Preflight(t.Context(), request); err == nil || !strings.Contains(err.Error(), "isolated") {
		t.Fatalf("error = %v", err)
	}
	if len(commands.calls) != 0 {
		t.Fatalf("docker commands ran with an unsafe runtime directory: %v", commands.calls)
	}
}

func resolvedTemporaryDirectory(t *testing.T) string {
	t.Helper()
	directory, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return directory
}

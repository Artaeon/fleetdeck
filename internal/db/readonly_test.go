package db

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryDriverAllowsMemoryDatabase(t *testing.T) {
	c, err := sql.Open(registryDriver, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err = c.Ping(); err != nil {
		t.Fatal(err)
	}
}

func TestReadOnlyRegistryAfterLastWriterCloses(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "registry.db")
	d, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if err = d.CreateProject(&Project{Name: "demo", Domain: "example.invalid", ProjectPath: "/private/demo"}); err != nil {
		t.Fatal(err)
	}
	if err = d.Close(); err != nil {
		t.Fatal(err)
	}
	// A fresh CLI-only server has no dashboard writer keeping these alive.
	for _, suffix := range []string{"", "-wal", "-shm"} {
		i, err := os.Stat(p + suffix)
		if err != nil {
			t.Fatal("registry sidecar missing after last writer closed:", suffix, err)
		}
		if i.Mode().Perm()&0077 != 0 {
			t.Fatal("registry data readable by other users:", suffix)
		}
		if err = os.Chmod(p+suffix, 0400); err != nil {
			t.Fatal(err)
		}
	}
	if err = os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0700) })
	for n := 0; n < 2; n++ {
		r, err := OpenReadOnly(p)
		if err != nil {
			t.Fatal("could not reopen a closed registry on a read-only filesystem:", err)
		}
		projects, err := r.ListProjects()
		if err != nil || len(projects) != 1 || projects[0].Name != "demo" {
			t.Fatal("read failed after writer closed:", err)
		}
		if err = r.UpdateProjectStatus("demo", "stopped"); err == nil {
			t.Fatal("read-only connection allowed writes")
		}
		if err = r.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReadOnlyRegistryNeverCreatesOrWrites(t *testing.T) {
	p := filepath.Join(t.TempDir(), "registry.db")
	if _, err := OpenReadOnly(p); err == nil {
		t.Fatal("created absent registry")
	}
	d, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err = d.CreateProject(&Project{Name: "demo", Domain: "example.invalid", ProjectPath: "/private/demo"}); err != nil {
		t.Fatal(err)
	}
	r, err := OpenReadOnly(p)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	projects, err := r.ListProjects()
	if err != nil || len(projects) != 1 {
		t.Fatal("read failed")
	}
	if err = r.UpdateProjectStatus("demo", "stopped"); err == nil {
		t.Fatal("read-only registry allowed writes")
	}
	if err = d.CreateProject(&Project{Name: "later", Domain: "later.example.invalid", ProjectPath: "/private/later"}); err != nil {
		t.Fatal(err)
	}
	projects, err = r.ListProjects()
	if err != nil || len(projects) != 2 {
		t.Fatal("read-only observer ignored a new committed WAL transaction")
	}
	before, err := os.ReadFile(p + "-wal")
	if err != nil || len(before) == 0 {
		t.Fatal("expected committed WAL frames", err)
	}
	if err = r.Close(); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(p + "-wal")
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("closing the observer modified or checkpointed the writer's WAL", err)
	}
}

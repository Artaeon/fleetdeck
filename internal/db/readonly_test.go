package db

import (
	"path/filepath"
	"testing"
)

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
}

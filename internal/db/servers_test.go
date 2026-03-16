package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	d, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() {
		d.Close()
		os.Remove(dbPath)
	})
	return d
}

func TestCreateAndGetServer(t *testing.T) {
	d := openTestDB(t)

	s := &Server{
		Name:    "prod",
		Host:    "164.68.121.198",
		Port:    "22",
		User:    "root",
		KeyPath: "/home/user/.ssh/id_ed25519",
	}
	if err := d.CreateServer(s); err != nil {
		t.Fatalf("CreateServer: %v", err)
	}

	if s.ID == "" {
		t.Error("expected non-empty ID after create")
	}
	if s.Status != "active" {
		t.Errorf("expected status 'active', got %q", s.Status)
	}

	got, err := d.GetServer("prod")
	if err != nil {
		t.Fatalf("GetServer: %v", err)
	}
	if got.Host != "164.68.121.198" {
		t.Errorf("expected host 164.68.121.198, got %q", got.Host)
	}
	if got.User != "root" {
		t.Errorf("expected user root, got %q", got.User)
	}
	if got.KeyPath != "/home/user/.ssh/id_ed25519" {
		t.Errorf("expected key path, got %q", got.KeyPath)
	}
}

func TestGetServerNotFound(t *testing.T) {
	d := openTestDB(t)

	_, err := d.GetServer("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent server")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestListServers(t *testing.T) {
	d := openTestDB(t)

	// Empty list
	servers, err := d.ListServers()
	if err != nil {
		t.Fatalf("ListServers: %v", err)
	}
	if len(servers) != 0 {
		t.Errorf("expected 0 servers, got %d", len(servers))
	}

	// Add two servers
	d.CreateServer(&Server{Name: "s1", Host: "1.1.1.1", User: "root"})
	d.CreateServer(&Server{Name: "s2", Host: "2.2.2.2", User: "deploy"})

	servers, err = d.ListServers()
	if err != nil {
		t.Fatalf("ListServers: %v", err)
	}
	if len(servers) != 2 {
		t.Errorf("expected 2 servers, got %d", len(servers))
	}
}

func TestUpdateServerStatus(t *testing.T) {
	d := openTestDB(t)

	d.CreateServer(&Server{Name: "s1", Host: "1.1.1.1", User: "root"})

	if err := d.UpdateServerStatus("s1", "unreachable"); err != nil {
		t.Fatalf("UpdateServerStatus: %v", err)
	}

	got, _ := d.GetServer("s1")
	if got.Status != "unreachable" {
		t.Errorf("expected status 'unreachable', got %q", got.Status)
	}
}

func TestUpdateServerStatusNotFound(t *testing.T) {
	d := openTestDB(t)

	err := d.UpdateServerStatus("nonexistent", "active")
	if err == nil {
		t.Fatal("expected error for nonexistent server")
	}
}

func TestDeleteServer(t *testing.T) {
	d := openTestDB(t)

	d.CreateServer(&Server{Name: "s1", Host: "1.1.1.1", User: "root"})

	if err := d.DeleteServer("s1"); err != nil {
		t.Fatalf("DeleteServer: %v", err)
	}

	_, err := d.GetServer("s1")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestDeleteServerNotFound(t *testing.T) {
	d := openTestDB(t)

	err := d.DeleteServer("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent server")
	}
}

func TestDeleteServerWithProjects(t *testing.T) {
	d := openTestDB(t)

	s := &Server{Name: "s1", Host: "1.1.1.1", User: "root"}
	d.CreateServer(s)

	// Create a project linked to this server
	p := &Project{
		Name:        "myapp",
		Domain:      "myapp.com",
		LinuxUser:   "myapp",
		ProjectPath: "/opt/fleetdeck/myapp",
		ServerID:    s.ID,
	}
	d.CreateProject(p)

	err := d.DeleteServer("s1")
	if err == nil {
		t.Fatal("expected error when deleting server with projects")
	}
	if !strings.Contains(err.Error(), "project(s) assigned") {
		t.Errorf("expected 'project(s) assigned' in error, got: %v", err)
	}
}

func TestGetServerByID(t *testing.T) {
	d := openTestDB(t)

	s := &Server{Name: "s1", Host: "1.1.1.1", User: "root"}
	d.CreateServer(s)

	got, err := d.GetServerByID(s.ID)
	if err != nil {
		t.Fatalf("GetServerByID: %v", err)
	}
	if got.Name != "s1" {
		t.Errorf("expected name 's1', got %q", got.Name)
	}
}

func TestServerDefaultPort(t *testing.T) {
	d := openTestDB(t)

	s := &Server{Name: "s1", Host: "1.1.1.1", User: "root"}
	d.CreateServer(s)

	got, _ := d.GetServer("s1")
	if got.Port != "22" {
		t.Errorf("expected default port '22', got %q", got.Port)
	}
}

func TestDuplicateServerName(t *testing.T) {
	d := openTestDB(t)

	d.CreateServer(&Server{Name: "s1", Host: "1.1.1.1", User: "root"})
	err := d.CreateServer(&Server{Name: "s1", Host: "2.2.2.2", User: "deploy"})
	if err == nil {
		t.Fatal("expected error for duplicate server name")
	}
}

func TestProjectServerIDField(t *testing.T) {
	d := openTestDB(t)

	s := &Server{Name: "prod", Host: "1.1.1.1", User: "root"}
	d.CreateServer(s)

	p := &Project{
		Name:        "myapp",
		Domain:      "myapp.com",
		LinuxUser:   "myapp",
		ProjectPath: "/opt/fleetdeck/myapp",
		ServerID:    s.ID,
	}
	d.CreateProject(p)

	got, _ := d.GetProject("myapp")
	if got.ServerID != s.ID {
		t.Errorf("expected server_id %q, got %q", s.ID, got.ServerID)
	}
}

package db

import (
	"path/filepath"
	"testing"
)

func TestServerVersionPersistsAndUnknownClearsOldObservation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inventory.db")
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.CreateServer(&Server{Name: "vps-one", Host: "server.example.com", User: "operator"}); err != nil {
		t.Fatal(err)
	}
	if err := d.UpdateServerVersion("vps-one", "1.2.3", "abcdef1234567"); err != nil {
		t.Fatal(err)
	}
	d.Close()
	d, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	s, err := d.GetServer("vps-one")
	if err != nil || s.FleetDeckVersion != "1.2.3" || !s.VersionCheckedAt.Valid {
		t.Fatal("observation not persisted")
	}
	list, err := d.ListServers()
	if err != nil || len(list) != 1 || list[0].FleetDeckCommit != "abcdef1234567" {
		t.Fatal("inventory lost build commit")
	}
	if err := d.UpdateServerVersion("vps-one", "", ""); err != nil {
		t.Fatal(err)
	}
	s, err = d.GetServerByID(s.ID)
	if err != nil || s.FleetDeckVersion != "" || s.FleetDeckCommit != "" || !s.VersionCheckedAt.Valid {
		t.Fatal("unknown result retained stale version")
	}
	if d.UpdateServerVersion("absent", "1.0.0", "") == nil {
		t.Fatal("missing server accepted")
	}
}

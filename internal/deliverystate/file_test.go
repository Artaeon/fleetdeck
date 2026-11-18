package deliverystate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSizeAndPermissionFailuresPreserveOriginalCheckpoint(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "state")
	if err := Write(path, "original", 32); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, "replacement", 3); err == nil {
		t.Fatal("accepted oversized checkpoint")
	}
	var got string
	if err := Read(path, &got, 32); err != nil || got != "original" {
		t.Fatal("lost checkpoint", err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if Read(link, &got, 32) == nil || Read(path, &got, 3) == nil {
		t.Fatal("accepted symlink or oversized read")
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if Read(path, &got, 32) == nil {
		t.Fatal("accepted public checkpoint")
	}
}

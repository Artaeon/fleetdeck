package cmd

import (
	"archive/tar"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckedSourceIsTheUploadedSnapshot(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte("services: {app: {image: 'example/app:1'}}\n"), 0600)
	os.WriteFile(filepath.Join(dir, "source.txt"), []byte("reviewed source"), 0600)
	f, report, err := checkedSourceArchive(context.Background(), dir, "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if report.Status != "checked" {
		t.Fatal(report.Status)
	}
	info, _ := f.Stat()
	if info.Mode().Perm() != 0600 {
		t.Fatal("source archive is not private")
	}
	os.WriteFile(filepath.Join(dir, "source.txt"), []byte("changed source after scan"), 0600)
	r := tar.NewReader(f)
	found := false
	for {
		h, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Name == "source.txt" {
			b, _ := io.ReadAll(r)
			if string(b) != "reviewed source" {
				t.Fatal("uploaded unchecked changed source")
			}
			found = true
		}
	}
	if !found {
		t.Fatal("missing source")
	}
}

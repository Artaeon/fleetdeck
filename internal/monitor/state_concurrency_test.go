package monitor

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestConcurrentStateWritesRemainPrivateAndComplete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := SaveState(path, []Target{{Name: "fixture"}}, []CheckResult{{Target: Target{Name: "fixture"}, Healthy: true}}); err != nil {
				t.Error(err)
			}
			if _, err := LoadState(path); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("state permissions: %v %v", info, err)
	}
	files, _ := os.ReadDir(filepath.Dir(path))
	if len(files) != 1 {
		t.Fatalf("temporary state files left behind: %v", files)
	}
}

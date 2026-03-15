package monitor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// MonitorState is the on-disk representation of monitor state, used for
// persistence across restarts.
type MonitorState struct {
	Targets   []Target               `json:"targets"`
	Results   map[string]CheckResult `json:"results"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// SaveState marshals the current targets and results to JSON and writes them
// atomically to path (write to a temporary file, then rename).
func SaveState(path string, targets []Target, results []CheckResult) error {
	resultMap := make(map[string]CheckResult, len(results))
	for _, r := range results {
		resultMap[r.Target.Name] = r
	}

	state := MonitorState{
		Targets:   targets,
		Results:   resultMap,
		UpdatedAt: time.Now(),
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling monitor state: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating state directory: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("writing temporary state file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("renaming state file: %w", err)
	}

	return nil
}

// LoadState reads and unmarshals a previously saved monitor state from path.
func LoadState(path string) (*MonitorState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading state file: %w", err)
	}

	var state MonitorState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("unmarshaling state file: %w", err)
	}

	return &state, nil
}

// SaveStateToDisk persists the monitor's current state to its configured
// StatePath. Returns nil if no StatePath is set.
func (m *Monitor) SaveStateToDisk() error {
	if m.StatePath == "" {
		return nil
	}

	results := m.Status()
	return SaveState(m.StatePath, m.targets, results)
}

// Package state manages the expand/collapse state of agent tree sessions.
// Expanded session IDs are persisted to ~/.config/orchard/state.json so the
// user's tree layout is restored on the next launch.
// This package never reads or writes anything under ~/.claude/.
package state

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type file struct {
	Expanded []string `json:"expanded"`
}

// configPath returns the absolute path to the state.json file.
func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "orchard", "state.json"), nil
}

// Load reads the expanded session list from disk.
// Returns an empty map (not an error) when the file does not yet exist.
func Load() (map[string]bool, error) {
	path, err := configPath()
	if err != nil {
		return make(map[string]bool), err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return make(map[string]bool), nil
	}
	if err != nil {
		return make(map[string]bool), err
	}
	var f file
	if err := json.Unmarshal(data, &f); err != nil {
		return make(map[string]bool), err
	}
	m := make(map[string]bool, len(f.Expanded))
	for _, id := range f.Expanded {
		m[id] = true
	}
	return m, nil
}

// Save writes the expanded session set to disk, creating the directory if needed.
func Save(expanded map[string]bool) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	ids := make([]string, 0, len(expanded))
	for id := range expanded {
		ids = append(ids, id)
	}
	data, err := json.Marshal(file{Expanded: ids})
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

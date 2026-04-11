// Package hidden manages the list of session IDs the user has hidden from
// the Orchard agent panel. Hidden state is persisted to
// ~/.config/orchard/hidden.json as a plain JSON array of session ID strings.
// This package never reads or writes anything under ~/.claude/.
package hidden

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func marshalIDs(ids []string) ([]byte, error)        { return json.Marshal(ids) }
func unmarshalIDs(data []byte, ids *[]string) error  { return json.Unmarshal(data, ids) }

// configPath returns the absolute path to the hidden.json file.
func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "orchard", "hidden.json"), nil
}

// Load reads the hidden session list from disk.
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
	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		return make(map[string]bool), err
	}
	m := make(map[string]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m, nil
}

// Save writes the hidden session map to disk, creating the directory if needed.
func Save(hidden map[string]bool) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	ids := make([]string, 0, len(hidden))
	for id := range hidden {
		ids = append(ids, id)
	}
	data, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

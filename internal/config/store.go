package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func configPath(name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "orchard", name), nil
}

func loadSet(name string) (map[string]bool, error) {
	path, err := configPath(name)
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

func saveSet(name string, ids map[string]bool) error {
	path, err := configPath(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	keys := make([]string, 0, len(ids))
	for id := range ids {
		keys = append(keys, id)
	}
	data, err := json.Marshal(keys)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func LoadHidden() (map[string]bool, error)   { return loadSet("hidden.json") }
func SaveHidden(h map[string]bool) error     { return saveSet("hidden.json", h) }
func LoadExpanded() (map[string]bool, error) { return loadSet("state.json") }
func SaveExpanded(e map[string]bool) error   { return saveSet("state.json", e) }

package hidden

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSaveRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hidden.json")

	// Override configPath for the test by writing directly.
	input := map[string]bool{"sess-a": true, "sess-b": true}
	if err := saveToPath(path, input); err != nil {
		t.Fatalf("saveToPath: %v", err)
	}

	got, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("loadFromPath: %v", err)
	}
	if !got["sess-a"] || !got["sess-b"] {
		t.Errorf("expected sess-a and sess-b to be loaded, got %v", got)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 entries, got %d", len(got))
	}
}

func TestLoad_MissingFileReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.json")

	got, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("expected no error for missing file, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestSave_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "hidden.json")

	if err := saveToPath(path, map[string]bool{"x": true}); err != nil {
		t.Fatalf("saveToPath: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file to exist after save: %v", err)
	}
}

// loadFromPath is the testable core of Load.
func loadFromPath(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return make(map[string]bool), nil
	}
	if err != nil {
		return make(map[string]bool), err
	}
	var ids []string
	if err := unmarshalIDs(data, &ids); err != nil {
		return make(map[string]bool), err
	}
	m := make(map[string]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m, nil
}

// saveToPath is the testable core of Save.
func saveToPath(path string, h map[string]bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	ids := make([]string, 0, len(h))
	for id := range h {
		ids = append(ids, id)
	}
	data, err := marshalIDs(ids)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

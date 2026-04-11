package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_FileNotExist(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	got, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestSaveAndLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	expanded := map[string]bool{
		"session:aabbccdd": true,
		"session:11223344": true,
	}
	if err := Save(expanded); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(got) != len(expanded) {
		t.Errorf("expected %d entries, got %d: %v", len(expanded), len(got), got)
	}
	for id := range expanded {
		if !got[id] {
			t.Errorf("expected %q to be in loaded map", id)
		}
	}
}

func TestSaveAndLoad_EmptyMap(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	if err := Save(map[string]bool{}); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	got, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map after saving empty, got %v", got)
	}
}

func TestSave_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	configDir := filepath.Join(dir, ".config", "orchard")
	if _, err := os.Stat(configDir); !os.IsNotExist(err) {
		t.Fatal("expected config dir to not exist yet")
	}

	if err := Save(map[string]bool{"x": true}); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(configDir, "state.json")); err != nil {
		t.Errorf("expected state.json to exist: %v", err)
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	configDir := filepath.Join(dir, ".config", "orchard")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "state.json"), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for invalid JSON, got nil")
	}
	if len(got) != 0 {
		t.Errorf("expected empty map on bad JSON, got %v", got)
	}
}

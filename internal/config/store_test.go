package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadHidden_SaveHidden_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	input := map[string]bool{"sess-a": true, "sess-b": true}
	if err := SaveHidden(input); err != nil {
		t.Fatalf("SaveHidden: %v", err)
	}
	got, err := LoadHidden()
	if err != nil {
		t.Fatalf("LoadHidden: %v", err)
	}
	if len(got) != len(input) {
		t.Errorf("expected %d entries, got %d", len(input), len(got))
	}
	for id := range input {
		if !got[id] {
			t.Errorf("expected %q in loaded map", id)
		}
	}
}

func TestLoadExpanded_SaveExpanded_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	input := map[string]bool{"session:aabbccdd": true, "session:11223344": true}
	if err := SaveExpanded(input); err != nil {
		t.Fatalf("SaveExpanded: %v", err)
	}
	got, err := LoadExpanded()
	if err != nil {
		t.Fatalf("LoadExpanded: %v", err)
	}
	if len(got) != len(input) {
		t.Errorf("expected %d entries, got %d", len(input), len(got))
	}
	for id := range input {
		if !got[id] {
			t.Errorf("expected %q in loaded map", id)
		}
	}
}

func TestLoadHidden_MissingFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	got, err := LoadHidden()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestLoadExpanded_MissingFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	got, err := LoadExpanded()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestSaveHidden_EmptyMap(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if err := SaveHidden(map[string]bool{}); err != nil {
		t.Fatalf("SaveHidden: %v", err)
	}
	got, err := LoadHidden()
	if err != nil {
		t.Fatalf("LoadHidden: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestSaveExpanded_EmptyMap(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if err := SaveExpanded(map[string]bool{}); err != nil {
		t.Fatalf("SaveExpanded: %v", err)
	}
	got, err := LoadExpanded()
	if err != nil {
		t.Fatalf("LoadExpanded: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestSaveHidden_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	configDir := filepath.Join(dir, ".config", "orchard")
	if _, err := os.Stat(configDir); !os.IsNotExist(err) {
		t.Fatal("expected config dir to not exist yet")
	}
	if err := SaveHidden(map[string]bool{"x": true}); err != nil {
		t.Fatalf("SaveHidden: %v", err)
	}
	if _, err := os.Stat(filepath.Join(configDir, "hidden.json")); err != nil {
		t.Errorf("expected hidden.json to exist: %v", err)
	}
}

func TestSaveExpanded_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	configDir := filepath.Join(dir, ".config", "orchard")
	if _, err := os.Stat(configDir); !os.IsNotExist(err) {
		t.Fatal("expected config dir to not exist yet")
	}
	if err := SaveExpanded(map[string]bool{"x": true}); err != nil {
		t.Fatalf("SaveExpanded: %v", err)
	}
	if _, err := os.Stat(filepath.Join(configDir, "state.json")); err != nil {
		t.Errorf("expected state.json to exist: %v", err)
	}
}

func TestLoadHidden_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	configDir := filepath.Join(dir, ".config", "orchard")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "hidden.json"), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := LoadHidden()
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	if len(got) != 0 {
		t.Errorf("expected empty map on bad JSON, got %v", got)
	}
}

func TestLoadExpanded_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	configDir := filepath.Join(dir, ".config", "orchard")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "state.json"), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := LoadExpanded()
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	if len(got) != 0 {
		t.Errorf("expected empty map on bad JSON, got %v", got)
	}
}

package config

import (
	"os"
	"path/filepath"
	"testing"
)

// --- LoadConfig ---

func TestLoadConfig_MissingFile_ReturnsZero(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Budget != 0 {
		t.Errorf("expected zero budget for missing file, got %v", cfg.Budget)
	}
}

func TestLoadConfig_ParsesBudget(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	cfgDir := filepath.Join(dir, ".config", "orchard")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte("budget = 300\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Budget != 300 {
		t.Errorf("expected budget=300, got %v", cfg.Budget)
	}
}

func TestLoadConfig_IgnoresComments(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	cfgDir := filepath.Join(dir, ".config", "orchard")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "# monthly cap\nbudget = 500\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Budget != 500 {
		t.Errorf("expected budget=500, got %v", cfg.Budget)
	}
}

func TestLoadConfig_InvalidBudget_IgnoredNotError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	cfgDir := filepath.Join(dir, ".config", "orchard")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte("budget = notanumber\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Budget != 0 {
		t.Errorf("expected zero budget for invalid value, got %v", cfg.Budget)
	}
}

func TestLoadConfig_ParsesMaxTokens(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	cfgDir := filepath.Join(dir, ".config", "orchard")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "budget = 300\nmax_tokens = 5000000\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MaxTokens != 5_000_000 {
		t.Errorf("expected max_tokens=5000000, got %v", cfg.MaxTokens)
	}
}

func TestLoadConfig_InvalidMaxTokens_IgnoredNotError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	cfgDir := filepath.Join(dir, ".config", "orchard")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte("max_tokens = notanumber\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MaxTokens != 0 {
		t.Errorf("expected zero max_tokens for invalid value, got %v", cfg.MaxTokens)
	}
}

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

func TestLoadConfig_ParsesWatchdogMinutes(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	cfgDir := filepath.Join(dir, ".config", "orchard")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte("watchdog_minutes = 10\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.WatchdogMinutes != 10 {
		t.Errorf("expected watchdog_minutes=10, got %v", cfg.WatchdogMinutes)
	}
}

func TestLoadConfig_ParsesCostAlert(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	cfgDir := filepath.Join(dir, ".config", "orchard")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte("cost_alert = 5.50\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CostAlert != 5.50 {
		t.Errorf("expected cost_alert=5.50, got %v", cfg.CostAlert)
	}
}

func TestLoadConfig_AllFields(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	cfgDir := filepath.Join(dir, ".config", "orchard")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "budget = 300\nmax_tokens = 5000000\nwatchdog_minutes = 10\ncost_alert = 5.5\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Budget != 300 {
		t.Errorf("expected budget=300, got %v", cfg.Budget)
	}
	if cfg.MaxTokens != 5_000_000 {
		t.Errorf("expected max_tokens=5000000, got %v", cfg.MaxTokens)
	}
	if cfg.WatchdogMinutes != 10 {
		t.Errorf("expected watchdog_minutes=10, got %v", cfg.WatchdogMinutes)
	}
	if cfg.CostAlert != 5.5 {
		t.Errorf("expected cost_alert=5.5, got %v", cfg.CostAlert)
	}
}

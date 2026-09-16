package wizard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func fakeKey(key string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key), Alt: false}
}

func fakeWindowSize(w, h int) tea.WindowSizeMsg {
	return tea.WindowSizeMsg{Width: w, Height: h}
}

func TestNewModel_InitialStep(t *testing.T) {
	m := NewModel(7070, "/tmp/settings.json", "/tmp/config.toml")
	if m.step != stepWelcome {
		t.Errorf("expected stepWelcome, got %d", m.step)
	}
	if m.port != 7070 {
		t.Errorf("expected port 7070, got %d", m.port)
	}
	if len(m.fields) != 3 {
		t.Errorf("expected 3 config fields, got %d", len(m.fields))
	}
}

func TestModel_View_WelcomeStep(t *testing.T) {
	m := NewModel(7070, "/tmp/settings.json", "/tmp/config.toml")
	view := m.View()
	if !strings.Contains(view, "orchard init") {
		t.Errorf("expected 'orchard init' in welcome view")
	}
	if !strings.Contains(view, "settings.json") {
		t.Errorf("expected settings.json reference in welcome view")
	}
}

func TestModel_QuitFromWelcome(t *testing.T) {
	m := NewModel(7070, "/tmp/settings.json", "/tmp/config.toml")
	m2, _ := m.Update(fakeKey("q"))
	fm := m2.(Model)
	if !fm.quitting {
		t.Error("expected quitting=true after q")
	}
}

func TestModel_WindowSizeMsg(t *testing.T) {
	m := NewModel(7070, "/tmp/s.json", "/tmp/c.toml")
	m2, _ := m.Update(fakeWindowSize(120, 40))
	fm := m2.(Model)
	if fm.width != 120 || fm.height != 40 {
		t.Errorf("expected 120x40, got %dx%d", fm.width, fm.height)
	}
}

func TestWriteConfig_AllFields(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	m := NewModel(7070, "", cfgPath)
	m.fields[0].value = "50"
	m.fields[1].value = "5"
	m.fields[2].value = "1.5"

	if err := m.writeConfig(); err != nil {
		t.Fatalf("writeConfig: %v", err)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "budget = 50") {
		t.Errorf("expected budget in config, got: %q", content)
	}
	if !strings.Contains(content, "watchdog_minutes = 5") {
		t.Errorf("expected watchdog_minutes in config, got: %q", content)
	}
	if !strings.Contains(content, "cost_alert = 1.5") {
		t.Errorf("expected cost_alert in config, got: %q", content)
	}
}

func TestWriteConfig_EmptyFields_NoFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	m := NewModel(7070, "", cfgPath)
	// Leave all fields empty.

	if err := m.writeConfig(); err != nil {
		t.Fatalf("writeConfig: %v", err)
	}

	// File should NOT be created since nothing was entered.
	if _, err := os.Stat(cfgPath); !os.IsNotExist(err) {
		t.Error("expected no config file when all fields are empty")
	}
}

func TestWriteConfig_CreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "orchard", "config.toml")
	m := NewModel(7070, "", cfgPath)
	m.fields[0].value = "100"

	if err := m.writeConfig(); err != nil {
		t.Fatalf("writeConfig: %v", err)
	}

	if _, err := os.Stat(cfgPath); err != nil {
		t.Errorf("expected config file to exist: %v", err)
	}
}

func TestModel_ConfigStep_TabAdvancesField(t *testing.T) {
	m := NewModel(7070, "", "")
	m.step = stepConfig
	m.activeField = 0

	m2, _ := m.Update(fakeKey("tab"))
	fm := m2.(Model)
	if fm.activeField != 1 {
		t.Errorf("expected activeField=1 after tab, got %d", fm.activeField)
	}
}

func TestModel_ConfigStep_NumberInput(t *testing.T) {
	m := NewModel(7070, "", "")
	m.step = stepConfig
	m.activeField = 0

	m2, _ := m.Update(fakeKey("5"))
	fm := m2.(Model)
	if fm.fields[0].value != "5" {
		t.Errorf("expected field value '5', got %q", fm.fields[0].value)
	}
}

func TestModel_ConfigStep_BackspaceWorks(t *testing.T) {
	m := NewModel(7070, "", "")
	m.step = stepConfig
	m.activeField = 0
	m.fields[0].value = "50"

	m2, _ := m.Update(fakeKey("backspace"))
	fm := m2.(Model)
	if fm.fields[0].value != "5" {
		t.Errorf("expected field value '5' after backspace, got %q", fm.fields[0].value)
	}
}

func TestModel_ConfigStep_SkipWithS(t *testing.T) {
	m := NewModel(7070, "", "")
	m.step = stepConfig

	m2, _ := m.Update(fakeKey("s"))
	fm := m2.(Model)
	if fm.step != stepDone {
		t.Errorf("expected stepDone after s, got %d", fm.step)
	}
}

func TestModel_View_DoneStep(t *testing.T) {
	m := NewModel(7070, "", "")
	m.step = stepDone
	view := m.View()
	if !strings.Contains(view, "Setup complete") {
		t.Errorf("expected 'Setup complete' in done view, got: %q", view[:min(100, len(view))])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

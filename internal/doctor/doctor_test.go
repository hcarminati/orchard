package doctor

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckPort_Available(t *testing.T) {
	// Pick a random available port.
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Skip("cannot listen on random port")
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	c := CheckPort(port)
	if !c.Pass {
		t.Errorf("expected pass for available port %d, got: %s", port, c.Message)
	}
}

func TestCheckPort_InUse(t *testing.T) {
	// Actually bind a port then check it.
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Skip("cannot listen")
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	c := CheckPort(port)
	if c.Pass {
		t.Errorf("expected fail for in-use port %d, got pass", port)
	}
	if c.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestCheckSettingsFile_Missing(t *testing.T) {
	// Override home to a temp dir.
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	c := CheckSettingsFile()
	if c.Pass {
		t.Error("expected fail when settings file missing")
	}
}

func TestCheckSettingsFile_Exists(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	claudeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := CheckSettingsFile()
	if !c.Pass {
		t.Errorf("expected pass when settings file exists, got: %s", c.Message)
	}
}

func TestCheckHooks_NoFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	c := CheckHooks(7070)
	if c.Pass {
		t.Error("expected fail when settings file missing")
	}
}

func TestCheckHooks_NoHooks(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	claudeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := CheckHooks(7070)
	if c.Pass {
		t.Errorf("expected fail when no hooks configured, got: %s", c.Message)
	}
}

func TestCheckHooks_Present(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	claudeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	settings := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PreToolUse": []map[string]string{
				{
					"type":    "command",
					"command": "curl -s -X POST http://localhost:7070/hook -H 'Content-Type: application/json' -d @- || true",
				},
			},
		},
	}
	data, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	c := CheckHooks(7070)
	if !c.Pass {
		t.Errorf("expected pass when hooks present, got: %s", c.Message)
	}
}

func TestCheckProjectsDir_Missing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	c := CheckProjectsDir()
	if c.Pass {
		t.Error("expected fail when projects dir missing")
	}
}

func TestCheckProjectsDir_Exists(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	projectsDir := filepath.Join(dir, ".claude", "projects")
	if err := os.MkdirAll(projectsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	c := CheckProjectsDir()
	if !c.Pass {
		t.Errorf("expected pass when projects dir exists, got: %s", c.Message)
	}
}

func TestCheckGoVersion(t *testing.T) {
	c := CheckGoVersion()
	if !c.Pass {
		t.Error("CheckGoVersion should always pass")
	}
	if c.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestAllPass(t *testing.T) {
	tests := []struct {
		name   string
		checks []Check
		want   bool
	}{
		{"all pass", []Check{{Pass: true}, {Pass: true}}, true},
		{"one fail", []Check{{Pass: true}, {Pass: false}}, false},
		{"all fail", []Check{{Pass: false}, {Pass: false}}, false},
		{"empty", []Check{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AllPass(tt.checks); got != tt.want {
				t.Errorf("AllPass = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckString(t *testing.T) {
	pass := Check{Pass: true, Message: "all good"}
	if got := pass.String(); got != "✓ all good" {
		t.Errorf("want '✓ all good', got %q", got)
	}
	fail := Check{Pass: false, Message: "something broke"}
	if got := fail.String(); got != "✗ something broke" {
		t.Errorf("want '✗ something broke', got %q", got)
	}
}

package setup

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeSettings writes a JSON object to a temp file and returns the path.
func writeSettings(t *testing.T, dir string, v interface{}) string {
	t.Helper()
	path := filepath.Join(dir, "settings.json")
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

// readSettings reads and parses the settings file.
func readSettings(t *testing.T, path string) map[string]json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var v map[string]json.RawMessage
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("parse: %v", err)
	}
	return v
}

func TestRunEmptySettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	// File does not exist yet.

	var buf bytes.Buffer
	results, err := Run(path, Options{Port: 7070, Stdout: &buf})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All five hooks should have been added.
	added := 0
	for _, r := range results {
		if r.Added {
			added++
		}
	}
	if added != len(allHookTypes) {
		t.Errorf("want %d added, got %d", len(allHookTypes), added)
	}

	// Output should contain "Added" lines.
	out := buf.String()
	if !strings.Contains(out, "Added PreToolUse hook") {
		t.Errorf("expected 'Added PreToolUse hook' in output, got: %s", out)
	}

	// File should now exist.
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected settings file to be created: %v", err)
	}
}

func TestRunExistingHooks(t *testing.T) {
	dir := t.TempDir()
	// Pre-populate with a different hook entry.
	initial := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PreToolUse": []map[string]string{
				{"type": "command", "command": "echo hello"},
			},
		},
	}
	path := writeSettings(t, dir, initial)

	var buf bytes.Buffer
	results, err := Run(path, Options{Port: 7070, Stdout: &buf})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// PreToolUse should still be added (existing entry is different).
	var preAdded bool
	for _, r := range results {
		if r.Hook == HookPreToolUse && r.Added {
			preAdded = true
		}
	}
	if !preAdded {
		t.Error("expected PreToolUse to be added alongside existing entry")
	}

	// Verify that the original echo entry is preserved.
	root := readSettings(t, path)
	var hooksObj map[string]json.RawMessage
	if err := json.Unmarshal(root["hooks"], &hooksObj); err != nil {
		t.Fatalf("parse hooks: %v", err)
	}
	var preEntries []HookEntry
	if err := json.Unmarshal(hooksObj["PreToolUse"], &preEntries); err != nil {
		t.Fatalf("parse PreToolUse: %v", err)
	}
	foundEcho := false
	foundOrchard := false
	for _, e := range preEntries {
		if e.Command == "echo hello" {
			foundEcho = true
		}
		if strings.Contains(e.Command, "localhost:7070") {
			foundOrchard = true
		}
	}
	if !foundEcho {
		t.Error("original echo entry was removed")
	}
	if !foundOrchard {
		t.Error("orchard entry was not added")
	}
}

func TestRunAlreadyConfigured(t *testing.T) {
	dir := t.TempDir()
	cmd := hookCommand(7070)
	initial := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PreToolUse": []map[string]string{
				{"type": "command", "command": cmd},
			},
		},
	}
	path := writeSettings(t, dir, initial)

	var buf bytes.Buffer
	results, err := Run(path, Options{Port: 7070, Stdout: &buf})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// PreToolUse should be reported as already present.
	for _, r := range results {
		if r.Hook == HookPreToolUse && r.Added {
			t.Error("PreToolUse should not be added again when already present")
		}
	}

	out := buf.String()
	if !strings.Contains(out, "already present") {
		t.Errorf("expected 'already present' in output, got: %s", out)
	}
}

func TestRunDryRun(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	var buf bytes.Buffer
	_, err := Run(path, Options{Port: 7070, DryRun: true, Stdout: &buf})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// File should NOT be created in dry-run mode.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("expected no file to be written in dry-run mode")
	}

	out := buf.String()
	if !strings.Contains(out, "dry-run") {
		t.Errorf("expected dry-run notice in output, got: %s", out)
	}
}

func TestRunCustomPort(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	var buf bytes.Buffer
	results, err := Run(path, Options{Port: 8080, Stdout: &buf})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All hooks should be added.
	for _, r := range results {
		if !r.Added {
			t.Errorf("hook %s should have been added", r.Hook)
		}
	}

	// File should contain the custom port.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(data), "localhost:8080") {
		t.Error("expected custom port 8080 in settings file")
	}
}

func TestHasOrchardEntry(t *testing.T) {
	tests := []struct {
		name   string
		list   []HookEntry
		port   int
		expect bool
	}{
		{
			name:   "empty list",
			list:   nil,
			port:   7070,
			expect: false,
		},
		{
			name: "matching entry",
			list: []HookEntry{
				{Type: "command", Command: hookCommand(7070)},
			},
			port:   7070,
			expect: true,
		},
		{
			name: "different port",
			list: []HookEntry{
				{Type: "command", Command: hookCommand(8080)},
			},
			port:   7070,
			expect: false,
		},
		{
			name: "unrelated entry",
			list: []HookEntry{
				{Type: "command", Command: "echo hello"},
			},
			port:   7070,
			expect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasOrchardEntry(tt.list, tt.port)
			if got != tt.expect {
				t.Errorf("hasOrchardEntry = %v, want %v", got, tt.expect)
			}
		})
	}
}

func TestParseHooksMalformed(t *testing.T) {
	root := map[string]json.RawMessage{
		"hooks": json.RawMessage(`"not an object"`),
	}
	hooks, err := parseHooks(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hooks) != 0 {
		t.Errorf("expected empty hooks map for malformed input, got %d entries", len(hooks))
	}
}

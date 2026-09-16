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

// readRawSettings reads the settings file and returns the raw hooks object.
func readRawHooks(t *testing.T, path string) map[string][]json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("parse: %v", err)
	}
	hooks, err := parseHooks(root)
	if err != nil {
		t.Fatalf("parse hooks: %v", err)
	}
	return hooks
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

	// File should now exist and contain the http hook URL.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected settings file to be created: %v", err)
	}
	if !strings.Contains(string(data), "localhost:7070") {
		t.Error("expected localhost:7070 in settings file")
	}
	if !strings.Contains(string(data), `"http"`) {
		t.Error("expected http hook type in settings file")
	}
}

func TestRunExistingHooks(t *testing.T) {
	dir := t.TempDir()
	// Pre-populate with an existing hook entry in the new group format.
	initial := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PreToolUse": []map[string]interface{}{
				{
					"hooks": []map[string]string{
						{"type": "command", "command": "echo hello"},
					},
				},
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

	// Verify that the original echo entry is preserved and the orchard entry is added.
	hooks := readRawHooks(t, path)
	preEntries := hooks["PreToolUse"]
	if len(preEntries) != 2 {
		t.Fatalf("expected 2 PreToolUse entries, got %d", len(preEntries))
	}

	joined := ""
	for _, raw := range preEntries {
		joined += string(raw)
	}
	if !strings.Contains(joined, "echo hello") {
		t.Error("original echo entry was removed")
	}
	if !strings.Contains(joined, "localhost:7070") {
		t.Error("orchard entry was not added")
	}
}

func TestRunAlreadyConfigured(t *testing.T) {
	dir := t.TempDir()
	// Pre-populate with the orchard http hook already present.
	initial := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PreToolUse": []map[string]interface{}{
				{
					"hooks": []map[string]string{
						{"type": "http", "url": hookURL(7070)},
					},
				},
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
	makeRaw := func(v interface{}) json.RawMessage {
		b, _ := json.Marshal(v)
		return b
	}

	tests := []struct {
		name   string
		list   []json.RawMessage
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
			name: "matching http entry",
			list: []json.RawMessage{
				makeRaw(orchardGroup(7070)),
			},
			port:   7070,
			expect: true,
		},
		{
			name: "different port",
			list: []json.RawMessage{
				makeRaw(orchardGroup(8080)),
			},
			port:   7070,
			expect: false,
		},
		{
			name: "unrelated entry",
			list: []json.RawMessage{
				makeRaw(HookGroup{Hooks: []HookHandler{{Type: "command", Command: "echo hello"}}}),
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

func TestOrchardGroupFormat(t *testing.T) {
	// Verify the generated group matches the Claude Code 2.x hook format exactly.
	group := orchardGroup(7070)
	b, err := json.Marshal(group)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, `"hooks"`) {
		t.Error("expected 'hooks' key in group JSON")
	}
	if !strings.Contains(s, `"http"`) {
		t.Error("expected 'http' type in group JSON")
	}
	if !strings.Contains(s, "localhost:7070") {
		t.Error("expected localhost:7070 in group JSON")
	}
	// Should NOT have a matcher (omitempty).
	if strings.Contains(s, `"matcher"`) {
		t.Error("unexpected 'matcher' key in group JSON — should be omitted when empty")
	}
}

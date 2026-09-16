// Package setup implements the `orchard setup` subcommand logic.
// It merges Orchard's hook configuration into ~/.claude/settings.json
// without removing or overwriting existing entries.
package setup

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// HookType identifies a Claude Code lifecycle hook.
type HookType string

const (
	HookPreToolUse  HookType = "PreToolUse"
	HookPostToolUse HookType = "PostToolUse"
	HookStop        HookType = "Stop"
	HookSubagentStop HookType = "SubagentStop"
	HookNotification HookType = "Notification"
)

// allHookTypes is the set of hooks Orchard needs to receive events from.
var allHookTypes = []HookType{
	HookPreToolUse,
	HookPostToolUse,
	HookStop,
	HookSubagentStop,
	HookNotification,
}

// HookEntry is the JSON structure for a single Claude Code hook entry.
type HookEntry struct {
	Type    string `json:"type"`    // "command"
	Command string `json:"command"` // shell command to run
}

// hookCommand returns the shell command for posting to orchard's hook server.
func hookCommand(port int) string {
	return fmt.Sprintf("curl -s -X POST http://localhost:%d/hook -H 'Content-Type: application/json' -d @- || true", port)
}

// Result describes what setup did for a single hook type.
type Result struct {
	Hook    HookType
	Added   bool   // true if the entry was newly added
	Message string // human-readable description
}

// Options controls orchard setup behavior.
type Options struct {
	Port    int
	DryRun  bool
	Stdout  io.Writer
}

// Run merges Orchard hook entries into settingsPath and reports results.
// If opts.DryRun is true, no file is written.
// Returns the list of Results (one per hook type) and any fatal error.
func Run(settingsPath string, opts Options) ([]Result, error) {
	port := opts.Port
	if port <= 0 {
		port = 7070
	}
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}

	// Read existing settings, defaulting to empty object if missing.
	var root map[string]json.RawMessage
	data, err := os.ReadFile(settingsPath)
	if os.IsNotExist(err) {
		root = make(map[string]json.RawMessage)
	} else if err != nil {
		return nil, fmt.Errorf("read %s: %w", settingsPath, err)
	} else {
		if err := json.Unmarshal(data, &root); err != nil {
			return nil, fmt.Errorf("parse %s: %w", settingsPath, err)
		}
	}

	// Parse the hooks section; it is an object keyed by hook type.
	hooks, err := parseHooks(root)
	if err != nil {
		return nil, fmt.Errorf("parse hooks: %w", err)
	}

	cmd := hookCommand(port)
	entry := HookEntry{Type: "command", Command: cmd}

	var results []Result
	for _, ht := range allHookTypes {
		list := hooks[string(ht)]
		if hasOrchardEntry(list, port) {
			results = append(results, Result{
				Hook:    ht,
				Added:   false,
				Message: fmt.Sprintf("• %s hook already present", ht),
			})
			continue
		}
		results = append(results, Result{
			Hook:    ht,
			Added:   true,
			Message: fmt.Sprintf("✓ Added %s hook", ht),
		})
		if !opts.DryRun {
			hooks[string(ht)] = append(list, entry)
		}
	}

	// Print results.
	for _, r := range results {
		fmt.Fprintln(opts.Stdout, r.Message)
	}

	if opts.DryRun {
		fmt.Fprintln(opts.Stdout, "(dry-run: no changes written)")
		return results, nil
	}

	// Only write if something changed.
	anyAdded := false
	for _, r := range results {
		if r.Added {
			anyAdded = true
			break
		}
	}
	if !anyAdded {
		return results, nil
	}

	// Serialise hooks back into root.
	hooksEncoded, err := encodeHooks(hooks)
	if err != nil {
		return nil, fmt.Errorf("encode hooks: %w", err)
	}
	root["hooks"] = hooksEncoded

	// Write back.
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal settings: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		return nil, fmt.Errorf("create dir: %w", err)
	}
	if err := os.WriteFile(settingsPath, out, 0o644); err != nil {
		return nil, fmt.Errorf("write %s: %w", settingsPath, err)
	}
	return results, nil
}

// parseHooks extracts the hooks section from root as map[hookTypeName][]HookEntry.
func parseHooks(root map[string]json.RawMessage) (map[string][]HookEntry, error) {
	hooks := make(map[string][]HookEntry)
	raw, ok := root["hooks"]
	if !ok {
		return hooks, nil
	}
	var hooksObj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &hooksObj); err != nil {
		return hooks, nil // if malformed, start fresh
	}
	for k, v := range hooksObj {
		var entries []HookEntry
		if err := json.Unmarshal(v, &entries); err != nil {
			// If the value isn't an array of HookEntry, leave empty — don't corrupt.
			continue
		}
		hooks[k] = entries
	}
	return hooks, nil
}

// encodeHooks serialises hooks back to json.RawMessage for embedding in root.
func encodeHooks(hooks map[string][]HookEntry) (json.RawMessage, error) {
	// Build a map[string]json.RawMessage for the hooks object.
	obj := make(map[string]json.RawMessage, len(hooks))
	for k, v := range hooks {
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		obj[k] = b
	}
	return json.Marshal(obj)
}

// hasOrchardEntry reports whether list already contains an entry that posts to
// orchard on the given port.
func hasOrchardEntry(list []HookEntry, port int) bool {
	marker := fmt.Sprintf("localhost:%d", port)
	for _, e := range list {
		if e.Type == "command" && containsSubstring(e.Command, marker) {
			return true
		}
	}
	return false
}

// containsSubstring reports whether s contains sub (avoids importing strings).
func containsSubstring(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	if len(sub) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// DefaultSettingsPath returns ~/.claude/settings.json.
func DefaultSettingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}

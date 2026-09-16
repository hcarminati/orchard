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
	HookPreToolUse   HookType = "PreToolUse"
	HookPostToolUse  HookType = "PostToolUse"
	HookStop         HookType = "Stop"
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

// HookHandler is the inner hook definition — what actually runs when the event fires.
// Claude Code 2.x supports "http" (POST to a URL) and "command" (shell command).
type HookHandler struct {
	Type    string `json:"type"`              // "http" or "command"
	URL     string `json:"url,omitempty"`     // for type "http"
	Command string `json:"command,omitempty"` // for type "command"
}

// HookGroup is one entry in a hook event's array.
// Claude Code 2.x nests handlers inside a group that carries an optional matcher.
// An empty or absent matcher matches all tools.
type HookGroup struct {
	Matcher string        `json:"matcher,omitempty"`
	Hooks   []HookHandler `json:"hooks"`
}

// hookURL returns the Orchard hook server URL for the given port.
func hookURL(port int) string {
	return fmt.Sprintf("http://localhost:%d/hook", port)
}

// orchardGroup returns the HookGroup that Orchard adds for each hook type.
func orchardGroup(port int) HookGroup {
	return HookGroup{
		Hooks: []HookHandler{
			{Type: "http", URL: hookURL(port)},
		},
	}
}

// Result describes what setup did for a single hook type.
type Result struct {
	Hook    HookType
	Added   bool   // true if the entry was newly added
	Message string // human-readable description
}

// Options controls orchard setup behavior.
type Options struct {
	Port   int
	DryRun bool
	Stdout io.Writer
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

	// Parse the hooks section; entries are kept as raw JSON to preserve
	// whatever format the user has (old or new) for non-Orchard hooks.
	hooks, err := parseHooks(root)
	if err != nil {
		return nil, fmt.Errorf("parse hooks: %w", err)
	}

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
			b, err := json.Marshal(orchardGroup(port))
			if err != nil {
				return nil, fmt.Errorf("marshal hook group: %w", err)
			}
			hooks[string(ht)] = append(list, json.RawMessage(b))
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

// parseHooks extracts the hooks section from root as map[hookTypeName][]json.RawMessage.
// Each element is kept as raw JSON so existing user hooks (old or new format) are
// preserved verbatim.
func parseHooks(root map[string]json.RawMessage) (map[string][]json.RawMessage, error) {
	hooks := make(map[string][]json.RawMessage)
	raw, ok := root["hooks"]
	if !ok {
		return hooks, nil
	}
	var hooksObj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &hooksObj); err != nil {
		return hooks, nil // if malformed, start fresh
	}
	for k, v := range hooksObj {
		var entries []json.RawMessage
		if err := json.Unmarshal(v, &entries); err != nil {
			// If the value isn't an array, leave empty — don't corrupt.
			continue
		}
		hooks[k] = entries
	}
	return hooks, nil
}

// encodeHooks serialises hooks back to json.RawMessage for embedding in root.
func encodeHooks(hooks map[string][]json.RawMessage) (json.RawMessage, error) {
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
// Orchard on the given port. It searches the raw JSON for the host:port string,
// which works for both the old command format and the new http format.
func hasOrchardEntry(list []json.RawMessage, port int) bool {
	marker := fmt.Sprintf("localhost:%d", port)
	for _, raw := range list {
		if containsSubstring(string(raw), marker) {
			return true
		}
	}
	return false
}

// containsSubstring reports whether s contains sub.
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

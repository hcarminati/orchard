// Package doctor implements the `orchard doctor` subcommand.
// It runs a series of health checks and reports results.
package doctor

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Check is the result of a single health check.
type Check struct {
	Name    string // short identifier
	Pass    bool
	Message string // human-readable detail
}

// String formats a check for terminal output: "✓ …" or "✗ …".
func (c Check) String() string {
	if c.Pass {
		return fmt.Sprintf("✓ %s", c.Message)
	}
	return fmt.Sprintf("✗ %s", c.Message)
}

// CheckPort reports whether the given TCP port is available (not in use).
func CheckPort(port int) Check {
	addr := fmt.Sprintf(":%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return Check{
			Name:    "port",
			Pass:    false,
			Message: fmt.Sprintf("port %d is already in use — run `orchard setup --port 8080`", port),
		}
	}
	_ = ln.Close()
	return Check{
		Name:    "port",
		Pass:    true,
		Message: fmt.Sprintf("port %d is available", port),
	}
}

// CheckSettingsFile reports whether ~/.claude/settings.json exists.
func CheckSettingsFile() Check {
	home, err := os.UserHomeDir()
	if err != nil {
		return Check{
			Name:    "settings",
			Pass:    false,
			Message: "could not determine home directory",
		}
	}
	path := filepath.Join(home, ".claude", "settings.json")
	if _, err := os.Stat(path); err != nil {
		return Check{
			Name:    "settings",
			Pass:    false,
			Message: "~/.claude/settings.json not found — run `orchard setup` to create it",
		}
	}
	return Check{
		Name:    "settings",
		Pass:    true,
		Message: "~/.claude/settings.json exists",
	}
}

// CheckHooks reports whether hook config is present in settings.json for the given port.
// It checks for at least one of PreToolUse, PostToolUse, or Stop.
func CheckHooks(port int) Check {
	home, err := os.UserHomeDir()
	if err != nil {
		return Check{
			Name:    "hooks",
			Pass:    false,
			Message: "could not determine home directory",
		}
	}
	path := filepath.Join(home, ".claude", "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return Check{
			Name:    "hooks",
			Pass:    false,
			Message: "~/.claude/settings.json not readable — run `orchard setup`",
		}
	}

	var root struct {
		Hooks map[string][]struct {
			Command string `json:"command"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &root); err != nil {
		return Check{
			Name:    "hooks",
			Pass:    false,
			Message: "settings.json is not valid JSON",
		}
	}

	marker := fmt.Sprintf("localhost:%d", port)
	foundTypes := []string{}
	for _, hookType := range []string{"PreToolUse", "PostToolUse", "Stop", "SubagentStop", "Notification"} {
		for _, e := range root.Hooks[hookType] {
			if containsSubstring(e.Command, marker) {
				foundTypes = append(foundTypes, hookType)
				break
			}
		}
	}
	if len(foundTypes) == 0 {
		return Check{
			Name:    "hooks",
			Pass:    false,
			Message: fmt.Sprintf("no Orchard hooks found in settings.json for port %d — run `orchard setup`", port),
		}
	}
	return Check{
		Name:    "hooks",
		Pass:    true,
		Message: fmt.Sprintf("hook config present: %s", strings.Join(foundTypes, ", ")),
	}
}

// CheckProjectsDir reports whether ~/.claude/projects/ exists and is readable.
func CheckProjectsDir() Check {
	home, err := os.UserHomeDir()
	if err != nil {
		return Check{
			Name:    "projects",
			Pass:    false,
			Message: "could not determine home directory",
		}
	}
	dir := filepath.Join(home, ".claude", "projects")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return Check{
				Name:    "projects",
				Pass:    false,
				Message: "~/.claude/projects/ not found — Claude Code has not been run yet",
			}
		}
		return Check{
			Name:    "projects",
			Pass:    false,
			Message: fmt.Sprintf("~/.claude/projects/ not readable: %v", err),
		}
	}
	return Check{
		Name:    "projects",
		Pass:    true,
		Message: fmt.Sprintf("~/.claude/projects/ exists (%d projects)", len(entries)),
	}
}

// CheckGoVersion reports the current Go runtime version.
func CheckGoVersion() Check {
	version := runtime.Version()
	return Check{
		Name:    "go",
		Pass:    true,
		Message: fmt.Sprintf("Go runtime: %s", version),
	}
}

// RunAll runs every health check for the given port and returns results.
func RunAll(port int) []Check {
	return []Check{
		CheckPort(port),
		CheckSettingsFile(),
		CheckHooks(port),
		CheckProjectsDir(),
		CheckGoVersion(),
	}
}

// AllPass reports whether every check passed.
func AllPass(checks []Check) bool {
	for _, c := range checks {
		if !c.Pass {
			return false
		}
	}
	return true
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

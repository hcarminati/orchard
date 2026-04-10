// Package session reads Claude Code session JSONL files from ~/.claude/projects/
// and converts them into agent nodes that hydrate the initial TUI state.
//
// Claude Code stores one JSONL file per session under a directory whose name
// is derived from the project's working directory with "/" replaced by "-".
// For example, /Users/alice/myapp → -Users-alice-myapp.
package session

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/hcarminati/orchard/internal/agent"
)

// record captures only the fields we care about from each JSONL line.
type record struct {
	Type      string `json:"type"`
	SessionID string `json:"sessionId"`
	CWD       string `json:"cwd"`
}

// Load reads the Claude Code project directory that corresponds to cwd and
// returns one agent.Node per unique session ID found. Nodes are returned with
// StatusDone because they represent historical (already-completed) sessions.
//
// If no matching project directory exists, Load returns nil, nil — this is not
// an error; the TUI will show "waiting for session…" instead.
func Load(cwd string) ([]agent.Node, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return loadFrom(filepath.Join(home, ".claude", "projects"), cwd)
}

// loadFrom is the testable core of Load. base is the projects directory
// (normally ~/.claude/projects); cwd is the working directory to match.
func loadFrom(base, cwd string) ([]agent.Node, error) {
	dir := filepath.Join(base, cwdToDir(cwd))

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no matching project directory — not an error
		}
		return nil, err
	}

	seen := make(map[string]bool)
	var nodes []agent.Node

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}

		ids, err := parseSessionIDs(filepath.Join(dir, entry.Name()))
		if err != nil {
			// Skip unreadable files — don't fail the whole load.
			continue
		}

		for _, id := range ids {
			if seen[id] {
				continue
			}
			seen[id] = true

			name := id
			if len(name) > 8 {
				name = "session:" + name[:8]
			}
			nodes = append(nodes, agent.Node{
				ID:     id,
				Name:   name,
				Status: agent.StatusDone,
			})
		}
	}

	return nodes, nil
}

// cwdToDir converts a filesystem path to the directory name Claude Code uses
// under ~/.claude/projects/ (forward slashes replaced with hyphens).
func cwdToDir(cwd string) string {
	return strings.ReplaceAll(cwd, "/", "-")
}

// parseSessionIDs reads a JSONL file line by line and returns the unique session
// IDs found within. Malformed lines are silently skipped.
func parseSessionIDs(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	seen := make(map[string]bool)
	var ids []string

	// 1 MB line buffer to handle large assistant messages without truncation.
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		var rec record
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			continue // skip malformed lines
		}
		if rec.SessionID != "" && !seen[rec.SessionID] {
			seen[rec.SessionID] = true
			ids = append(ids, rec.SessionID)
		}
	}

	return ids, scanner.Err()
}

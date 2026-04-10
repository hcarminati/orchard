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
	"time"

	"github.com/hcarminati/orchard/internal/agent"
)

// rawMessage holds just the fields we inspect: the role and raw content bytes.
// Content is kept as json.RawMessage because it can be either a string or an
// array of content blocks, depending on the message type.
type rawMessage struct {
	Role       string          `json:"role"`
	ContentRaw json.RawMessage `json:"content"`
}

// contentBlock represents a single block in a message content array.
// We only care about tool_use blocks (to reconstruct PreToolUse events).
type contentBlock struct {
	Type  string          `json:"type"`
	Name  string          `json:"name"`  // populated for tool_use blocks
	Input json.RawMessage `json:"input"` // raw JSON tool parameters for tool_use blocks
}

// record captures the fields we need from each JSONL line.
type record struct {
	Type      string     `json:"type"`
	SessionID string     `json:"sessionId"`
	CWD       string     `json:"cwd"`
	Timestamp string     `json:"timestamp"`
	Message   rawMessage `json:"message"`
}

// Load reads the Claude Code project directory that corresponds to cwd and
// returns one agent.Node per unique session ID found. Each node's Events slice
// is populated with the tool-use history extracted from the conversation, so
// the events tab is populated when Orchard restarts.
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

		fileNodes, err := parseNodes(filepath.Join(dir, entry.Name()))
		if err != nil {
			// Skip unreadable files — don't fail the whole load.
			continue
		}

		for _, n := range fileNodes {
			if seen[n.ID] {
				continue
			}
			seen[n.ID] = true
			nodes = append(nodes, n)
		}
	}

	return nodes, nil
}

// cwdToDir converts a filesystem path to the directory name Claude Code uses
// under ~/.claude/projects/ (forward slashes replaced with hyphens).
func cwdToDir(cwd string) string {
	return strings.ReplaceAll(cwd, "/", "-")
}

// parseNodes reads a JSONL file and returns one node per unique session ID.
// Tool-use events are extracted from assistant message content blocks and
// attached to each node, so the events tab is populated on restart.
// Malformed lines are silently skipped.
func parseNodes(path string) ([]agent.Node, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	nodeMap := make(map[string]*agent.Node)
	var order []string

	// 1 MB line buffer to handle large assistant messages without truncation.
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		var rec record
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			continue // skip malformed lines
		}
		if rec.SessionID == "" {
			continue
		}

		// Register the node the first time we see this session ID.
		if _, exists := nodeMap[rec.SessionID]; !exists {
			name := rec.SessionID
			if len(name) > 8 {
				name = "session:" + name[:8]
			}
			n := &agent.Node{
				ID:     rec.SessionID,
				Name:   name,
				Status: agent.StatusDone,
			}
			nodeMap[rec.SessionID] = n
			order = append(order, rec.SessionID)
		}

		// Only assistant messages carry tool_use blocks.
		if rec.Message.Role != "assistant" {
			continue
		}

		// content can be a string (simple text reply) or an array of blocks.
		// Ignore string content — it carries no tool calls.
		var blocks []contentBlock
		if err := json.Unmarshal(rec.Message.ContentRaw, &blocks); err != nil {
			continue
		}

		ts := parseTimestamp(rec.Timestamp)
		node := nodeMap[rec.SessionID]
		for _, block := range blocks {
			if block.Type == "tool_use" && block.Name != "" {
				node.Events = append(node.Events, agent.Event{
					Type:      "PreToolUse",
					SessionID: rec.SessionID,
					Tool:      block.Name,
					Input:     string(block.Input),
					Timestamp: ts,
				})
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	nodes := make([]agent.Node, 0, len(order))
	for _, id := range order {
		nodes = append(nodes, *nodeMap[id])
	}
	return nodes, nil
}

// parseTimestamp parses an RFC3339 timestamp string, returning the zero time
// on failure rather than propagating an error.
func parseTimestamp(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t.Local()
}

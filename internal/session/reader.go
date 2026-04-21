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

// rawMessage holds just the fields we inspect: the role, model, raw content bytes,
// and token usage. Content is kept as json.RawMessage because it can be either a
// string or an array of content blocks, depending on the message type.
type rawMessage struct {
	Role       string          `json:"role"`
	Model      string          `json:"model"`
	ContentRaw json.RawMessage `json:"content"`
	Usage      agent.Usage     `json:"usage"`
}

// subagentMeta is the content of agent-{agentId}.meta.json files written by
// Claude Code alongside each subagent session.
type subagentMeta struct {
	AgentType   string `json:"agentType"`
	Description string `json:"description"`
}

// subagentRecord captures the fields we need from each line of a subagent JSONL.
// The structure is the same as a parent session record except the subagent's own
// ID is in agentId (not sessionId — that field holds the parent's session ID).
type subagentRecord struct {
	Type      string     `json:"type"`
	AgentID   string     `json:"agentId"`
	SessionID string     `json:"sessionId"` // parent session ID
	Timestamp string     `json:"timestamp"`
	Message   rawMessage `json:"message"`
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
	Type       string     `json:"type"`
	SessionID  string     `json:"sessionId"`
	ParentUUID string     `json:"parentUuid"` // set for subagent sessions
	CWD        string     `json:"cwd"`
	Timestamp  string     `json:"timestamp"`
	Message    rawMessage `json:"message"`
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

		// Check for a subagents/ directory alongside this session file.
		// Claude Code stores each subagent as {sessionId}/subagents/agent-{agentId}.jsonl
		// with a companion agent-{agentId}.meta.json holding agentType and description.
		sessionID := strings.TrimSuffix(entry.Name(), ".jsonl")
		subNodes, _ := loadSubagentNodes(filepath.Join(dir, sessionID, "subagents"), sessionID)
		for _, n := range subNodes {
			if seen[n.ID] {
				continue
			}
			seen[n.ID] = true
			nodes = append(nodes, n)
		}
	}

	// Sort so parents are always before their children. tree.AddNode requires the
	// parent to already exist in the tree when the child is added.
	return topoSort(nodes), nil
}

// topoSort returns nodes ordered so every parent appears before its children.
// Nodes whose ParentID is not in the set (or is empty) are treated as roots.
func topoSort(nodes []agent.Node) []agent.Node {
	inSet := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		inSet[n.ID] = true
	}

	result := make([]agent.Node, 0, len(nodes))
	added := make(map[string]bool, len(nodes))

	// Roots first: nodes with no parent, or whose parent is outside this set.
	for _, n := range nodes {
		if n.ParentID == "" || !inSet[n.ParentID] {
			result = append(result, n)
			added[n.ID] = true
		}
	}

	// Repeatedly emit nodes whose parent is already emitted.
	for len(result) < len(nodes) {
		progress := false
		for _, n := range nodes {
			if added[n.ID] {
				continue
			}
			if added[n.ParentID] {
				result = append(result, n)
				added[n.ID] = true
				progress = true
			}
		}
		if !progress {
			break // remaining nodes form cycles or reference missing parents
		}
	}

	// Append any stragglers (should not happen in practice).
	for _, n := range nodes {
		if !added[n.ID] {
			result = append(result, n)
		}
	}

	return result
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

		// Capture the parent relationship from any record that carries it.
		if rec.ParentUUID != "" && nodeMap[rec.SessionID].ParentID == "" {
			nodeMap[rec.SessionID].ParentID = rec.ParentUUID
		}

		// Only assistant messages carry tool_use blocks, model info, and token usage.
		if rec.Message.Role != "assistant" {
			continue
		}

		node := nodeMap[rec.SessionID]

		// Capture the model from the first assistant message that declares it.
		if node.Model == agent.ModelUnknown && rec.Message.Model != "" {
			node.Model = agent.ParseModel(rec.Message.Model)
		}

		// Accumulate token usage from every assistant message.
		node.Usage.Add(rec.Message.Usage)

		// content can be a string (simple text reply) or an array of blocks.
		// Ignore string content — it carries no tool calls.
		var blocks []contentBlock
		if err := json.Unmarshal(rec.Message.ContentRaw, &blocks); err != nil {
			continue
		}

		ts := parseTimestamp(rec.Timestamp)
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

// loadSubagentNodes reads a {sessionId}/subagents/ directory and returns one
// agent.Node per subagent meta file found. Each node uses the agentId as its ID,
// the parent sessionId as its ParentID, and the agentType from meta.json as its
// display name. Tool-use events are extracted from the companion JSONL file.
// Missing or unreadable entries are silently skipped.
func loadSubagentNodes(subagentsDir, parentID string) ([]agent.Node, error) {
	entries, err := os.ReadDir(subagentsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var nodes []agent.Node
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".meta.json") {
			continue
		}

		// Read meta.json to get agentType ("Explore", "Plan", etc.).
		metaData, err := os.ReadFile(filepath.Join(subagentsDir, name))
		if err != nil {
			continue
		}
		var meta subagentMeta
		if err := json.Unmarshal(metaData, &meta); err != nil || meta.AgentType == "" {
			continue
		}

		// Extract the agentId by stripping "agent-" prefix and ".meta.json" suffix.
		agentID := strings.TrimSuffix(strings.TrimPrefix(name, "agent-"), ".meta.json")

		// Parse the companion JSONL for tool-use events and spawn timestamp.
		jsonlPath := filepath.Join(subagentsDir, "agent-"+agentID+".jsonl")
		events, model, usage := parseSubagentEvents(jsonlPath, agentID)
		spawnedAt := firstJSONLTimestamp(jsonlPath)

		displayName := meta.Description
		if displayName == "" {
			displayName = meta.AgentType
		}
		if len([]rune(displayName)) > 25 {
			displayName = string([]rune(displayName)[:25]) + "…"
		}

		nodes = append(nodes, agent.Node{
			ID:        agentID,
			ParentID:  parentID,
			Name:      displayName,
			Prompt:    meta.Description,
			Model:     model,
			SpawnedAt: spawnedAt,
			Status:    agent.StatusDone,
			Children:  []string{},
			Tools:     []string{},
			Skills:    []string{},
			Events:    events,
			Usage:     usage,
		})
	}
	return nodes, nil
}

// firstJSONLTimestamp returns the timestamp from the first line of a JSONL file.
// Returns the zero time if the file cannot be read or has no timestamp field.
func firstJSONLTimestamp(path string) time.Time {
	f, err := os.Open(path)
	if err != nil {
		return time.Time{}
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	if scanner.Scan() {
		var rec struct {
			Timestamp string `json:"timestamp"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &rec); err == nil {
			return parseTimestamp(rec.Timestamp)
		}
	}
	return time.Time{}
}

// parseSubagentEvents reads a subagent JSONL file and extracts PreToolUse events
// from tool_use blocks in assistant messages, the same way parseNodes does for
// parent sessions. agentID is used as the SessionID on each returned event so
// they are associated with the subagent's own node. The model is extracted from
// the first assistant message that declares it.
func parseSubagentEvents(path, agentID string) ([]agent.Event, agent.Model, agent.Usage) {
	f, err := os.Open(path)
	if err != nil {
		return nil, agent.ModelUnknown, agent.Usage{}
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var events []agent.Event
	model := agent.ModelUnknown
	var usage agent.Usage
	for scanner.Scan() {
		var rec subagentRecord
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			continue
		}
		if rec.Message.Role != "assistant" {
			continue
		}
		if model == agent.ModelUnknown && rec.Message.Model != "" {
			model = agent.ParseModel(rec.Message.Model)
		}
		usage.Add(rec.Message.Usage)
		var blocks []contentBlock
		if err := json.Unmarshal(rec.Message.ContentRaw, &blocks); err != nil {
			continue
		}
		ts := parseTimestamp(rec.Timestamp)
		for _, block := range blocks {
			if block.Type == "tool_use" && block.Name != "" {
				events = append(events, agent.Event{
					Type:      "PreToolUse",
					SessionID: agentID,
					Tool:      block.Name,
					Input:     string(block.Input),
					Timestamp: ts,
				})
			}
		}
	}
	return events, model, usage
}

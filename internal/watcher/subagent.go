// Package watcher scans for subagent JSONL files written by Claude Code and
// tails them in real time. It bridges the gap between hook-based event delivery
// and file-based event recording: subagent tool calls that are written to JSONL
// but not delivered via HTTP hooks are synthesized into agent.Event values and
// fed back into the TUI.
//
// The flow is:
//  1. Parent fires PreToolUse[Agent] → Orchard creates a placeholder child node.
//  2. After 500 ms, Scan() reads ~/.claude/projects/{cwdDir}/{sessionID}/subagents/
//     and returns synthetic events (one per tool call in the subagent JSONL) plus
//     TailTarget descriptors.
//  3. Tail() starts a goroutine that polls each JSONL for new lines and emits
//     further events as the subagent continues working.
//
// Synthesized events carry the subagent's agentID as SessionID and the parent's
// sessionID as ParentID, so agent.Tree.ApplyEvent can claim the placeholder and
// alias the real agentID to it — exactly as it would for a live hook event.
package watcher

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hcarminati/orchard/internal/agent"
)

// SubagentResult groups the synthetic events and tail target for one subagent.
type SubagentResult struct {
	AgentID     string
	Name        string // display name: Description if set, else AgentType
	Description string // raw description from meta.json, used as Prompt
	Events      []agent.Event
	Target      TailTarget
}

// TailTarget describes a JSONL file and the byte offset from which to start
// reading new content (end of what was already loaded by Scan).
type TailTarget struct {
	Path    string // absolute path to agent-{agentID}.jsonl
	AgentID string
	Offset  int64
}

// subagentMeta mirrors the agent-{agentID}.meta.json written by Claude Code.
type subagentMeta struct {
	AgentType   string `json:"agentType"`
	Description string `json:"description"`
}

// subagentRecord mirrors one line of a subagent JSONL file.
type subagentRecord struct {
	AgentID   string     `json:"agentId"`
	SessionID string     `json:"sessionId"` // parent session ID
	Timestamp string     `json:"timestamp"`
	Message   rawMessage `json:"message"`
}

type rawMessage struct {
	Role       string          `json:"role"`
	ContentRaw json.RawMessage `json:"content"`
}

type contentBlock struct {
	Type  string          `json:"type"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// Scan reads subagentsDir (typically ~/.claude/projects/{cwdDir}/{sessionID}/subagents/)
// and returns one SubagentResult per agent-*.meta.json found. Each result includes
// the synthetic agent.Event values extracted from the companion JSONL and a
// TailTarget pointing to the end of what was read, ready for live tailing.
// parentID is the session ID of the parent that spawned the subagent; it is
// embedded in every synthesized event's ParentID so ApplyEvent can claim the
// matching placeholder node.
// Missing or unreadable files are silently skipped.
func Scan(subagentsDir, parentID string) []SubagentResult {
	entries, err := os.ReadDir(subagentsDir)
	if err != nil {
		return nil
	}

	var results []SubagentResult
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".meta.json") {
			continue
		}

		agentID := strings.TrimSuffix(strings.TrimPrefix(name, "agent-"), ".meta.json")
		if agentID == "" {
			continue
		}

		// Read meta.json for display name and description (prompt).
		metaData, err := os.ReadFile(filepath.Join(subagentsDir, name))
		if err != nil {
			continue
		}
		var meta subagentMeta
		if err := json.Unmarshal(metaData, &meta); err != nil || meta.AgentType == "" {
			continue
		}

		displayName := meta.Description
		if displayName == "" {
			displayName = meta.AgentType
		}
		if len([]rune(displayName)) > 25 {
			displayName = string([]rune(displayName)[:25]) + "…"
		}

		jsonlPath := filepath.Join(subagentsDir, "agent-"+agentID+".jsonl")
		events, offset := parseJSONL(jsonlPath, agentID, parentID)

		results = append(results, SubagentResult{
			AgentID:     agentID,
			Name:        displayName,
			Description: meta.Description,
			Events:      events,
			Target:      TailTarget{Path: jsonlPath, AgentID: agentID, Offset: offset},
		})
	}
	return results
}

// parseJSONL reads path and returns synthetic agent.Event values for every
// tool_use block found in assistant messages, plus the byte offset at the end
// of the file so the caller can tail from there. The first record encountered
// (regardless of role) also emits a synthetic PreToolUse with Tool="_subagent_init"
// to ensure the placeholder node is claimed even when no tool calls exist yet.
func parseJSONL(path, agentID, parentID string) ([]agent.Event, int64) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var events []agent.Event
	var offset int64
	claimedPlaceholder := false

	for scanner.Scan() {
		line := scanner.Bytes()
		offset += int64(len(line)) + 1 // +1 for the newline

		var rec subagentRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}

		// Emit one synthetic event with parentID set so ApplyEvent can claim
		// the placeholder that was created by the parent's PreToolUse[Agent] hook.
		// We use a private sentinel tool name so the UI can filter it out if desired.
		if !claimedPlaceholder {
			claimedPlaceholder = true
			events = append(events, agent.Event{
				Type:      "PreToolUse",
				SessionID: agentID,
				ParentID:  parentID,
				Tool:      "_subagent_init",
				Timestamp: parseTimestamp(rec.Timestamp),
			})
		}

		if rec.Message.Role != "assistant" {
			continue
		}
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
					ParentID:  parentID,
					Tool:      block.Name,
					Input:     string(block.Input),
					Timestamp: ts,
				})
			}
		}
	}

	return events, offset
}

const (
	tailPollInterval = 200 * time.Millisecond
	tailMaxSilence   = 10 * time.Minute
)

// Tail starts a background goroutine that reads new lines appended to
// target.Path after target.Offset bytes. Each tool_use block found in
// assistant messages is synthesized into an agent.Event and sent to out.
// The goroutine exits after tailMaxSilence of no new content or when out
// is closed. Sends are non-blocking: events are dropped if out is full.
func Tail(target TailTarget, parentID string, out chan<- agent.Event) {
	go tailJSONL(target, parentID, out)
}

func tailJSONL(target TailTarget, parentID string, out chan<- agent.Event) {
	f, err := os.Open(target.Path)
	if err != nil {
		return
	}
	defer f.Close()

	if _, err := f.Seek(target.Offset, io.SeekStart); err != nil {
		return
	}

	reader := bufio.NewReader(f)
	lastActivity := time.Now()

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if time.Since(lastActivity) > tailMaxSilence {
				return
			}
			time.Sleep(tailPollInterval)
			continue
		}

		lastActivity = time.Now()
		line = strings.TrimRight(line, "\n\r")
		if line == "" {
			continue
		}

		var rec subagentRecord
		if jsonErr := json.Unmarshal([]byte(line), &rec); jsonErr != nil {
			continue
		}
		if rec.Message.Role != "assistant" {
			continue
		}
		var blocks []contentBlock
		if jsonErr := json.Unmarshal(rec.Message.ContentRaw, &blocks); jsonErr != nil {
			continue
		}
		ts := parseTimestamp(rec.Timestamp)
		for _, block := range blocks {
			if block.Type == "tool_use" && block.Name != "" {
				select {
				case out <- agent.Event{
					Type:      "PreToolUse",
					SessionID: target.AgentID,
					ParentID:  parentID,
					Tool:      block.Name,
					Input:     string(block.Input),
					Timestamp: ts,
				}:
				default:
					// Drop if consumer is behind; never block the goroutine.
				}
			}
		}
	}
}

func parseTimestamp(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t.Local()
}

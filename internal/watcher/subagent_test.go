package watcher

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hcarminati/orchard/internal/agent"
)

// writeFile is a test helper that creates a file at path with the given content.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeFile %s: %v", path, err)
	}
}

// metaJSON returns a minimal meta.json body for agentType.
func metaJSON(agentType string) string {
	b, _ := json.Marshal(map[string]string{"agentType": agentType})
	return string(b)
}

// jsonlLine returns one JSONL record with the given role and optional tool_use blocks.
func jsonlLine(t *testing.T, role string, tools []string) string {
	t.Helper()
	var blocks []map[string]any
	for _, name := range tools {
		blocks = append(blocks, map[string]any{
			"type":  "tool_use",
			"name":  name,
			"input": map[string]string{},
		})
	}
	content := any(blocks)
	if blocks == nil {
		content = "hello"
	}
	rec := map[string]any{
		"agentId":   "agent-abc",
		"sessionId": "parent-session",
		"timestamp": "2026-01-01T00:00:00Z",
		"message": map[string]any{
			"role":    role,
			"content": content,
		},
	}
	b, _ := json.Marshal(rec)
	return string(b)
}

// --- Scan tests ---

func TestScan_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	results := Scan(dir, "parent")
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty dir, got %d", len(results))
	}
}

func TestScan_MissingDir(t *testing.T) {
	results := Scan("/nonexistent/path/subagents", "parent")
	if results != nil {
		t.Errorf("expected nil results for missing dir, got %v", results)
	}
}

func TestScan_IgnoresNonMetaFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "agent-abc.jsonl"), "{}")
	writeFile(t, filepath.Join(dir, "something.txt"), "noise")
	results := Scan(dir, "parent")
	if len(results) != 0 {
		t.Errorf("expected 0 results (no .meta.json files), got %d", len(results))
	}
}

func TestScan_EmptyJSONL_ReturnsInitEvent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "agent-abc.meta.json"), metaJSON("Explore"))
	// No JSONL file — Scan should still return one result (no events, offset 0).
	results := Scan(dir, "parent-session")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].AgentID != "abc" {
		t.Errorf("expected agentID 'abc', got %q", results[0].AgentID)
	}
	if len(results[0].Events) != 0 {
		t.Errorf("expected 0 events for empty JSONL, got %d", len(results[0].Events))
	}
}

func TestScan_WithAssistantToolCalls_ReturnsEvents(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "agent-abc.meta.json"), metaJSON("Explore"))

	jsonl := jsonlLine(t, "user", nil) + "\n" +
		jsonlLine(t, "assistant", []string{"Read", "Bash"}) + "\n"
	writeFile(t, filepath.Join(dir, "agent-abc.jsonl"), jsonl)

	results := Scan(dir, "parent-session")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]

	// Expect: 1 init event (from first record) + 2 tool events (Read, Bash).
	if len(r.Events) != 3 {
		t.Fatalf("expected 3 events (init + Read + Bash), got %d: %v", len(r.Events), r.Events)
	}
	if r.Events[0].Tool != "_subagent_init" {
		t.Errorf("expected first event Tool='_subagent_init', got %q", r.Events[0].Tool)
	}
	if r.Events[1].Tool != "Read" {
		t.Errorf("expected second event Tool='Read', got %q", r.Events[1].Tool)
	}
	if r.Events[2].Tool != "Bash" {
		t.Errorf("expected third event Tool='Bash', got %q", r.Events[2].Tool)
	}
	// All events must carry the correct SessionID and ParentID for tree routing.
	for i, e := range r.Events {
		if e.SessionID != "abc" {
			t.Errorf("event[%d].SessionID = %q, want 'abc'", i, e.SessionID)
		}
		if e.ParentID != "parent-session" {
			t.Errorf("event[%d].ParentID = %q, want 'parent-session'", i, e.ParentID)
		}
		if e.Type != "PreToolUse" {
			t.Errorf("event[%d].Type = %q, want 'PreToolUse'", i, e.Type)
		}
	}
}

func TestScan_UserOnlyJSONL_ReturnsInitEventOnly(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "agent-xyz.meta.json"), metaJSON("Plan"))
	writeFile(t, filepath.Join(dir, "agent-xyz.jsonl"), jsonlLine(t, "user", nil)+"\n")

	results := Scan(dir, "parent")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	// Only the init claim event should be present; no tool calls in user message.
	if len(results[0].Events) != 1 {
		t.Errorf("expected 1 event (init only), got %d", len(results[0].Events))
	}
	if results[0].Events[0].Tool != "_subagent_init" {
		t.Errorf("expected _subagent_init, got %q", results[0].Events[0].Tool)
	}
}

func TestScan_TailTargetOffset(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "agent-abc.meta.json"), metaJSON("Explore"))
	content := jsonlLine(t, "assistant", []string{"Read"}) + "\n"
	writeFile(t, filepath.Join(dir, "agent-abc.jsonl"), content)

	results := Scan(dir, "parent")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	// Offset must equal the file size so tailing starts at the end.
	info, _ := os.Stat(filepath.Join(dir, "agent-abc.jsonl"))
	if results[0].Target.Offset != info.Size() {
		t.Errorf("expected offset %d (file size), got %d", info.Size(), results[0].Target.Offset)
	}
}

// --- Tail tests ---

func TestTail_SendsNewEvents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent-abc.jsonl")

	// Write an initial line so the file exists; tail starts after it.
	initial := jsonlLine(t, "user", nil) + "\n"
	writeFile(t, path, initial)

	ch := make(chan agent.Event, 16)
	target := TailTarget{Path: path, AgentID: "abc", Offset: int64(len(initial))}
	Tail(target, "parent", ch)

	// Append a new assistant line with one tool call.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open for append: %v", err)
	}
	newLine := jsonlLine(t, "assistant", []string{"Grep"}) + "\n"
	if _, err := f.WriteString(newLine); err != nil {
		t.Fatalf("write: %v", err)
	}
	f.Close()

	select {
	case e := <-ch:
		if e.Tool != "Grep" {
			t.Errorf("expected Tool='Grep', got %q", e.Tool)
		}
		if e.SessionID != "abc" {
			t.Errorf("expected SessionID='abc', got %q", e.SessionID)
		}
		if e.ParentID != "parent" {
			t.Errorf("expected ParentID='parent', got %q", e.ParentID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for tail event")
	}
}

func TestTail_NonAssistantLinesIgnored(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent-abc.jsonl")
	writeFile(t, path, "")

	ch := make(chan agent.Event, 4)
	target := TailTarget{Path: path, AgentID: "abc", Offset: 0}
	Tail(target, "parent", ch)

	// Append a user message (no tool calls expected).
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	_, err := f.WriteString(jsonlLine(t, "user", nil) + "\n")
	f.Close()
	if err != nil {
		t.Fatal(err)
	}

	// Give the goroutine time to process.
	time.Sleep(500 * time.Millisecond)
	if len(ch) != 0 {
		t.Errorf("expected 0 events for user-only message, got %d", len(ch))
	}
}

// --- parseTimestamp ---

func TestParseTimestamp_ValidRFC3339(t *testing.T) {
	ts := parseTimestamp("2026-01-01T12:00:00Z")
	if ts.IsZero() {
		t.Error("expected non-zero time for valid RFC3339 string")
	}
}

func TestParseTimestamp_Invalid(t *testing.T) {
	ts := parseTimestamp("not-a-timestamp")
	if !ts.IsZero() {
		t.Error("expected zero time for invalid timestamp string")
	}
}

func TestParseJSONL_EmitsTokenUsageEvent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent-abc.jsonl")
	line := `{"agentId":"abc","sessionId":"parent","timestamp":"2026-01-01T00:00:00Z","message":{"role":"assistant","model":"claude-sonnet-4-6","content":[],"usage":{"input_tokens":1000,"output_tokens":250,"cache_read_input_tokens":500}}}` + "\n"
	if err := os.WriteFile(path, []byte(line), 0600); err != nil {
		t.Fatal(err)
	}

	events, _ := parseJSONL(path, "abc", "parent")

	var usageEvents []agent.Event
	for _, e := range events {
		if e.Type == "TokenUsage" {
			usageEvents = append(usageEvents, e)
		}
	}
	if len(usageEvents) != 1 {
		t.Fatalf("expected 1 TokenUsage event, got %d", len(usageEvents))
	}
	u := usageEvents[0].Usage
	if u.InputTokens != 1000 {
		t.Errorf("InputTokens: got %d, want 1000", u.InputTokens)
	}
	if u.OutputTokens != 250 {
		t.Errorf("OutputTokens: got %d, want 250", u.OutputTokens)
	}
	if u.CacheReadInputTokens != 500 {
		t.Errorf("CacheReadInputTokens: got %d, want 500", u.CacheReadInputTokens)
	}
}

func TestParseJSONL_NoTokenUsageEventWhenZeroUsage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent-abc.jsonl")
	line := `{"agentId":"abc","sessionId":"parent","timestamp":"2026-01-01T00:00:00Z","message":{"role":"assistant","content":[]}}` + "\n"
	if err := os.WriteFile(path, []byte(line), 0600); err != nil {
		t.Fatal(err)
	}

	events, _ := parseJSONL(path, "abc", "parent")

	for _, e := range events {
		if e.Type == "TokenUsage" {
			t.Errorf("expected no TokenUsage event when usage is zero, got one: %+v", e)
		}
	}
}


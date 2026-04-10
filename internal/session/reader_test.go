package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hcarminati/orchard/internal/agent"
)

// writeTempJSONL creates a temporary JSONL file with the given lines and returns its path.
func writeTempJSONL(t *testing.T, dir, name string, lines []string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	content := ""
	for _, l := range lines {
		content += l + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write temp JSONL: %v", err)
	}
	return path
}

func TestCwdToDir(t *testing.T) {
	cases := []struct {
		cwd  string
		want string
	}{
		{"/Users/alice/myapp", "-Users-alice-myapp"},
		{"/home/bob/project", "-home-bob-project"},
		{"/single", "-single"},
	}
	for _, c := range cases {
		got := cwdToDir(c.cwd)
		if got != c.want {
			t.Errorf("cwdToDir(%q) = %q, want %q", c.cwd, got, c.want)
		}
	}
}

func TestParseNodes_ValidLines(t *testing.T) {
	dir := t.TempDir()
	path := writeTempJSONL(t, dir, "test.jsonl", []string{
		`{"type":"user","sessionId":"aaa-111","cwd":"/foo"}`,
		`{"type":"assistant","sessionId":"aaa-111","cwd":"/foo"}`,
		`{"type":"user","sessionId":"bbb-222","cwd":"/foo"}`,
	})

	nodes, err := parseNodes(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 unique nodes, got %d: %v", len(nodes), nodes)
	}
}

func TestParseNodes_MalformedLinesSkipped(t *testing.T) {
	dir := t.TempDir()
	path := writeTempJSONL(t, dir, "test.jsonl", []string{
		`{"type":"user","sessionId":"good-id","cwd":"/foo"}`,
		`{bad json`,
		`not json at all`,
		`{"type":"user","sessionId":"good-id2","cwd":"/foo"}`,
	})

	nodes, err := parseNodes(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 2 {
		t.Errorf("expected 2 nodes (malformed lines skipped), got %d: %v", len(nodes), nodes)
	}
}

func TestParseNodes_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := writeTempJSONL(t, dir, "empty.jsonl", nil)

	nodes, err := parseNodes(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes from empty file, got %d", len(nodes))
	}
}

func TestParseNodes_ExtractsToolUseEvents(t *testing.T) {
	dir := t.TempDir()
	path := writeTempJSONL(t, dir, "test.jsonl", []string{
		`{"type":"user","sessionId":"s1","cwd":"/foo","timestamp":"2026-01-01T15:04:05.000Z","message":{"role":"user","content":"hello"}}`,
		`{"type":"assistant","sessionId":"s1","cwd":"/foo","timestamp":"2026-01-01T15:04:06.000Z","message":{"role":"assistant","content":[{"type":"tool_use","name":"Bash"},{"type":"tool_use","name":"Read"}]}}`,
	})

	nodes, err := parseNodes(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if len(nodes[0].Events) != 2 {
		t.Fatalf("expected 2 events, got %d: %v", len(nodes[0].Events), nodes[0].Events)
	}
	if nodes[0].Events[0].Tool != "Bash" {
		t.Errorf("expected first event tool=Bash, got %q", nodes[0].Events[0].Tool)
	}
	if nodes[0].Events[1].Tool != "Read" {
		t.Errorf("expected second event tool=Read, got %q", nodes[0].Events[1].Tool)
	}
	for _, e := range nodes[0].Events {
		if e.Type != "PreToolUse" {
			t.Errorf("expected event type PreToolUse, got %q", e.Type)
		}
		if e.SessionID != "s1" {
			t.Errorf("expected event sessionID=s1, got %q", e.SessionID)
		}
		if e.Timestamp.IsZero() {
			t.Error("expected non-zero timestamp on event")
		}
	}
}

func TestParseNodes_StringContentIgnored(t *testing.T) {
	// Assistant messages with string content (not array) should not produce events.
	dir := t.TempDir()
	path := writeTempJSONL(t, dir, "test.jsonl", []string{
		`{"type":"assistant","sessionId":"s1","cwd":"/foo","timestamp":"2026-01-01T15:04:05.000Z","message":{"role":"assistant","content":"just a text reply"}}`,
	})

	nodes, err := parseNodes(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if len(nodes[0].Events) != 0 {
		t.Errorf("expected 0 events for string content, got %d", len(nodes[0].Events))
	}
}

func TestParseNodes_NonToolUseBlocksIgnored(t *testing.T) {
	// Blocks with type != "tool_use" should not produce events.
	dir := t.TempDir()
	path := writeTempJSONL(t, dir, "test.jsonl", []string{
		`{"type":"assistant","sessionId":"s1","cwd":"/foo","timestamp":"2026-01-01T15:04:05.000Z","message":{"role":"assistant","content":[{"type":"text","text":"hello"},{"type":"tool_use","name":"Glob"}]}}`,
	})

	nodes, err := parseNodes(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes[0].Events) != 1 {
		t.Errorf("expected 1 event (only tool_use block), got %d", len(nodes[0].Events))
	}
	if nodes[0].Events[0].Tool != "Glob" {
		t.Errorf("expected tool=Glob, got %q", nodes[0].Events[0].Tool)
	}
}

func TestLoad_NoMatchingDirectory(t *testing.T) {
	// Point loadFrom at a base dir that has no matching project subdirectory.
	nodes, err := loadFrom(t.TempDir(), "/this/path/does/not/exist")
	if err != nil {
		t.Fatalf("expected nil error for missing directory, got %v", err)
	}
	if nodes != nil {
		t.Errorf("expected nil nodes for missing directory, got %v", nodes)
	}
}

func TestLoad_ReturnsNodesForMatchingSessions(t *testing.T) {
	// Build a fake ~/.claude/projects/<dir> structure in a temp dir.
	base := t.TempDir()
	cwd := "/fake/project"
	projectDir := filepath.Join(base, cwdToDir(cwd))
	if err := os.MkdirAll(projectDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Write two JSONL files, each with a unique session ID.
	writeTempJSONL(t, projectDir, "session-a.jsonl", []string{
		`{"type":"user","sessionId":"aaaa-bbbb-cccc","cwd":"/fake/project"}`,
		`{"type":"assistant","sessionId":"aaaa-bbbb-cccc","cwd":"/fake/project"}`,
	})
	writeTempJSONL(t, projectDir, "session-b.jsonl", []string{
		`{"type":"user","sessionId":"dddd-eeee-ffff","cwd":"/fake/project"}`,
	})

	nodes, err := loadFrom(base, cwd)
	if err != nil {
		t.Fatalf("loadFrom: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d: %v", len(nodes), nodes)
	}
	ids := map[string]bool{nodes[0].ID: true, nodes[1].ID: true}
	if !ids["aaaa-bbbb-cccc"] || !ids["dddd-eeee-ffff"] {
		t.Errorf("unexpected node IDs: %v", nodes)
	}
	for _, n := range nodes {
		if n.Status != agent.StatusDone {
			t.Errorf("node %q: expected StatusDone, got %d", n.ID, n.Status)
		}
	}
}

func TestLoad_RehydrationIdempotent(t *testing.T) {
	// Verify that calling loadFrom twice on the same data produces identical
	// nodes — re-hydration on restart is deterministic.
	base := t.TempDir()
	cwd := "/project"
	projectDir := filepath.Join(base, cwdToDir(cwd))
	if err := os.MkdirAll(projectDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeTempJSONL(t, projectDir, "session.jsonl", []string{
		`{"type":"user","sessionId":"id-one","cwd":"/project"}`,
		`{"type":"assistant","sessionId":"id-one","cwd":"/project"}`,
		`{"type":"user","sessionId":"id-two","cwd":"/project"}`,
	})

	nodes1, err := loadFrom(base, cwd)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	nodes2, err := loadFrom(base, cwd)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}

	if len(nodes1) != len(nodes2) {
		t.Fatalf("re-hydration mismatch: first=%v second=%v", nodes1, nodes2)
	}
	for i := range nodes1 {
		if nodes1[i].ID != nodes2[i].ID {
			t.Errorf("nodes[%d]: first=%q second=%q", i, nodes1[i].ID, nodes2[i].ID)
		}
	}
}

func TestLoad_NonJSONLFilesIgnored(t *testing.T) {
	base := t.TempDir()
	cwd := "/proj"
	projectDir := filepath.Join(base, cwdToDir(cwd))
	if err := os.MkdirAll(projectDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Write a .jsonl file and a .txt file; only the .jsonl should be read.
	writeTempJSONL(t, projectDir, "session.jsonl", []string{
		`{"type":"user","sessionId":"valid-id","cwd":"/proj"}`,
	})
	if err := os.WriteFile(filepath.Join(projectDir, "notes.txt"), []byte("ignore me"), 0600); err != nil {
		t.Fatalf("write txt: %v", err)
	}
	// Also add a subdirectory — should be skipped.
	if err := os.MkdirAll(filepath.Join(projectDir, "subdir"), 0700); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}

	nodes, err := loadFrom(base, cwd)
	if err != nil {
		t.Fatalf("loadFrom: %v", err)
	}
	if len(nodes) != 1 || nodes[0].ID != "valid-id" {
		t.Errorf("expected 1 node with ID 'valid-id', got %v", nodes)
	}
}

func TestLoad_DuplicateIDsAcrossFiles(t *testing.T) {
	base := t.TempDir()
	cwd := "/proj"
	projectDir := filepath.Join(base, cwdToDir(cwd))
	if err := os.MkdirAll(projectDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Same session ID in two different files — should produce one node.
	writeTempJSONL(t, projectDir, "a.jsonl", []string{
		`{"type":"user","sessionId":"shared-id","cwd":"/proj"}`,
	})
	writeTempJSONL(t, projectDir, "b.jsonl", []string{
		`{"type":"assistant","sessionId":"shared-id","cwd":"/proj"}`,
	})

	nodes, err := loadFrom(base, cwd)
	if err != nil {
		t.Fatalf("loadFrom: %v", err)
	}
	if len(nodes) != 1 {
		t.Errorf("expected 1 deduplicated node, got %d: %v", len(nodes), nodes)
	}
}

func TestLoad_NodeNames(t *testing.T) {
	// Verify that long IDs get the "session:XXXXXXXX" name treatment via loadFrom.
	base := t.TempDir()
	cwd := "/myproject"
	projectDir := filepath.Join(base, cwdToDir(cwd))
	if err := os.MkdirAll(projectDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeTempJSONL(t, projectDir, "s.jsonl", []string{
		`{"type":"user","sessionId":"abcdefghijklmnop","cwd":"/myproject"}`,
	})

	nodes, err := loadFrom(base, cwd)
	if err != nil || len(nodes) == 0 {
		t.Fatalf("loadFrom: err=%v nodes=%v", err, nodes)
	}
	if nodes[0].Name != "session:abcdefgh" {
		t.Errorf("expected name 'session:abcdefgh', got %q", nodes[0].Name)
	}
	if nodes[0].Status != agent.StatusDone {
		t.Errorf("expected StatusDone, got %d", nodes[0].Status)
	}
}

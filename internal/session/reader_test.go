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

func TestParseTimestamp_ValidRFC3339(t *testing.T) {
	ts := parseTimestamp("2026-01-01T15:04:05.000Z")
	if ts.IsZero() {
		t.Error("expected non-zero time for valid RFC3339 input")
	}
	if ts.UTC().Year() != 2026 || ts.UTC().Month() != 1 || ts.UTC().Day() != 1 {
		t.Errorf("unexpected date: %v", ts)
	}
}

func TestParseTimestamp_InvalidInput_ReturnsZero(t *testing.T) {
	for _, s := range []string{"", "not-a-date", "2026/01/01"} {
		ts := parseTimestamp(s)
		if !ts.IsZero() {
			t.Errorf("parseTimestamp(%q) = %v, want zero time", s, ts)
		}
	}
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

func TestParseNodes_ParentUUID_SetsParentID(t *testing.T) {
	dir := t.TempDir()
	path := writeTempJSONL(t, dir, "child.jsonl", []string{
		`{"type":"user","sessionId":"child-session","parentUuid":"parent-session","cwd":"/foo","timestamp":"2026-01-01T10:00:00Z","message":{"role":"user","content":"hi"}}`,
	})

	nodes, err := parseNodes(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if nodes[0].ParentID != "parent-session" {
		t.Errorf("expected ParentID='parent-session', got %q", nodes[0].ParentID)
	}
}

func TestTopoSort_ParentsBeforeChildren(t *testing.T) {
	nodes := []agent.Node{
		{ID: "child", ParentID: "parent"},
		{ID: "grandchild", ParentID: "child"},
		{ID: "parent"},
	}

	sorted := topoSort(nodes)

	if len(sorted) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(sorted))
	}

	pos := make(map[string]int, len(sorted))
	for i, n := range sorted {
		pos[n.ID] = i
	}
	if pos["parent"] >= pos["child"] {
		t.Errorf("parent (%d) must come before child (%d)", pos["parent"], pos["child"])
	}
	if pos["child"] >= pos["grandchild"] {
		t.Errorf("child (%d) must come before grandchild (%d)", pos["child"], pos["grandchild"])
	}
}

// --- Subagent loading ---

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestLoadSubagentNodes_PrefersDescriptionOverAgentType(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "agent-abc123.meta.json"), `{"agentType":"Explore","description":"find stuff"}`)
	writeFile(t, filepath.Join(dir, "agent-abc123.jsonl"), "")

	nodes, err := loadSubagentNodes(dir, "parent-session-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	n := nodes[0]
	if n.ID != "abc123" {
		t.Errorf("expected ID 'abc123', got %q", n.ID)
	}
	// Description is preferred over agentType.
	if n.Name != "find stuff" {
		t.Errorf("expected Name 'find stuff', got %q", n.Name)
	}
	if n.ParentID != "parent-session-id" {
		t.Errorf("expected ParentID 'parent-session-id', got %q", n.ParentID)
	}
	if n.Status != agent.StatusDone {
		t.Errorf("expected StatusDone, got %d", n.Status)
	}
}

func TestLoadSubagentNodes_FallsBackToAgentTypeWhenNoDescription(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "agent-abc123.meta.json"), `{"agentType":"Explore","description":""}`)
	writeFile(t, filepath.Join(dir, "agent-abc123.jsonl"), "")

	nodes, err := loadSubagentNodes(dir, "parent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if nodes[0].Name != "Explore" {
		t.Errorf("expected fallback to agentType 'Explore', got %q", nodes[0].Name)
	}
}

func TestLoadSubagentNodes_TruncatesLongDescription(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "agent-abc123.meta.json"), `{"agentType":"Explore","description":"This is a very long description that exceeds the limit"}`)
	writeFile(t, filepath.Join(dir, "agent-abc123.jsonl"), "")

	nodes, err := loadSubagentNodes(dir, "parent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	name := nodes[0].Name
	runes := []rune(name)
	if len(runes) > 26 { // 25 chars + ellipsis
		t.Errorf("expected name truncated to ≤26 runes, got %d: %q", len(runes), name)
	}
	if runes[len(runes)-1] != '…' {
		t.Errorf("expected name to end with ellipsis, got %q", name)
	}
}

func TestLoadSubagentNodes_MissingDirReturnsNil(t *testing.T) {
	nodes, err := loadSubagentNodes("/does/not/exist/subagents", "parent")
	if err != nil {
		t.Fatalf("expected nil error for missing dir, got %v", err)
	}
	if nodes != nil {
		t.Errorf("expected nil nodes, got %v", nodes)
	}
}

func TestLoadSubagentNodes_SkipsEntryWithoutMetaJSON(t *testing.T) {
	dir := t.TempDir()
	// Only the JSONL — no meta.json. Should produce no nodes.
	writeFile(t, filepath.Join(dir, "agent-abc123.jsonl"), "")

	nodes, err := loadSubagentNodes(dir, "parent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes when meta.json is absent, got %d", len(nodes))
	}
}

func TestLoadSubagentNodes_SkipsMalformedMeta(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "agent-abc123.meta.json"), `{bad json`)
	writeFile(t, filepath.Join(dir, "agent-abc123.jsonl"), "")

	nodes, err := loadSubagentNodes(dir, "parent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes for malformed meta, got %d", len(nodes))
	}
}

func TestLoadSubagentNodes_SkipsMetaWithEmptyAgentType(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "agent-abc123.meta.json"), `{"agentType":"","description":"no type"}`)
	writeFile(t, filepath.Join(dir, "agent-abc123.jsonl"), "")

	nodes, err := loadSubagentNodes(dir, "parent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes when agentType is empty, got %d", len(nodes))
	}
}

func TestLoadSubagentNodes_MultipleAgents(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "agent-aaa.meta.json"), `{"agentType":"Explore"}`)
	writeFile(t, filepath.Join(dir, "agent-aaa.jsonl"), "")
	writeFile(t, filepath.Join(dir, "agent-bbb.meta.json"), `{"agentType":"Plan"}`)
	writeFile(t, filepath.Join(dir, "agent-bbb.jsonl"), "")

	nodes, err := loadSubagentNodes(dir, "parent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}
}

func TestParseSubagentEvents_ExtractsToolUseBlocks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent-xyz.jsonl")
	writeFile(t, path, `{"type":"user","agentId":"xyz","sessionId":"parent","timestamp":"2026-01-01T10:00:00Z","message":{"role":"user","content":"do it"}}
{"type":"assistant","agentId":"xyz","sessionId":"parent","timestamp":"2026-01-01T10:00:01Z","message":{"role":"assistant","content":[{"type":"tool_use","name":"Read","input":{"file_path":"/foo.go"}}]}}
{"type":"assistant","agentId":"xyz","sessionId":"parent","timestamp":"2026-01-01T10:00:02Z","message":{"role":"assistant","content":[{"type":"tool_use","name":"Glob","input":{"pattern":"**/*.go"}}]}}
`)

	events, _ := parseSubagentEvents(path, "xyz")
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Tool != "Read" {
		t.Errorf("expected first event Tool=Read, got %q", events[0].Tool)
	}
	if events[1].Tool != "Glob" {
		t.Errorf("expected second event Tool=Glob, got %q", events[1].Tool)
	}
	for _, e := range events {
		if e.SessionID != "xyz" {
			t.Errorf("expected SessionID=xyz, got %q", e.SessionID)
		}
		if e.Type != "PreToolUse" {
			t.Errorf("expected Type=PreToolUse, got %q", e.Type)
		}
	}
}

func TestFirstJSONLTimestamp_ReturnsFirstTimestamp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.jsonl")
	writeFile(t, path,
		`{"type":"user","agentId":"x","timestamp":"2026-04-10T14:00:56.521Z"}`+"\n"+
			`{"type":"assistant","agentId":"x","timestamp":"2026-04-10T14:00:58.000Z"}`+"\n")

	ts := firstJSONLTimestamp(path)
	if ts.IsZero() {
		t.Fatal("expected non-zero timestamp")
	}
	if ts.Minute() != 0 || ts.Second() != 56 {
		t.Errorf("expected timestamp at 14:00:56, got %v", ts)
	}
}

func TestFirstJSONLTimestamp_MissingFileReturnsZero(t *testing.T) {
	ts := firstJSONLTimestamp("/does/not/exist.jsonl")
	if !ts.IsZero() {
		t.Errorf("expected zero time for missing file, got %v", ts)
	}
}

func TestLoadSubagentNodes_SetsPromptAndSpawnedAt(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "agent-abc.meta.json"), `{"agentType":"Explore","description":"search the codebase"}`)
	writeFile(t, filepath.Join(dir, "agent-abc.jsonl"),
		`{"type":"user","agentId":"abc","timestamp":"2026-04-10T14:01:00.000Z"}`+"\n")

	nodes, err := loadSubagentNodes(dir, "parent-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	n := nodes[0]
	if n.Prompt != "search the codebase" {
		t.Errorf("expected Prompt='search the codebase', got %q", n.Prompt)
	}
	if n.SpawnedAt.IsZero() {
		t.Error("expected non-zero SpawnedAt")
	}
}

func TestParseSubagentEvents_MissingFileReturnsNil(t *testing.T) {
	events, _ := parseSubagentEvents("/does/not/exist.jsonl", "xyz")
	if events != nil {
		t.Errorf("expected nil for missing file, got %v", events)
	}
}

func TestLoadFrom_LoadsSubagentNodesUnderParent(t *testing.T) {
	base := t.TempDir()
	cwd := "/myproject"
	projectDir := filepath.Join(base, cwdToDir(cwd))

	// Parent session JSONL (writeFile creates the directory).
	writeFile(t, filepath.Join(projectDir, "sess-aaa.jsonl"),
		`{"type":"user","sessionId":"sess-aaa","cwd":"/myproject"}`+"\n")

	// Subagents directory for that session.
	subDir := filepath.Join(projectDir, "sess-aaa", "subagents")
	writeFile(t, filepath.Join(subDir, "agent-sub1.meta.json"), `{"agentType":"Explore","description":"find files"}`)
	writeFile(t, filepath.Join(subDir, "agent-sub1.jsonl"),
		`{"type":"assistant","agentId":"sub1","sessionId":"sess-aaa","timestamp":"2026-01-01T10:00:00Z","message":{"role":"assistant","content":[{"type":"tool_use","name":"Glob","input":{"pattern":"*.go"}}]}}`)

	nodes, err := loadFrom(base, cwd)
	if err != nil {
		t.Fatalf("loadFrom: %v", err)
	}

	// Should have parent + child.
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes (parent + subagent), got %d: %v", len(nodes), nodes)
	}

	// Find the subagent node.
	var sub *agent.Node
	for i := range nodes {
		if nodes[i].ID == "sub1" {
			sub = &nodes[i]
		}
	}
	if sub == nil {
		t.Fatal("expected subagent node with ID 'sub1'")
	}
	// Description is preferred over agentType.
	if sub.Name != "find files" {
		t.Errorf("expected subagent Name='find files', got %q", sub.Name)
	}
	if sub.ParentID != "sess-aaa" {
		t.Errorf("expected subagent ParentID='sess-aaa', got %q", sub.ParentID)
	}
	if len(sub.Events) != 1 || sub.Events[0].Tool != "Glob" {
		t.Errorf("expected 1 Glob event, got %d events: %v", len(sub.Events), sub.Events)
	}
}

func TestLoadFrom_ParentUUID_EstablishesHierarchy(t *testing.T) {
	base := t.TempDir()
	cwd := "/myproject"
	projectDir := filepath.Join(base, cwdToDir(cwd))
	if err := os.MkdirAll(projectDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Parent session file.
	writeTempJSONL(t, projectDir, "parent.jsonl", []string{
		`{"type":"user","sessionId":"parent-id","cwd":"/myproject","timestamp":"2026-01-01T10:00:00Z","message":{"role":"user","content":"hi"}}`,
	})
	// Child session file referencing the parent.
	writeTempJSONL(t, projectDir, "child.jsonl", []string{
		`{"type":"user","sessionId":"child-id","parentUuid":"parent-id","cwd":"/myproject","timestamp":"2026-01-01T10:01:00Z","message":{"role":"user","content":"sub"}}`,
	})

	nodes, err := loadFrom(base, cwd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}

	// Parent must come before child in the returned slice (topoSort guarantee).
	parentIdx, childIdx := -1, -1
	for i, n := range nodes {
		switch n.ID {
		case "parent-id":
			parentIdx = i
		case "child-id":
			childIdx = i
		}
	}
	if parentIdx < 0 || childIdx < 0 {
		t.Fatal("expected both parent and child nodes")
	}
	if parentIdx >= childIdx {
		t.Errorf("parent (%d) must appear before child (%d) in sorted output", parentIdx, childIdx)
	}
	if nodes[childIdx].ParentID != "parent-id" {
		t.Errorf("expected child.ParentID='parent-id', got %q", nodes[childIdx].ParentID)
	}
}

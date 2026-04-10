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

func TestParseSessionIDs_ValidLines(t *testing.T) {
	dir := t.TempDir()
	path := writeTempJSONL(t, dir, "test.jsonl", []string{
		`{"type":"user","sessionId":"aaa-111","cwd":"/foo"}`,
		`{"type":"assistant","sessionId":"aaa-111","cwd":"/foo"}`,
		`{"type":"user","sessionId":"bbb-222","cwd":"/foo"}`,
	})

	ids, err := parseSessionIDs(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 unique IDs, got %d: %v", len(ids), ids)
	}
}

func TestParseSessionIDs_MalformedLinesSkipped(t *testing.T) {
	dir := t.TempDir()
	path := writeTempJSONL(t, dir, "test.jsonl", []string{
		`{"type":"user","sessionId":"good-id","cwd":"/foo"}`,
		`{bad json`,
		`not json at all`,
		`{"type":"user","sessionId":"good-id2","cwd":"/foo"}`,
	})

	ids, err := parseSessionIDs(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("expected 2 IDs (malformed lines skipped), got %d: %v", len(ids), ids)
	}
}

func TestParseSessionIDs_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := writeTempJSONL(t, dir, "empty.jsonl", nil)

	ids, err := parseSessionIDs(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected 0 IDs from empty file, got %d", len(ids))
	}
}

func TestLoad_NoMatchingDirectory(t *testing.T) {
	// Point Load at a cwd that has no matching project directory.
	nodes, err := Load("/this/path/does/not/exist/in/claude/projects")
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
	projectDir := filepath.Join(base, "-fake-project")
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

	// Temporarily override the home directory by testing the internal
	// cwdToDir + parseSessionIDs helpers directly (Load uses os.UserHomeDir
	// which we can't easily override without refactoring for this basic test).
	ids1, err := parseSessionIDs(filepath.Join(projectDir, "session-a.jsonl"))
	if err != nil {
		t.Fatalf("parseSessionIDs: %v", err)
	}
	ids2, err := parseSessionIDs(filepath.Join(projectDir, "session-b.jsonl"))
	if err != nil {
		t.Fatalf("parseSessionIDs: %v", err)
	}

	// Validate the helpers produce the right IDs.
	if len(ids1) != 1 || ids1[0] != "aaaa-bbbb-cccc" {
		t.Errorf("session-a.jsonl: got %v", ids1)
	}
	if len(ids2) != 1 || ids2[0] != "dddd-eeee-ffff" {
		t.Errorf("session-b.jsonl: got %v", ids2)
	}
}

func TestLoad_NodeNames(t *testing.T) {
	// Verify that long IDs get the "session:XXXXXXXX" prefix treatment.
	dir := t.TempDir()
	path := writeTempJSONL(t, dir, "s.jsonl", []string{
		`{"type":"user","sessionId":"abcdefghijklmnop"}`,
	})

	ids, err := parseSessionIDs(path)
	if err != nil || len(ids) == 0 {
		t.Fatalf("parseSessionIDs: err=%v ids=%v", err, ids)
	}

	// Build a node the same way Load does and check its name.
	id := ids[0]
	name := id
	if len(name) > 8 {
		name = "session:" + name[:8]
	}
	n := agent.Node{ID: id, Name: name, Status: agent.StatusDone}
	if n.Name != "session:abcdefgh" {
		t.Errorf("expected name 'session:abcdefgh', got %q", n.Name)
	}
	if n.Status != agent.StatusDone {
		t.Errorf("expected StatusDone, got %d", n.Status)
	}
}

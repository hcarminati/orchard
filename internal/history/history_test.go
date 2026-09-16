package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeJSONL writes a series of records as newline-delimited JSON to path.
func writeJSONL(t *testing.T, path string, records []map[string]any) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, rec := range records {
		if err := enc.Encode(rec); err != nil {
			t.Fatal(err)
		}
	}
}

// makeRecord returns a minimal JSONL record map.
func makeRecord(sessionID, ts, role, content string) map[string]any {
	rec := map[string]any{
		"sessionId": sessionID,
		"timestamp": ts,
	}
	if role != "" {
		rec["message"] = map[string]any{
			"role":    role,
			"content": content,
		}
	}
	return rec
}

const ts1 = "2026-01-15T10:00:00Z"
const ts2 = "2026-01-15T10:05:00Z"
const ts3 = "2026-01-16T09:00:00Z"

func TestList_EmptyDirReturnsNil(t *testing.T) {
	dir := t.TempDir()
	sessions, err := List(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sessions != nil {
		t.Errorf("expected nil, got %v", sessions)
	}
}

func TestList_MissingDirReturnsNil(t *testing.T) {
	sessions, err := List("/does/not/exist/xyzzy")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sessions != nil {
		t.Errorf("expected nil, got %v", sessions)
	}
}

func TestList_SingleFile_BasicMeta(t *testing.T) {
	dir := t.TempDir()
	writeJSONL(t, filepath.Join(dir, "abc123.jsonl"), []map[string]any{
		makeRecord("abc123", ts1, "", ""),
		makeRecord("abc123", ts2, "assistant", "hello"),
	})

	sessions, err := List(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	s := sessions[0]
	if s.ID != "abc123" {
		t.Errorf("ID: got %q, want %q", s.ID, "abc123")
	}
	if s.EventCount != 2 {
		t.Errorf("EventCount: got %d, want 2", s.EventCount)
	}
	if s.StartTime.IsZero() {
		t.Error("StartTime should not be zero")
	}
}

func TestList_SortedNewestFirst(t *testing.T) {
	dir := t.TempDir()
	writeJSONL(t, filepath.Join(dir, "older.jsonl"), []map[string]any{
		makeRecord("older", ts1, "", ""),
	})
	writeJSONL(t, filepath.Join(dir, "newer.jsonl"), []map[string]any{
		makeRecord("newer", ts3, "", ""),
	})

	sessions, err := List(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
	if sessions[0].ID != "newer" {
		t.Errorf("expected newest first, got %q first", sessions[0].ID)
	}
}

func TestList_SnippetExtracted_StringContent(t *testing.T) {
	dir := t.TempDir()
	writeJSONL(t, filepath.Join(dir, "sess.jsonl"), []map[string]any{
		{"sessionId": "sess", "timestamp": ts1, "message": map[string]any{
			"role":    "user",
			"content": "what does this code do?",
		}},
	})

	sessions, _ := List(dir)
	if len(sessions) != 1 {
		t.Fatal("expected 1 session")
	}
	if sessions[0].Snippet != "what does this code do?" {
		t.Errorf("Snippet: got %q", sessions[0].Snippet)
	}
}

func TestList_SnippetExtracted_ArrayContent(t *testing.T) {
	dir := t.TempDir()
	content, _ := json.Marshal([]map[string]any{
		{"type": "text", "text": "explain this function"},
		{"type": "tool_result", "content": "ignored"},
	})
	writeJSONL(t, filepath.Join(dir, "sess.jsonl"), []map[string]any{
		{"sessionId": "sess", "timestamp": ts1, "message": map[string]any{
			"role":    "user",
			"content": json.RawMessage(content),
		}},
	})

	sessions, _ := List(dir)
	if len(sessions) != 1 {
		t.Fatal("expected 1 session")
	}
	if sessions[0].Snippet != "explain this function" {
		t.Errorf("Snippet: got %q", sessions[0].Snippet)
	}
}

func TestList_SnippetTruncatedAt60Runes(t *testing.T) {
	dir := t.TempDir()
	long := "this is a very long prompt that definitely exceeds the sixty rune limit for snippets"
	writeJSONL(t, filepath.Join(dir, "sess.jsonl"), []map[string]any{
		{"sessionId": "sess", "timestamp": ts1, "message": map[string]any{
			"role":    "user",
			"content": long,
		}},
	})

	sessions, _ := List(dir)
	if len(sessions) != 1 {
		t.Fatal("expected 1 session")
	}
	runes := []rune(sessions[0].Snippet)
	if len(runes) > 60 {
		t.Errorf("Snippet should be ≤60 runes, got %d", len(runes))
	}
}

func TestList_NonJSONLFilesIgnored(t *testing.T) {
	dir := t.TempDir()
	// Write a non-jsonl file and a directory — both should be ignored.
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "subdir"), 0755); err != nil {
		t.Fatal(err)
	}
	writeJSONL(t, filepath.Join(dir, "real.jsonl"), []map[string]any{
		makeRecord("real", ts1, "", ""),
	})

	sessions, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Errorf("expected 1 session (ignoring .txt and subdir), got %d", len(sessions))
	}
	if sessions[0].ID != "real" {
		t.Errorf("unexpected ID: %q", sessions[0].ID)
	}
}

func TestList_EndTimeDiffersFromStart(t *testing.T) {
	dir := t.TempDir()
	writeJSONL(t, filepath.Join(dir, "sess.jsonl"), []map[string]any{
		makeRecord("sess", ts1, "", ""),
		makeRecord("sess", ts2, "", ""),
	})

	sessions, _ := List(dir)
	if len(sessions) != 1 {
		t.Fatal("expected 1 session")
	}
	s := sessions[0]
	if !s.EndTime.After(s.StartTime) {
		t.Errorf("EndTime %v should be after StartTime %v", s.EndTime, s.StartTime)
	}
}

func TestList_MalformedLinesSkipped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sess.jsonl")
	content := `{"sessionId":"sess","timestamp":"` + ts1 + `"}
not valid json at all
{"sessionId":"sess","timestamp":"` + ts2 + `"}
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	sessions, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].EventCount != 2 {
		t.Errorf("EventCount: got %d, want 2 (malformed line skipped)", sessions[0].EventCount)
	}
}

func TestCwdToDir(t *testing.T) {
	tests := []struct {
		cwd  string
		want string
	}{
		{"/Users/alice/myapp", "-Users-alice-myapp"},
		{"/foo/bar/baz", "-foo-bar-baz"},
		{"relative", "relative"},
	}
	for _, tc := range tests {
		got := cwdToDir(tc.cwd)
		if got != tc.want {
			t.Errorf("cwdToDir(%q) = %q, want %q", tc.cwd, got, tc.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		max      int
		wantLen  int
		wantNoNL bool
	}{
		{"hello world", 60, 11, true},
		{"abc\ndef", 60, 7, true}, // newlines replaced with spaces, no truncation needed for short
		{string(make([]rune, 100)), 60, 60, false},
	}
	for _, tc := range tests {
		got := truncate(tc.input, tc.max)
		if len([]rune(got)) > tc.max {
			t.Errorf("truncate: result too long: %d runes", len([]rune(got)))
		}
		if tc.wantNoNL && got != "" {
			// just check no literal newline
			for _, r := range got {
				if r == '\n' {
					t.Errorf("truncate: result contains newline")
				}
			}
		}
	}
}

func TestFormatDur(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{45 * time.Second, "45s"},
		{60 * time.Second, "1m"},
		{90 * time.Second, "1m30s"},
		{2*time.Minute + 5*time.Second, "2m05s"},
	}
	for _, tc := range tests {
		got := formatDur(tc.d)
		if got != tc.want {
			t.Errorf("formatDur(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestZeroTimeSessionsSortedLast(t *testing.T) {
	dir := t.TempDir()
	// File with no valid timestamps
	if err := os.WriteFile(filepath.Join(dir, "notime.jsonl"), []byte(`{"sessionId":"notime"}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// File with a valid timestamp
	writeJSONL(t, filepath.Join(dir, "hastime.jsonl"), []map[string]any{
		makeRecord("hastime", ts1, "", ""),
	})

	sessions, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
	if sessions[0].ID != "hastime" {
		t.Errorf("session with timestamp should be first, got %q", sessions[0].ID)
	}
	if sessions[1].ID != "notime" {
		t.Errorf("session without timestamp should be last, got %q", sessions[1].ID)
	}
}

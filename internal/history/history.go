// Package history lists past Claude Code sessions from ~/.claude/projects/
// and provides a Bubbletea TUI for browsing and selecting them.
package history

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SessionMeta is a summary of one top-level Claude Code session JSONL file.
type SessionMeta struct {
	ID         string    // session UUID (derived from filename, no .jsonl)
	File       string    // absolute path to the JSONL file
	StartTime  time.Time // timestamp of the first parseable record
	EndTime    time.Time // timestamp of the last parseable record
	EventCount int       // number of parseable records (proxy for session length)
	Snippet    string    // first ~60 runes of the first user prompt
}

// histRecord is the minimal shape of each JSONL line we need to inspect.
type histRecord struct {
	SessionID string          `json:"sessionId"`
	Timestamp string          `json:"timestamp"`
	Message   histMessage     `json:"message"`
}

type histMessage struct {
	Role       string          `json:"role"`
	ContentRaw json.RawMessage `json:"content"`
}

// contentBlock is one element of a content array.
type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// List reads dir and returns one SessionMeta per .jsonl file found at the
// top level of the directory. Sessions are sorted newest-first by StartTime.
// If dir does not exist, List returns nil, nil.
func List(dir string) ([]SessionMeta, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var sessions []SessionMeta
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		meta, err := parseSessionMeta(path)
		if err != nil {
			continue // skip unreadable files
		}
		sessions = append(sessions, meta)
	}

	// Sort newest-first; zero StartTime sessions fall to the end.
	sort.SliceStable(sessions, func(i, j int) bool {
		ti, tj := sessions[i].StartTime, sessions[j].StartTime
		if ti.IsZero() {
			return false
		}
		if tj.IsZero() {
			return true
		}
		return ti.After(tj)
	})

	return sessions, nil
}

// ListForCWD resolves the ~/.claude/projects/ directory for cwd and calls List.
func ListForCWD(cwd string) ([]SessionMeta, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(home, ".claude", "projects", cwdToDir(cwd))
	return List(dir)
}

// cwdToDir converts a filesystem path to the directory name Claude Code uses
// under ~/.claude/projects/ (forward slashes replaced with hyphens).
func cwdToDir(cwd string) string {
	return strings.ReplaceAll(cwd, "/", "-")
}

// parseSessionMeta does a single-pass scan of a JSONL file to extract metadata.
func parseSessionMeta(path string) (SessionMeta, error) {
	f, err := os.Open(path)
	if err != nil {
		return SessionMeta{}, err
	}
	defer f.Close()

	id := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	meta := SessionMeta{
		ID:   id,
		File: path,
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		var rec histRecord
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			continue
		}
		if rec.SessionID == "" {
			continue
		}

		meta.EventCount++

		// Track first and last timestamps.
		ts := parseTimestamp(rec.Timestamp)
		if !ts.IsZero() {
			if meta.StartTime.IsZero() {
				meta.StartTime = ts
			}
			meta.EndTime = ts
		}

		// Extract snippet from the first user message.
		if meta.Snippet == "" && rec.Message.Role == "user" && len(rec.Message.ContentRaw) > 0 {
			meta.Snippet = extractSnippet(rec.Message.ContentRaw)
		}
	}

	if err := scanner.Err(); err != nil {
		return SessionMeta{}, err
	}

	return meta, nil
}

// extractSnippet extracts up to 60 runes of text from a content field that
// may be either a JSON string or an array of content blocks.
func extractSnippet(raw json.RawMessage) string {
	// Try plain string first.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return truncate(s, 60)
	}

	// Try array of content blocks; find the first text-type block.
	var blocks []contentBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				return truncate(b.Text, 60)
			}
		}
	}

	return ""
}

// truncate replaces newlines with spaces and caps the string at maxRunes runes.
func truncate(s string, maxRunes int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes])
	}
	return s
}

// parseTimestamp parses an RFC3339 timestamp string, returning the zero time
// on failure.
func parseTimestamp(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t.Local()
}

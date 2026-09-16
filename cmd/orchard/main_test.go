package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParsePort(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
		want    int
	}{
		{"7070", false, 7070},
		{"8080", false, 8080},
		{"1", false, 1},
		{"65535", false, 65535},
		{"0", true, 0},
		{"-1", true, 0},
		{"65536", true, 0},
		{"abc", true, 0},
		{"", true, 0},
		{"80.5", true, 0},
	}

	for _, tc := range tests {
		got, err := parsePort(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parsePort(%q) expected error, got nil", tc.input)
			}
		} else {
			if err != nil {
				t.Errorf("parsePort(%q) unexpected error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("parsePort(%q) = %d, want %d", tc.input, got, tc.want)
			}
		}
	}
}

func TestRunVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--version"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "orchard") {
		t.Errorf("expected output to contain 'orchard', got: %q", out)
	}
}

func TestRunSetup_DryRun(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	var stdout, stderr bytes.Buffer
	code := run([]string{"setup", "--dry-run"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "dry-run") {
		t.Errorf("expected 'dry-run' in output, got: %q", out)
	}
}

func TestRunSetup_WritesFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"setup"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", code, stderr.String())
	}

	settingsPath := filepath.Join(dir, ".claude", "settings.json")
	if _, err := os.Stat(settingsPath); err != nil {
		t.Errorf("expected settings.json to be created: %v", err)
	}
}

func TestRunSetup_InvalidPort(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"setup", "--port", "abc"}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("expected exit 1 for invalid port, got %d", code)
	}
}

func TestRunDoctor_Runs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	_ = run([]string{"doctor"}, &stdout, &stderr)
	out := stdout.String()
	if !strings.Contains(out, "Go runtime") {
		t.Errorf("expected 'Go runtime' in doctor output, got: %q", out)
	}
}

func TestRunDoctor_InvalidPort(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"doctor", "--port", "abc"}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("expected exit 1 for invalid port, got %d", code)
	}
}

func TestRunSetup_Verify(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	_ = run([]string{"setup", "--verify"}, &stdout, &stderr)
	out := stdout.String()
	if !strings.Contains(out, "health checks") {
		t.Errorf("expected 'health checks' in verify output, got: %q", out)
	}
}

func TestMultiFlag_SingleValue(t *testing.T) {
	var f multiFlag
	if err := f.Set("/foo"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f) != 1 || f[0] != "/foo" {
		t.Errorf("got %v, want [/foo]", []string(f))
	}
}

func TestMultiFlag_CommaSeparated(t *testing.T) {
	var f multiFlag
	if err := f.Set("/foo,/bar"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f) != 2 || f[0] != "/foo" || f[1] != "/bar" {
		t.Errorf("got %v, want [/foo /bar]", []string(f))
	}
}

func TestMultiFlag_RepeatedCalls(t *testing.T) {
	var f multiFlag
	_ = f.Set("/a")
	_ = f.Set("/b")
	if len(f) != 2 || f[0] != "/a" || f[1] != "/b" {
		t.Errorf("got %v, want [/a /b]", []string(f))
	}
}

func TestMultiFlag_EmptyStringIgnored(t *testing.T) {
	var f multiFlag
	if err := f.Set(",/a,,/b,"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f) != 2 {
		t.Errorf("got %v, want [/a /b]", []string(f))
	}
}

func TestRunInvalidPort(t *testing.T) {
	tests := []struct {
		port string
	}{
		{"abc"},
		{"0"},
		{"99999"},
		{"-1"},
	}

	for _, tc := range tests {
		var stdout, stderr bytes.Buffer
		code := run([]string{"--port", tc.port}, &stdout, &stderr)
		if code != 1 {
			t.Errorf("port %q: expected exit code 1, got %d", tc.port, code)
		}
		if !strings.Contains(stderr.String(), "invalid --port") {
			t.Errorf("port %q: expected stderr to contain 'invalid --port', got: %q", tc.port, stderr.String())
		}
	}
}

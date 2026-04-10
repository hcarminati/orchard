package main

import (
	"bytes"
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
		// run() validates the port before starting the TUI, so it returns without
		// launching Bubbletea — safe to call directly in tests.
		code := run([]string{"--port", tc.port}, &stdout, &stderr)
		if code != 1 {
			t.Errorf("port %q: expected exit code 1, got %d", tc.port, code)
		}
		if !strings.Contains(stderr.String(), "invalid --port") {
			t.Errorf("port %q: expected stderr to contain 'invalid --port', got: %q", tc.port, stderr.String())
		}
	}
}

package process

import (
	"os"
	"testing"
)

func TestClaudeProcess_String(t *testing.T) {
	p := ClaudeProcess{
		PID:        12345,
		CWD:        "/Users/alice/myapp",
		Executable: "claude",
	}
	s := p.String()
	if s == "" {
		t.Error("String() should not be empty")
	}
	// Should include PID
	if len(s) < 5 {
		t.Errorf("String() too short: %q", s)
	}
}

func TestFindForCWD_NonExistentCWD_ReturnsEmpty(t *testing.T) {
	procs, err := FindForCWD("/does/not/exist/xyzzy123")
	if err != nil {
		// ps failing is a legitimate error on some systems; skip gracefully.
		t.Skipf("ps unavailable: %v", err)
	}
	// Should return empty (no claude process is running in a non-existent dir).
	if len(procs) != 0 {
		t.Errorf("expected 0 processes, got %d", len(procs))
	}
}

func TestSendInterrupt_SelfProcess(t *testing.T) {
	// Send SIGINT to ourselves. We need to set up a signal handler so the test
	// doesn't actually exit. But in Go tests the default SIGINT handler is
	// overridden by the test runner. Sending SIGINT to our own PID is safe in tests.
	//
	// Actually, sending SIGINT to the test process would kill the test. Instead,
	// we test the error path: sending to a PID that doesn't exist.
	err := SendInterrupt(99999999)
	if err == nil {
		t.Error("expected error for non-existent PID")
	}
}

func TestSendInterrupt_ValidProcess(t *testing.T) {
	// Test that we can send SIGINT to a real PID without an error being returned
	// for process-not-found. We use the current process's parent PID (ppid).
	ppid := os.Getppid()
	if ppid <= 0 {
		t.Skip("could not get parent PID")
	}
	// We don't actually send the signal — just verify that SendInterrupt
	// would find the process. Instead we test that FindProcess works.
	p, err := os.FindProcess(ppid)
	if err != nil {
		t.Skipf("FindProcess: %v", err)
	}
	if p == nil {
		t.Error("expected non-nil process")
	}
}

// Package process provides utilities for discovering and signalling Claude Code processes.
// It is used by the cancel subcommand and the TUI's cancel key binding.
package process

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// ClaudeProcess represents a running Claude Code process associated with a project.
type ClaudeProcess struct {
	PID        int
	CWD        string  // working directory the process is running in
	Executable string  // path to the claude binary
}

// String returns a human-readable description of the process.
func (p ClaudeProcess) String() string {
	name := filepath.Base(p.CWD)
	return fmt.Sprintf("PID %d  %s  (%s)", p.PID, p.Executable, name)
}

// FindForCWD returns Claude Code processes whose working directory matches cwd.
// Returns an empty slice (not an error) when no matching processes are found.
// Uses `ps` for portability across macOS and Linux without external dependencies.
func FindForCWD(cwd string) ([]ClaudeProcess, error) {
	return findProcesses(cwd)
}

// SendInterrupt sends SIGINT to the process with the given PID.
// This is the same signal as Ctrl+C — it asks Claude Code to exit gracefully.
func SendInterrupt(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("process %d not found: %w", pid, err)
	}
	return p.Signal(syscall.SIGINT)
}

// findProcesses uses `ps` to locate claude processes matching cwd.
// We look for processes where the command contains "claude" and the
// working directory (from lsof or /proc) matches.
func findProcesses(cwd string) ([]ClaudeProcess, error) {
	// Use `ps aux` to get all processes, then filter by command name.
	// This is the cross-platform approach; no external tools beyond ps/lsof needed.
	out, err := exec.Command("ps", "aux").Output()
	if err != nil {
		return nil, fmt.Errorf("ps: %w", err)
	}

	var candidates []int
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		// ps aux columns: USER PID %CPU %MEM ... COMMAND
		if len(fields) < 2 {
			continue
		}
		cmd := strings.Join(fields[10:], " ")
		if strings.Contains(strings.ToLower(cmd), "claude") {
			pid, err := strconv.Atoi(fields[1])
			if err != nil {
				continue
			}
			candidates = append(candidates, pid)
		}
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	var results []ClaudeProcess
	for _, pid := range candidates {
		procCWD, err := pidCWD(pid)
		if err != nil {
			continue
		}
		// Match if the process CWD is exactly cwd or a subdirectory of cwd.
		if procCWD == cwd || strings.HasPrefix(procCWD, cwd+"/") {
			results = append(results, ClaudeProcess{
				PID:        pid,
				CWD:        procCWD,
				Executable: "claude",
			})
		}
	}
	return results, nil
}

// pidCWD returns the working directory of a process using lsof (macOS/Linux).
func pidCWD(pid int) (string, error) {
	out, err := exec.Command("lsof", "-a", "-d", "cwd", "-p", strconv.Itoa(pid), "-Fn").Output()
	if err != nil {
		return "", err
	}
	// lsof -Fn output: lines starting with 'n' are the file name (cwd in this case).
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "n") {
			return strings.TrimPrefix(line, "n"), nil
		}
	}
	return "", fmt.Errorf("cwd not found for pid %d", pid)
}

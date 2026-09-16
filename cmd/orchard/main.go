// main is the entry point for the Orchard binary.
// In Go, every executable program has exactly one `package main` with one `func main()`.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	// Bubbletea is the TUI framework. We alias it as `tea` — that's the convention
	// across all Bubbletea projects so you'll see `tea.Cmd`, `tea.Msg` etc.
	tea "github.com/charmbracelet/bubbletea"

	"github.com/hcarminati/orchard/internal/agent"
	"github.com/hcarminati/orchard/internal/config"
	"github.com/hcarminati/orchard/internal/doctor"
	"github.com/hcarminati/orchard/internal/history"
	"github.com/hcarminati/orchard/internal/hooks"
	"github.com/hcarminati/orchard/internal/process"
	"github.com/hcarminati/orchard/internal/replay"
	"github.com/hcarminati/orchard/internal/session"
	"github.com/hcarminati/orchard/internal/setup"
	"github.com/hcarminati/orchard/internal/ui"
)

// version is the current release. Updated at build time via -ldflags when
// the release workflow cuts a tag; falls back to "dev" during local runs.
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run parses args and starts Orchard. It returns an exit code so it can be
// tested without calling os.Exit directly.
func run(args []string, stdout, stderr io.Writer) int {
	// Route subcommands before the main flag set so they get their own flags.
	if len(args) > 0 {
		switch args[0] {
		case "replay":
			return runReplay(args[1:], stdout, stderr)
		case "history":
			return runHistory(args[1:], stdout, stderr)
		case "cancel":
			return runCancel(args[1:], stdout, stderr)
		case "setup":
			return runSetup(args[1:], stdout, stderr)
		case "doctor":
			return runDoctor(args[1:], stdout, stderr)
		}
	}

	fs := flag.NewFlagSet("orchard", flag.ContinueOnError)
	fs.SetOutput(stderr)

	showVersion := fs.Bool("version", false, "print version and exit")
	demo := fs.Bool("demo", false, "populate with fake agents for local testing")
	portStr := fs.String("port", "7070", "port for the Claude Code hook server")
	project := fs.String("project", "", "project working directory to watch (default: current directory; deprecated, use --watch)")
	var watchPaths multiFlag
	fs.Var(&watchPaths, "watch", "project directory to observe; repeat for multiple: --watch /a --watch /b")

	if err := fs.Parse(args); err != nil {
		// flag.ContinueOnError already wrote the error to stderr.
		return 2
	}

	if *showVersion {
		fmt.Fprintln(stdout, "orchard", version)
		return 0
	}

	port, err := parsePort(*portStr)
	if err != nil {
		fmt.Fprintf(stderr, "error: invalid --port %q: %v\n", *portStr, err)
		return 1
	}

	// Resolve the set of working directories to watch.
	cwds := []string(watchPaths)
	if *project != "" {
		cwds = append(cwds, *project)
	}
	if len(cwds) == 0 {
		var d string
		d, err = os.Getwd()
		if err != nil {
			fmt.Fprintf(stderr, "error: could not determine working directory: %v\n", err)
			return 1
		}
		cwds = []string{d}
	}
	// Deduplicate while preserving order.
	{
		seen := make(map[string]bool)
		deduped := cwds[:0]
		for _, c := range cwds {
			if !seen[c] {
				seen[c] = true
				deduped = append(deduped, c)
			}
		}
		cwds = deduped
	}
	// cwd is the primary working directory (first in the list), used for
	// subagents root computation and single-project-mode features.
	cwd := cwds[0]

	// Load existing session data from ~/.claude/projects/ to hydrate the initial tree.
	// Errors here are non-fatal: the TUI will show "waiting for session…" instead.
	var initialNodes []agent.Node
	if *demo {
		initialNodes = demoNodes()
	} else {
		for _, c := range cwds {
			nodes, lerr := session.Load(c)
			if lerr != nil {
				fmt.Fprintf(stderr, "warning: could not load session data for %s: %v\n", c, lerr)
				continue
			}
			// Tag nodes with their project directory when watching multiple projects.
			if len(cwds) > 1 {
				for i := range nodes {
					nodes[i].ProjectDir = c
				}
			}
			initialNodes = append(initialNodes, nodes...)
		}
	}

	// Load hidden session IDs from ~/.config/orchard/hidden.json.
	// Errors are non-fatal; the TUI starts with no hidden sessions.
	hiddenIDs, err := config.LoadHidden()
	if err != nil {
		fmt.Fprintf(stderr, "warning: could not load hidden sessions: %v\n", err)
	}

	// Load expanded session IDs from ~/.config/orchard/state.json.
	// Errors are non-fatal; all sessions start collapsed by default.
	expandedIDs, err := config.LoadExpanded()
	if err != nil {
		fmt.Fprintf(stderr, "warning: could not load session state: %v\n", err)
	}

	// Load user config from ~/.config/orchard/config.toml.
	// Errors are non-fatal; missing file is the common case and returns zero Config.
	orchardCfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(stderr, "warning: could not load config: %v\n", err)
	}

	// Compute the subagents root directory so the TUI can scan for new subagent
	// JSONL files when a PreToolUse[Agent] event arrives.
	// Path: ~/.claude/projects/{cwdDir}/ where cwdDir is cwd with "/" → "-".
	var subagentsRoot string
	if home, herr := os.UserHomeDir(); herr == nil {
		cwdDir := strings.ReplaceAll(cwd, "/", "-")
		subagentsRoot = filepath.Join(home, ".claude", "projects", cwdDir)
	}

	// Create a buffered channel for hook events.
	// The buffer prevents the HTTP handler from stalling if the TUI is briefly busy.
	eventCh := make(chan agent.Event, 256)

	// Start the hook server in the background. It blocks on ListenAndServe,
	// so it must run in its own goroutine. http.ErrServerClosed is expected
	// on clean shutdown and is not logged.
	addr := fmt.Sprintf(":%d", port)
	hookServer := hooks.NewServer(cwds, eventCh, addr)
	go func() {
		if err := hookServer.Start(); err != nil && err != http.ErrServerClosed {
			// Log to stderr but do not crash the TUI — the user can still use
			// Orchard in read-only mode (session JSONL only) if the port is taken.
			fmt.Fprintf(stderr, "hook server: %v\n", err)
		}
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = hookServer.Shutdown(ctx)
	}()

	// Create a new Bubbletea program, passing it our UI model and options.
	// Think of the program as the event loop — it handles keyboard input,
	// window resize events, and calls our model's Update/View functions.
	p := tea.NewProgram(
		ui.New(initialNodes, eventCh, subagentsRoot, hiddenIDs, expandedIDs, orchardCfg.Budget, orchardCfg.MaxTokens, orchardCfg.WatchdogMinutes, orchardCfg.CostAlert), // model seeded with session data + live event channel
		tea.WithAltScreen(),           // use the terminal's alternate screen buffer so we
		// don't mess up the user's scrollback history
		tea.WithMouseCellMotion(), // enable mouse click support for node focus
	)

	// p.Run() starts the TUI and blocks until the user quits.
	// The first return value would be the final model state — we don't need it here.
	if _, err := p.Run(); err != nil {
		// If something goes wrong, print the error to stderr (not stdout) and
		// exit with a non-zero code so scripts can detect the failure.
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

// multiFlag is a flag.Value that accumulates multiple --watch values into a slice.
// Comma-separated values in a single flag invocation are also split, so
// --watch a,b and --watch a --watch b are equivalent.
type multiFlag []string

func (f *multiFlag) String() string { return strings.Join(*f, ",") }
func (f *multiFlag) Set(v string) error {
	for _, s := range strings.Split(v, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			*f = append(*f, s)
		}
	}
	return nil
}

// parsePort validates that s is an integer in the valid TCP port range [1, 65535].
func parsePort(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("must be an integer")
	}
	if n < 1 || n > 65535 {
		return 0, fmt.Errorf("must be between 1 and 65535")
	}
	return n, nil
}

// runReplay handles the `orchard replay` subcommand.
// It loads a past session from ~/.claude/projects/ and replays its events
// through the standard TUI at a configurable speed.
func runReplay(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("orchard replay", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: orchard replay [--project PATH] [--speed N]")
		fmt.Fprintln(stderr, "")
		fmt.Fprintln(stderr, "Replay a past Claude Code session in the Orchard TUI.")
		fmt.Fprintln(stderr, "")
		fmt.Fprintln(stderr, "flags:")
		fs.PrintDefaults()
	}

	project := fs.String("project", "", "project working directory whose session to replay (default: current directory)")
	speed := fs.Float64("speed", 1.0, "playback speed multiplier (e.g. 2 = double speed, 0.5 = half speed)")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	cwd := *project
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			fmt.Fprintf(stderr, "error: could not determine working directory: %v\n", err)
			return 1
		}
	}

	nodes, err := session.Load(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "error: could not load session: %v\n", err)
		return 1
	}
	if len(nodes) == 0 {
		fmt.Fprintln(stderr, "error: no session data found for this project. Run `orchard` from the project directory first.")
		return 1
	}

	dur := replay.Duration(nodes)

	// Print a brief summary to stderr before the TUI takes over.
	fmt.Fprintf(stderr, "replaying session: %d agents, %s at %.1fx speed\n",
		len(nodes), formatReplayDur(dur), *speed)

	// Create a buffered event channel and start the replay in the background.
	eventCh := make(chan agent.Event, 256)
	done := make(chan struct{})
	go func() {
		replay.Run(nodes, eventCh, replay.Options{Speed: *speed}, done)
	}()

	// Start the TUI with empty initial nodes — events arrive through the channel.
	p := tea.NewProgram(
		ui.New(nil, eventCh, "", nil, nil, 0, 0, 0, 0),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		close(done)
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	close(done)
	return 0
}

// runSetup handles the `orchard setup` subcommand.
// It non-destructively merges Orchard's hook configuration into ~/.claude/settings.json.
func runSetup(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("orchard setup", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: orchard setup [--port PORT] [--dry-run] [--verify]")
		fmt.Fprintln(stderr, "")
		fmt.Fprintln(stderr, "Merges Orchard hook config into ~/.claude/settings.json.")
		fmt.Fprintln(stderr, "")
		fmt.Fprintln(stderr, "flags:")
		fs.PrintDefaults()
	}

	portStr := fs.String("port", "7070", "hook server port")
	dryRun := fs.Bool("dry-run", false, "print what would be changed without writing")
	verify := fs.Bool("verify", false, "run health checks after setup (like orchard doctor)")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	port, err := parsePort(*portStr)
	if err != nil {
		fmt.Fprintf(stderr, "error: invalid --port %q: %v\n", *portStr, err)
		return 1
	}

	settingsPath, err := setup.DefaultSettingsPath()
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	_, err = setup.Run(settingsPath, setup.Options{
		Port:   port,
		DryRun: *dryRun,
		Stdout: stdout,
	})
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	if *verify {
		fmt.Fprintln(stdout, "")
		fmt.Fprintln(stdout, "Running health checks…")
		checks := doctor.RunAll(port)
		for _, c := range checks {
			fmt.Fprintln(stdout, c.String())
		}
		if !doctor.AllPass(checks) {
			return 1
		}
	}

	return 0
}

// runDoctor handles the `orchard doctor` subcommand.
// It runs a series of health checks and prints results.
func runDoctor(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("orchard doctor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: orchard doctor [--port PORT]")
		fmt.Fprintln(stderr, "")
		fmt.Fprintln(stderr, "Runs health checks for the Orchard installation.")
		fmt.Fprintln(stderr, "")
		fmt.Fprintln(stderr, "flags:")
		fs.PrintDefaults()
	}

	portStr := fs.String("port", "7070", "hook server port to check")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	port, err := parsePort(*portStr)
	if err != nil {
		fmt.Fprintf(stderr, "error: invalid --port %q: %v\n", *portStr, err)
		return 1
	}

	checks := doctor.RunAll(port)
	for _, c := range checks {
		fmt.Fprintln(stdout, c.String())
	}

	if !doctor.AllPass(checks) {
		return 1
	}
	return 0
}

// runCancel handles the `orchard cancel` subcommand.
// It discovers Claude Code processes for the project and sends SIGINT.
func runCancel(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("orchard cancel", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: orchard cancel [--project PATH] [--dry-run]")
		fmt.Fprintln(stderr, "")
		fmt.Fprintln(stderr, "Cancels running Claude Code sessions for a project by sending SIGINT.")
		fmt.Fprintln(stderr, "")
		fmt.Fprintln(stderr, "flags:")
		fs.PrintDefaults()
	}

	project := fs.String("project", "", "project working directory (default: current directory)")
	dryRun := fs.Bool("dry-run", false, "show which processes would be cancelled without sending the signal")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	cwd := *project
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			fmt.Fprintf(stderr, "error: could not determine working directory: %v\n", err)
			return 1
		}
	}

	procs, err := process.FindForCWD(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "error: could not discover processes: %v\n", err)
		return 1
	}

	if len(procs) == 0 {
		fmt.Fprintln(stdout, "no Claude Code processes found for this project.")
		return 0
	}

	for _, p := range procs {
		if *dryRun {
			fmt.Fprintf(stdout, "would cancel: %s\n", p.String())
			continue
		}
		if err := process.SendInterrupt(p.PID); err != nil {
			fmt.Fprintf(stderr, "error cancelling %s: %v\n", p.String(), err)
		} else {
			fmt.Fprintf(stdout, "cancelled: %s\n", p.String())
		}
	}

	return 0
}

// runHistory handles the `orchard history` subcommand.
// It opens a TUI that lets the user browse, replay, or export past sessions.
func runHistory(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("orchard history", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: orchard history [--project PATH]")
		fmt.Fprintln(stderr, "")
		fmt.Fprintln(stderr, "Browse past Claude Code sessions for a project.")
		fmt.Fprintln(stderr, "")
		fmt.Fprintln(stderr, "flags:")
		fs.PrintDefaults()
	}

	project := fs.String("project", "", "project working directory (default: current directory)")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	cwd := *project
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			fmt.Fprintf(stderr, "error: could not determine working directory: %v\n", err)
			return 1
		}
	}

	sessions, err := history.ListForCWD(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "error: could not list sessions: %v\n", err)
		return 1
	}
	if len(sessions) == 0 {
		fmt.Fprintln(stderr, "no past sessions found for this project.")
		return 0
	}

	result, err := history.Run(sessions)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	switch result.Action {
	case history.ActionReplay:
		return runReplaySession(result.Session, stdout, stderr)
	case history.ActionExport:
		return runExportSession(result.Session, stdout, stderr)
	}
	return 0
}

// runReplaySession replays a specific session selected from the history picker.
func runReplaySession(s history.SessionMeta, stdout, stderr io.Writer) int {
	nodes, err := session.LoadFile(s.File)
	if err != nil {
		fmt.Fprintf(stderr, "error: could not load session file: %v\n", err)
		return 1
	}
	if len(nodes) == 0 {
		fmt.Fprintln(stderr, "error: session file contains no agent data.")
		return 1
	}

	dur := replay.Duration(nodes)
	fmt.Fprintf(stderr, "replaying session %s: %d agents, %s\n",
		s.ID[:8], len(nodes), formatReplayDur(dur))

	eventCh := make(chan agent.Event, 256)
	done := make(chan struct{})
	go func() {
		replay.Run(nodes, eventCh, replay.Options{Speed: 1.0}, done)
	}()

	p := tea.NewProgram(
		ui.New(nil, eventCh, "", nil, nil, 0, 0, 0, 0),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	if _, err := p.Run(); err != nil {
		close(done)
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	close(done)
	return 0
}

// runExportSession exports a session's raw JSONL to stdout.
func runExportSession(s history.SessionMeta, stdout, stderr io.Writer) int {
	f, err := os.Open(s.File)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	defer f.Close()

	if _, err := io.Copy(stdout, f); err != nil {
		fmt.Fprintf(stderr, "error writing export: %v\n", err)
		return 1
	}
	return 0
}

// formatReplayDur formats a duration for the replay startup message.
func formatReplayDur(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	if s == 0 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dm%02ds", m, s)
}

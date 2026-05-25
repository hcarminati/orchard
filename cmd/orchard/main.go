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
	"github.com/hcarminati/orchard/internal/hooks"
	"github.com/hcarminati/orchard/internal/replay"
	"github.com/hcarminati/orchard/internal/session"
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
	if len(args) > 0 && args[0] == "replay" {
		return runReplay(args[1:], stdout, stderr)
	}

	fs := flag.NewFlagSet("orchard", flag.ContinueOnError)
	fs.SetOutput(stderr)

	showVersion := fs.Bool("version", false, "print version and exit")
	demo := fs.Bool("demo", false, "populate with fake agents for local testing")
	portStr := fs.String("port", "7070", "port for the Claude Code hook server")
	project := fs.String("project", "", "override the project working directory (default: current directory)")

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

	// Determine which working directory to filter hook events for.
	cwd := *project
	if cwd == "" {
		cwd, err = os.Getwd()
		if err != nil {
			fmt.Fprintf(stderr, "error: could not determine working directory: %v\n", err)
			return 1
		}
	}

	// Load existing session data from ~/.claude/projects/ to hydrate the initial tree.
	// Errors here are non-fatal: the TUI will show "waiting for session…" instead.
	var initialNodes []agent.Node
	if *demo {
		initialNodes = demoNodes()
	} else {
		initialNodes, err = session.Load(cwd)
		if err != nil {
			fmt.Fprintf(stderr, "warning: could not load session data: %v\n", err)
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
	hookServer := hooks.NewServer(cwd, eventCh, addr)
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
		ui.New(initialNodes, eventCh, subagentsRoot, hiddenIDs, expandedIDs, orchardCfg.Budget, orchardCfg.MaxTokens), // model seeded with session data + live event channel
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
		ui.New(nil, eventCh, "", nil, nil, 0, 0),
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

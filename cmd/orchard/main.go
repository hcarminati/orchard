// main is the entry point for the Orchard binary.
// In Go, every executable program has exactly one `package main` with one `func main()`.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	// Bubbletea is the TUI framework. We alias it as `tea` — that's the convention
	// across all Bubbletea projects so you'll see `tea.Cmd`, `tea.Msg` etc.
	tea "github.com/charmbracelet/bubbletea"

	"github.com/hcarminati/orchard/internal/agent"
	"github.com/hcarminati/orchard/internal/hooks"
	"github.com/hcarminati/orchard/internal/session"
	"github.com/hcarminati/orchard/internal/ui"
)

// version is the current release. Updated at build time via -ldflags when
// the release workflow cuts a tag; falls back to "dev" during local runs.
var version = "dev"

func main() {
	// --- flags ---
	showVersion := flag.Bool("version", false, "print version and exit")
	port := flag.String("port", "7070", "port for the Claude Code hook server")
	project := flag.String("project", "", "override the project working directory (default: current directory)")
	flag.Parse()

	if *showVersion {
		fmt.Println("orchard", version)
		os.Exit(0)
	}

	// Determine which working directory to filter hook events for.
	cwd := *project
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: could not determine working directory: %v\n", err)
			os.Exit(1)
		}
	}

	// Load existing session data from ~/.claude/projects/ to hydrate the initial tree.
	// Errors here are non-fatal: the TUI will show "waiting for session…" instead.
	initialNodes, err := session.Load(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not load session data: %v\n", err)
	}

	// Create a buffered channel for hook events.
	// The buffer prevents the HTTP handler from stalling if the TUI is briefly busy.
	eventCh := make(chan agent.Event, 256)

	// Start the hook server in the background. It blocks on ListenAndServe,
	// so it must run in its own goroutine. http.ErrServerClosed is expected
	// on clean shutdown and is not logged.
	hookServer := hooks.NewServer(cwd, eventCh, ":"+*port)
	go func() {
		if err := hookServer.Start(); err != nil && err != http.ErrServerClosed {
			// Log to stderr but do not crash the TUI — the user can still use
			// Orchard in read-only mode (session JSONL only) if the port is taken.
			fmt.Fprintf(os.Stderr, "hook server: %v\n", err)
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
		ui.New(initialNodes, eventCh), // model seeded with session data + live event channel
		tea.WithAltScreen(),           // use the terminal's alternate screen buffer so we
		// don't mess up the user's scrollback history
	)

	// p.Run() starts the TUI and blocks until the user quits.
	// The first return value would be the final model state — we don't need it here.
	if _, err := p.Run(); err != nil {
		// If something goes wrong, print the error to stderr (not stdout) and
		// exit with a non-zero code so scripts can detect the failure.
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

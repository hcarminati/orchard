// main is the entry point for the Orchard binary.
// In Go, every executable program has exactly one `package main` with one `func main()`.
package main

import (
	"fmt"
	"os"

	// Bubbletea is the TUI framework. We alias it as `tea` — that's the convention
	// across all Bubbletea projects so you'll see `tea.Cmd`, `tea.Msg` etc.
	tea "github.com/charmbracelet/bubbletea"

	// Our own UI package, defined in internal/ui/model.go.
	// `internal/` means this code can only be imported by other code in this repo.
	"github.com/hcarminati/orchard/internal/ui"
)

func main() {
	// Create a new Bubbletea program, passing it our UI model and options.
	// Think of the program as the event loop — it handles keyboard input,
	// window resize events, and calls our model's Update/View functions.
	p := tea.NewProgram(
		ui.New(),          // our starting model (see internal/ui/model.go)
		tea.WithAltScreen(), // use the terminal's alternate screen buffer so we
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

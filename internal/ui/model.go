// Package ui contains all TUI components and layout logic for Orchard.
// It follows the Bubbletea pattern: a Model struct holds all state, and
// three methods — Init, Update, View — define how the app behaves.
package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// panel is a custom type representing which panel is currently focused.
// Using a named type (instead of a plain int) makes the code self-documenting
// and prevents accidentally mixing it up with other integers.
type panel int

// These are the two panels. `iota` is a Go shortcut that auto-increments:
// panelAgents = 0, panelEvents = 1.
const (
	panelAgents panel = iota
	panelEvents
)

// Fixed heights for the header and footer rows (in terminal lines).
const (
	headerHeight = 1
	footerHeight = 1
)

// Color palette used throughout the TUI.
// lipgloss.Color accepts any hex color string.
var (
	colorAccent = lipgloss.Color("#7C3AED") // violet — used for active panel border and title
	colorMuted  = lipgloss.Color("#6B7280") // gray — used for inactive elements and descriptions
	colorGreen  = lipgloss.Color("#10B981") // green — running agent status dot
	colorYellow = lipgloss.Color("#F59E0B") // yellow — idle agent status dot
	colorFg     = lipgloss.Color("#F9FAFB") // near-white — primary text
)

// Model holds all the state for the Orchard TUI.
// In Bubbletea, the model is a value type (not a pointer), meaning it gets
// copied on every update. This keeps state changes predictable and testable.
type Model struct {
	width       int   // current terminal width in columns
	height      int   // current terminal height in rows
	activePanel panel // which panel currently has keyboard focus
}

// New creates and returns a fresh Model with sensible defaults.
// This is called once at startup from main.go.
func New() Model {
	return Model{activePanel: panelAgents}
}

// Init is called once when the program starts.
// It can return a `tea.Cmd` to kick off background work (like fetching data).
// We have nothing to do at startup yet, so we return nil.
func (m Model) Init() tea.Cmd { return nil }

// Update is the heart of Bubbletea. Every time something happens — a keypress,
// a window resize, a background task finishing — Bubbletea calls Update with a
// message describing what happened. Update returns a new model (with updated state)
// and optionally a command to run next (like fetching more data or quitting).
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// `switch msg.(type)` is a Go type switch — it checks what kind of message arrived.
	switch msg := msg.(type) {

	// The terminal was resized. Store the new dimensions so View can use them.
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	// A key was pressed.
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			// tea.Quit is a built-in command that tells Bubbletea to stop the program.
			return m, tea.Quit
		case "tab":
			// Cycle between panels. The `% 2` wraps back to 0 after reaching 1,
			// so it toggles: 0 → 1 → 0 → 1 ...
			m.activePanel = (m.activePanel + 1) % 2
		}
	}

	// Return the (possibly updated) model and no command.
	// Bubbletea will call View() with this new model to redraw the screen.
	return m, nil
}

// View converts the current model state into a string that gets printed to the terminal.
// Bubbletea calls this after every Update. Think of it as a render function —
// it should be pure (no side effects) and fast.
func (m Model) View() string {
	// Don't try to render before we know the terminal size.
	// Bubbletea sends a WindowSizeMsg almost immediately, so this is brief.
	if m.width == 0 {
		return "loading…"
	}

	// The body gets whatever height is left after the header and footer take their rows.
	bodyH := m.height - headerHeight - footerHeight

	// JoinVertical stacks strings on top of each other with newlines between them.
	// lipgloss.Left means left-align each piece.
	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderHeader(),
		m.renderBody(bodyH),
		m.renderFooter(),
	)
}

// renderHeader builds the top bar: "orchard" on the left, session status on the right.
func (m Model) renderHeader() string {
	// Render the left and right pieces with their styles.
	left := lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Render(" orchard")
	right := lipgloss.NewStyle().Foreground(colorMuted).Render("no active session ")

	// lipgloss.Width measures the visible width of a styled string (ignoring invisible
	// ANSI escape codes that carry the color information).
	// We fill the gap between left and right with spaces to push them to opposite edges.
	gap := max(0, m.width-lipgloss.Width(left)-lipgloss.Width(right))
	return left + strings.Repeat(" ", gap) + right
}

// renderFooter builds the bottom keybindings bar.
func (m Model) renderFooter() string {
	// `bind` is a local helper function (only exists inside renderFooter).
	// It formats a single key + description pair: bold key, muted description.
	bind := func(key, desc string) string {
		k := lipgloss.NewStyle().Bold(true).Foreground(colorFg).Render(key)
		d := lipgloss.NewStyle().Foreground(colorMuted).Render(" " + desc + "  ")
		return k + d
	}

	content := " " + bind("tab", "switch panel") + bind("q", "quit")

	// Pad to full width so the footer bar extends across the whole terminal.
	gap := max(0, m.width-lipgloss.Width(content))
	return content + strings.Repeat(" ", gap)
}

// renderBody builds the two-panel layout that fills the space between header and footer.
func (m Model) renderBody(height int) string {
	// Give the left (Agents) panel 35% of the width, right (Events) panel gets the rest.
	leftW := m.width * 35 / 100
	rightW := m.width - leftW

	// Render each panel, passing whether it's currently active (focused).
	left := m.renderPanel("Agents", m.agentsContent(), leftW, height, m.activePanel == panelAgents)
	right := m.renderPanel("Events", m.eventsContent(), rightW, height, m.activePanel == panelEvents)

	// JoinHorizontal places the two panels side by side.
	// lipgloss.Top means align them to the top edge if they differ in height.
	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

// renderPanel draws a single bordered panel with a title and content inside.
// `active` controls whether the border is highlighted (focused) or muted (unfocused).
func (m Model) renderPanel(title, content string, width, height int, active bool) string {
	// Focused panel gets the accent color, unfocused gets a subtle gray.
	borderColor := colorMuted
	if active {
		borderColor = colorAccent
	}

	// The border takes 1 character on each side, so the inner content area
	// is 2 columns narrower and 2 rows shorter than the outer panel dimensions.
	innerW := max(1, width-2)
	innerH := max(1, height-2)

	titleStr := lipgloss.NewStyle().Bold(true).Foreground(colorFg).Padding(0, 1).Render(title)

	// Build the panel style: rounded corners, colored border, fixed inner size.
	// Setting Width and Height here ensures the panel always fills its allocated space,
	// even if the content is shorter than the panel — lipgloss pads with empty lines.
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(innerW).
		Height(innerH)

	// Stack the title on top of the content, then render them inside the bordered box.
	return style.Render(lipgloss.JoinVertical(lipgloss.Left, titleStr, content))
}

// agentsContent returns placeholder content for the Agents panel.
// This will be replaced with a real agent tree in v0.3.
func (m Model) agentsContent() string {
	// dot renders a colored circle character — used as a status indicator.
	dot := func(c lipgloss.Color) string {
		return lipgloss.NewStyle().Foreground(c).Render("●")
	}

	// A pre-built style for the muted tree-branch characters (├─ └─).
	muted := lipgloss.NewStyle().Foreground(colorMuted)

	// Build each line of the placeholder tree manually.
	// In v0.3 this will be generated dynamically from real agent data.
	lines := []string{
		"  " + dot(colorGreen) + " main-agent",
		"  " + muted.Render("├─ ") + dot(colorGreen) + " explore",
		"  " + muted.Render("└─ ") + dot(colorYellow) + " plan",
	}

	// Join the lines into a single string with newlines between them.
	return strings.Join(lines, "\n")
}

// eventsContent returns placeholder content for the Events panel.
// This will show real agent event logs once the data pipeline is built in v0.2/v0.4.
func (m Model) eventsContent() string {
	return lipgloss.NewStyle().
		Foreground(colorMuted).
		Padding(0, 1).
		Render("Focus an agent to view its event log.")
}

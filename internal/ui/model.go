package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type panel int

const (
	panelAgents panel = iota
	panelEvents
)

const (
	headerHeight = 1
	footerHeight = 1
)

var (
	colorAccent = lipgloss.Color("#7C3AED")
	colorMuted  = lipgloss.Color("#6B7280")
	colorGreen  = lipgloss.Color("#10B981")
	colorYellow = lipgloss.Color("#F59E0B")
	colorFg     = lipgloss.Color("#F9FAFB")
)

// Model is the root Bubbletea model for the Orchard TUI.
type Model struct {
	width       int
	height      int
	activePanel panel
}

// New returns an initialized Model.
func New() Model {
	return Model{activePanel: panelAgents}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			m.activePanel = (m.activePanel + 1) % 2
		}
	}
	return m, nil
}

func (m Model) View() string {
	if m.width == 0 {
		return "loading…"
	}
	bodyH := m.height - headerHeight - footerHeight
	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderHeader(),
		m.renderBody(bodyH),
		m.renderFooter(),
	)
}

func (m Model) renderHeader() string {
	left := lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Render(" orchard")
	right := lipgloss.NewStyle().Foreground(colorMuted).Render("no active session ")
	gap := max(0, m.width-lipgloss.Width(left)-lipgloss.Width(right))
	return left + strings.Repeat(" ", gap) + right
}

func (m Model) renderFooter() string {
	bind := func(key, desc string) string {
		k := lipgloss.NewStyle().Bold(true).Foreground(colorFg).Render(key)
		d := lipgloss.NewStyle().Foreground(colorMuted).Render(" " + desc + "  ")
		return k + d
	}
	content := " " + bind("tab", "switch panel") + bind("q", "quit")
	gap := max(0, m.width-lipgloss.Width(content))
	return content + strings.Repeat(" ", gap)
}

func (m Model) renderBody(height int) string {
	leftW := m.width * 35 / 100
	rightW := m.width - leftW
	left := m.renderPanel("Agents", m.agentsContent(), leftW, height, m.activePanel == panelAgents)
	right := m.renderPanel("Events", m.eventsContent(), rightW, height, m.activePanel == panelEvents)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func (m Model) renderPanel(title, content string, width, height int, active bool) string {
	borderColor := colorMuted
	if active {
		borderColor = colorAccent
	}
	innerW := max(1, width-2)
	innerH := max(1, height-2)

	titleStr := lipgloss.NewStyle().Bold(true).Foreground(colorFg).Padding(0, 1).Render(title)
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(innerW).
		Height(innerH)
	return style.Render(lipgloss.JoinVertical(lipgloss.Left, titleStr, content))
}

func (m Model) agentsContent() string {
	dot := func(c lipgloss.Color) string {
		return lipgloss.NewStyle().Foreground(c).Render("●")
	}
	muted := lipgloss.NewStyle().Foreground(colorMuted)
	lines := []string{
		"  " + dot(colorGreen) + " main-agent",
		"  " + muted.Render("├─ ") + dot(colorGreen) + " explore",
		"  " + muted.Render("└─ ") + dot(colorYellow) + " plan",
	}
	return strings.Join(lines, "\n")
}

func (m Model) eventsContent() string {
	return lipgloss.NewStyle().
		Foreground(colorMuted).
		Padding(0, 1).
		Render("Focus an agent to view its event log.")
}

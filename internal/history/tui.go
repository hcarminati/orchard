package history

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Action indicates what the user chose when the history picker exits.
type Action int

const (
	ActionQuit   Action = iota
	ActionReplay        // user pressed enter
	ActionExport        // user pressed e
)

// Result is returned by Run() to the caller.
type Result struct {
	Action  Action
	Session SessionMeta
}

// Color palette — matches internal/ui/render.go values without importing it.
var (
	colorAccent  = lipgloss.Color("#7C3AED")
	colorMuted   = lipgloss.Color("#6B7280")
	colorFg      = lipgloss.Color("#F9FAFB")
	colorAmber   = lipgloss.Color("#F59E0B")
	colorBg      = lipgloss.Color("#111827")

	styleTitle    = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	styleCursor   = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	styleMuted    = lipgloss.NewStyle().Foreground(colorMuted)
	styleSnippet  = lipgloss.NewStyle().Foreground(colorMuted).Italic(true)
	styleDate     = lipgloss.NewStyle().Foreground(colorFg)
	styleFooter   = lipgloss.NewStyle().Background(colorBg).Foreground(colorMuted)
	styleSelected = lipgloss.NewStyle().Foreground(colorAccent)
	styleAmber    = lipgloss.NewStyle().Foreground(colorAmber)
)

// Model is the Bubbletea model for the session history picker.
type Model struct {
	sessions []SessionMeta
	cursor   int
	scroll   int
	height   int
	width    int
	action   Action
	chosen   *SessionMeta
	quitting bool
}

// NewModel creates a Model initialized with the given sessions.
func NewModel(sessions []SessionMeta) Model {
	return Model{
		sessions: sessions,
		height:   24, // sensible default; overridden by WindowSizeMsg
		width:    80,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.width = msg.Width
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			m.action = ActionQuit
			return m, tea.Quit

		case "j", "down":
			if m.cursor < len(m.sessions)-1 {
				m.cursor++
				m.clampScroll()
			}

		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
				m.clampScroll()
			}

		case "enter":
			if len(m.sessions) > 0 {
				s := m.sessions[m.cursor]
				m.chosen = &s
				m.action = ActionReplay
				return m, tea.Quit
			}

		case "e":
			if len(m.sessions) > 0 {
				s := m.sessions[m.cursor]
				m.chosen = &s
				m.action = ActionExport
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

// visibleCount returns how many sessions fit in the current terminal height.
// Each session occupies 2 lines (summary + snippet).
func (m Model) visibleCount() int {
	// Reserve: 1 title, 1 blank, 1 footer = 3 lines overhead
	available := m.height - 3
	if available < 2 {
		return 1
	}
	return available / 2
}

// clampScroll adjusts scroll so cursor is always visible.
func (m *Model) clampScroll() {
	vc := m.visibleCount()
	if m.cursor < m.scroll {
		m.scroll = m.cursor
	}
	if m.cursor >= m.scroll+vc {
		m.scroll = m.cursor - vc + 1
	}
}

func (m Model) View() string {
	if m.quitting && m.chosen == nil {
		return ""
	}

	var sb strings.Builder

	// Title
	sb.WriteString(styleTitle.Render("  orchard history"))
	count := fmt.Sprintf(" — %d session", len(m.sessions))
	if len(m.sessions) != 1 {
		count += "s"
	}
	sb.WriteString(styleMuted.Render(count))
	sb.WriteString("\n\n")

	if len(m.sessions) == 0 {
		sb.WriteString(styleMuted.Render("  No past sessions found.\n"))
	} else {
		vc := m.visibleCount()
		end := m.scroll + vc
		if end > len(m.sessions) {
			end = len(m.sessions)
		}

		for i := m.scroll; i < end; i++ {
			s := m.sessions[i]
			selected := i == m.cursor

			// Cursor indicator
			indicator := "  "
			if selected {
				indicator = styleCursor.Render("▶ ")
			}

			// Date and duration
			dateStr := formatDate(s.StartTime)
			dur := ""
			if !s.StartTime.IsZero() && !s.EndTime.IsZero() && s.EndTime.After(s.StartTime) {
				dur = " · " + formatDur(s.EndTime.Sub(s.StartTime))
			}

			// Event count
			evStr := fmt.Sprintf(" · %d events", s.EventCount)

			// Summary line
			var summaryLine string
			if selected {
				summaryLine = indicator + styleSelected.Render(dateStr+dur+evStr)
			} else {
				summaryLine = indicator + styleDate.Render(dateStr) + styleMuted.Render(dur+evStr)
			}

			// Pad to width
			paddingNeeded := m.width - lipgloss.Width(summaryLine)
			if paddingNeeded > 0 {
				summaryLine += strings.Repeat(" ", paddingNeeded)
			}

			sb.WriteString(summaryLine)
			sb.WriteString("\n")

			// Snippet line (indented, muted)
			snippet := s.Snippet
			if snippet == "" {
				snippet = styleMuted.Render("    (no prompt found)")
			} else {
				snippet = styleSnippet.Render("    " + snippet)
			}
			sb.WriteString(snippet)
			sb.WriteString("\n")
		}

		// Scroll indicator
		if len(m.sessions) > vc {
			pct := 0
			if len(m.sessions) > 1 {
				pct = m.cursor * 100 / (len(m.sessions) - 1)
			}
			sb.WriteString(styleAmber.Render(fmt.Sprintf("  ↕ %d%%  (%d/%d)", pct, m.cursor+1, len(m.sessions))))
			sb.WriteString("\n")
		}
	}

	// Footer
	footer := styleFooter.Render("  ↑↓ / jk move  ·  enter replay  ·  e export  ·  q quit")
	// Pad footer to full width
	footerPad := m.width - lipgloss.Width(footer)
	if footerPad > 0 {
		footer += strings.Repeat(" ", footerPad)
	}
	sb.WriteString(footer)

	return sb.String()
}

// formatDate formats a time for display in the session list.
func formatDate(t time.Time) string {
	if t.IsZero() {
		return "unknown date"
	}
	now := time.Now()
	if t.Year() == now.Year() && t.YearDay() == now.YearDay() {
		return "Today " + t.Format("15:04")
	}
	if t.Year() == now.Year() {
		return t.Format("Jan 02  15:04")
	}
	return t.Format("2006 Jan 02  15:04")
}

// formatDur formats a duration compactly (e.g. "3m12s", "45s").
func formatDur(d time.Duration) string {
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

// Run starts the history picker TUI and blocks until the user quits.
// It returns the user's chosen action and the selected session (if any).
func Run(sessions []SessionMeta) (Result, error) {
	m := NewModel(sessions)
	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return Result{}, err
	}
	fm := finalModel.(Model)
	result := Result{Action: fm.action}
	if fm.chosen != nil {
		result.Session = *fm.chosen
	}
	return result, nil
}

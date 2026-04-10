package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/hcarminati/orchard/internal/agent"
)

// Color palette used throughout the TUI.
var (
	colorAccent = lipgloss.Color("#7C3AED") // violet — used for active panel border and title
	colorMuted  = lipgloss.Color("#6B7280") // gray — used for inactive elements and descriptions
	colorGreen  = lipgloss.Color("#10B981") // green — running agent status dot
	colorYellow = lipgloss.Color("#F59E0B") // yellow — idle agent status dot
	colorRed    = lipgloss.Color("#EF4444") // red — error agent status dot
	colorFg     = lipgloss.Color("#F9FAFB") // near-white — primary text
)

// renderFooter builds the bottom keybindings bar.
func (m Model) renderFooter() string {
	bind := func(key, desc string) string {
		k := lipgloss.NewStyle().Bold(true).Foreground(colorFg).Render(key)
		d := lipgloss.NewStyle().Foreground(colorMuted).Render(" " + desc + "  ")
		return k + d
	}

	content := " " + bind("j/k", "navigate")
	if m.cursorHasChildren() {
		content += bind("space", "expand/collapse")
	}
	if m.cursorInGroup() {
		content += bind("enter", "mark winner")
	}
	content += bind("f", "filter:"+m.statusFilter.label())
	if len(rightTabs) > 1 {
		content += bind("[/]", "switch tab")
	}
	content += bind("tab", "switch panel") + bind("q", "quit")

	// Pad to full width so the footer bar extends across the whole terminal.
	gap := max(0, m.width-lipgloss.Width(content))
	return content + strings.Repeat(" ", gap)
}

// renderBody builds the two-panel layout.
func (m Model) renderBody(height int) string {
	leftW := m.width * 35 / 100
	rightW := m.width - leftW

	// Title color matches border color (active = accent, inactive = muted).
	leftActive := m.activePanel == panelAgents
	leftBorderColor := colorMuted
	if leftActive {
		leftBorderColor = colorAccent
	}
	agentCount := lipgloss.NewStyle().Foreground(leftBorderColor).Render(fmt.Sprintf(" · %d", len(m.agents.Nodes)))
	agentsTitle := lipgloss.NewStyle().Bold(true).Foreground(leftBorderColor).Render("Agents") + agentCount
	left := m.renderPanel(agentsTitle, m.agentsContent(), leftW, height, leftActive)

	rightActive := m.activePanel == panelEvents
	rightBorderColor := colorMuted
	if rightActive {
		rightBorderColor = colorAccent
	}
	right := m.renderPanel(m.tabStripTitle(rightBorderColor), m.rightTabContent(), rightW, height, rightActive)

	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

// tabStripTitle returns the tab labels formatted for embedding in the top border.
// The active tab is wrapped in brackets; inactive tabs are muted.
// borderColor is passed so the active label can match the panel border.
func (m Model) tabStripTitle(borderColor lipgloss.Color) string {
	activeStyle := lipgloss.NewStyle().Bold(true).Foreground(borderColor)
	inactiveStyle := lipgloss.NewStyle().Foreground(colorMuted)

	var parts []string
	for i, t := range rightTabs {
		if i == m.activeRightTab {
			parts = append(parts, activeStyle.Render("["+t.label()+"]"))
		} else {
			parts = append(parts, inactiveStyle.Render(t.label()))
		}
	}
	return strings.Join(parts, " ")
}

// rightTabContent returns the body content for the currently active right panel tab.
func (m Model) rightTabContent() string {
	if m.activeRightTab < len(rightTabs) {
		switch rightTabs[m.activeRightTab] {
		case tabEvents:
			return m.eventsContent()
		case tabFiles:
			return m.filesContent()
		}
	}
	return ""
}

// filesContent returns placeholder content for the Files panel.
func (m Model) filesContent() string {
	return lipgloss.NewStyle().
		Foreground(colorMuted).
		Padding(0, 1).
		Render("Focus an agent to view its file activity.")
}

// renderPanel draws a bordered panel with the title embedded in the top border:
//
//	╭─Title──────────────────────────╮
//	│ content …                      │
//	╰────────────────────────────────╯
//
// title may carry ANSI codes; lipgloss.Width measures its visible width.
// Lipgloss renders the full panel first (reliable sizing on resize), then the
// top border line is replaced with a custom one that embeds the title.
func (m Model) renderPanel(title, content string, width, height int, active bool) string {
	borderColor := colorMuted
	if active {
		borderColor = colorAccent
	}

	bs := lipgloss.NewStyle().Foreground(borderColor)
	innerW := max(1, width-2)
	innerH := max(1, height-2)

	rendered := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(innerW).
		Height(innerH).
		Render(content)

	if title != "" {
		// ╭─Title──╮: ╭(1) + ─(1) + title + ─…(n) + ╮(1) = width
		titleW := lipgloss.Width(title)
		dashCount := max(0, width-3-titleW)
		customTop := bs.Render("╭─") + title + bs.Render(strings.Repeat("─", dashCount)+"╮")
		if nl := strings.Index(rendered, "\n"); nl != -1 {
			rendered = customTop + rendered[nl:]
		}
	}

	return rendered
}

// agentsContent renders the Agents panel body.
// When no session is active it shows a "waiting" prompt; otherwise it renders
// the agent tree with depth-based indentation. Nodes with children show a
// collapse/expand indicator (▶/▼). Sibling nodes sharing a GroupID are grouped
// under a "parallel × N" header at root level. The focused node is highlighted
// with ">"; when a winner is marked in a group the others are dimmed.
func (m Model) agentsContent() string {
	waiting := lipgloss.NewStyle().Foreground(colorMuted).Padding(0, 1).Render("Waiting for session…")
	if !m.hasSession || len(m.agents.Nodes) == 0 {
		return waiting
	}

	// innerW is the usable width of the agents panel content area.
	// Each line must fit within this width to prevent wrapping, which would
	// shift the Y coordinates of subsequent rows and break click-to-row mapping.
	leftW := m.width * 35 / 100
	innerW := max(1, leftW-2)

	// truncate clamps a rendered line to innerW visible columns. If the line
	// is wider than innerW, it is cut to innerW-1 and an ellipsis is appended
	// so the user knows the name continues beyond the panel edge.
	truncate := func(line string) string {
		if lipgloss.Width(line) <= innerW {
			return line
		}
		return lipgloss.NewStyle().MaxWidth(innerW-1).Render(line) + "…"
	}

	dot := func(c lipgloss.Color) string {
		return lipgloss.NewStyle().Foreground(c).Render("●")
	}

	expandIcon := func(id string, hasChildren bool) string {
		if !hasChildren {
			return ""
		}
		if m.collapsed[id] {
			return "▶ "
		}
		return "▼ "
	}

	vn := m.visibleNodes()

	// Slice to the visible viewport so the panel doesn't overflow.
	viewH := m.agentsPanelInnerH()
	start := m.scrollOffset
	end := min(start+viewH, len(vn))
	if start > len(vn) {
		start = len(vn)
	}
	visible := vn[start:end]

	var lines []string

	for i, entry := range visible {
		focused := (start + i) == m.cursor

		// Virtual group header row — collapsible, not backed by a real node.
		if entry.groupID != "" {
			count := 0
			for _, id := range m.agents.Roots {
				if n := m.agents.Nodes[id]; n != nil && n.GroupID == entry.groupID {
					count++
				}
			}
			collapsed := m.collapsedGroups[entry.groupID]
			icon := "▼ "
			if collapsed {
				icon = "▶ "
			}
			prefix := "  "
			if focused {
				prefix = "> "
			}
			lines = append(lines, truncate(prefix+icon+lipgloss.NewStyle().Foreground(colorMuted).Render(
				fmt.Sprintf("parallel × %d", count),
			)))
			continue
		}

		n := m.agents.Nodes[entry.id]
		if n == nil {
			continue
		}

		indent := strings.Repeat("  ", entry.depth)
		icon := expandIcon(entry.id, len(n.Children) > 0)

		// Group members get an extra visual indent level under the header.
		if n.GroupID != "" && entry.depth == 0 {
			hasWinner := false
			for _, id := range m.agents.Roots {
				if on := m.agents.Nodes[id]; on != nil && on.GroupID == n.GroupID && on.Winner {
					hasWinner = true
					break
				}
			}
			prefix := "    "
			if focused {
				prefix = "  > "
			}
			indicator := ""
			if n.Winner {
				indicator = " ✓"
			}
			line := prefix + icon + dot(statusColor(n.Status)) + " " + n.Name + indicator
			if n.Status == agent.StatusError && n.ErrorMsg != "" {
				line += " " + lipgloss.NewStyle().Foreground(colorRed).Render("✗ "+n.ErrorMsg)
			}
			if hasWinner && !n.Winner {
				line = lipgloss.NewStyle().Foreground(colorMuted).Render(line)
			}
			lines = append(lines, truncate(line))
			continue
		}

		// Ungrouped root node or any child node.
		prefix := indent + "  "
		if focused {
			prefix = indent + "> "
		}
		line := prefix + icon + dot(statusColor(n.Status)) + " " + n.Name
		if n.Status == agent.StatusError && n.ErrorMsg != "" {
			line += " " + lipgloss.NewStyle().Foreground(colorRed).Render("✗ "+n.ErrorMsg)
		}
		lines = append(lines, truncate(line))
	}

	if len(lines) == 0 {
		return waiting
	}
	return strings.Join(lines, "\n")
}

// eventsContent renders the event log for the currently focused agent node.
// Each entry shows a timestamp, event type, and tool name where applicable.
// The list is scrollable via j/k when the right panel has focus.
func (m Model) eventsContent() string {
	muted := lipgloss.NewStyle().Foreground(colorMuted).Padding(0, 1)

	if !m.hasSession {
		return muted.Render("Waiting for session…")
	}

	node := m.focusedNode()
	if node == nil {
		return muted.Render("Focus an agent to view its event log.")
	}

	if len(node.Events) == 0 {
		return muted.Render("No events yet.")
	}

	leftW := m.width * 35 / 100
	rightW := m.width - leftW
	innerW := max(1, rightW-2)
	viewH := max(1, m.height-footerHeight-2)

	tsStyle := lipgloss.NewStyle().Foreground(colorMuted)
	toolStyle := lipgloss.NewStyle().Foreground(colorAccent)

	var lines []string
	for _, e := range node.Events {
		ts := tsStyle.Render(e.Timestamp.Format("Jan 02 15:04:05"))
		line := " " + ts + "  " + e.Type
		if e.Tool != "" {
			line += "  " + toolStyle.Render(e.Tool)
		}
		if lipgloss.Width(line) > innerW {
			line = lipgloss.NewStyle().MaxWidth(innerW - 1).Render(line) + "…"
		}
		lines = append(lines, line)
	}

	maxStart := max(0, len(lines)-viewH)
	start := min(m.eventScroll, maxStart)
	end := min(start+viewH, len(lines))
	return strings.Join(lines[start:end], "\n")
}

// statusColor maps an agent status to its indicator color.
func statusColor(s agent.Status) lipgloss.Color {
	switch s {
	case agent.StatusRunning:
		return colorGreen
	case agent.StatusDone:
		return colorMuted
	case agent.StatusError:
		return colorRed
	default:
		return colorYellow
	}
}

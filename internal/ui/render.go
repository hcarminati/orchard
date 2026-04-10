package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/hcarminati/orchard/internal/agent"
)

// Color palette used throughout the TUI.
// lipgloss.Color accepts any hex color string.
var (
	colorAccent = lipgloss.Color("#7C3AED") // violet — used for active panel border and title
	colorMuted  = lipgloss.Color("#6B7280") // gray — used for inactive elements and descriptions
	colorGreen  = lipgloss.Color("#10B981") // green — running agent status dot
	colorYellow = lipgloss.Color("#F59E0B") // yellow — idle agent status dot
	colorRed    = lipgloss.Color("#EF4444") // red — error agent status dot
	colorFg     = lipgloss.Color("#F9FAFB") // near-white — primary text
)

// renderHeader builds the top bar: "orchard" on the left, session status on the right.
func (m Model) renderHeader() string {
	left := lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Render(" orchard")

	var statusText string
	if m.hasSession {
		statusText = fmt.Sprintf("%d agents", len(m.agents.Nodes))
	} else {
		statusText = "waiting for session…"
	}
	right := lipgloss.NewStyle().Foreground(colorMuted).Render(statusText + " ")

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

	content := " " + bind("j/k", "navigate")
	if m.cursorHasChildren() {
		content += bind("space", "expand/collapse")
	}
	if m.cursorInGroup() {
		content += bind("enter", "mark winner")
	}
	content += bind("f", "filter:"+m.statusFilter.label())
	content += bind("tab", "switch panel") + bind("q", "quit")

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

	dot := func(c lipgloss.Color) string {
		return lipgloss.NewStyle().Foreground(c).Render("●")
	}

	// expandIcon returns the collapse/expand indicator for a node.
	expandIcon := func(id string, hasChildren bool) string {
		if !hasChildren {
			return ""
		}
		if m.collapsed[id] {
			return "▶ "
		}
		return "▼ "
	}

	var lines []string
	vn := m.visibleNodes()

	for i, entry := range vn {
		focused := i == m.cursor

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
			lines = append(lines, prefix+icon+lipgloss.NewStyle().Foreground(colorMuted).Render(
				fmt.Sprintf("parallel × %d", count),
			))
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
			lines = append(lines, line)
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
		lines = append(lines, line)
	}

	if len(lines) == 0 {
		return waiting
	}
	return strings.Join(lines, "\n")
}

// eventsContent returns placeholder content for the Events panel.
// Real per-agent event logs will be shown here in v0.4.
func (m Model) eventsContent() string {
	return lipgloss.NewStyle().
		Foreground(colorMuted).
		Padding(0, 1).
		Render("Focus an agent to view its event log.")
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

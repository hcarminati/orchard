package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
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

	cursor := lipgloss.NewStyle().Foreground(colorAccent).Render(">")

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
				prefix = cursor + " "
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
				prefix = "  " + cursor + " "
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
		var prefix string
		connector := ""
		if entry.depth > 0 {
			// Child nodes: show └─ connector preceded by parent-level indent.
			connector = "└─ "
			base := strings.Repeat("  ", entry.depth-1)
			if focused {
				prefix = base + cursor + " "
			} else {
				prefix = base + "  "
			}
		} else {
			if focused {
				prefix = cursor + " "
			} else {
				prefix = "  "
			}
		}
		line := prefix + connector + icon + dot(statusColor(n.Status)) + " " + n.Name
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
// Tool call events (PreToolUse / PostToolUse) can be expanded with enter to
// reveal their full input JSON and output. The list is scrollable via j/k.
func (m Model) eventsContent() string {
	muted := lipgloss.NewStyle().Foreground(colorMuted).Padding(0, 1)

	if !m.hasSession {
		return muted.Render("Waiting for session…")
	}

	node := m.focusedNode()
	if node == nil {
		return muted.Render("Focus an agent to view its event log.")
	}

	innerW := m.innerWidth()
	viewH := max(1, m.height-footerHeight-2)

	// Spawn context header — only for child subagent nodes.
	var headerLines []string
	if node.ParentID != "" {
		parentLabel := "session:" + node.ParentID
		if len(node.ParentID) > 8 {
			parentLabel = "session:" + node.ParentID[:8]
		}
		spawnLine := "Spawned by " + parentLabel
		if !node.SpawnedAt.IsZero() {
			spawnLine += "  at " + node.SpawnedAt.Format("Jan 02 15:04:05")
		}

		prompt := node.Prompt
		if len([]rune(prompt)) > 80 {
			prompt = string([]rune(prompt)[:80]) + "…"
		}
		promptLine := "Prompt: " + prompt

		divider := strings.Repeat("─", min(innerW, 48))

		mStyle := lipgloss.NewStyle().Foreground(colorMuted)
		headerLines = []string{
			mStyle.Render(spawnLine),
			mStyle.Render(promptLine),
			mStyle.Render(divider),
		}
	}

	if len(node.Events) == 0 {
		noEvents := muted.Render("No events yet.")
		if len(headerLines) > 0 {
			return strings.Join(headerLines, "\n") + "\n" + noEvents
		}
		return noEvents
	}

	tsStyle := lipgloss.NewStyle().Foreground(colorMuted)
	toolStyle := lipgloss.NewStyle().Foreground(colorAccent)
	mutedStyle := lipgloss.NewStyle().Foreground(colorMuted)
	cursorStyle := lipgloss.NewStyle().Foreground(colorAccent)
	borderStyle := lipgloss.NewStyle().Foreground(colorAccent)

	var allLines []string
	for idx, e := range node.Events {
		isSelected := idx == m.eventCursor
		key := eventKey(node.ID, idx)
		expanded := isToolEvent(e) && m.expandedEvents[key]

		prefix := "  "
		if isSelected {
			prefix = cursorStyle.Render("> ")
		}

		warningStyle := lipgloss.NewStyle().Foreground(colorYellow)
		ts := tsStyle.Render(e.Timestamp.Format("Jan 02 15:04:05"))
		header := prefix + ts + "  " + e.Type

		switch e.Type {
		case "Notification":
			header += "  " + mutedStyle.Render("►")
			if e.Message != "" {
				msg := truncRunes(strings.ReplaceAll(e.Message, "\n", " "), 40)
				header += "  " + mutedStyle.Render(msg)
			}
		case "PermissionRequest":
			header += "  " + warningStyle.Render("⚠")
			tool := e.Tool
			if tool == "" {
				tool = e.Message
			}
			if tool != "" {
				header += "  " + warningStyle.Render(tool)
			}
		default:
			if e.Tool != "" {
				header += "  " + toolStyle.Render(e.Tool)
			}
			if isToolEvent(e) {
				if expanded {
					header += "  " + mutedStyle.Render("▼")
				} else {
					header += "  " + mutedStyle.Render("▶")
				}
			}
		}

		if !expanded {
			// Collapsed: short input preview (≤20 chars) appended to the header.
			if isToolEvent(e) && e.Input != "" {
				preview := "  " + inputPreview(e.Input)
				full := header + preview
				if lipgloss.Width(full) > innerW {
					full = lipgloss.NewStyle().MaxWidth(innerW-1).Render(full) + "…"
				}
				allLines = append(allLines, full)
			} else {
				if lipgloss.Width(header) > innerW {
					header = lipgloss.NewStyle().MaxWidth(innerW-1).Render(header) + "…"
				}
				allLines = append(allLines, header)
			}
		} else {
			// Expanded: header, then bordered KV block for input, then output.
			allLines = append(allLines, header)
			bar := borderStyle.Render("│")
			home, _ := os.UserHomeDir()
			if e.Input != "" {
				allLines = append(allLines, bar+"  "+mutedStyle.Render("Input:"))
				for _, l := range formatKV(e.Input, home) {
					line := bar + "    " + l
					if lipgloss.Width(line) > innerW {
						line = lipgloss.NewStyle().MaxWidth(innerW-1).Render(line) + "…"
					}
					allLines = append(allLines, line)
				}
			}
			if e.Response != "" {
				allLines = append(allLines, bar+"  "+mutedStyle.Render("Output:"))
				for _, l := range formatKV(e.Response, home) {
					line := bar + "    " + l
					if lipgloss.Width(line) > innerW {
						line = lipgloss.NewStyle().MaxWidth(innerW-1).Render(line) + "…"
					}
					allLines = append(allLines, line)
				}
			}
			allLines = append(allLines, bar)
		}
	}

	// Reserve space for the header so event scrolling doesn't overlap it.
	eventsViewH := max(1, viewH-len(headerLines))
	maxStart := max(0, len(allLines)-eventsViewH)
	start := min(m.eventScroll, maxStart)
	end := min(start+eventsViewH, len(allLines))
	eventSection := strings.Join(allLines[start:end], "\n")
	if len(headerLines) > 0 {
		return strings.Join(headerLines, "\n") + "\n" + eventSection
	}
	return eventSection
}

// inputPreview returns a short (≤20 rune) summary of a JSON tool input for
// the collapsed event row. It extracts the most informative string value.
func inputPreview(input string) string {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(input), &obj); err != nil {
		return truncRunes(strings.ReplaceAll(input, "\n", " "), 20)
	}
	// Prefer high-signal keys that tend to carry the most context.
	for _, k := range []string{"command", "cmd", "file_path", "path", "pattern", "query", "prompt"} {
		raw, ok := obj[k]
		if !ok {
			continue
		}
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			return truncRunes(strings.ReplaceAll(s, "\n", " "), 20)
		}
	}
	// Fall back to first string value in sorted key order.
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		var s string
		if err := json.Unmarshal(obj[k], &s); err == nil {
			return truncRunes(strings.ReplaceAll(s, "\n", " "), 20)
		}
	}
	return fmt.Sprintf("{%d fields}", len(obj))
}

// formatKV formats a JSON string as indented "key: value" lines.
// String values are truncated at 60 runes and home-dir prefixes replaced with ~.
// Falls back to plain line-split display for non-object JSON and plain text.
func formatKV(s, home string) []string {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(s), &obj); err != nil {
		// Plain text or non-object JSON — split on newlines and return as-is.
		return strings.Split(s, "\n")
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, k := range keys {
		lines = append(lines, k+": "+kvValue(obj[k], home))
	}
	return lines
}

// kvValue formats a single raw JSON value for KV display.
func kvValue(raw json.RawMessage, home string) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if home != "" && strings.HasPrefix(s, home) {
			s = "~" + s[len(home):]
		}
		return truncRunes(strings.ReplaceAll(s, "\n", "↵"), 60)
	}
	str := strings.TrimSpace(string(raw))
	if str == "true" || str == "false" || str == "null" {
		return str
	}
	if len(str) > 0 && (str[0] == '-' || (str[0] >= '0' && str[0] <= '9')) {
		return str // number
	}
	if len(str) > 0 && str[0] == '[' {
		var arr []json.RawMessage
		if err := json.Unmarshal(raw, &arr); err == nil {
			return fmt.Sprintf("[%d items]", len(arr))
		}
	}
	return "{…}" // nested object
}

// truncRunes truncates s to at most n runes, appending … if trimmed.
func truncRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// wrapString splits s into lines of at most width runes, splitting first on
// existing newlines then on the width boundary.
func wrapString(s string, width int) []string {
	if width <= 0 || s == "" {
		return []string{s}
	}
	var result []string
	for _, physical := range strings.Split(s, "\n") {
		runes := []rune(physical)
		if len(runes) == 0 {
			result = append(result, "")
			continue
		}
		for len(runes) > 0 {
			n := width
			if n > len(runes) {
				n = len(runes)
			}
			result = append(result, string(runes[:n]))
			runes = runes[n:]
		}
	}
	if len(result) == 0 {
		return []string{""}
	}
	return result
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

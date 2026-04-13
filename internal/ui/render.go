package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/hcarminati/orchard/internal/agent"
)

// Color palette used throughout the TUI.
var (
	colorAccent = lipgloss.Color("#7C3AED") // violet — used for active panel border and title
	colorMuted  = lipgloss.Color("#6B7280") // gray — used for inactive elements and descriptions
	colorGreen  = lipgloss.Color("#10B981") // green — running agent status dot / Skill tool label
	colorYellow = lipgloss.Color("#F59E0B") // yellow — idle agent status dot / Bash tool label
	colorRed    = lipgloss.Color("#EF4444") // red — error agent status dot
	colorFg     = lipgloss.Color("#F9FAFB") // near-white — primary text / Notification label
	colorBlue   = lipgloss.Color("#3B82F6") // blue — Read / WebFetch tool label
	colorCoral  = lipgloss.Color("#F87171") // coral — Edit / Write tool label
	colorTeal   = lipgloss.Color("#14B8A6") // teal — Grep / Glob tool label
)

// toolColor returns the label color for a tool name in the Events panel.
// Unknown tools fall back to colorFg so unrecognized names render in plain
// primary text rather than disappearing into the muted background.
func toolColor(tool string) lipgloss.Color {
	switch tool {
	case "Bash":
		return colorYellow
	case "Read", "WebFetch":
		return colorBlue
	case "Edit", "Write":
		return colorCoral
	case "Grep", "Glob", "ToolSearch", "WebSearch":
		return colorTeal
	case "Agent", "TaskCreate", "TaskUpdate":
		return colorAccent
	case "Skill":
		return colorGreen
	default:
		if strings.HasPrefix(tool, "mcp__") {
			return colorBlue
		}
		return colorFg
	}
}

// renderFooter builds the bottom keybindings bar.
func (m Model) renderFooter() string {
	bind := func(key, desc string) string {
		k := lipgloss.NewStyle().Bold(true).Foreground(colorFg).Render(key)
		d := lipgloss.NewStyle().Foreground(colorMuted).Render(" " + desc + "  ")
		return k + d
	}

	// Confirmation prompt replaces the normal bar while awaiting hide confirmation.
	if m.confirmHide != "" {
		label := m.confirmHide
		if len(label) > 8 {
			label = label[:8]
		}
		prompt := lipgloss.NewStyle().Foreground(colorFg).Render("Hide session:"+label+" from view?") +
			lipgloss.NewStyle().Foreground(colorMuted).Render("  ") +
			lipgloss.NewStyle().Bold(true).Foreground(colorYellow).Render("[y]") +
			lipgloss.NewStyle().Foreground(colorMuted).Render(" yes  ") +
			lipgloss.NewStyle().Bold(true).Foreground(colorFg).Render("[N]") +
			lipgloss.NewStyle().Foreground(colorMuted).Render(" cancel")
		gap := max(0, m.width-lipgloss.Width(prompt))
		return " " + prompt + strings.Repeat(" ", gap)
	}

	// Transient status message (e.g. "Cannot hide active session").
	if m.hideStatusMsg != "" {
		msg := lipgloss.NewStyle().Foreground(colorYellow).Render(m.hideStatusMsg)
		gap := max(0, m.width-lipgloss.Width(msg)-1)
		return " " + msg + strings.Repeat(" ", gap)
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

	// [d] hide — only for top-level non-running nodes in the agents panel, outside hidden view.
	if m.activePanel == panelAgents && m.statusFilter != filterHidden {
		vn := m.visibleNodes()
		if m.cursor < len(vn) {
			entry := vn[m.cursor]
			if entry.depth == 0 && entry.groupID == "" && m.agents.Nodes[entry.id] != nil {
				if m.effectiveStatus(entry.id) != agent.StatusRunning {
					content += bind("[d]", "hide")
				}
			}
		}
	}

	// [r] restore — only in the hidden filter view.
	if m.activePanel == panelAgents && m.statusFilter == filterHidden {
		vn := m.visibleNodes()
		if m.cursor < len(vn) {
			content += bind("[r]", "restore")
		}
	}

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
	visibleCount := 0
	for _, v := range m.visibleNodes() {
		if v.id != "" { // skip virtual group header rows
			visibleCount++
		}
	}
	agentCount := lipgloss.NewStyle().Foreground(leftBorderColor).Render(fmt.Sprintf(" · %d", visibleCount))
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

	// Context header shown for all nodes: "Spawned by" for child subagents,
	// session identification for root sessions.
	var headerLines []string
	mStyle := lipgloss.NewStyle().Foreground(colorMuted)
	divider := strings.Repeat("─", min(innerW, 48))
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
		promptLine := "prompt: " + prompt

		headerLines = []string{
			mStyle.Render(spawnLine),
		}
		if prompt != "" {
			headerLines = append(headerLines, mStyle.Render(promptLine))
		}
		headerLines = append(headerLines, mStyle.Render(divider))
	} else {
		sessionLabel := node.Name
		if sessionLabel == "" {
			if len(node.ID) > 8 {
				sessionLabel = "session:" + node.ID[:8]
			} else {
				sessionLabel = "session:" + node.ID
			}
		}
		headerLines = []string{
			mStyle.Render(sessionLabel),
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
	mutedStyle := lipgloss.NewStyle().Foreground(colorMuted)
	cursorStyle := lipgloss.NewStyle().Foreground(colorAccent)
	borderStyle := lipgloss.NewStyle().Foreground(colorAccent)

	// hasPendingPermission reports whether the event immediately after idx is a
	// PermissionRequest that is absorbed into this row's display.
	hasPendingPermission := func(idx int) bool {
		next := idx + 1
		return next < len(node.Events) && isAbsorbedPermission(node.Events, next)
	}

	home, _ := os.UserHomeDir()
	var allLines []string
	for idx, e := range node.Events {
		// Hidden events (absorbed PermissionRequests and internal sentinels)
		// are not rendered — skip them entirely.
		if isHiddenEvent(node.Events, idx) {
			continue
		}
		isSelected := idx == m.eventCursor
		key := eventKey(node.ID, idx)
		expanded := isToolEvent(e) && m.expandedEvents[key]

		prefix := "  "
		if isSelected {
			prefix = cursorStyle.Render("> ")
		}

		warningStyle := lipgloss.NewStyle().Foreground(colorYellow)
		ts := tsStyle.Render(e.Timestamp.Format("Jan 02 15:04:05"))
		header := prefix + ts + "  " + mutedStyle.Render(e.Type)

		switch e.Type {
		case "Notification":
			if expanded {
				header += "  " + mutedStyle.Render("▼")
			} else {
				if e.Message != "" {
					msg := truncRunes(strings.ReplaceAll(e.Message, "\n", " "), 40)
					header += "  " + mutedStyle.Render("►") + "  " + mutedStyle.Render(msg)
				}
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
			if expanded {
				header += "  " + mutedStyle.Render("▼")
			}
		case "SkillTrigger":
			skillStyle := lipgloss.NewStyle().Foreground(colorGreen)
			label := "⚡"
			if e.Tool != "" {
				label += " " + e.Tool
			}
			header += "  " + skillStyle.Render(label)
		default:
			if e.Tool != "" {
				header += "  " + lipgloss.NewStyle().Foreground(toolColor(e.Tool)).Render(e.Tool)
			}
			if isToolEvent(e) {
				if expanded {
					header += "  " + mutedStyle.Render("▼")
				} else if hasPendingPermission(idx) {
					header += "  " + warningStyle.Render("⚠")
				} else {
					header += "  " + mutedStyle.Render("▶")
				}
			}
		}

		if !expanded {
			if e.Type == "PermissionRequest" && e.Input != "" {
				preview := permissionPreview(e.Tool, e.Input, home)
				if preview != "" {
					header += "  " + mutedStyle.Render("►") + "  " + mutedStyle.Render(preview)
				}
				if lipgloss.Width(header) > innerW {
					header = lipgloss.NewStyle().MaxWidth(innerW-1).Render(header) + "…"
				}
				allLines = append(allLines, header)
			} else if isToolEvent(e) && e.Input != "" {
				preview := "  " + inputPreview(e.Input, home)
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
			// Expanded: header then bordered content block.
			allLines = append(allLines, header)
			bar := borderStyle.Render("│")
			if e.Type == "Notification" && e.Message != "" {
				allLines = append(allLines, bar+"  "+mutedStyle.Render("Message:"))
				for _, l := range strings.Split(e.Message, "\n") {
					line := bar + "    " + l
					if lipgloss.Width(line) > innerW {
						line = lipgloss.NewStyle().MaxWidth(innerW-1).Render(line) + "…"
					}
					allLines = append(allLines, line)
				}
			}
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
// the collapsed event row. It extracts the most informative string value and
// replaces the home directory prefix with ~.
func inputPreview(input, home string) string {
	tilde := func(s string) string {
		if home != "" && strings.HasPrefix(s, home) {
			return "~" + s[len(home):]
		}
		return s
	}
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
			return truncRunes(strings.ReplaceAll(tilde(s), "\n", " "), 20)
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
			return truncRunes(strings.ReplaceAll(tilde(s), "\n", " "), 20)
		}
	}
	return fmt.Sprintf("{%d fields}", len(obj))
}

// permissionPreview extracts the most relevant field from a PermissionRequest
// tool input for inline display in the collapsed event row.
//   - Bash       → command field
//   - Read/Edit  → file_path with home directory replaced by ~
//   - Write      → file_path (raw)
//   - other      → raw input truncated to 40 runes
func permissionPreview(tool, input, home string) string {
	if input == "" {
		return ""
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(input), &obj); err != nil {
		return truncRunes(strings.ReplaceAll(input, "\n", " "), 40)
	}
	getString := func(key string) (string, bool) {
		raw, ok := obj[key]
		if !ok {
			return "", false
		}
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", false
		}
		return s, true
	}
	switch tool {
	case "Bash":
		if s, ok := getString("command"); ok {
			return truncRunes(strings.ReplaceAll(s, "\n", " "), 40)
		}
	case "Read", "Edit":
		if s, ok := getString("file_path"); ok {
			if home != "" && strings.HasPrefix(s, home) {
				s = "~" + s[len(home):]
			}
			return truncRunes(s, 40)
		}
	case "Write":
		if s, ok := getString("file_path"); ok {
			if home != "" && strings.HasPrefix(s, home) {
				s = "~" + s[len(home):]
			}
			return truncRunes(s, 40)
		}
	}
	return truncRunes(strings.ReplaceAll(input, "\n", " "), 40)
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

// dimBody wraps each line of a pre-rendered body string in ANSI dim styling
// so it appears visually receded when a modal overlay is active.
func dimBody(body string) string {
	lines := strings.Split(body, "\n")
	for i, l := range lines {
		lines[i] = "\033[2m" + l + "\033[22m"
	}
	return strings.Join(lines, "\n")
}

// overlayCenter places the modal string centered over the background string.
// For lines within the modal's vertical range, the left portion (startX columns)
// is taken from the background; the modal line follows; spaces fill the remainder.
// Lines outside the modal range pass through unchanged.
func overlayCenter(bg, modal string, bgW, bgH int) string {
	bgLines := strings.Split(bg, "\n")
	modalLines := strings.Split(modal, "\n")

	modalW := 0
	for _, l := range modalLines {
		if w := lipgloss.Width(l); w > modalW {
			modalW = w
		}
	}
	modalH := len(modalLines)
	startX := (bgW - modalW) / 2
	startY := (bgH - modalH) / 2

	result := make([]string, bgH)
	for i := 0; i < bgH; i++ {
		bgLine := ""
		if i < len(bgLines) {
			bgLine = bgLines[i]
		}
		modalIdx := i - startY
		if modalIdx < 0 || modalIdx >= len(modalLines) {
			result[i] = bgLine
			continue
		}
		// Left slice of background: exactly startX visible columns.
		left := lipgloss.NewStyle().MaxWidth(startX).Render(bgLine)
		left = lipgloss.NewStyle().Width(startX).Render(left)
		// Right padding so the full terminal width is covered.
		right := strings.Repeat(" ", max(0, bgW-startX-modalW))
		result[i] = left + modalLines[modalIdx] + right
	}
	return strings.Join(result, "\n")
}

// modalEventPosition returns the 1-based position of m.eventCursor within
// the focused node's visible (non-absorbed) event list, and the total count.
func (m Model) modalEventPosition() (current, total int) {
	node := m.focusedNode()
	if node == nil {
		return 1, 1
	}
	pos := 0
	current = 1
	for i := range node.Events {
		if isHiddenEvent(node.Events, i) {
			continue
		}
		pos++
		if i == m.eventCursor {
			current = pos
		}
	}
	return current, pos
}

// renderModalFooterBar returns the single-line footer shown at the bottom of
// the terminal while the detail modal is open.
func (m Model) renderModalFooterBar() string {
	bind := func(key, desc string) string {
		k := lipgloss.NewStyle().Bold(true).Foreground(colorFg).Render(key)
		d := lipgloss.NewStyle().Foreground(colorMuted).Render(" " + desc + "  ")
		return k + d
	}
	content := " " + bind("[←/→]", "prev/next") + bind("[j/k]", "scroll") + bind("[esc]", "close")
	gap := max(0, m.width-lipgloss.Width(content))
	return content + strings.Repeat(" ", gap)
}

// formatDuration formats a duration as a human-readable string.
func formatDuration(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	mins := int(d.Minutes())
	secs := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%ds", mins, secs)
}

// kvValueFull formats a single raw JSON value for modal display without truncation.
func kvValueFull(raw json.RawMessage, home string) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if home != "" && strings.HasPrefix(s, home) {
			s = "~" + s[len(home):]
		}
		return s
	}
	str := strings.TrimSpace(string(raw))
	if str == "true" || str == "false" || str == "null" {
		return str
	}
	if len(str) > 0 && (str[0] == '-' || (str[0] >= '0' && str[0] <= '9')) {
		return str
	}
	if len(str) > 0 && str[0] == '[' {
		var arr []json.RawMessage
		if err := json.Unmarshal(raw, &arr); err == nil {
			return fmt.Sprintf("[%d items]", len(arr))
		}
	}
	return "{…}"
}

// formatKVModal formats a JSON string as key: value lines for the detail modal.
// Values are fully expanded (no truncation) and wrapped at width runes.
func formatKVModal(s, home string, width int) []string {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(s), &obj); err != nil {
		return wrapString(strings.TrimSpace(s), width)
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var lines []string
	for _, k := range keys {
		val := kvValueFull(obj[k], home)
		if strings.Contains(val, "\n") {
			lines = append(lines, k+":")
			for _, vl := range strings.Split(val, "\n") {
				lines = append(lines, wrapString("  "+vl, width)...)
			}
		} else {
			lines = append(lines, wrapString(k+": "+val, width)...)
		}
	}
	return lines
}

// buildModalContent constructs the scrollable lines for the detail modal body.
// formatInputAsDiff formats an Edit or Write tool input as a colored diff view.
// file_path and scalar fields are shown as key-value pairs above the diff block.
// old_string lines are prefixed with "- " in coral; new_string lines with "+ " in green.
// For Write, the "content" field is treated as a pure insertion (no old lines).
// The diff is capped at 20 lines; overflow is shown as "… (+N more lines)".
func formatInputAsDiff(input, home string, width int) []string {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(input), &obj); err != nil {
		return formatKVModal(input, home, width)
	}

	getString := func(key string) string {
		raw, ok := obj[key]
		if !ok {
			return ""
		}
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return ""
		}
		return s
	}

	tilde := func(s string) string {
		if home != "" && strings.HasPrefix(s, home) {
			return "~" + s[len(home):]
		}
		return s
	}

	muted := lipgloss.NewStyle().Foreground(colorMuted)
	removeStyle := lipgloss.NewStyle().Foreground(colorCoral)
	addStyle := lipgloss.NewStyle().Foreground(colorGreen)

	var lines []string

	// Metadata: file_path and replace_all shown as key-value pairs.
	if fp := getString("file_path"); fp != "" {
		lines = append(lines, "file_path: "+tilde(fp))
	}
	if raw, ok := obj["replace_all"]; ok {
		lines = append(lines, "replace_all: "+strings.TrimSpace(string(raw)))
	}

	oldStr := getString("old_string")
	newStr := getString("new_string")
	// Write uses "content" as the new content (pure insertion).
	if oldStr == "" && newStr == "" {
		newStr = getString("content")
	}

	if oldStr == "" && newStr == "" {
		return lines
	}

	lines = append(lines, "")
	divider := muted.Render(strings.Repeat("─", min(width, 41)))
	lines = append(lines, divider)

	const prefixW = 2  // length of "- " / "+ "
	const maxLines = 20

	var diffLines []string

	appendWrapped := func(style lipgloss.Style, prefix, s string) {
		s = strings.ReplaceAll(s, "\t", "    ") // expand tabs before rune-counting
		wrapped := wrapString(s, max(1, width-prefixW))
		for i, wl := range wrapped {
			if i == 0 {
				diffLines = append(diffLines, style.Render(prefix)+wl)
			} else {
				diffLines = append(diffLines, "  "+wl)
			}
		}
	}

	if oldStr != "" {
		for _, l := range strings.Split(oldStr, "\n") {
			appendWrapped(removeStyle, "- ", l)
		}
	}
	if newStr != "" {
		for _, l := range strings.Split(newStr, "\n") {
			appendWrapped(addStyle, "+ ", l)
		}
	}

	if len(diffLines) > maxLines {
		extra := len(diffLines) - maxLines
		diffLines = diffLines[:maxLines]
		diffLines = append(diffLines, muted.Render(fmt.Sprintf("… (+%d more lines)", extra)))
	}

	lines = append(lines, diffLines...)
	lines = append(lines, divider)
	return lines
}

func (m Model) buildModalContent(e agent.Event, node *agent.Node, home string, width int) []string {
	muted := lipgloss.NewStyle().Foreground(colorMuted)
	warn := lipgloss.NewStyle().Foreground(colorYellow).Bold(true)
	divider := muted.Render(strings.Repeat("─", min(width, 48)))

	var lines []string

	switch e.Type {
	case "PermissionRequest":
		lines = append(lines, warn.Render("⚠  Awaiting approval"), "")
	case "Stop", "SubagentStop":
		if len(node.Events) > 1 {
			first := node.Events[0]
			dur := e.Timestamp.Sub(first.Timestamp)
			if dur > 0 {
				lines = append(lines, muted.Render("Duration: "+formatDuration(dur)), "")
			}
		}
	}

	if e.Message != "" {
		lines = append(lines, muted.Render("Message:"))
		for _, l := range wrapString(e.Message, width-2) {
			lines = append(lines, "  "+l)
		}
		lines = append(lines, "")
	}

	if e.Input != "" {
		if e.Tool == "Edit" || e.Tool == "Write" {
			lines = append(lines, formatInputAsDiff(e.Input, home, width)...)
		} else {
			lines = append(lines, muted.Render("Input:"))
			for _, l := range formatKVModal(e.Input, home, width-2) {
				lines = append(lines, "  "+l)
			}
		}
		if e.Response != "" {
			lines = append(lines, "", divider, "")
		}
	}

	if e.Response != "" {
		lines = append(lines, muted.Render("Output:"))
		for _, l := range formatKVModal(e.Response, home, width-2) {
			lines = append(lines, "  "+l)
		}
	}

	return lines
}

// renderDetailModal renders the full-screen detail modal overlay for the
// currently focused event. bodyH is the height of the body area (excluding footer).
func (m Model) renderDetailModal(bodyH int) string {
	node := m.focusedNode()
	if node == nil || len(node.Events) == 0 {
		return ""
	}
	idx := m.eventCursor
	if idx >= len(node.Events) {
		idx = len(node.Events) - 1
	}
	e := node.Events[idx]
	home, _ := os.UserHomeDir()

	modalW := max(20, m.width*80/100)
	modalH := max(6, bodyH*80/100)

	// Inner width: border(1) + space(1) on each side = 4 total.
	innerW := max(1, modalW-4)
	// Height layout: top border(1) + content + separator(1) + footer(1) + bottom border(1).
	contentH := max(1, modalH-4)

	contentLines := m.buildModalContent(e, node, home, innerW)

	// Clamp modal scroll.
	maxScroll := max(0, len(contentLines)-contentH)
	scroll := min(m.modalScroll, maxScroll)
	visEnd := min(scroll+contentH, len(contentLines))
	visible := contentLines[scroll:visEnd]

	// Pad to contentH so the box is a fixed size.
	padded := make([]string, contentH)
	for i := range padded {
		if i < len(visible) {
			padded[i] = visible[i]
		}
	}

	bs := lipgloss.NewStyle().Foreground(colorAccent)

	// Title: "EventType · ToolName  (cur / tot)"
	cur, tot := m.modalEventPosition()
	posStr := lipgloss.NewStyle().Foreground(colorMuted).Render(fmt.Sprintf("(%d / %d)", cur, tot))
	title := lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Render(e.Type)
	if e.Tool != "" {
		title += lipgloss.NewStyle().Foreground(colorMuted).Render(" · ") +
			lipgloss.NewStyle().Foreground(toolColor(e.Tool)).Render(e.Tool)
	}
	title += "  " + posStr

	titleW := lipgloss.Width(title)
	topDashes := max(0, modalW-3-titleW)
	topBorder := bs.Render("╭─") + title + bs.Render(strings.Repeat("─", topDashes)+"╮")

	sep := bs.Render("├") + bs.Render(strings.Repeat("─", modalW-2)) + bs.Render("┤")

	// Internal footer hints.
	footerMuted := lipgloss.NewStyle().Foreground(colorMuted)
	footerBold := lipgloss.NewStyle().Bold(true).Foreground(colorFg)
	footerText := footerBold.Render("[←/→]") + footerMuted.Render(" prev/next  ") +
		footerBold.Render("[j/k]") + footerMuted.Render(" scroll  ") +
		footerBold.Render("[esc]") + footerMuted.Render(" close")
	footerPad := strings.Repeat(" ", max(0, innerW-lipgloss.Width(footerText)))
	footerLine := bs.Render("│") + " " + footerText + footerPad + " " + bs.Render("│")

	bottomBorder := bs.Render("╰") + bs.Render(strings.Repeat("─", modalW-2)) + bs.Render("╯")

	var out []string
	out = append(out, topBorder)
	for _, cl := range padded {
		if lipgloss.Width(cl) > innerW {
			cl = lipgloss.NewStyle().MaxWidth(innerW - 1).Render(cl) + "…"
		}
		rendered := lipgloss.NewStyle().Width(innerW).Render(cl)
		out = append(out, bs.Render("│")+" "+rendered+" "+bs.Render("│"))
	}
	out = append(out, sep, footerLine, bottomBorder)

	return strings.Join(out, "\n")
}

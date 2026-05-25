package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// tokenCost holds per-million-token USD prices for one model.
type tokenCost struct {
	inputPer1M      float64
	outputPer1M     float64
	cacheWritePer1M float64
	cacheReadPer1M  float64
}

// modelCosts maps known model variants to their published Claude API pricing.
var modelCosts = map[agent.Model]tokenCost{
	agent.ModelHaiku:  {inputPer1M: 0.80, outputPer1M: 4.00, cacheWritePer1M: 1.00, cacheReadPer1M: 0.08},
	agent.ModelSonnet: {inputPer1M: 3.00, outputPer1M: 15.00, cacheWritePer1M: 3.75, cacheReadPer1M: 0.30},
	agent.ModelOpus:   {inputPer1M: 15.00, outputPer1M: 75.00, cacheWritePer1M: 18.75, cacheReadPer1M: 1.50},
}

// estimateCost returns the estimated USD cost for u at the given model's pricing.
// Returns 0 when the model has no known pricing or usage is zero.
func estimateCost(u agent.Usage, m agent.Model) float64 {
	p, ok := modelCosts[m]
	if !ok || u.IsZero() {
		return 0
	}
	return (float64(u.InputTokens)*p.inputPer1M +
		float64(u.OutputTokens)*p.outputPer1M +
		float64(u.CacheCreationInputTokens)*p.cacheWritePer1M +
		float64(u.CacheReadInputTokens)*p.cacheReadPer1M) / 1_000_000
}

// formatCost formats a USD cost for compact display.
// Returns empty string for zero cost.
// ≥ $10: integer ("$167"); < $1 or $1–$9.99: two decimal places ("$0.50", "$3.14").
func formatCost(cost float64) string {
	if cost == 0 {
		return ""
	}
	if cost >= 10 {
		return fmt.Sprintf("$%d", int(cost))
	}
	return fmt.Sprintf("$%.2f", cost)
}

// formatTokenCount formats an integer token count for compact display:
// values below 1000 are shown as-is; thousands are shown as "2.3k"; millions as "1.2M".
func formatTokenCount(n int) string {
	if n >= 1_000_000 {
		s := strings.TrimRight(fmt.Sprintf("%.1f", float64(n)/1_000_000), "0")
		return strings.TrimRight(s, ".") + "M"
	}
	if n >= 1000 {
		s := strings.TrimRight(fmt.Sprintf("%.1f", float64(n)/1000), "0")
		return strings.TrimRight(s, ".") + "k"
	}
	return fmt.Sprintf("%d", n)
}

// subtreeCost returns the total estimated cost for nodeID and all its descendants.
// Each node's cost is computed using its own model so mixed-model subtrees are
// priced correctly.
func (m Model) subtreeCost(nodeID string) float64 {
	n := m.agents.Nodes[nodeID]
	if n == nil {
		return 0
	}
	cost := estimateCost(n.Usage, n.Model)
	for _, childID := range n.Children {
		cost += m.subtreeCost(childID)
	}
	return cost
}

// totalCost returns the total estimated cost across all sessions.
func (m Model) totalCost() float64 {
	var total float64
	for _, rootID := range m.agents.Roots {
		total += m.subtreeCost(rootID)
	}
	return total
}

// subtreeTokens returns the total token count for nodeID and all its descendants.
func (m Model) subtreeTokens(nodeID string) int {
	n := m.agents.Nodes[nodeID]
	if n == nil {
		return 0
	}
	u := n.Usage
	total := u.InputTokens + u.OutputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
	for _, childID := range n.Children {
		total += m.subtreeTokens(childID)
	}
	return total
}

// totalTokens returns the total token count across all sessions.
func (m Model) totalTokens() int {
	var total int
	for _, rootID := range m.agents.Roots {
		total += m.subtreeTokens(rootID)
	}
	return total
}

// renderTokensWithMax formats totalTok for display in the Agents title.
// With no max: "1.2M" in the standard border color.
// With max: "1.2M/5M" — amber when ≥80% of max, red when ≥100%.
func (m Model) renderTokensWithMax(totalTok int, borderColor lipgloss.Color) string {
	tokStr := formatTokenCount(totalTok)
	if m.maxTokens <= 0 {
		return lipgloss.NewStyle().Foreground(borderColor).Render(tokStr)
	}
	ratio := float64(totalTok) / float64(m.maxTokens)
	tokColor := budgetCostColor(ratio, borderColor)
	maxStr := formatTokenCount(m.maxTokens)
	return lipgloss.NewStyle().Foreground(tokColor).Render(tokStr + "/" + maxStr)
}

// budgetCostColor returns the color for cost display given the spend ratio and
// the panel's border color. Exported-by-name only within the package for tests.
func budgetCostColor(ratio float64, borderColor lipgloss.Color) lipgloss.Color {
	switch {
	case ratio >= 1.0:
		return colorRed
	case ratio >= 0.8:
		return colorYellow
	default:
		return borderColor
	}
}

// renderCostWithBudget formats totalCost for display in the Agents title.
// With no budget: "$167" in the standard border color.
// With budget: "$167/$300" — amber when ≥80% of budget, red when ≥100%.
func (m Model) renderCostWithBudget(totalCost float64, borderColor lipgloss.Color) string {
	costStr := formatCost(totalCost)
	if m.budget <= 0 {
		return lipgloss.NewStyle().Foreground(borderColor).Render(costStr)
	}
	ratio := totalCost / m.budget
	costColor := budgetCostColor(ratio, borderColor)
	budgetStr := formatCost(m.budget)
	return lipgloss.NewStyle().Foreground(costColor).Render(costStr + "/" + budgetStr)
}

// renderCostAlertBanner returns a one-line amber banner when the focused
// session's cost exceeds costAlert and the user hasn't dismissed it.
// Returns "" when the alert is not triggered or the threshold is disabled.
func (m Model) renderCostAlertBanner() string {
	if m.costAlert <= 0 {
		return ""
	}
	node := m.focusedNode()
	if node == nil {
		return ""
	}
	// Walk up to the root session so the cost is for the whole session.
	rootID := node.ID
	for {
		n := m.agents.Nodes[rootID]
		if n == nil || n.ParentID == "" {
			break
		}
		rootID = n.ParentID
	}
	if m.costAlertDismiss[rootID] {
		return ""
	}
	sessionCost := m.subtreeCost(rootID)
	if sessionCost <= m.costAlert {
		return ""
	}
	msg := fmt.Sprintf("⚠ Cost alert: session cost %s exceeded threshold %s   esc to dismiss",
		formatCost(sessionCost), formatCost(m.costAlert))
	banner := lipgloss.NewStyle().
		Foreground(colorYellow).
		Width(m.width).
		Render(" " + msg)
	return banner
}

// renderSearchBar returns a one-line search input bar when the search is open.
func (m Model) renderSearchBar() string {
	if !m.searchOpen {
		return ""
	}
	cursor := lipgloss.NewStyle().Foreground(colorAccent).Render("█")
	query := lipgloss.NewStyle().Foreground(colorFg).Render(m.searchQuery)
	prefix := lipgloss.NewStyle().Foreground(colorMuted).Render("/")
	bar := " " + prefix + query + cursor
	gap := max(0, m.width-lipgloss.Width(bar))
	return bar + strings.Repeat(" ", gap)
}

// loopBadge returns a warning string when a node is stuck in a tool-call loop,
// empty string otherwise. The badge shows the tool name and repetition count
// so the developer can see at a glance what is repeating.
func loopBadge(n *agent.Node) string {
	if n.ConsecutiveTools < agent.LoopThreshold {
		return ""
	}
	label := fmt.Sprintf("⚠ loop×%d", n.ConsecutiveTools)
	return lipgloss.NewStyle().Foreground(colorYellow).Bold(true).Render(label)
}

// watchdogBadge returns a badge string for the watchdog timer state.
// Returns "⏱" (amber) at the first threshold and "⏱⏱" (red) at the second.
func (m Model) watchdogBadge(nodeID string) string {
	count := m.watchdogFired[nodeID]
	switch count {
	case 1:
		return lipgloss.NewStyle().Foreground(colorYellow).Bold(true).Render("⏱")
	case 2:
		return lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render("⏱⏱")
	default:
		if count > 2 {
			return lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render("⏱⏱")
		}
		return ""
	}
}

// shortToolName returns a compact display label for a tool name.
// MCP tools (prefixed "mcp__") are abbreviated to "mcp:suffix" where suffix is
// the portion after the last "__". All other names are lowercased.
func shortToolName(tool string) string {
	if strings.HasPrefix(tool, "mcp__") {
		parts := strings.Split(tool, "__")
		return "mcp:" + strings.ToLower(parts[len(parts)-1])
	}
	return strings.ToLower(tool)
}

// nodePills returns a compact pill string rendered for a node row.
// Tool names are shown in their semantic color; skill names get a green ▸ prefix.
// Returns empty string when the node has no recorded tools or skills.
func nodePills(n *agent.Node) string {
	if len(n.Tools) == 0 && len(n.Skills) == 0 {
		return ""
	}
	var parts []string
	for _, t := range n.Tools {
		parts = append(parts, lipgloss.NewStyle().Foreground(toolColor(t)).Render(shortToolName(t)))
	}
	for _, s := range n.Skills {
		name := s
		if strings.Contains(s, ":") {
			// "figma:use" → "use", keep short
			idx := strings.LastIndex(s, ":")
			name = s[idx+1:]
		}
		parts = append(parts, lipgloss.NewStyle().Foreground(colorGreen).Render("▸"+name))
	}
	return strings.Join(parts, " ")
}

// statusSummary counts running/idle/done/error across all visible real nodes
// (group header rows are skipped). Only non-zero buckets are included in the
// returned parts slice, ordered: running → idle → done → error.
func (m Model) statusSummary() []string {
	var running, idle, done, errored int
	for _, v := range m.visibleNodes() {
		if v.id == "" {
			continue // skip group header rows
		}
		n := m.agents.Nodes[v.id]
		if n == nil {
			continue
		}
		switch n.Status {
		case agent.StatusRunning:
			running++
		case agent.StatusIdle:
			idle++
		case agent.StatusDone:
			done++
		case agent.StatusError:
			errored++
		}
	}
	var parts []string
	if running > 0 {
		parts = append(parts, fmt.Sprintf("%d running", running))
	}
	if idle > 0 {
		parts = append(parts, fmt.Sprintf("%d idle", idle))
	}
	if done > 0 {
		parts = append(parts, fmt.Sprintf("%d done", done))
	}
	if errored > 0 {
		parts = append(parts, fmt.Sprintf("%d error", errored))
	}
	return parts
}

// statusLabel returns a short lowercase string for a node status, used in the events header.
func statusLabel(s agent.Status) string {
	switch s {
	case agent.StatusRunning:
		return "running"
	case agent.StatusIdle:
		return "idle"
	case agent.StatusDone:
		return "done"
	case agent.StatusError:
		return "error"
	default:
		return ""
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

	// Search bar replaces the normal footer while search is open.
	if m.searchOpen {
		return m.renderSearchBar()
	}

	content := " " + bind("j/k", "navigate")
	if m.timelineMode {
		content += bind("t", "tree view")
	} else {
		content += bind("t", "timeline")
	}
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
	content += bind("/", "search") + bind("?", "help") + bind("tab", "switch panel") + bind("q", "quit")

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
// agentsPanelW returns the width of the left (Agents) panel.
// The panel takes ~40% of the terminal width, bounded between a minimum of 32
// and a ceiling that ensures the events panel always gets at least 50 columns.
func (m Model) agentsPanelW() int {
	const agentsMin = 32
	const eventsMin = 50
	w := m.width * 40 / 100
	return max(agentsMin, min(w, m.width-eventsMin))
}

func (m Model) renderBody(height int) string {
	leftW := m.agentsPanelW()
	rightW := m.width - leftW

	// Title color matches border color (active = accent, inactive = muted).
	leftActive := m.activePanel == panelAgents
	leftBorderColor := colorMuted
	if leftActive {
		leftBorderColor = colorAccent
	}
	agentsTitleText := "Agents"
	if m.timelineMode {
		agentsTitleText = "Agents [Timeline]"
	}
	agentsTitle := lipgloss.NewStyle().Bold(true).Foreground(leftBorderColor).Render(agentsTitleText)
	sep := lipgloss.NewStyle().Foreground(leftBorderColor).Render(" · ")
	// Status summary: "3 running · 1 idle · 2 done" (only non-zero buckets).
	for _, part := range m.statusSummary() {
		agentsTitle += sep + lipgloss.NewStyle().Foreground(leftBorderColor).Render(part)
	}
	if totalCost := m.totalCost(); totalCost > 0 {
		agentsTitle += sep + m.renderCostWithBudget(totalCost, leftBorderColor)
	}
	if totalTok := m.totalTokens(); totalTok > 0 {
		agentsTitle += sep + m.renderTokensWithMax(totalTok, leftBorderColor)
	}
	var leftContent string
	if m.timelineMode {
		leftContent = m.timelineContent()
	} else {
		leftContent = m.agentsContent()
	}
	left := m.renderPanel(agentsTitle, leftContent, leftW, height, leftActive)

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
// When the focused agent has a known model, it is appended as a muted badge.
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
	title := strings.Join(parts, " ")

	if node := m.focusedNode(); node != nil {
		if node.Model != agent.ModelUnknown {
			title += "  " + lipgloss.NewStyle().Foreground(colorMuted).Render("· "+string(node.Model))
		}
		if !node.Usage.IsZero() {
			tokStr := "↑" + formatTokenCount(node.Usage.InputTokens) + " ↓" + formatTokenCount(node.Usage.OutputTokens)
			title += "  " + lipgloss.NewStyle().Foreground(colorMuted).Render("· "+tokStr)
			if cost := formatCost(estimateCost(node.Usage, node.Model)); cost != "" {
				title += "  " + lipgloss.NewStyle().Foreground(colorMuted).Render("· "+cost)
			}
		}
	}

	return title
}

// rightTabContent returns the body content for the currently active right panel tab.
func (m Model) rightTabContent() string {
	if m.activeRightTab < len(rightTabs) {
		switch rightTabs[m.activeRightTab] {
		case tabEvents:
			return m.eventsContent()
		case tabFiles:
			return m.filesContent()
		case tabMCP:
			return m.mcpContent()
		}
	}
	return ""
}

// mcpContent renders the MCP Servers tab: lists servers from ~/.claude/settings.json.
func (m Model) mcpContent() string {
	muted := lipgloss.NewStyle().Foreground(colorMuted).Padding(0, 1)

	servers, err := loadMCPServers()
	if err != nil || len(servers) == 0 {
		return muted.Render("No MCP servers configured. Add them to ~/.claude/settings.json.")
	}

	innerW := m.innerWidth()
	var lines []string
	nameStyle := lipgloss.NewStyle().Bold(true).Foreground(colorFg)
	descStyle := lipgloss.NewStyle().Foreground(colorMuted)

	for _, s := range servers {
		nameLine := nameStyle.Render(s.Name)
		if s.Command != "" {
			cmd := s.Command
			if len([]rune(cmd)) > innerW-4 {
				cmd = string([]rune(cmd)[:innerW-5]) + "…"
			}
			lines = append(lines, " "+nameLine)
			lines = append(lines, "   "+descStyle.Render(cmd))
		} else if s.URL != "" {
			lines = append(lines, " "+nameLine)
			lines = append(lines, "   "+descStyle.Render(s.URL))
		} else {
			lines = append(lines, " "+nameLine)
		}
		lines = append(lines, "")
	}
	// Trim trailing blank line.
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

// mcpServer holds the parsed configuration for a single MCP server entry.
type mcpServer struct {
	Name    string
	Command string
	URL     string
}

// loadMCPServers reads ~/.claude/settings.json and returns the list of configured MCP servers.
// Returns nil and no error when the file does not exist or has no mcpServers key.
func loadMCPServers() ([]mcpServer, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var raw struct {
		MCPServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
			URL     string   `json:"url"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	servers := make([]mcpServer, 0, len(raw.MCPServers))
	names := make([]string, 0, len(raw.MCPServers))
	for n := range raw.MCPServers {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		entry := raw.MCPServers[n]
		cmd := entry.Command
		if cmd != "" && len(entry.Args) > 0 {
			cmd += " " + strings.Join(entry.Args, " ")
		}
		servers = append(servers, mcpServer{Name: n, Command: cmd, URL: entry.URL})
	}
	return servers, nil
}

// helpSection groups related keybindings for the help overlay.
type helpSection struct {
	title    string
	bindings [][2]string // [key, description] pairs
}

// helpSections returns all sections to display in the help overlay.
func helpSections() []helpSection {
	return []helpSection{
		{
			title: "Navigation",
			bindings: [][2]string{
				{"j / ↓", "move cursor down"},
				{"k / ↑", "move cursor up"},
				{"tab", "switch panel focus"},
				{"space", "expand / collapse node"},
			},
		},
		{
			title: "Events",
			bindings: [][2]string{
				{"enter", "open detail modal / mark winner"},
				{"g", "jump to first event"},
				{"G", "jump to last event"},
				{"← / →", "prev / next event (in modal)"},
				{"esc", "close modal"},
				{"/", "open fuzzy search"},
			},
		},
		{
			title: "Tree",
			bindings: [][2]string{
				{"f", "cycle filter: all → running → errored → hidden"},
				{"t / T", "toggle timeline view"},
				{"d", "hide session (top-level, non-running)"},
				{"r", "restore hidden session"},
			},
		},
		{
			title: "View Modes",
			bindings: [][2]string{
				{"[ / ]", "switch right-panel tab"},
			},
		},
		{
			title: "Session Management",
			bindings: [][2]string{
				{"q / ctrl+c", "quit"},
				{"?", "toggle this help overlay"},
			},
		},
	}
}

// renderHelpOverlay renders the full-screen help overlay showing all keybindings.
func (m Model) renderHelpOverlay(bodyH int) string {
	bs := lipgloss.NewStyle().Foreground(colorAccent)
	heading := lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(colorFg)
	descStyle := lipgloss.NewStyle().Foreground(colorMuted)
	mutedStyle := lipgloss.NewStyle().Foreground(colorMuted)

	modalW := max(50, m.width*70/100)
	innerW := max(1, modalW-4)

	var contentLines []string
	contentLines = append(contentLines, heading.Render("Current State"))
	filterLabel := lipgloss.NewStyle().Foreground(colorFg).Render("filter: " + m.statusFilter.label())
	panelLabel := "agents"
	if m.activePanel == panelEvents {
		panelLabel = "events"
	}
	activeLabel := lipgloss.NewStyle().Foreground(colorFg).Render("panel: " + panelLabel)
	tabLabel := lipgloss.NewStyle().Foreground(colorFg).Render("tab: " + rightTabs[m.activeRightTab].label())
	contentLines = append(contentLines, "  "+filterLabel+"   "+activeLabel+"   "+tabLabel)
	contentLines = append(contentLines, mutedStyle.Render(strings.Repeat("─", min(innerW, 44))))
	contentLines = append(contentLines, "")

	const keyColW = 18
	for _, sec := range helpSections() {
		contentLines = append(contentLines, heading.Render(sec.title))
		for _, b := range sec.bindings {
			rendered := keyStyle.Render(b[0])
			padW := max(0, keyColW-lipgloss.Width(rendered))
			line := "  " + rendered + strings.Repeat(" ", padW) + descStyle.Render(b[1])
			contentLines = append(contentLines, line)
		}
		contentLines = append(contentLines, "")
	}
	for len(contentLines) > 0 && contentLines[len(contentLines)-1] == "" {
		contentLines = contentLines[:len(contentLines)-1]
	}

	modalH := max(8, len(contentLines)+4)
	if modalH > bodyH-2 {
		modalH = bodyH - 2
	}
	contentH := max(1, modalH-4)

	visEnd := min(contentH, len(contentLines))
	visible := contentLines[:visEnd]
	padded := make([]string, contentH)
	for i := range padded {
		if i < len(visible) {
			padded[i] = visible[i]
		}
	}

	title := heading.Render("Help") + mutedStyle.Render("  · ?/esc/q to close")
	titleW := lipgloss.Width(title)
	topDashes := max(0, modalW-3-titleW)
	topBorder := bs.Render("╭─") + title + bs.Render(strings.Repeat("─", topDashes)+"╮")
	sep := bs.Render("├") + bs.Render(strings.Repeat("─", modalW-2)) + bs.Render("┤")

	footerMuted := lipgloss.NewStyle().Foreground(colorMuted)
	footerBold := lipgloss.NewStyle().Bold(true).Foreground(colorFg)
	footerText := footerBold.Render("[?/esc/q]") + footerMuted.Render(" close")
	footerPad := strings.Repeat(" ", max(0, innerW-lipgloss.Width(footerText)))
	footerLine := bs.Render("│") + " " + footerText + footerPad + " " + bs.Render("│")
	bottomBorder := bs.Render("╰") + bs.Render(strings.Repeat("─", modalW-2)) + bs.Render("╯")

	var out []string
	out = append(out, topBorder)
	for _, cl := range padded {
		if lipgloss.Width(cl) > innerW {
			cl = lipgloss.NewStyle().MaxWidth(innerW-1).Render(cl) + "…"
		}
		rendered := lipgloss.NewStyle().Width(innerW).Render(cl)
		out = append(out, bs.Render("│")+" "+rendered+" "+bs.Render("│"))
	}
	out = append(out, sep, footerLine, bottomBorder)
	return strings.Join(out, "\n")
}

// fileChange holds one file-modification event for display in the Files tab.
type fileChange struct {
	op        string    // "write", "edit", "create", "notebook"
	path      string    // file path from tool input
	agentName string    // name of the agent that made the change
	when      time.Time // event timestamp
}

// fileChangeTool reports whether a PostToolUse event represents a file write/edit.
func fileChangeTool(tool string) (op string, ok bool) {
	switch tool {
	case "Write":
		return "write", true
	case "Edit", "MultiEdit":
		return "edit", true
	case "NotebookEdit":
		return "notebook", true
	default:
		return "", false
	}
}

// extractFilePath parses file_path from a tool input JSON string.
// Returns empty string when the field is absent or the JSON is malformed.
func extractFilePath(input string) string {
	var v struct {
		FilePath string `json:"file_path"`
	}
	if err := json.Unmarshal([]byte(input), &v); err != nil {
		return ""
	}
	return v.FilePath
}

// allFileChanges collects all file-write events across every agent node,
// ordered oldest-first.
func (m Model) allFileChanges() []fileChange {
	var changes []fileChange
	// Walk nodes in tree order so changes are collected in a predictable sequence.
	var walk func(nodeID string)
	walk = func(nodeID string) {
		n := m.agents.Nodes[nodeID]
		if n == nil {
			return
		}
		for _, e := range n.Events {
			if e.Type != "PostToolUse" {
				continue
			}
			op, ok := fileChangeTool(e.Tool)
			if !ok {
				continue
			}
			path := extractFilePath(e.Input)
			if path == "" {
				continue
			}
			changes = append(changes, fileChange{
				op:        op,
				path:      path,
				agentName: n.Name,
				when:      e.Timestamp,
			})
		}
		for _, childID := range n.Children {
			walk(childID)
		}
	}
	for _, rootID := range m.sortedRoots() {
		walk(rootID)
	}
	return changes
}

// filesContent renders the Files tab: all file writes/edits across every agent,
// most-recent first, with operation label, truncated path, agent, and relative time.
func (m Model) filesContent() string {
	muted := lipgloss.NewStyle().Foreground(colorMuted).Padding(0, 1)

	if !m.hasSession {
		return muted.Render("Waiting for session…")
	}

	changes := m.allFileChanges()
	if len(changes) == 0 {
		return muted.Render("No file writes or edits recorded yet.")
	}

	innerW := m.innerWidth()

	// Show most-recent changes first.
	lines := make([]string, 0, len(changes))
	now := time.Now()
	for i := len(changes) - 1; i >= 0; i-- {
		fc := changes[i]

		// Operation label with color.
		var opColor lipgloss.Color
		switch fc.op {
		case "write":
			opColor = colorCoral
		case "edit":
			opColor = colorBlue
		default:
			opColor = colorTeal
		}
		opLabel := lipgloss.NewStyle().Foreground(opColor).Render(fmt.Sprintf("%-7s", fc.op))

		// Relative timestamp.
		age := now.Sub(fc.when)
		var ageStr string
		switch {
		case age < time.Minute:
			ageStr = "just now"
		case age < time.Hour:
			ageStr = fmt.Sprintf("%dm ago", int(age.Minutes()))
		default:
			ageStr = fmt.Sprintf("%dh ago", int(age.Hours()))
		}
		ageLabel := lipgloss.NewStyle().Foreground(colorMuted).Render(ageStr)

		// Agent name (capped).
		agent := fc.agentName
		if len([]rune(agent)) > 12 {
			agent = string([]rune(agent)[:11]) + "…"
		}
		agentLabel := lipgloss.NewStyle().Foreground(colorMuted).Render(agent)

		// File path — use the budget remaining after labels.
		// layout: "op      path…   agent  age"
		labelW := 7 + 1 + 12 + 1 + 8 + 2 // rough column budget
		pathBudget := max(10, innerW-labelW)
		path := fc.path
		// Show only the last N path components when path is long.
		if len([]rune(path)) > pathBudget {
			path = "…" + string([]rune(path)[len([]rune(path))-pathBudget+1:])
		}
		pathLabel := lipgloss.NewStyle().Foreground(colorFg).Render(path)

		line := opLabel + " " + pathLabel + "  " + agentLabel + "  " + ageLabel
		if lipgloss.Width(line) > innerW {
			line = lipgloss.NewStyle().MaxWidth(innerW-1).Render(line) + "…"
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
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
		// Truncate the title if it would overflow the panel width.
		maxTitleW := max(0, width-3)
		titleW := lipgloss.Width(title)
		if titleW > maxTitleW {
			title = lipgloss.NewStyle().MaxWidth(max(0, maxTitleW-1)).Render(title) + "…"
			titleW = lipgloss.Width(title)
		}
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
	innerW := max(1, m.agentsPanelW()-2)

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
			nameContent := prefix + icon + dot(statusColor(n.Status)) + " " + n.Name + indicator
			if n.Status == agent.StatusError && n.ErrorMsg != "" {
				nameContent += " " + lipgloss.NewStyle().Foreground(colorRed).Render("✗ "+n.ErrorMsg)
			} else {
				if wb := m.watchdogBadge(n.ID); wb != "" {
					nameContent += "  " + wb
				}
				if badge := loopBadge(n); badge != "" {
					nameContent += "  " + badge
				} else if pills := nodePills(n); pills != "" {
					nameContent += "  " + pills
				}
			}
			line := truncate(nameContent)
			if hasWinner && !n.Winner {
				line = lipgloss.NewStyle().Foreground(colorMuted).Render(line)
			}
			lines = append(lines, line)
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
		nameContent := prefix + connector + icon + dot(statusColor(n.Status)) + " " + n.Name
		if n.Status == agent.StatusError && n.ErrorMsg != "" {
			nameContent += " " + lipgloss.NewStyle().Foreground(colorRed).Render("✗ "+n.ErrorMsg)
		} else {
			if wb := m.watchdogBadge(n.ID); wb != "" {
				nameContent += "  " + wb
			}
			if badge := loopBadge(n); badge != "" {
				nameContent += "  " + badge
			} else if pills := nodePills(n); pills != "" {
				nameContent += "  " + pills
			}
		}
		line := truncate(nameContent)
		lines = append(lines, line)
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
		if lipgloss.Width(spawnLine) > innerW {
			spawnLine = lipgloss.NewStyle().MaxWidth(innerW-1).Render(spawnLine) + "…"
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
		headerLabel := sessionLabel
		childCount := len(node.Children)
		if childCount > 0 {
			noun := "subagent"
			if childCount != 1 {
				noun = "subagents"
			}
			headerLabel += fmt.Sprintf("  ·  %d %s", childCount, noun)
		}
		headerLabel += "  ·  " + statusLabel(node.Status)
		if lipgloss.Width(headerLabel) > innerW {
			headerLabel = lipgloss.NewStyle().MaxWidth(innerW-1).Render(headerLabel) + "…"
		}
		headerLines = []string{
			mStyle.Render(headerLabel),
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

	// searchFilter returns true if the event matches the current search query.
	// Empty query matches all events.
	searchFilter := func(e agent.Event) bool {
		if m.searchQuery == "" {
			return true
		}
		q := strings.ToLower(m.searchQuery)
		return strings.Contains(strings.ToLower(e.Tool), q) ||
			strings.Contains(strings.ToLower(e.Type), q) ||
			strings.Contains(strings.ToLower(e.Input), q) ||
			strings.Contains(strings.ToLower(e.Response), q) ||
			strings.Contains(strings.ToLower(e.Message), q)
	}

	home, _ := os.UserHomeDir()
	var allLines []string
	for idx, e := range node.Events {
		// Hidden events (absorbed PermissionRequests and internal sentinels)
		// are not rendered — skip them entirely.
		if isHiddenEvent(node.Events, idx) {
			continue
		}
		// Search filter: skip events that don't match the current query.
		if !searchFilter(e) {
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
					msg := strings.ReplaceAll(e.Message, "\n", " ")
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

// inputPreview returns a summary of a JSON tool input for the collapsed event
// row. It extracts the most informative string value and replaces the home
// directory prefix with ~. No truncation is applied here; the caller clips the
// full row to the panel width.
func inputPreview(input, home string) string {
	tilde := func(s string) string {
		if home != "" && strings.HasPrefix(s, home) {
			return "~" + s[len(home):]
		}
		return s
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(input), &obj); err != nil {
		return strings.ReplaceAll(input, "\n", " ")
	}
	// Prefer high-signal keys that tend to carry the most context.
	for _, k := range []string{"command", "cmd", "file_path", "path", "pattern", "query", "prompt"} {
		raw, ok := obj[k]
		if !ok {
			continue
		}
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			return strings.ReplaceAll(tilde(s), "\n", " ")
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
			return strings.ReplaceAll(tilde(s), "\n", " ")
		}
	}
	return fmt.Sprintf("{%d fields}", len(obj))
}

// permissionPreview extracts the most relevant field from a PermissionRequest
// tool input for inline display in the collapsed event row.
//   - Bash       → command field
//   - Read/Edit  → file_path with home directory replaced by ~
//   - Write      → file_path (raw)
//   - other      → raw input (newlines replaced)
//
// No truncation is applied here; the caller clips the full row to the panel width.
func permissionPreview(tool, input, home string) string {
	if input == "" {
		return ""
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(input), &obj); err != nil {
		return strings.ReplaceAll(input, "\n", " ")
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
			return strings.ReplaceAll(s, "\n", " ")
		}
	case "Read", "Edit":
		if s, ok := getString("file_path"); ok {
			if home != "" && strings.HasPrefix(s, home) {
				s = "~" + s[len(home):]
			}
			return s
		}
	case "Write":
		if s, ok := getString("file_path"); ok {
			if home != "" && strings.HasPrefix(s, home) {
				s = "~" + s[len(home):]
			}
			return s
		}
	}
	return strings.ReplaceAll(input, "\n", " ")
}

// formatKV formats a JSON string as indented "key: value" lines.
// Home-dir prefixes are replaced with ~. No truncation is applied here;
// the caller clips each line to the panel width.
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
		return strings.ReplaceAll(s, "\n", "↵")
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

// renderHelpFooterBar returns the single-line footer shown while the help overlay is open.
func (m Model) renderHelpFooterBar() string {
	bind := func(key, desc string) string {
		k := lipgloss.NewStyle().Bold(true).Foreground(colorFg).Render(key)
		d := lipgloss.NewStyle().Foreground(colorMuted).Render(" " + desc + "  ")
		return k + d
	}
	content := " " + bind("[?/esc/q]", "close help")
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

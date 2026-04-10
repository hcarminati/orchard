// Package ui contains all TUI components and layout logic for Orchard.
// It follows the Bubbletea pattern: a Model struct holds all state, and
// three methods — Init, Update, View — define how the app behaves.
//
// Data flows into the TUI exclusively via Bubbletea messages (hookEventMsg).
// This package does not import internal/hooks or internal/session directly;
// callers pass a pre-built channel and initial nodes so the package boundary
// stays clean.
package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hcarminati/orchard/internal/agent"
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

// filterMode controls which nodes are shown in the agent tree.
type filterMode int

const (
	filterAll     filterMode = iota // show every node
	filterRunning                   // show only nodes with StatusRunning
	filterErrored                   // show only nodes with StatusError
)

// filterLabel returns the display name for the current filter mode.
func (f filterMode) label() string {
	switch f {
	case filterRunning:
		return "running"
	case filterErrored:
		return "errored"
	default:
		return "all"
	}
}

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
	colorRed    = lipgloss.Color("#EF4444") // red — error agent status dot
	colorFg     = lipgloss.Color("#F9FAFB") // near-white — primary text
)

// idleDuration is how long after the last PostToolUse event before a session
// transitions from Running to Idle.
const idleDuration = 3 * time.Second

// doneDuration is how long a session can stay Idle with no new activity before
// it is assumed closed and transitions to Done (grey).
const doneDuration = 10 * time.Minute

// hookEventMsg wraps an incoming hook event as a Bubbletea message.
// Keeping it unexported prevents callers from constructing it directly;
// it is only ever produced by waitForEvent.
type hookEventMsg struct{ event agent.Event }

// idleTimeoutMsg is sent by scheduleIdle when a session has been quiet for
// idleDuration. The gen field lets Update discard stale timers: if a new
// tool call arrived after the timer was scheduled, the generation will have
// been incremented and the timer is ignored.
type idleTimeoutMsg struct {
	sessionID string
	gen       int
}

// doneTimeoutMsg is sent by scheduleDone when a session has been Idle for
// doneDuration with no new activity, indicating the session is likely closed.
type doneTimeoutMsg struct {
	sessionID string
	gen       int
}

// visibleNode is a single entry in the flattened, depth-first traversal of the
// visible agent tree. Nodes whose ancestors are collapsed are excluded.
// When groupID is non-empty the entry is a virtual group header row (no real
// node behind it); id is empty in that case.
type visibleNode struct {
	id      string
	depth   int
	groupID string // non-empty only for virtual parallel-group header rows
}

// Model holds all the state for the Orchard TUI.
// In Bubbletea, the model is a value type (not a pointer), meaning it gets
// copied on every update. This keeps state changes predictable and testable.
type Model struct {
	width       int                // current terminal width in columns
	height      int                // current terminal height in rows
	activePanel panel              // which panel currently has keyboard focus
	agents      agent.Tree         // live agent hierarchy
	hasSession  bool               // whether any session data has been received
	eventCh     <-chan agent.Event // nil when no hook server is running
	timerGen    map[string]int     // per-session idle timer generation; incremented to cancel stale timers
	cursor          int             // index into visibleNodes() for the focused node
	collapsed       map[string]bool // set of node IDs whose subtrees are currently hidden
	collapsedGroups map[string]bool // set of GroupIDs whose members are currently hidden
	statusFilter    filterMode      // which nodes to show in the agent tree
}

// New creates a Model initialized with session data and a hook event channel.
//
// nodes is the initial set of agents loaded from JSONL on startup (may be nil).
// eventCh delivers incoming hook events from the embedded HTTP server; pass nil
// to run without live updates (useful in tests).
func New(nodes []agent.Node, eventCh <-chan agent.Event) Model {
	tree := agent.NewTree()
	for _, n := range nodes {
		tree.AddNode(n)
	}
	return Model{
		activePanel:     panelAgents,
		agents:          tree,
		hasSession:      len(nodes) > 0,
		eventCh:         eventCh,
		timerGen:        make(map[string]int),
		collapsed:       make(map[string]bool),
		collapsedGroups: make(map[string]bool),
		statusFilter:    filterAll,
	}
}

// waitForEvent returns a Cmd that blocks until the next event arrives on ch,
// then returns it as a hookEventMsg. The TUI re-issues this command after each
// event so the listener stays alive for the lifetime of the program.
func waitForEvent(ch <-chan agent.Event) tea.Cmd {
	return func() tea.Msg {
		return hookEventMsg{event: <-ch}
	}
}

// scheduleIdle returns a Cmd that sends an idleTimeoutMsg after idleDuration.
// gen is the current timer generation for sessionID; if a newer tool call
// arrives before the timer fires, the generation will be incremented and the
// message will be ignored in Update.
func scheduleIdle(sessionID string, gen int) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(idleDuration)
		return idleTimeoutMsg{sessionID: sessionID, gen: gen}
	}
}

// scheduleDone returns a Cmd that sends a doneTimeoutMsg after doneDuration.
// If any new activity arrives before the timer fires, the generation will be
// incremented and the message will be ignored in Update.
func scheduleDone(sessionID string, gen int) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(doneDuration)
		return doneTimeoutMsg{sessionID: sessionID, gen: gen}
	}
}

// Init is called once when the program starts.
// If an event channel is present, it kicks off the first waitForEvent listener.
func (m Model) Init() tea.Cmd {
	if m.eventCh == nil {
		return nil
	}
	return waitForEvent(m.eventCh)
}

// Update is the heart of Bubbletea. Every time something happens — a keypress,
// a window resize, a background task finishing — Bubbletea calls Update with a
// message describing what happened. Update returns a new model (with updated state)
// and optionally a command to run next.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		case "j", "down":
			n := len(m.visibleNodes())
			if n > 0 && m.cursor < n-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case " ":
			vn := m.visibleNodes()
			if m.cursor < len(vn) {
				entry := vn[m.cursor]
				if entry.groupID != "" {
					m.collapsedGroups[entry.groupID] = !m.collapsedGroups[entry.groupID]
				} else if node := m.agents.Nodes[entry.id]; node != nil && len(node.Children) > 0 {
					m.collapsed[entry.id] = !m.collapsed[entry.id]
				}
			}
			// Clamp cursor: collapsing may shrink the visible list below the cursor index.
			if newLen := len(m.visibleNodes()); m.cursor >= newLen {
				m.cursor = max(0, newLen-1)
			}
		case "f":
			// Cycle filter: All → Running → Errored → All.
			m.statusFilter = (m.statusFilter + 1) % 3
			m.cursor = 0
		case "enter":
			vn := m.visibleNodes()
			if m.cursor < len(vn) {
				id := vn[m.cursor].id
				if focused, ok := m.agents.Nodes[id]; ok && focused.GroupID != "" {
					gid := focused.GroupID
					// Un-mark all siblings in this group, then mark the focused node.
					for _, rid := range m.agents.Roots {
						if rn, ok := m.agents.Nodes[rid]; ok && rn.GroupID == gid {
							rn.Winner = false
						}
					}
					focused.Winner = true
				}
			}
		}

	// A hook event arrived from the HTTP server.
	case hookEventMsg:
		e := msg.event
		m.agents.ApplyEvent(e)
		m.hasSession = true
		cmd := waitForEvent(m.eventCh)
		switch e.Type {
		case "PostToolUse":
			// Schedule an idle transition after the tool completes.
			// Increment the generation so any previously scheduled timer is invalidated.
			m.timerGen[e.SessionID]++
			cmd = tea.Batch(cmd, scheduleIdle(e.SessionID, m.timerGen[e.SessionID]))
		case "PreToolUse":
			// A new tool call started — invalidate any pending idle or done timer.
			m.timerGen[e.SessionID]++
		case "Stop", "SubagentStop":
			// Turn ended. Schedule a done transition after doneDuration of inactivity.
			// If the user sends another message before the timer fires, PreToolUse will
			// increment the generation and the timer will be discarded.
			m.timerGen[e.SessionID]++
			cmd = tea.Batch(cmd, scheduleDone(e.SessionID, m.timerGen[e.SessionID]))
		}
		return m, cmd

	// An idle timer fired. Transition to Idle only if the generation still matches
	// (i.e. no new tool call arrived after the timer was scheduled).
	case idleTimeoutMsg:
		if m.timerGen[msg.sessionID] == msg.gen {
			if node, ok := m.agents.Nodes[msg.sessionID]; ok && node.Status == agent.StatusRunning {
				node.Status = agent.StatusIdle
			}
		}

	// A done timer fired. Transition to Done only if the generation still matches
	// (i.e. the session has been Idle for doneDuration with no new activity).
	case doneTimeoutMsg:
		if m.timerGen[msg.sessionID] == msg.gen {
			if node, ok := m.agents.Nodes[msg.sessionID]; ok && node.Status == agent.StatusIdle {
				node.Status = agent.StatusDone
			}
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

// sortedRoots returns root IDs with StatusError nodes first, preserving
// relative order within each tier. This ensures errored agents are always
// visible at the top of the list without requiring the user to scroll.
func (m Model) sortedRoots() []string {
	out := make([]string, 0, len(m.agents.Roots))
	for _, id := range m.agents.Roots {
		if n := m.agents.Nodes[id]; n != nil && n.Status == agent.StatusError {
			out = append(out, id)
		}
	}
	for _, id := range m.agents.Roots {
		if n := m.agents.Nodes[id]; n != nil && n.Status != agent.StatusError {
			out = append(out, id)
		}
	}
	return out
}

// nodeMatchesFilter reports whether n should be shown under the current filter.
func (m Model) nodeMatchesFilter(n *agent.Node) bool {
	switch m.statusFilter {
	case filterRunning:
		return n.Status == agent.StatusRunning
	case filterErrored:
		return n.Status == agent.StatusError
	default: // filterAll
		return true
	}
}

// visibleNodes returns the flat, pre-order depth-first traversal of the agent
// tree. Collapsed nodes hide their subtrees; collapsed parallel groups hide
// their members entirely. Nodes excluded by the status filter are hidden along
// with their subtrees. Parallel groups are represented by a virtual header
// row (groupID non-empty, id empty) that the cursor can land on.
func (m Model) visibleNodes() []visibleNode {
	var result []visibleNode
	seenGroups := map[string]bool{}

	var walk func(id string, depth int)
	walk = func(id string, depth int) {
		node := m.agents.Nodes[id]
		if node == nil {
			return
		}

		// Status filter: skip nodes (and their subtrees) that don't match.
		if !m.nodeMatchesFilter(node) {
			return
		}

		// Root-level grouped nodes share a virtual header row.
		if node.GroupID != "" && depth == 0 {
			if !seenGroups[node.GroupID] {
				seenGroups[node.GroupID] = true
				result = append(result, visibleNode{groupID: node.GroupID})
			}
			if m.collapsedGroups[node.GroupID] {
				return // hide all members when group is collapsed
			}
		}

		result = append(result, visibleNode{id: id, depth: depth})
		if !m.collapsed[id] {
			for _, childID := range node.Children {
				walk(childID, depth+1)
			}
		}
	}
	for _, id := range m.sortedRoots() {
		walk(id, 0)
	}
	return result
}

// cursorInGroup reports whether the currently focused node belongs to a parallel group.
func (m Model) cursorInGroup() bool {
	vn := m.visibleNodes()
	if m.cursor >= len(vn) {
		return false
	}
	n := m.agents.Nodes[vn[m.cursor].id]
	return n != nil && n.GroupID != ""
}

// cursorHasChildren reports whether the currently focused node (or group header)
// can be collapsed/expanded with space.
func (m Model) cursorHasChildren() bool {
	vn := m.visibleNodes()
	if m.cursor >= len(vn) {
		return false
	}
	entry := vn[m.cursor]
	if entry.groupID != "" {
		return true // group headers are always collapsible
	}
	n := m.agents.Nodes[entry.id]
	return n != nil && len(n.Children) > 0
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

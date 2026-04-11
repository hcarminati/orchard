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
	"math"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hcarminati/orchard/internal/agent"
)

// panel identifies which panel currently has keyboard focus.
type panel int

const (
	panelAgents panel = iota
	panelEvents
)

// rightTab identifies a tab in the right panel tab strip.
type rightTab int

const (
	tabEvents rightTab = iota
	tabFiles
)

// rightTabs is the ordered list of tabs shown in the right panel.
// Adding a new tab only requires appending it here and adding a case to label().
var rightTabs = []rightTab{tabEvents, tabFiles}

// label returns the display name for a right panel tab.
func (t rightTab) label() string {
	switch t {
	case tabEvents:
		return "Events"
	case tabFiles:
		return "Files"
	default:
		return "?"
	}
}

// filterMode controls which nodes are shown in the agent tree.
type filterMode int

const (
	filterAll     filterMode = iota // show every node
	filterRunning                   // show only nodes with StatusRunning
	filterErrored                   // show only nodes with StatusError
)

// label returns the display name for the current filter mode.
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

// footerHeight is the number of terminal lines reserved for the keybindings bar.
const footerHeight = 1

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

// Model holds all the state for the Orchard TUI.
// In Bubbletea, the model is a value type (not a pointer), meaning it gets
// copied on every update. This keeps state changes predictable and testable.
type Model struct {
	width           int                // current terminal width in columns
	height          int                // current terminal height in rows
	activePanel     panel              // which panel currently has keyboard focus
	agents          agent.Tree         // live agent hierarchy
	hasSession      bool               // whether any session data has been received
	eventCh         <-chan agent.Event // nil when no hook server is running
	timerGen        map[string]int     // per-session idle timer generation; incremented to cancel stale timers
	cursor          int                // index into visibleNodes() for the focused node
	scrollOffset    int                // index of the first visible row in the agents panel
	eventScroll     int                // index of the first visible row in the events panel
	eventCursor     int                // index into the focused node's Events slice (selected row)
	expandedEvents  map[string]bool    // set of "nodeID:eventIdx" keys for expanded tool events
	collapsed       map[string]bool    // set of node IDs whose subtrees are currently hidden
	collapsedGroups map[string]bool    // set of GroupIDs whose members are currently hidden
	statusFilter    filterMode         // which nodes to show in the agent tree
	activeRightTab  int                // index into rightTabs for the currently shown right-panel tab
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
		expandedEvents:  make(map[string]bool),
		statusFilter:    filterAll,
		eventScroll:     math.MaxInt, // will be clamped to real max on first render
	}
}

// tabAtX maps an X offset (relative to the start of the tab strip inside the
// right panel's top border) to a tab index. Returns (index, true) when the
// click lands on a tab label, (0, false) otherwise.
// The strip layout mirrors tabStripTitle: active tabs are "[Label]", inactive
// are "Label", separated by single spaces.
func (m Model) tabAtX(x int) (int, bool) {
	offset := 0
	for i, t := range rightTabs {
		var label string
		if i == m.activeRightTab {
			label = "[" + t.label() + "]"
		} else {
			label = t.label()
		}
		w := len(label) // all tab labels are ASCII
		if x >= offset && x < offset+w {
			return i, true
		}
		offset += w + 1 // +1 for the space separator between tabs
	}
	return 0, false
}

// agentsPanelInnerH returns the number of content rows available inside the
// Agents panel. It accounts for the footer row and the top+bottom panel border.
func (m Model) agentsPanelInnerH() int {
	return max(1, m.height-footerHeight-2)
}

// clampScroll adjusts scrollOffset so the cursor row is always inside the
// visible viewport. Call this after any operation that may move the cursor or
// change the list length.
func (m *Model) clampScroll() {
	h := m.agentsPanelInnerH()
	n := len(m.visibleNodes())
	// Scroll down: cursor moved below the bottom of the viewport.
	if m.cursor >= m.scrollOffset+h {
		m.scrollOffset = m.cursor - h + 1
	}
	// Scroll up: cursor moved above the top of the viewport.
	if m.cursor < m.scrollOffset {
		m.scrollOffset = m.cursor
	}
	// Clamp offset so we don't scroll past the end of the list.
	maxOffset := max(0, n-h)
	if m.scrollOffset > maxOffset {
		m.scrollOffset = maxOffset
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

// focusedNode returns the agent.Node currently under the cursor, or nil when
// the cursor is on a group header or no nodes exist.
func (m Model) focusedNode() *agent.Node {
	vn := m.visibleNodes()
	if m.cursor >= len(vn) {
		return nil
	}
	id := vn[m.cursor].id
	if id == "" {
		return nil // group header, not a real node
	}
	return m.agents.Nodes[id]
}

// eventKey returns the map key for a specific event within a node,
// used to track expanded state in expandedEvents.
func eventKey(nodeID string, idx int) string {
	return nodeID + ":" + strconv.Itoa(idx)
}

// isToolEvent reports whether an event can be expanded to show input/output.
func isToolEvent(e agent.Event) bool {
	return e.Type == "PreToolUse" || e.Type == "PostToolUse" ||
		e.Type == "PermissionRequest" || e.Type == "Notification"
}

// innerWidth returns the usable content width inside the right panel.
func (m Model) innerWidth() int {
	leftW := m.width * 35 / 100
	rightW := m.width - leftW
	return max(1, rightW-2)
}

// wrappedLineCount returns how many display lines s occupies when wrapped at
// width runes per line, splitting first on existing newlines.
func wrappedLineCount(s string, width int) int {
	if width <= 0 || s == "" {
		return 1
	}
	count := 0
	for _, physical := range strings.Split(s, "\n") {
		runes := []rune(physical)
		if len(runes) == 0 {
			count++
			continue
		}
		count += (len(runes) + width - 1) / width
	}
	if count == 0 {
		return 1
	}
	return count
}

// linesForEvent returns the number of screen lines that the event at idx
// occupies, accounting for whether it is currently expanded.
func (m *Model) linesForEvent(node *agent.Node, idx int) int {
	e := node.Events[idx]
	key := eventKey(node.ID, idx)
	if !isToolEvent(e) || !m.expandedEvents[key] {
		return 1
	}
	innerW := m.innerWidth()
	const indent = 5 // "│    " prefix
	if e.Type == "Notification" {
		// Expanded Notification: header + "│  Message:" label + message lines + trailing "│".
		if e.Message == "" {
			return 2 // header + trailing │
		}
		return 3 + wrappedLineCount(e.Message, innerW-indent)
	}
	count := 3 // header line + "│  Input:" label + trailing "│"
	count += wrappedLineCount(e.Input, innerW-indent)
	if e.Response != "" {
		count += 1 + wrappedLineCount(e.Response, innerW-indent)
	}
	return count
}

// clampEventScroll adjusts eventScroll so it stays within the bounds of the
// focused node's event list. Call this after scrolling or switching agents.
func (m *Model) clampEventScroll() {
	node := m.focusedNode()
	if node == nil {
		m.eventScroll = 0
		return
	}
	headerH := 0
	if node.ParentID != "" {
		headerH = 3
	}
	viewH := max(1, m.height-footerHeight-2-headerH)
	totalLines := 0
	for i := range node.Events {
		totalLines += m.linesForEvent(node, i)
	}
	maxScroll := max(0, totalLines-viewH)
	if m.eventScroll > maxScroll {
		m.eventScroll = maxScroll
	}
	if m.eventScroll < 0 {
		m.eventScroll = 0
	}
}

// eventAtLine returns the index of the event that occupies the given rendered
// line offset (0-based from the top of all rendered event lines). Returns
// (0, false) if lineOffset is beyond the last event.
func (m *Model) eventAtLine(node *agent.Node, lineOffset int) (int, bool) {
	line := 0
	for i := range node.Events {
		n := m.linesForEvent(node, i)
		if lineOffset < line+n {
			return i, true
		}
		line += n
	}
	return 0, false
}

// scrollToCursor adjusts eventScroll so the event at eventCursor is visible.
func (m *Model) scrollToCursor() {
	node := m.focusedNode()
	if node == nil || len(node.Events) == 0 {
		m.eventScroll = 0
		return
	}
	m.eventCursor = max(0, min(m.eventCursor, len(node.Events)-1))
	headerH := 0
	if node.ParentID != "" {
		headerH = 3
	}
	viewH := max(1, m.height-footerHeight-2-headerH)
	firstLine := 0
	for i := 0; i < m.eventCursor; i++ {
		firstLine += m.linesForEvent(node, i)
	}
	lastLine := firstLine + m.linesForEvent(node, m.eventCursor) - 1
	if firstLine < m.eventScroll {
		m.eventScroll = firstLine
	}
	if lastLine >= m.eventScroll+viewH {
		m.eventScroll = lastLine - viewH + 1
	}
	m.clampEventScroll()
}

// scrollEventToBottom sets eventScroll and eventCursor to show the most-recent
// event. Safe to call before a window size is known.
func (m *Model) scrollEventToBottom() {
	node := m.focusedNode()
	if node != nil && len(node.Events) > 0 {
		m.eventCursor = len(node.Events) - 1
	} else {
		m.eventCursor = 0
	}
	m.eventScroll = math.MaxInt
	m.clampEventScroll()
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

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.clampScroll()
		m.scrollEventToBottom()

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			m.activePanel = (m.activePanel + 1) % 2
		case "j", "down":
			if m.activePanel == panelEvents {
				if node := m.focusedNode(); node != nil && m.eventCursor < len(node.Events)-1 {
					m.eventCursor++
					m.scrollToCursor()
				}
			} else {
				n := len(m.visibleNodes())
				if n > 0 && m.cursor < n-1 {
					m.cursor++
					m.scrollEventToBottom()
					m.clampScroll()
				}
			}
		case "k", "up":
			if m.activePanel == panelEvents {
				if m.eventCursor > 0 {
					m.eventCursor--
					m.scrollToCursor()
				}
			} else {
				if m.cursor > 0 {
					m.cursor--
					m.scrollEventToBottom()
					m.clampScroll()
				}
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
			// Clamp cursor and scroll: collapsing may shrink the visible list.
			if newLen := len(m.visibleNodes()); m.cursor >= newLen {
				m.cursor = max(0, newLen-1)
			}
			m.clampScroll()
		case "[":
			n := len(rightTabs)
			m.activeRightTab = (m.activeRightTab - 1 + n) % n
		case "]":
			m.activeRightTab = (m.activeRightTab + 1) % len(rightTabs)
		case "f":
			// Cycle filter: All → Running → Errored → All.
			m.statusFilter = (m.statusFilter + 1) % 3
			m.cursor = 0
			m.scrollOffset = 0
		case "enter":
			if m.activePanel == panelEvents {
				node := m.focusedNode()
				if node != nil && m.eventCursor < len(node.Events) {
					e := node.Events[m.eventCursor]
					if isToolEvent(e) {
						key := eventKey(node.ID, m.eventCursor)
						m.expandedEvents[key] = !m.expandedEvents[key]
						m.scrollToCursor()
					}
				}
			} else {
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
		}

	case tea.MouseMsg:
		// Scroll wheel / trackpad: route to whichever panel the pointer is over.
		if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
			delta := 1
			if msg.Button == tea.MouseButtonWheelUp {
				delta = -1
			}
			leftW := m.width * 35 / 100
			if msg.X < leftW {
				n := len(m.visibleNodes())
				h := m.agentsPanelInnerH()
				m.scrollOffset = max(0, min(m.scrollOffset+delta, max(0, n-h)))
			} else {
				m.eventScroll += delta
				m.clampEventScroll()
			}
		}
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			leftW := m.width * 35 / 100
			if msg.X < leftW {
				// Agents panel: content rows begin after the top border (row 1).
				// Add scrollOffset to translate screen row → list index.
				const contentTop = 1
				idx := msg.Y - contentTop + m.scrollOffset
				vn := m.visibleNodes()
				if idx >= 0 && idx < len(vn) {
					m.activePanel = panelAgents
					if idx != m.cursor {
						m.scrollEventToBottom()
					}
					m.cursor = idx
					// Toggle collapse state, mirroring the space-bar handler.
					entry := vn[idx]
					if entry.groupID != "" {
						m.collapsedGroups[entry.groupID] = !m.collapsedGroups[entry.groupID]
					} else if node := m.agents.Nodes[entry.id]; node != nil && len(node.Children) > 0 {
						m.collapsed[entry.id] = !m.collapsed[entry.id]
					}
					// Clamp cursor and scroll: collapsing may shrink the visible list.
					if newLen := len(m.visibleNodes()); m.cursor >= newLen {
						m.cursor = max(0, newLen-1)
					}
					m.clampScroll()
				}
			} else if msg.Y == 0 {
				// Right panel top border: tabs start 2 columns in (after ╭─).
				xInStrip := msg.X - leftW - 2
				if tab, ok := m.tabAtX(xInStrip); ok {
					m.activePanel = panelEvents
					m.activeRightTab = tab
				}
			} else if msg.X >= leftW && rightTabs[m.activeRightTab] == tabEvents {
				// Right panel content area: move event cursor to clicked row.
				const contentTop = 1
				if node := m.focusedNode(); node != nil {
					headerH := 0
					if node.ParentID != "" {
						headerH = 3
					}
					lineOffset := msg.Y - contentTop - headerH + m.eventScroll
					if lineOffset >= 0 {
						if idx, ok := m.eventAtLine(node, lineOffset); ok {
							m.activePanel = panelEvents
							if idx == m.eventCursor && isToolEvent(node.Events[idx]) {
								// Clicking the already-selected tool event toggles expansion.
								key := eventKey(node.ID, idx)
								m.expandedEvents[key] = !m.expandedEvents[key]
							}
							m.eventCursor = idx
							m.scrollToCursor()
						}
					}
				}
			}
		}

	// A hook event arrived from the HTTP server.
	case hookEventMsg:
		e := msg.event
		m.agents.ApplyEvent(e)
		m.hasSession = true
		if n := m.focusedNode(); n != nil && n.ID == e.SessionID {
			m.scrollEventToBottom()
		}
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

	return m, nil
}

// View renders the current model state. Pure — no side effects.
func (m Model) View() string {
	if m.width == 0 {
		return "loading…"
	}

	bodyH := m.height - footerHeight

	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderBody(bodyH),
		m.renderFooter(),
	)
}

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
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hcarminati/orchard/internal/agent"
	"github.com/hcarminati/orchard/internal/config"
	"github.com/hcarminati/orchard/internal/watcher"
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
	tabMCP
)

// rightTabs is the ordered list of tabs shown in the right panel.
// Adding a new tab only requires appending it here and adding a case to label().
var rightTabs = []rightTab{tabEvents, tabFiles, tabMCP}

// label returns the display name for a right panel tab.
func (t rightTab) label() string {
	switch t {
	case tabEvents:
		return "Events"
	case tabFiles:
		return "Files"
	case tabMCP:
		return "MCP"
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
	filterHidden                    // show only hidden sessions (for restore)
)

// label returns the display name for the current filter mode.
func (f filterMode) label() string {
	switch f {
	case filterRunning:
		return "running"
	case filterErrored:
		return "errored"
	case filterHidden:
		return "hidden"
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

// stateSavedMsg is returned by the save-state command when the write completes.
type stateSavedMsg struct{ err error }

// hideSavedMsg is returned by the save-hidden command when the write completes.
// The error is nil on success.
type hideSavedMsg struct{ err error }

// hookEventMsg wraps an incoming hook event as a Bubbletea message.
// Keeping it unexported prevents callers from constructing it directly;
// it is only ever produced by waitForEvent.
type hookEventMsg struct{ event agent.Event }

// subagentScanResultMsg is returned by scanSubagentsCmd after scanning a
// subagents directory. It carries one SubagentResult per file found so the
// Update handler can inject historical events and start live tailing.
type subagentScanResultMsg struct {
	results  []watcher.SubagentResult
	parentID string // session ID of the parent that spawned these subagents
}

// subagentTailMsg is returned by waitForTailEvent when a new event arrives
// from a tailing goroutine. It carries the channel so the handler can
// re-issue the wait command and keep tailing.
type subagentTailMsg struct {
	event agent.Event
	ch    <-chan agent.Event
}

// watchdogTickMsg is sent by scheduleWatchdog after watchdogMinutes. The TUI
// uses it to show a ⏱ badge on the node if no tool call has arrived since the
// timer was scheduled.
type watchdogTickMsg struct {
	sessionID string
	gen       int
}

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

// lineHeightKey is the cache key for linesForEvent results.
// It encodes all inputs that affect the computed height so cache hits are exact.
type lineHeightKey struct {
	nodeID   string
	idx      int
	expanded bool
	width    int
}

// Model holds all the state for the Orchard TUI.
// In Bubbletea, the model is a value type (not a pointer), meaning it gets
// copied on every update. This keeps state changes predictable and testable.
type Model struct {
	width           int                    // current terminal width in columns
	height          int                    // current terminal height in rows
	activePanel     panel                  // which panel currently has keyboard focus
	agents          agent.Tree             // live agent hierarchy
	hasSession      bool                   // whether any session data has been received
	eventCh         <-chan agent.Event     // nil when no hook server is running
	timerGen        map[string]int         // per-session idle timer generation; incremented to cancel stale timers
	cursor          int                    // index into visibleNodes() for the focused node
	scrollOffset    int                    // index of the first visible row in the agents panel
	eventScroll     int                    // index of the first visible row in the events panel
	eventCursor     int                    // index into the focused node's Events slice (selected row)
	expandedEvents  map[string]bool        // set of "nodeID:eventIdx" keys for expanded tool events
	lineHeightCache map[lineHeightKey]int  // cached linesForEvent results; invalidated on width change or expand toggle
	modalOpen       bool                   // whether the detail modal is visible
	modalScroll     int                    // vertical scroll offset within modal content
	lastClickTime   time.Time              // timestamp of the last left-click in the events panel
	lastClickIdx    int                    // event index of the last left-click in the events panel
	collapsed       map[string]bool        // set of node IDs whose subtrees are currently hidden
	collapsedGroups map[string]bool        // set of GroupIDs whose members are currently hidden
	statusFilter    filterMode             // which nodes to show in the agent tree
	activeRightTab  int                    // index into rightTabs for the currently shown right-panel tab
	hiddenSessions  map[string]bool        // session IDs hidden from the agent list; persisted to hidden.json
	confirmHide     string                 // non-empty: session ID awaiting hide confirmation
	hideStatusMsg   string                 // transient one-line status (e.g. "Cannot hide active session")
	pendingSave     bool                   // true when hiddenSessions was mutated before Init() ran (auto-hide at startup)
	subagentsRoot    string          // base dir for subagent discovery: ~/.claude/projects/{cwdDir}/
	watchedSubagents map[string]bool // agentIDs we have already started watching (scan+tail), to prevent duplicates
	budget           float64         // monthly spend cap in USD; 0 means unconfigured
	maxTokens        int             // monthly token cap; 0 means unconfigured
	watchdogMinutes  int             // minutes without a tool call before ⏱ badge; 0 disables
	watchdogGen      map[string]int  // per-session watchdog timer generation
	watchdogFired    map[string]int  // per-session watchdog fire count (0=ok, 1=⏱, 2=⏱⏱)
	costAlert        float64         // per-session USD alert threshold; 0 disables
	timelineMode     bool            // when true, the left panel shows the timeline view
	searchOpen       bool            // whether the fuzzy search bar is active
	searchQuery      string          // current search filter typed by the user
	costAlertDismiss map[string]bool // session IDs where the cost alert has been dismissed
	bellSent         map[string]bool // session IDs where the terminal bell has already fired
	helpOpen         bool            // whether the full-screen help overlay is visible
	cancelOpen       bool            // whether the cancel-agent overlay is visible
}

// autoHideAge is how long a session must be inactive before it is automatically
// hidden from the agent list on startup.
const autoHideAge = 7 * 24 * time.Hour

// New creates a Model initialized with session data and a hook event channel.
//
// nodes is the initial set of agents loaded from JSONL on startup (may be nil).
// eventCh delivers incoming hook events from the embedded HTTP server; pass nil
// to run without live updates (useful in tests).
// subagentsRoot is the base directory for subagent discovery (e.g.
// ~/.claude/projects/-Users-alice-myapp/). Pass "" to disable live subagent
// file watching.
// hiddenIDs is the set of session IDs loaded from hidden.json; pass nil for none.
// expandedIDs is the set of root session IDs that were expanded on last exit,
// loaded from state.json; pass nil for none. All root sessions start collapsed
// by default — running sessions are auto-expanded, and any ID in expandedIDs
// is also expanded.
func New(nodes []agent.Node, eventCh <-chan agent.Event, subagentsRoot string, hiddenIDs map[string]bool, expandedIDs map[string]bool, budget float64, maxTokens int, watchdogMinutes int, costAlert float64) Model {
	m := newWithClock(nodes, eventCh, hiddenIDs, time.Now())
	m.subagentsRoot = subagentsRoot
	m.budget = budget
	m.maxTokens = maxTokens
	m.watchdogMinutes = watchdogMinutes
	m.costAlert = costAlert
	// Collapse all root nodes by default.
	for _, id := range m.agents.Roots {
		if m.effectiveStatus(id) == agent.StatusRunning {
			// Auto-expand the currently active session so the user immediately
			// sees what is happening.
			continue
		}
		if expandedIDs[id] {
			// Restore the expanded state from the previous session.
			continue
		}
		m.collapsed[id] = true
	}
	return m
}

// newWithClock is the testable core of New. now is used as the reference time
// for the auto-hide age check; pass time.Time{} to disable auto-hide entirely.
func newWithClock(nodes []agent.Node, eventCh <-chan agent.Event, hiddenIDs map[string]bool, now time.Time) Model {
	tree := agent.NewTree()
	for _, n := range nodes {
		tree.AddNode(n)
	}
	if hiddenIDs == nil {
		hiddenIDs = make(map[string]bool)
	}

	// Auto-hide root sessions that have had no activity in the past week.
	// Only sessions with at least one event are considered; sessions with no
	// events are new/unknown and should remain visible.
	// If now is zero, auto-hide is disabled (used in tests).
	pendingSave := false
	if !now.IsZero() {
		cutoff := now.Add(-autoHideAge)
		for _, id := range tree.Roots {
			if hiddenIDs[id] {
				continue
			}
			node := tree.Nodes[id]
			if node == nil || len(node.Events) == 0 {
				continue
			}
			// Walk the subtree to find the most recent event across all descendants.
			var latest time.Time
			var walkLatest func(string)
			walkLatest = func(nodeID string) {
				n := tree.Nodes[nodeID]
				if n == nil {
					return
				}
				if len(n.Events) > 0 {
					if t := n.Events[len(n.Events)-1].Timestamp; t.After(latest) {
						latest = t
					}
				}
				for _, child := range n.Children {
					walkLatest(child)
				}
			}
			walkLatest(id)
			if !latest.IsZero() && latest.Before(cutoff) {
				hiddenIDs[id] = true
				pendingSave = true
			}
		}
	}

	return Model{
		activePanel:      panelAgents,
		agents:           tree,
		hasSession:       len(nodes) > 0,
		eventCh:          eventCh,
		timerGen:         make(map[string]int),
		expandedEvents:   make(map[string]bool),
		lineHeightCache:  make(map[lineHeightKey]int),
		collapsed:        make(map[string]bool),
		collapsedGroups:  make(map[string]bool),
		statusFilter:     filterAll,
		eventScroll:      math.MaxInt, // will be clamped to real max on first render
		hiddenSessions:   hiddenIDs,
		pendingSave:      pendingSave,
		watchedSubagents: make(map[string]bool),
		watchdogGen:      make(map[string]int),
		watchdogFired:    make(map[string]int),
		costAlertDismiss: make(map[string]bool),
		bellSent:         make(map[string]bool),
	}
}

// saveHidden returns a Cmd that persists the current hiddenSessions map to disk.
func saveHidden(h map[string]bool) tea.Cmd {
	// Copy the map so the goroutine has a stable snapshot.
	snap := make(map[string]bool, len(h))
	for k, v := range h {
		snap[k] = v
	}
	return func() tea.Msg {
		return hideSavedMsg{err: config.SaveHidden(snap)}
	}
}

// saveState returns a Cmd that persists the current expand/collapse state to
// disk. Only root-level sessions are saved — the set of roots that are NOT
// collapsed is written as the "expanded" list so new sessions start collapsed
// by default on the next launch.
func saveState(collapsed map[string]bool, roots []string) tea.Cmd {
	expanded := make(map[string]bool, len(roots))
	for _, id := range roots {
		if !collapsed[id] {
			expanded[id] = true
		}
	}
	return func() tea.Msg {
		return stateSavedMsg{err: config.SaveExpanded(expanded)}
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

// isToolEvent reports whether an event can be expanded inline to show input/output.
func isToolEvent(e agent.Event) bool {
	return e.Type == "PreToolUse" || e.Type == "PostToolUse" ||
		e.Type == "PermissionRequest" || e.Type == "Notification"
}

// isHiddenEvent reports whether the event at idx should be completely invisible
// in the events panel — either because it is a PermissionRequest absorbed into
// the preceding row, or because it is an internal sentinel injected by the
// watcher (Tool == "_subagent_init") that has no user-visible meaning.
func isHiddenEvent(events []agent.Event, idx int) bool {
	if idx < len(events) && events[idx].Tool == "_subagent_init" {
		return true
	}
	return isAbsorbedPermission(events, idx)
}

// isAbsorbedPermission reports whether the event at idx is a PermissionRequest
// that should be collapsed into the preceding PreToolUse row. The three
// conditions must all hold: same tool name, identical input, and arrival within
// one second of the PreToolUse.
func isAbsorbedPermission(events []agent.Event, idx int) bool {
	if idx == 0 || idx >= len(events) {
		return false
	}
	e, prev := events[idx], events[idx-1]
	if e.Type != "PermissionRequest" || prev.Type != "PreToolUse" {
		return false
	}
	if e.Tool != prev.Tool || e.Input != prev.Input {
		return false
	}
	diff := e.Timestamp.Sub(prev.Timestamp)
	if diff < 0 {
		diff = -diff
	}
	return diff <= time.Second
}

// innerWidth returns the usable content width inside the right panel.
func (m Model) innerWidth() int {
	return max(1, m.width-m.agentsPanelW()-2)
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
// occupies, accounting for whether it is currently expanded inline.
// Hidden events (absorbed PermissionRequests and internal sentinels) return 0.
func (m *Model) linesForEvent(node *agent.Node, idx int) int {
	if isHiddenEvent(node.Events, idx) {
		return 0
	}
	e := node.Events[idx]
	eKey := eventKey(node.ID, idx)
	expanded := isToolEvent(e) && m.expandedEvents[eKey]

	cacheKey := lineHeightKey{nodeID: node.ID, idx: idx, expanded: expanded, width: m.innerWidth()}
	if h, ok := m.lineHeightCache[cacheKey]; ok {
		return h
	}

	var h int
	if !expanded {
		h = 1
	} else {
		innerW := m.innerWidth()
		const indent = 5 // "│    " prefix
		if e.Type == "Notification" {
			if e.Message == "" {
				h = 2 // header + trailing │
			} else {
				h = 3 + wrappedLineCount(e.Message, innerW-indent)
			}
		} else {
			h = 3 // header + "│  Input:" + trailing "│"
			h += wrappedLineCount(e.Input, innerW-indent)
			if e.Response != "" {
				h += 1 + wrappedLineCount(e.Response, innerW-indent)
			}
		}
	}

	m.lineHeightCache[cacheKey] = h
	return h
}

// clampEventScroll adjusts eventScroll so it stays within the bounds of the
// focused node's event list. Call this after scrolling or switching agents.
func (m *Model) clampEventScroll() {
	node := m.focusedNode()
	if node == nil {
		m.eventScroll = 0
		return
	}
	headerH := 2 // session/spawn line + divider
	if node.ParentID != "" && node.Prompt != "" {
		headerH = 3 // child with prompt adds a prompt line
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
	headerH := 2 // session/spawn line + divider
	if node.ParentID != "" && node.Prompt != "" {
		headerH = 3 // child with prompt adds a prompt line
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
// Absorbed PermissionRequest events are skipped: the cursor lands on the
// preceding PreToolUse row instead.
func (m *Model) scrollEventToBottom() {
	node := m.focusedNode()
	if node != nil && len(node.Events) > 0 {
		m.eventCursor = len(node.Events) - 1
		for m.eventCursor > 0 && isHiddenEvent(node.Events, m.eventCursor) {
			m.eventCursor--
		}
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

// bellMsg is returned by bellCmd when the terminal bell write completes.
type bellMsg struct{}

// bellCmd returns a Cmd that writes the terminal bell character (\a) to stderr
// and returns a bellMsg. Using stderr keeps the bell out of any captured output.
func bellCmd() tea.Cmd {
	return func() tea.Msg {
		os.Stderr.WriteString("\a") //nolint:errcheck
		return bellMsg{}
	}
}

// scheduleWatchdog returns a Cmd that sends a watchdogTickMsg after d. If a
// new tool call arrives before the timer fires, the generation is incremented
// and the message is discarded in Update.
func scheduleWatchdog(sessionID string, gen int, d time.Duration) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(d)
		return watchdogTickMsg{sessionID: sessionID, gen: gen}
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

// scanSubagentsCmd returns a Cmd that waits delay, then scans subagentsDir
// for new subagent JSONL files, returning a subagentScanResultMsg. The delay
// gives Claude Code time to create the file after the PreToolUse[Agent] hook fires.
func scanSubagentsCmd(subagentsDir, parentID string, delay time.Duration) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(delay)
		return subagentScanResultMsg{
			results:  watcher.Scan(subagentsDir, parentID),
			parentID: parentID,
		}
	}
}

// waitForTailEvent returns a Cmd that blocks until the next event arrives on ch,
// then wraps it in a subagentTailMsg so Update can both apply the event and
// re-issue the wait. A closed channel causes a nil return (no message).
func waitForTailEvent(ch <-chan agent.Event) tea.Cmd {
	return func() tea.Msg {
		e, ok := <-ch
		if !ok {
			return nil // channel closed; stop tailing
		}
		return subagentTailMsg{event: e, ch: ch}
	}
}

// Init is called once when the program starts.
// Kicks off the hook event listener and, if sessions were auto-hidden at
// startup, persists the updated hidden list to disk.
func (m Model) Init() tea.Cmd {
	var cmds []tea.Cmd
	if m.eventCh != nil {
		cmds = append(cmds, waitForEvent(m.eventCh))
	}
	if m.pendingSave {
		cmds = append(cmds, saveHidden(m.hiddenSessions))
	}
	return tea.Batch(cmds...)
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
		m.lineHeightCache = make(map[lineHeightKey]int) // panel width changed; all cached heights are stale
		m.clampScroll()
		m.scrollEventToBottom()

	case tea.KeyMsg:
		if m.modalOpen {
			switch msg.String() {
			case "esc", "q":
				m.modalOpen = false
				m.modalScroll = 0
			case "j", "down":
				m.modalScroll++
			case "k", "up":
				if m.modalScroll > 0 {
					m.modalScroll--
				}
			case "right", "l":
				if node := m.focusedNode(); node != nil && m.eventCursor < len(node.Events)-1 {
					m.eventCursor++
					for m.eventCursor < len(node.Events)-1 && isHiddenEvent(node.Events, m.eventCursor) {
						m.eventCursor++
					}
					m.modalScroll = 0
				}
			case "left", "h":
				if m.eventCursor > 0 {
					m.eventCursor--
					if node := m.focusedNode(); node != nil {
						for m.eventCursor > 0 && isHiddenEvent(node.Events, m.eventCursor) {
							m.eventCursor--
						}
					}
					m.modalScroll = 0
				}
			}
			return m, nil
		}
		// Help overlay intercepts all keys while open.
		if m.helpOpen {
			switch msg.String() {
			case "?", "esc", "q":
				m.helpOpen = false
			}
			return m, nil
		}
		// Cancel overlay intercepts all keys while open.
		if m.cancelOpen {
			switch msg.String() {
			case "esc", "q", "X":
				m.cancelOpen = false
			}
			return m, nil
		}
		// Search bar intercepts all keys while open.
		if m.searchOpen {
			switch msg.String() {
			case "esc":
				m.searchOpen = false
				m.searchQuery = ""
			case "enter":
				m.searchOpen = false
			case "backspace", "ctrl+h":
				if len(m.searchQuery) > 0 {
					runes := []rune(m.searchQuery)
					m.searchQuery = string(runes[:len(runes)-1])
				}
			default:
				// Accept printable characters into the search query.
				s := msg.String()
				if len(s) == 1 && s[0] >= 0x20 {
					m.searchQuery += s
				}
			}
			return m, nil
		}
		// Clear any transient status message on the next keypress.
		m.hideStatusMsg = ""
		// Confirmation prompt: intercept all keys while awaiting hide confirmation.
		if m.confirmHide != "" {
			if msg.String() == "y" {
				m.hiddenSessions[m.confirmHide] = true
				m.confirmHide = ""
				// Clamp cursor in case the hidden node was the last one.
				if newLen := len(m.visibleNodes()); m.cursor >= newLen {
					m.cursor = max(0, newLen-1)
				}
				m.clampScroll()
				return m, saveHidden(m.hiddenSessions)
			}
			m.confirmHide = ""
			return m, nil
		}
		switch msg.String() {
		case "?":
			m.helpOpen = true
			return m, nil
		case "X":
			m.cancelOpen = true
			return m, nil
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			m.activePanel = (m.activePanel + 1) % 2
		case "j", "down":
			if m.activePanel == panelEvents {
				if node := m.focusedNode(); node != nil && m.eventCursor < len(node.Events)-1 {
					m.eventCursor++
					for m.eventCursor < len(node.Events)-1 && isHiddenEvent(node.Events, m.eventCursor) {
						m.eventCursor++
					}
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
					if node := m.focusedNode(); node != nil {
						for m.eventCursor > 0 && isHiddenEvent(node.Events, m.eventCursor) {
							m.eventCursor--
						}
					}
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
			if m.activePanel == panelEvents {
				// Toggle inline expand/collapse for the focused event row.
				node := m.focusedNode()
				if node != nil && m.eventCursor < len(node.Events) && isToolEvent(node.Events[m.eventCursor]) {
					key := eventKey(node.ID, m.eventCursor)
					m.expandedEvents[key] = !m.expandedEvents[key]
					w := m.innerWidth()
					delete(m.lineHeightCache, lineHeightKey{nodeID: node.ID, idx: m.eventCursor, expanded: true, width: w})
					delete(m.lineHeightCache, lineHeightKey{nodeID: node.ID, idx: m.eventCursor, expanded: false, width: w})
				}
			} else {
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
				return m, saveState(m.collapsed, m.agents.Roots)
			}
		case "[":
			n := len(rightTabs)
			m.activeRightTab = (m.activeRightTab - 1 + n) % n
		case "]":
			m.activeRightTab = (m.activeRightTab + 1) % len(rightTabs)
		case "f":
			// Cycle filter: All → Running → Errored → Hidden → All.
			m.statusFilter = (m.statusFilter + 1) % 4
			m.cursor = 0
			m.scrollOffset = 0
		case "r":
			// Restore a hidden session when in the hidden filter view.
			if m.activePanel == panelAgents && m.statusFilter == filterHidden {
				vn := m.visibleNodes()
				if m.cursor < len(vn) {
					delete(m.hiddenSessions, vn[m.cursor].id)
					if newLen := len(m.visibleNodes()); m.cursor >= newLen {
						m.cursor = max(0, newLen-1)
					}
					return m, saveHidden(m.hiddenSessions)
				}
			}
		case "d":
			// Hide a top-level parent session (not children, not running sessions).
			if m.activePanel == panelAgents && m.statusFilter != filterHidden {
				vn := m.visibleNodes()
				if m.cursor < len(vn) {
					entry := vn[m.cursor]
					if entry.depth == 0 && entry.groupID == "" && m.agents.Nodes[entry.id] != nil {
						if m.effectiveStatus(entry.id) == agent.StatusRunning {
							m.hideStatusMsg = "Cannot hide active session"
						} else {
							m.confirmHide = entry.id
						}
					}
				}
			}
		case "t", "T":
			m.timelineMode = !m.timelineMode
		case "/":
			// Open fuzzy search in the events panel.
			m.searchOpen = true
			m.searchQuery = ""
			m.activePanel = panelEvents
		case "esc":
			// Dismiss cost alert banner if one is showing.
			if node := m.focusedNode(); node != nil {
				m.costAlertDismiss[node.ID] = true
			}
		case "G":
			if m.activePanel == panelEvents {
				m.scrollEventToBottom()
			}
		case "g":
			if m.activePanel == panelEvents {
				node := m.focusedNode()
				if node != nil {
					m.eventCursor = 0
					for m.eventCursor < len(node.Events)-1 && isHiddenEvent(node.Events, m.eventCursor) {
						m.eventCursor++
					}
				}
				m.eventScroll = 0
			}
		case "enter":
			if m.activePanel == panelEvents {
				node := m.focusedNode()
				if node != nil && m.eventCursor < len(node.Events) {
					m.modalOpen = true
					m.modalScroll = 0
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
		// Scroll wheel / trackpad: route to modal when open, otherwise to whichever panel the pointer is over.
		if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
			delta := 1
			if msg.Button == tea.MouseButtonWheelUp {
				delta = -1
			}
			if m.modalOpen {
				m.modalScroll = max(0, m.modalScroll+delta)
			} else {
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
					return m, saveState(m.collapsed, m.agents.Roots)
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
					headerH := 2 // session/spawn line + divider
					if node.ParentID != "" && node.Prompt != "" {
						headerH = 3 // child with prompt adds a prompt line
					}
					lineOffset := msg.Y - contentTop - headerH + m.eventScroll
					if lineOffset >= 0 {
						if idx, ok := m.eventAtLine(node, lineOffset); ok {
							m.activePanel = panelEvents
							// Double-click (same event within 400ms) opens the modal.
							if idx == m.lastClickIdx && !m.lastClickTime.IsZero() &&
								time.Since(m.lastClickTime) < 400*time.Millisecond {
								m.modalOpen = true
								m.modalScroll = 0
								m.lastClickTime = time.Time{}
							} else {
								// Single click: select + toggle inline expand for tool events.
								if idx == m.eventCursor && isToolEvent(node.Events[idx]) {
									key := eventKey(node.ID, idx)
									m.expandedEvents[key] = !m.expandedEvents[key]
									w := m.innerWidth()
									delete(m.lineHeightCache, lineHeightKey{nodeID: node.ID, idx: idx, expanded: true, width: w})
									delete(m.lineHeightCache, lineHeightKey{nodeID: node.ID, idx: idx, expanded: false, width: w})
								}
								m.lastClickTime = time.Now()
								m.lastClickIdx = idx
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
		// Auto-unhide: if a live event arrives for a hidden session, restore it
		// so the user can see the newly active agent without having to unhide manually.
		// Use the aliased node ID (in case this session was matched to a placeholder).
		unhideID := e.SessionID
		if alias, ok := m.agents.SessionAlias(e.SessionID); ok {
			unhideID = alias
		}
		cmd := waitForEvent(m.eventCh)
		if m.hiddenSessions[unhideID] {
			delete(m.hiddenSessions, unhideID)
			cmd = tea.Batch(cmd, saveHidden(m.hiddenSessions))
		}
		switch e.Type {
		case "PostToolUse":
			// Schedule an idle transition after the tool completes.
			// Increment the generation so any previously scheduled timer is invalidated.
			m.timerGen[e.SessionID]++
			cmd = tea.Batch(cmd, scheduleIdle(e.SessionID, m.timerGen[e.SessionID]))
			// (Re)schedule the watchdog timer if enabled. Any new tool call resets the clock.
			if m.watchdogMinutes > 0 {
				m.watchdogGen[e.SessionID]++
				m.watchdogFired[e.SessionID] = 0 // reset badge on new activity
				d := time.Duration(m.watchdogMinutes) * time.Minute
				cmd = tea.Batch(cmd, scheduleWatchdog(e.SessionID, m.watchdogGen[e.SessionID], d))
			}
			// Fire terminal bell when a new session error arrives.
			if node, ok := m.agents.Nodes[e.SessionID]; ok && node.Status == agent.StatusError {
				if !m.bellSent[e.SessionID] {
					m.bellSent[e.SessionID] = true
					cmd = tea.Batch(cmd, bellCmd())
				}
			}
		case "PreToolUse":
			// A new tool call started — invalidate any pending idle or done timer and watchdog.
			m.timerGen[e.SessionID]++
			m.watchdogGen[e.SessionID]++
			m.watchdogFired[e.SessionID] = 0
			// When the parent spawns a subagent, begin watching for its JSONL file.
			// We delay 500 ms to give Claude Code time to create the file.
			if e.Tool == "Agent" && m.subagentsRoot != "" {
				subagentsDir := filepath.Join(m.subagentsRoot, e.SessionID, "subagents")
				cmd = tea.Batch(cmd, scanSubagentsCmd(subagentsDir, e.SessionID, 500*time.Millisecond))
			}
		case "Stop", "SubagentStop":
			// Turn ended. Schedule a done transition after doneDuration of inactivity.
			// If the user sends another message before the timer fires, PreToolUse will
			// increment the generation and the timer will be discarded.
			m.timerGen[e.SessionID]++
			cmd = tea.Batch(cmd, scheduleDone(e.SessionID, m.timerGen[e.SessionID]))
			// Bell on done (only once per session).
			if !m.bellSent[e.SessionID] {
				m.bellSent[e.SessionID] = true
				cmd = tea.Batch(cmd, bellCmd())
			}
		}
		return m, cmd

	// Subagent JSONL scan completed. Inject historical events into the tree for
	// any newly discovered subagents and start live tailing each JSONL file.
	case subagentScanResultMsg:
		var cmds []tea.Cmd
		for _, r := range msg.results {
			if m.watchedSubagents[r.AgentID] {
				continue // already watching; avoid duplicate injection and tailing
			}
			m.watchedSubagents[r.AgentID] = true
			// Inject historical events. The first event carries ParentID so
			// ApplyEvent claims the placeholder node created by PreToolUse[Agent].
			for _, e := range r.Events {
				m.agents.ApplyEvent(e)
			}
			// After claiming, update the node's Name and Prompt from meta.json.
			// The placeholder was created with name=subagent_type; the description
			// is richer and should take precedence.
			nodeID := r.AgentID
			if alias, ok := m.agents.SessionAlias(r.AgentID); ok {
				nodeID = alias
			}
			if node, ok := m.agents.Nodes[nodeID]; ok {
				if r.Name != "" {
					node.Name = r.Name
				}
				if r.Description != "" {
					node.Prompt = r.Description
				}
			}
			// Start tailing for live updates. Each new line becomes a subagentTailMsg
			// that re-issues the wait, keeping the tail alive until the file goes quiet.
			ch := make(chan agent.Event, 64)
			watcher.Tail(r.Target, msg.parentID, ch)
			cmds = append(cmds, waitForTailEvent(ch))
		}
		if len(cmds) > 0 {
			return m, tea.Batch(cmds...)
		}

	// A live event arrived from a tailing goroutine. Apply it to the tree and
	// re-issue the wait so the tail continues.
	case subagentTailMsg:
		m.agents.ApplyEvent(msg.event)
		m.hasSession = true
		if n := m.focusedNode(); n != nil && n.ID == msg.event.SessionID {
			m.scrollEventToBottom()
		}
		return m, waitForTailEvent(msg.ch)

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

	// A watchdog timer fired. Increment the fired count for the node if the
	// generation still matches (no new tool call reset the timer).
	case watchdogTickMsg:
		if m.watchdogGen[msg.sessionID] == msg.gen {
			if node, ok := m.agents.Nodes[msg.sessionID]; ok && node.Status == agent.StatusRunning {
				count := m.watchdogFired[msg.sessionID] + 1
				m.watchdogFired[msg.sessionID] = count
				// Schedule a second tick at 2× the watchdog interval to escalate the badge.
				if count == 1 && m.watchdogMinutes > 0 {
					m.watchdogGen[msg.sessionID]++
					d := time.Duration(m.watchdogMinutes) * time.Minute
					return m, scheduleWatchdog(msg.sessionID, m.watchdogGen[msg.sessionID], d)
				}
			}
		}

	case hideSavedMsg:
		// Save completed; nothing to do (errors are silently dropped — the TUI
		// should not crash because a config write failed).

	case stateSavedMsg:
		// Save completed; nothing to do.
	}

	return m, nil
}

// View renders the current model state. Pure — no side effects.
func (m Model) View() string {
	if m.width == 0 {
		return "loading…"
	}

	// Reserve one line for the cost alert banner if it should be shown.
	costBanner := m.renderCostAlertBanner()
	bannerH := 0
	if costBanner != "" {
		bannerH = 1
	}

	bodyH := m.height - footerHeight - bannerH
	body := m.renderBody(bodyH)
	footer := m.renderFooter()

	if m.cancelOpen {
		body = overlayCenter(dimBody(body), m.renderCancelOverlay(), m.width, bodyH)
	} else if m.helpOpen {
		body = overlayCenter(dimBody(body), m.renderHelpOverlay(bodyH), m.width, bodyH)
	} else if m.modalOpen {
		body = overlayCenter(dimBody(body), m.renderDetailModal(bodyH), m.width, bodyH)
		footer = m.renderModalFooterBar()
	}

	parts := []string{body}
	if costBanner != "" {
		parts = append(parts, costBanner)
	}
	parts = append(parts, footer)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

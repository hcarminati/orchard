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

// Fixed heights for the header and footer rows (in terminal lines).
const (
	headerHeight = 1
	footerHeight = 1
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
	collapsed       map[string]bool    // set of node IDs whose subtrees are currently hidden
	collapsedGroups map[string]bool    // set of GroupIDs whose members are currently hidden
	statusFilter    filterMode         // which nodes to show in the agent tree
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

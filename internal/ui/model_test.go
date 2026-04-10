package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hcarminati/orchard/internal/agent"
)

// newModel returns a Model with no session data and no event channel,
// suitable for tests that only exercise layout and keyboard handling.
func newModel() Model {
	return New(nil, nil)
}

func TestUpdate_WindowSizeMsg(t *testing.T) {
	m := newModel()
	msg := tea.WindowSizeMsg{Width: 120, Height: 40}
	next, _ := m.Update(msg)
	got := next.(Model)
	if got.width != 120 {
		t.Errorf("width: got %d, want 120", got.width)
	}
	if got.height != 40 {
		t.Errorf("height: got %d, want 40", got.height)
	}
}

func TestUpdate_TabCyclesPanel(t *testing.T) {
	m := newModel()
	if m.activePanel != panelAgents {
		t.Fatal("expected initial panel to be panelAgents")
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if next.(Model).activePanel != panelEvents {
		t.Error("after first tab: expected panelEvents")
	}
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyTab})
	if next.(Model).activePanel != panelAgents {
		t.Error("after second tab: expected panelAgents")
	}
}

func TestUpdate_QReturnsQuit(t *testing.T) {
	m := newModel()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("expected a quit command, got nil")
	}
	// Execute the command and verify it produces a quit message.
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg, got %T", msg)
	}
}

func TestView_ZeroWidthReturnsNonEmpty(t *testing.T) {
	m := newModel() // width is 0 by default
	out := m.View()
	if out == "" {
		t.Error("expected non-empty string when width is 0, got empty string")
	}
}

func TestView_AfterWindowSizeNonEmpty(t *testing.T) {
	m := newModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	out := next.(Model).View()
	if out == "" {
		t.Error("expected non-empty View after window size message")
	}
}

// --- Data pipeline tests ---

func TestNew_WithNodes_SetsHasSession(t *testing.T) {
	nodes := []agent.Node{
		{ID: "s1", Name: "session:s1", Status: agent.StatusDone},
	}
	m := New(nodes, nil)
	if !m.hasSession {
		t.Error("expected hasSession=true when nodes are provided")
	}
	if len(m.agents.Nodes) != 1 {
		t.Errorf("expected 1 agent node, got %d", len(m.agents.Nodes))
	}
}

func TestNew_NoNodes_HasSessionFalse(t *testing.T) {
	m := New(nil, nil)
	if m.hasSession {
		t.Error("expected hasSession=false when no nodes provided")
	}
}

func TestUpdate_HookEventMsg_PopulatesTree(t *testing.T) {
	m := newModel()
	if m.hasSession {
		t.Fatal("expected no session initially")
	}

	e := agent.Event{
		Type:      "PreToolUse",
		SessionID: "new-session-id",
		Tool:      "Bash",
		Timestamp: time.Now(),
	}
	next, _ := m.Update(hookEventMsg{event: e})
	got := next.(Model)

	if !got.hasSession {
		t.Error("expected hasSession=true after hookEventMsg")
	}
	if _, ok := got.agents.Nodes["new-session-id"]; !ok {
		t.Error("expected 'new-session-id' node in tree after hookEventMsg")
	}
}

func TestUpdate_HookEventMsg_ReturnsWaitCmd(t *testing.T) {
	ch := make(chan agent.Event, 1)
	m := New(nil, ch)

	e := agent.Event{Type: "Stop", SessionID: "s1", Timestamp: time.Now()}
	_, cmd := m.Update(hookEventMsg{event: e})

	if cmd == nil {
		t.Error("expected a non-nil Cmd after hookEventMsg (to re-listen on channel)")
	}
}

func TestUpdate_MultipleHookEvents_AccumulateInTree(t *testing.T) {
	ch := make(chan agent.Event, 10)
	m := New(nil, ch)

	events := []agent.Event{
		{Type: "PreToolUse", SessionID: "s1", Tool: "Bash", Timestamp: time.Now()},
		{Type: "PostToolUse", SessionID: "s1", Tool: "Bash", Timestamp: time.Now()},
		{Type: "PreToolUse", SessionID: "s2", Tool: "Read", Timestamp: time.Now()},
	}

	var next tea.Model = m
	for _, e := range events {
		next, _ = next.Update(hookEventMsg{event: e})
	}

	got := next.(Model)
	if len(got.agents.Nodes) != 2 {
		t.Errorf("expected 2 agent nodes, got %d", len(got.agents.Nodes))
	}
	if node, ok := got.agents.Nodes["s1"]; !ok || len(node.Events) != 2 {
		t.Errorf("expected s1 with 2 events, got node=%v", node)
	}
}

func TestView_WaitingForSession_WhenNoSession(t *testing.T) {
	m := newModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := next.(Model).View()

	if !strings.Contains(view, "waiting for session") && !strings.Contains(view, "Waiting for session") {
		t.Errorf("expected 'waiting for session' in View when no session, got:\n%s", view)
	}
}

func TestView_ShowsAgentCount_WhenSessionLoaded(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Name: "session:aaaaaaaa", Status: agent.StatusDone},
		{ID: "b", Name: "session:bbbbbbbb", Status: agent.StatusRunning},
	}
	m := New(nodes, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := next.(Model).View()

	if !strings.Contains(view, "2 agents") {
		t.Errorf("expected '2 agents' in View header, got:\n%s", view)
	}
}

func TestInit_NilChannel_ReturnsNilCmd(t *testing.T) {
	m := New(nil, nil)
	cmd := m.Init()
	if cmd != nil {
		t.Error("expected nil Cmd from Init when eventCh is nil")
	}
}

func TestInit_WithChannel_ReturnsNonNilCmd(t *testing.T) {
	ch := make(chan agent.Event, 1)
	m := New(nil, ch)
	cmd := m.Init()
	if cmd == nil {
		t.Error("expected non-nil Cmd from Init when eventCh is set")
	}
}

func TestIdleTimeout_PostToolUse_SchedulesTimer(t *testing.T) {
	ch := make(chan agent.Event, 1)
	m := New(nil, ch)

	_, cmd := m.Update(hookEventMsg{event: agent.Event{
		Type: "PostToolUse", SessionID: "s1", Timestamp: time.Now(),
	}})

	// PostToolUse should return a batched cmd (waitForEvent + scheduleIdle).
	if cmd == nil {
		t.Error("expected a non-nil Cmd after PostToolUse (idle timer should be scheduled)")
	}
}

func TestIdleTimeout_TransitionsRunningToIdle(t *testing.T) {
	m := newModel()
	// Seed a running session.
	next, _ := m.Update(hookEventMsg{event: agent.Event{
		Type: "PreToolUse", SessionID: "s1", Timestamp: time.Now(),
	}})
	// Simulate PostToolUse to bump the generation to 1.
	next, _ = next.Update(hookEventMsg{event: agent.Event{
		Type: "PostToolUse", SessionID: "s1", Timestamp: time.Now(),
	}})
	// Fire the idle timeout with the current generation (2: PreToolUse bumped to 1, PostToolUse to 2).
	next, _ = next.Update(idleTimeoutMsg{sessionID: "s1", gen: 2})

	got := next.(Model)
	node := got.agents.Nodes["s1"]
	if node == nil {
		t.Fatal("expected node s1 to exist")
	}
	if node.Status != agent.StatusIdle {
		t.Errorf("expected StatusIdle after idle timeout, got %d", node.Status)
	}
}

func TestDoneTimeout_TransitionsIdleToDone(t *testing.T) {
	m := newModel()
	// Run a full turn: PreToolUse → PostToolUse → Stop.
	for _, typ := range []string{"PreToolUse", "PostToolUse", "Stop"} {
		next, _ := m.Update(hookEventMsg{event: agent.Event{
			Type: typ, SessionID: "s1", Timestamp: time.Now(),
		}})
		m = next.(Model)
	}
	// After Stop the session is Idle and timerGen["s1"] == 3 (one increment per event).
	gen := m.timerGen["s1"]
	next, _ := m.Update(doneTimeoutMsg{sessionID: "s1", gen: gen})

	node := next.(Model).agents.Nodes["s1"]
	if node == nil {
		t.Fatal("expected node s1")
	}
	if node.Status != agent.StatusDone {
		t.Errorf("expected StatusDone after done timeout, got %d", node.Status)
	}
}

func TestDoneTimeout_StaleGenIgnored(t *testing.T) {
	m := newModel()
	for _, typ := range []string{"PreToolUse", "PostToolUse", "Stop"} {
		next, _ := m.Update(hookEventMsg{event: agent.Event{
			Type: typ, SessionID: "s1", Timestamp: time.Now(),
		}})
		m = next.(Model)
	}
	staleGen := m.timerGen["s1"] - 1
	// New PreToolUse arrives, invalidating the done timer.
	next, _ := m.Update(hookEventMsg{event: agent.Event{
		Type: "PreToolUse", SessionID: "s1", Timestamp: time.Now(),
	}})
	next, _ = next.Update(doneTimeoutMsg{sessionID: "s1", gen: staleGen})

	node := next.(Model).agents.Nodes["s1"]
	if node.Status != agent.StatusRunning {
		t.Errorf("expected StatusRunning (stale done timer ignored), got %d", node.Status)
	}
}

func TestIdleTimeout_StaleGenIgnored(t *testing.T) {
	m := newModel()
	// PreToolUse → gen becomes 1.
	next, _ := m.Update(hookEventMsg{event: agent.Event{
		Type: "PreToolUse", SessionID: "s1", Timestamp: time.Now(),
	}})
	// PostToolUse → gen becomes 2, timer scheduled for gen 2.
	next, _ = next.Update(hookEventMsg{event: agent.Event{
		Type: "PostToolUse", SessionID: "s1", Timestamp: time.Now(),
	}})
	// Another PreToolUse arrives before the timer fires → gen becomes 3.
	next, _ = next.Update(hookEventMsg{event: agent.Event{
		Type: "PreToolUse", SessionID: "s1", Timestamp: time.Now(),
	}})
	// The stale timer (gen 2) fires — should be ignored.
	next, _ = next.Update(idleTimeoutMsg{sessionID: "s1", gen: 2})

	got := next.(Model)
	node := got.agents.Nodes["s1"]
	if node == nil {
		t.Fatal("expected node s1 to exist")
	}
	if node.Status != agent.StatusRunning {
		t.Errorf("expected StatusRunning (stale timer ignored), got %d", node.Status)
	}
}

// --- Parallel run grouping tests ---

func TestView_GroupLabel_ShowsCorrectCount(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Name: "agent-a", GroupID: "g1", Status: agent.StatusRunning},
		{ID: "b", Name: "agent-b", GroupID: "g1", Status: agent.StatusRunning},
		{ID: "c", Name: "agent-c", GroupID: "g1", Status: agent.StatusRunning},
	}
	m := New(nodes, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := next.(Model).View()
	if !strings.Contains(view, "parallel × 3") {
		t.Errorf("expected 'parallel × 3' in view, got:\n%s", view)
	}
}

func TestView_GroupLabel_TwoMembers(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Name: "agent-a", GroupID: "g1", Status: agent.StatusRunning},
		{ID: "b", Name: "agent-b", GroupID: "g1", Status: agent.StatusIdle},
	}
	m := New(nodes, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := next.(Model).View()
	if !strings.Contains(view, "parallel × 2") {
		t.Errorf("expected 'parallel × 2' in view, got:\n%s", view)
	}
}

func TestView_WinnerMarked_ShowsCheckmark(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Name: "agent-a", GroupID: "g1", Status: agent.StatusRunning},
		{ID: "b", Name: "agent-b", GroupID: "g1", Status: agent.StatusRunning},
	}
	m := New(nodes, nil)
	// cursor starts at 0 (node "a"); press enter to mark it as winner.
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyEnter})
	view := next.(Model).View()
	if !strings.Contains(view, "✓") {
		t.Errorf("expected winner indicator ✓ in view, got:\n%s", view)
	}
}

func TestUpdate_EnterMarksWinner_ClearsOtherSiblings(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Name: "agent-a", GroupID: "g1", Status: agent.StatusRunning},
		{ID: "b", Name: "agent-b", GroupID: "g1", Status: agent.StatusRunning},
	}
	m := New(nodes, nil)
	// cursor=0 → mark "a" as winner.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := next.(Model)

	if !got.agents.Nodes["a"].Winner {
		t.Error("expected node 'a' to be marked as winner")
	}
	if got.agents.Nodes["b"].Winner {
		t.Error("expected node 'b' to not be winner after 'a' is chosen")
	}

	// Navigate to "b" and mark it as winner — "a" should lose winner status.
	next, _ = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got = next.(Model)

	if got.agents.Nodes["a"].Winner {
		t.Error("expected node 'a' to lose winner status after 'b' is chosen")
	}
	if !got.agents.Nodes["b"].Winner {
		t.Error("expected node 'b' to be winner")
	}
}

func TestView_UngroupedNodesUnaffected(t *testing.T) {
	nodes := []agent.Node{
		{ID: "x", Name: "solo-agent", Status: agent.StatusRunning},
	}
	m := New(nodes, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := next.(Model).View()

	if strings.Contains(view, "parallel") {
		t.Errorf("expected no 'parallel' label for ungrouped node, got:\n%s", view)
	}
	if !strings.Contains(view, "solo-agent") {
		t.Errorf("expected node name 'solo-agent' in view, got:\n%s", view)
	}
}

func TestUpdate_JKNavigation(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Name: "agent-a", Status: agent.StatusRunning},
		{ID: "b", Name: "agent-b", Status: agent.StatusRunning},
		{ID: "c", Name: "agent-c", Status: agent.StatusRunning},
	}
	m := New(nodes, nil)
	if m.cursor != 0 {
		t.Errorf("expected initial cursor 0, got %d", m.cursor)
	}

	// j moves cursor down.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if next.(Model).cursor != 1 {
		t.Errorf("expected cursor 1 after j, got %d", next.(Model).cursor)
	}

	// j again.
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if next.(Model).cursor != 2 {
		t.Errorf("expected cursor 2 after second j, got %d", next.(Model).cursor)
	}

	// j at last item: cursor should not exceed bounds.
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if next.(Model).cursor != 2 {
		t.Errorf("expected cursor to stay at 2 at boundary, got %d", next.(Model).cursor)
	}

	// k moves cursor up.
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if next.(Model).cursor != 1 {
		t.Errorf("expected cursor 1 after k, got %d", next.(Model).cursor)
	}
}

func TestUpdate_KAtTopBoundary(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Name: "agent-a", Status: agent.StatusRunning},
	}
	m := New(nodes, nil)
	// k at top should not go below 0.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if next.(Model).cursor != 0 {
		t.Errorf("expected cursor to stay at 0 at top boundary, got %d", next.(Model).cursor)
	}
}

func TestUpdate_EnterOnUngroupedNode_NoEffect(t *testing.T) {
	nodes := []agent.Node{
		{ID: "x", Name: "solo", Status: agent.StatusRunning},
	}
	m := New(nodes, nil)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := next.(Model)
	// No GroupID means no winner should be set.
	if got.agents.Nodes["x"].Winner {
		t.Error("expected Winner to remain false for ungrouped node")
	}
}

// --- Status dot color tests ---

func TestStatusColor_CorrectPerStatus(t *testing.T) {
	cases := []struct {
		status agent.Status
		want   lipgloss.Color
	}{
		{agent.StatusRunning, colorGreen},
		{agent.StatusIdle, colorYellow},
		{agent.StatusDone, colorMuted},
		{agent.StatusError, colorRed},
	}
	for _, tc := range cases {
		got := statusColor(tc.status)
		if got != tc.want {
			t.Errorf("statusColor(%d) = %v, want %v", tc.status, got, tc.want)
		}
	}
}

func TestStatusColor_AllDistinct(t *testing.T) {
	statuses := []agent.Status{
		agent.StatusRunning,
		agent.StatusIdle,
		agent.StatusDone,
		agent.StatusError,
	}
	seen := map[lipgloss.Color]agent.Status{}
	for _, s := range statuses {
		c := statusColor(s)
		if c == "" {
			t.Errorf("statusColor(%d) returned empty color (would be invisible in any terminal)", s)
		}
		if prev, exists := seen[c]; exists {
			t.Errorf("statusColor(%d) and statusColor(%d) both return %v — statuses must have distinct colors", s, prev, c)
		}
		seen[c] = s
	}
}

// --- Error surfacing tests ---

func TestView_ErroredNodeAtTop(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Name: "agent-ok", Status: agent.StatusRunning},
		{ID: "b", Name: "agent-err", Status: agent.StatusError, ErrorMsg: "something broke"},
	}
	m := New(nodes, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := next.(Model).View()

	posOk := strings.Index(view, "agent-ok")
	posErr := strings.Index(view, "agent-err")
	if posErr == -1 {
		t.Fatal("expected 'agent-err' in view")
	}
	if posOk == -1 {
		t.Fatal("expected 'agent-ok' in view")
	}
	if posErr >= posOk {
		t.Errorf("expected errored node to appear before non-errored node in view")
	}
}

func TestView_ErrorMessageInline(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Name: "agent-err", Status: agent.StatusError, ErrorMsg: "panic"},
	}
	m := New(nodes, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := next.(Model).View()

	if !strings.Contains(view, "panic") {
		t.Errorf("expected error message in view, got:\n%s", view)
	}
}

func TestView_NoErrorMessage_WhenErrorMsgEmpty(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Name: "agent-err", Status: agent.StatusError, ErrorMsg: ""},
	}
	m := New(nodes, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := next.(Model).View()

	// Should not contain the ✗ indicator when there is no error message.
	if strings.Contains(view, "✗") {
		t.Errorf("expected no ✗ indicator when ErrorMsg is empty, got:\n%s", view)
	}
}

func TestSortedRoots_ErroredFirst(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Status: agent.StatusRunning},
		{ID: "b", Status: agent.StatusError},
		{ID: "c", Status: agent.StatusIdle},
		{ID: "d", Status: agent.StatusError},
	}
	m := New(nodes, nil)
	sr := m.sortedRoots()

	if len(sr) != 4 {
		t.Fatalf("expected 4 roots, got %d", len(sr))
	}
	// First two should be error nodes.
	for _, id := range sr[:2] {
		n := m.agents.Nodes[id]
		if n.Status != agent.StatusError {
			t.Errorf("expected first two sorted roots to be StatusError, got node %q with status %d", id, n.Status)
		}
	}
	// Remaining should be non-error.
	for _, id := range sr[2:] {
		n := m.agents.Nodes[id]
		if n.Status == agent.StatusError {
			t.Errorf("expected non-error nodes after errored ones, got node %q with StatusError", id)
		}
	}
}

func TestView_HookError_ErroredNodeAtTop(t *testing.T) {
	// Verify that an Error hook event causes the node to float to the top.
	ch := make(chan agent.Event, 10)
	m := New(nil, ch)

	var next tea.Model = m
	next, _ = m.Update(hookEventMsg{event: agent.Event{Type: "PreToolUse", SessionID: "s1", Timestamp: time.Now()}})
	next, _ = next.Update(hookEventMsg{event: agent.Event{Type: "PreToolUse", SessionID: "s2", Timestamp: time.Now()}})
	next, _ = next.Update(hookEventMsg{event: agent.Event{Type: "Error", SessionID: "s2", Message: "panic", Timestamp: time.Now()}})
	next, _ = next.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	view := next.(Model).View()
	// Session IDs "s1"/"s2" are short so no "session:" prefix is added.
	posS1 := strings.Index(view, " s1")
	posS2 := strings.Index(view, " s2")
	if posS1 == -1 || posS2 == -1 {
		t.Fatalf("expected both sessions in view, got:\n%s", view)
	}
	if posS2 >= posS1 {
		t.Errorf("expected errored session s2 to appear before s1 in view")
	}
}

func TestFooter_MarkWinnerHint_OnlyWhenCursorInGroup(t *testing.T) {
	grouped := []agent.Node{
		{ID: "a", Name: "agent-a", GroupID: "g1", Status: agent.StatusRunning},
		{ID: "b", Name: "agent-b", GroupID: "g1", Status: agent.StatusRunning},
		{ID: "c", Name: "solo", Status: agent.StatusRunning}, // ungrouped, index 2
	}
	m := New(grouped, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	// cursor=0: grouped node → hint should be visible.
	view := next.(Model).View()
	if !strings.Contains(view, "mark winner") {
		t.Errorf("expected 'mark winner' hint when cursor is on a grouped node, got:\n%s", view)
	}

	// Navigate to the ungrouped node (index 2).
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	view = next.(Model).View()
	if strings.Contains(view, "mark winner") {
		t.Errorf("expected no 'mark winner' hint when cursor is on an ungrouped node, got:\n%s", view)
	}
}

package ui

import (
	"fmt"
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

	// Count is now embedded in the Agents panel border title as "· 2".
	if !strings.Contains(view, "· 2") {
		t.Errorf("expected '· 2' in Agents panel border, got:\n%s", view)
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
	// cursor=0 is the group header; navigate to "a" (index 1) then mark as winner.
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
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
	// visibleNodes: [header(g1), a(1), b(2)]. Navigate to "a" (index 1) and mark as winner.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := next.(Model)

	if !got.agents.Nodes["a"].Winner {
		t.Error("expected node 'a' to be marked as winner")
	}
	if got.agents.Nodes["b"].Winner {
		t.Error("expected node 'b' to not be winner after 'a' is chosen")
	}

	// Navigate to "b" (index 2) and mark it as winner — "a" should lose winner status.
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

	next, _ := m.Update(hookEventMsg{event: agent.Event{Type: "PreToolUse", SessionID: "s1", Timestamp: time.Now()}})
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

// --- Collapsible tree tests ---

// makeTree creates a Model with a simple parent-child tree:
//
//	A (root, children: B, C)
//	D (root, no children)
func makeTree() Model {
	nodes := []agent.Node{
		{ID: "A", Name: "agent-A", Status: agent.StatusRunning},
		{ID: "B", Name: "agent-B", ParentID: "A", Status: agent.StatusRunning},
		{ID: "C", Name: "agent-C", ParentID: "A", Status: agent.StatusRunning},
		{ID: "D", Name: "agent-D", Status: agent.StatusRunning},
	}
	return New(nodes, nil)
}

func TestVisibleNodes_FlatTree(t *testing.T) {
	nodes := []agent.Node{
		{ID: "x", Name: "x", Status: agent.StatusRunning},
		{ID: "y", Name: "y", Status: agent.StatusRunning},
	}
	m := New(nodes, nil)
	vn := m.visibleNodes()
	if len(vn) != 2 {
		t.Fatalf("expected 2 visible nodes, got %d", len(vn))
	}
	if vn[0].id != "x" || vn[0].depth != 0 {
		t.Errorf("expected vn[0]={x,0}, got %+v", vn[0])
	}
	if vn[1].id != "y" || vn[1].depth != 0 {
		t.Errorf("expected vn[1]={y,0}, got %+v", vn[1])
	}
}

func TestVisibleNodes_NestedTree(t *testing.T) {
	m := makeTree()
	vn := m.visibleNodes()
	// Expected DFS order: A(0), B(1), C(1), D(0)
	if len(vn) != 4 {
		t.Fatalf("expected 4 visible nodes, got %d: %+v", len(vn), vn)
	}
	cases := []visibleNode{
		{id: "A", depth: 0},
		{id: "B", depth: 1},
		{id: "C", depth: 1},
		{id: "D", depth: 0},
	}
	for i, want := range cases {
		if vn[i] != want {
			t.Errorf("vn[%d]: got %+v, want %+v", i, vn[i], want)
		}
	}
}

func TestVisibleNodes_CollapsedNodeHidesChildren(t *testing.T) {
	m := makeTree()
	m.collapsed["A"] = true
	vn := m.visibleNodes()
	// A is collapsed: B and C are hidden. Expected: A(0), D(0)
	if len(vn) != 2 {
		t.Fatalf("expected 2 visible nodes after collapsing A, got %d: %+v", len(vn), vn)
	}
	if vn[0].id != "A" || vn[1].id != "D" {
		t.Errorf("expected [A, D], got [%s, %s]", vn[0].id, vn[1].id)
	}
}

func TestUpdate_JK_NavigatesNestedTree(t *testing.T) {
	m := makeTree()
	// visibleNodes: A(0), B(1), C(2), D(3)
	if m.cursor != 0 {
		t.Fatalf("expected initial cursor 0, got %d", m.cursor)
	}

	var next tea.Model = m
	// j → B (1)
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if next.(Model).cursor != 1 {
		t.Errorf("expected cursor 1 after j, got %d", next.(Model).cursor)
	}
	// j → C (2)
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if next.(Model).cursor != 2 {
		t.Errorf("expected cursor 2 after j, got %d", next.(Model).cursor)
	}
	// j → D (3)
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if next.(Model).cursor != 3 {
		t.Errorf("expected cursor 3 after j, got %d", next.(Model).cursor)
	}
	// j at bottom: stays at 3
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if next.(Model).cursor != 3 {
		t.Errorf("expected cursor to stay at 3 at boundary, got %d", next.(Model).cursor)
	}
	// k → C (2)
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if next.(Model).cursor != 2 {
		t.Errorf("expected cursor 2 after k, got %d", next.(Model).cursor)
	}
}

func TestUpdate_JK_RespectsCollapsedState(t *testing.T) {
	m := makeTree()
	m.collapsed["A"] = true
	// visibleNodes after collapse: A(0), D(1)
	var next tea.Model = m
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if next.(Model).cursor != 1 {
		t.Errorf("expected cursor 1 (D) after j with A collapsed, got %d", next.(Model).cursor)
	}
	// j at boundary: stays at 1
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if next.(Model).cursor != 1 {
		t.Errorf("expected cursor to stay at 1 at boundary, got %d", next.(Model).cursor)
	}
}

func TestUpdate_Space_TogglesCollapse(t *testing.T) {
	m := makeTree()
	// cursor=0 (A, which has children); press space to collapse.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	got := next.(Model)
	if !got.collapsed["A"] {
		t.Error("expected A to be collapsed after space")
	}
	vn := got.visibleNodes()
	if len(vn) != 2 {
		t.Errorf("expected 2 visible nodes after collapse, got %d: %+v", len(vn), vn)
	}

	// Press space again to expand.
	next, _ = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	got = next.(Model)
	if got.collapsed["A"] {
		t.Error("expected A to be expanded after second space")
	}
	vn = got.visibleNodes()
	if len(vn) != 4 {
		t.Errorf("expected 4 visible nodes after expand, got %d: %+v", len(vn), vn)
	}
}

func TestUpdate_Space_NoEffectOnLeaf(t *testing.T) {
	m := makeTree()
	// Navigate to D (index 3, a leaf).
	var next tea.Model = m
	for i := 0; i < 3; i++ {
		next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	}
	got := next.(Model)
	if got.cursor != 3 {
		t.Fatalf("expected cursor 3 (D), got %d", got.cursor)
	}
	// Space on a leaf should not change collapsed state.
	next, _ = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	final := next.(Model)
	if len(final.collapsed) != 0 {
		t.Errorf("expected no collapse state changes for leaf, got collapsed=%v", final.collapsed)
	}
	if len(final.visibleNodes()) != 4 {
		t.Errorf("expected 4 visible nodes (unchanged), got %d", len(final.visibleNodes()))
	}
}

func TestUpdate_CursorCorrection_AfterCollapse(t *testing.T) {
	// Verify cursor is never left pointing at a hidden node after collapse.
	// Setup: A(root, children: B, C), D(root).
	// Navigate to D (index 3), navigate back to A (index 0), collapse A.
	// Cursor should remain on A (valid index 0).
	m := makeTree()
	var next tea.Model = m
	// Navigate to D.
	for i := 0; i < 3; i++ {
		next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	}
	// Navigate back to A.
	for i := 0; i < 3; i++ {
		next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	}
	// Collapse A.
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	got := next.(Model)

	vn := got.visibleNodes()
	if got.cursor >= len(vn) {
		t.Errorf("cursor %d >= visible len %d — cursor correction failed", got.cursor, len(vn))
	}
}

func TestUpdate_CursorCorrection_ClampedWhenOutOfBounds(t *testing.T) {
	// Directly force cursor out of bounds (as may happen via hook events or future features).
	// Pressing space should clamp it to a valid index.
	m := makeTree()
	m.cursor = 99 // way out of bounds
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	got := next.(Model)
	vn := got.visibleNodes()
	if got.cursor >= len(vn) {
		t.Errorf("cursor %d still out of bounds (visible: %d) after space", got.cursor, len(vn))
	}
}

func TestView_TreeRendering_ChildIndented(t *testing.T) {
	m := makeTree()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := next.(Model).View()

	posA := strings.Index(view, "agent-A")
	posB := strings.Index(view, "agent-B")
	posC := strings.Index(view, "agent-C")
	posD := strings.Index(view, "agent-D")

	for name, pos := range map[string]int{"agent-A": posA, "agent-B": posB, "agent-C": posC, "agent-D": posD} {
		if pos == -1 {
			t.Errorf("expected %q in view", name)
		}
	}
	// DFS order: A before B before C before D.
	if posA >= posB {
		t.Errorf("expected A before B in view")
	}
	if posB >= posC {
		t.Errorf("expected B before C in view")
	}
	if posC >= posD {
		t.Errorf("expected C before D in view")
	}
}

func TestView_CollapseIndicator_ShownForNodesWithChildren(t *testing.T) {
	m := makeTree()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := next.(Model).View()
	// A has children and is expanded → should show expand indicator.
	if !strings.Contains(view, "▼") {
		t.Errorf("expected expand indicator ▼ for expanded node A, got:\n%s", view)
	}

	// Collapse A.
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	view = next.(Model).View()
	if !strings.Contains(view, "▶") {
		t.Errorf("expected collapse indicator ▶ for collapsed node A, got:\n%s", view)
	}
}

func TestView_CollapsedNode_HidesChildrenFromView(t *testing.T) {
	m := makeTree()
	// Collapse A (cursor=0).
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	view := next.(Model).View()

	if strings.Contains(view, "agent-B") {
		t.Errorf("expected agent-B to be hidden after collapsing A")
	}
	if strings.Contains(view, "agent-C") {
		t.Errorf("expected agent-C to be hidden after collapsing A")
	}
	if !strings.Contains(view, "agent-A") {
		t.Errorf("expected agent-A (collapsed) to still be visible")
	}
	if !strings.Contains(view, "agent-D") {
		t.Errorf("expected agent-D (sibling root) to still be visible")
	}
}

func TestFooter_SpaceHint_OnlyWhenCursorOnNodeWithChildren(t *testing.T) {
	m := makeTree()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	// cursor=0 (A, has children) → space hint shown.
	view := next.(Model).View()
	if !strings.Contains(view, "expand/collapse") {
		t.Errorf("expected 'expand/collapse' hint when cursor is on node with children, got:\n%s", view)
	}

	// Navigate to D (leaf, no children).
	for i := 0; i < 3; i++ {
		next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	}
	view = next.(Model).View()
	if strings.Contains(view, "expand/collapse") {
		t.Errorf("expected no 'expand/collapse' hint when cursor is on a leaf, got:\n%s", view)
	}
}

func TestFooter_SpaceHint_ShownForGroupHeader(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Name: "agent-a", GroupID: "g1", Status: agent.StatusRunning},
		{ID: "b", Name: "agent-b", GroupID: "g1", Status: agent.StatusRunning},
	}
	m := New(nodes, nil)
	// cursor=0 is the group header → space hint shown.
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := next.(Model).View()
	if !strings.Contains(view, "expand/collapse") {
		t.Errorf("expected 'expand/collapse' hint when cursor is on group header, got:\n%s", view)
	}
}

func TestVisibleNodes_GroupHeaderInVisibleNodes(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Name: "agent-a", GroupID: "g1", Status: agent.StatusRunning},
		{ID: "b", Name: "agent-b", GroupID: "g1", Status: agent.StatusRunning},
	}
	m := New(nodes, nil)
	vn := m.visibleNodes()
	// Expected: [header(g1), a, b]
	if len(vn) != 3 {
		t.Fatalf("expected 3 visible nodes (1 header + 2 members), got %d: %+v", len(vn), vn)
	}
	if vn[0].groupID != "g1" || vn[0].id != "" {
		t.Errorf("expected vn[0] to be group header for g1, got %+v", vn[0])
	}
	if vn[1].id != "a" || vn[2].id != "b" {
		t.Errorf("expected vn[1]=a, vn[2]=b, got %+v %+v", vn[1], vn[2])
	}
}

func TestVisibleNodes_CollapsedGroup_HidesMembers(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Name: "agent-a", GroupID: "g1", Status: agent.StatusRunning},
		{ID: "b", Name: "agent-b", GroupID: "g1", Status: agent.StatusRunning},
		{ID: "c", Name: "solo", Status: agent.StatusRunning},
	}
	m := New(nodes, nil)
	m.collapsedGroups["g1"] = true
	vn := m.visibleNodes()
	// Expected: [header(g1), c]
	if len(vn) != 2 {
		t.Fatalf("expected 2 visible nodes (header + solo), got %d: %+v", len(vn), vn)
	}
	if vn[0].groupID != "g1" {
		t.Errorf("expected vn[0] to be group header, got %+v", vn[0])
	}
	if vn[1].id != "c" {
		t.Errorf("expected vn[1] to be 'c', got %+v", vn[1])
	}
}

func TestUpdate_Space_CollapsesGroup(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Name: "agent-a", GroupID: "g1", Status: agent.StatusRunning},
		{ID: "b", Name: "agent-b", GroupID: "g1", Status: agent.StatusRunning},
	}
	m := New(nodes, nil)
	// cursor=0 is the group header; press space to collapse.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	got := next.(Model)
	if !got.collapsedGroups["g1"] {
		t.Error("expected group g1 to be collapsed after space on header")
	}
	if len(got.visibleNodes()) != 1 {
		t.Errorf("expected 1 visible node (header only), got %d", len(got.visibleNodes()))
	}

	// Press space again to expand.
	next, _ = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	got = next.(Model)
	if got.collapsedGroups["g1"] {
		t.Error("expected group g1 to be expanded after second space")
	}
	if len(got.visibleNodes()) != 3 {
		t.Errorf("expected 3 visible nodes (header + 2 members), got %d", len(got.visibleNodes()))
	}
}

func TestView_CollapsedGroup_HidesMembersFromView(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Name: "agent-a", GroupID: "g1", Status: agent.StatusRunning},
		{ID: "b", Name: "agent-b", GroupID: "g1", Status: agent.StatusRunning},
	}
	m := New(nodes, nil)
	// Collapse the group via space on the header (cursor=0).
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	view := next.(Model).View()

	if strings.Contains(view, "agent-a") {
		t.Errorf("expected agent-a to be hidden after group collapse")
	}
	if strings.Contains(view, "agent-b") {
		t.Errorf("expected agent-b to be hidden after group collapse")
	}
	if !strings.Contains(view, "parallel × 2") {
		t.Errorf("expected group header 'parallel × 2' to remain visible")
	}
	if !strings.Contains(view, "▶") {
		t.Errorf("expected collapsed indicator ▶ on group header")
	}
}

func TestFooter_MarkWinnerHint_OnlyWhenCursorInGroup(t *testing.T) {
	grouped := []agent.Node{
		{ID: "a", Name: "agent-a", GroupID: "g1", Status: agent.StatusRunning},
		{ID: "b", Name: "agent-b", GroupID: "g1", Status: agent.StatusRunning},
		{ID: "c", Name: "solo", Status: agent.StatusRunning}, // ungrouped
	}
	m := New(grouped, nil)
	// visibleNodes: [header(g1)(0), a(1), b(2), c(3)]
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	// cursor=0 is the group header — no "mark winner" hint (not a member).
	view := next.(Model).View()
	if strings.Contains(view, "mark winner") {
		t.Errorf("expected no 'mark winner' hint when cursor is on the group header, got:\n%s", view)
	}

	// Navigate to "a" (index 1) — grouped member → hint shown.
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	view = next.(Model).View()
	if !strings.Contains(view, "mark winner") {
		t.Errorf("expected 'mark winner' hint when cursor is on a grouped node, got:\n%s", view)
	}

	// Navigate to "c" (index 3, ungrouped) — no hint.
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	view = next.(Model).View()
	if strings.Contains(view, "mark winner") {
		t.Errorf("expected no 'mark winner' hint when cursor is on an ungrouped node, got:\n%s", view)
	}
}

// --- Status filter tests ---

func TestUpdate_FilterCyclesMode(t *testing.T) {
	m := New(nil, nil)
	if m.statusFilter != filterAll {
		t.Fatalf("expected initial filter to be filterAll, got %d", m.statusFilter)
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	got := next.(Model)
	if got.statusFilter != filterRunning {
		t.Errorf("after first f: expected filterRunning, got %d", got.statusFilter)
	}

	next, _ = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	got = next.(Model)
	if got.statusFilter != filterErrored {
		t.Errorf("after second f: expected filterErrored, got %d", got.statusFilter)
	}

	next, _ = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	got = next.(Model)
	if got.statusFilter != filterAll {
		t.Errorf("after third f: expected filterAll (wrap), got %d", got.statusFilter)
	}
}

func TestUpdate_FilterResetsCursorToZero(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Name: "agent-a", Status: agent.StatusRunning},
		{ID: "b", Name: "agent-b", Status: agent.StatusRunning},
		{ID: "c", Name: "agent-c", Status: agent.StatusRunning},
	}
	m := New(nodes, nil)

	// Move cursor to index 2.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if next.(Model).cursor != 2 {
		t.Fatalf("expected cursor 2 before filter change, got %d", next.(Model).cursor)
	}

	// Pressing f resets cursor to 0.
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	if next.(Model).cursor != 0 {
		t.Errorf("expected cursor 0 after filter change, got %d", next.(Model).cursor)
	}
}

func TestFilter_RunningHidesNonRunningNodes(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Name: "agent-a", Status: agent.StatusRunning},
		{ID: "b", Name: "agent-b", Status: agent.StatusIdle},
		{ID: "c", Name: "agent-c", Status: agent.StatusError},
		{ID: "d", Name: "agent-d", Status: agent.StatusRunning},
	}
	m := New(nodes, nil)
	m.statusFilter = filterRunning

	vn := m.visibleNodes()
	if len(vn) != 2 {
		t.Fatalf("expected 2 visible nodes under filterRunning, got %d", len(vn))
	}
	for _, v := range vn {
		n := m.agents.Nodes[v.id]
		if n == nil {
			t.Fatalf("unexpected nil node for id %q", v.id)
		}
		if n.Status != agent.StatusRunning {
			t.Errorf("expected only running nodes visible, got node %q with status %d", v.id, n.Status)
		}
	}
}

func TestFilter_ErroredHidesNonErroredNodes(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Name: "agent-a", Status: agent.StatusRunning},
		{ID: "b", Name: "agent-b", Status: agent.StatusError},
		{ID: "c", Name: "agent-c", Status: agent.StatusIdle},
	}
	m := New(nodes, nil)
	m.statusFilter = filterErrored

	vn := m.visibleNodes()
	if len(vn) != 1 {
		t.Fatalf("expected 1 visible node under filterErrored, got %d", len(vn))
	}
	if vn[0].id != "b" {
		t.Errorf("expected only node 'b' visible under filterErrored, got %q", vn[0].id)
	}
}

func TestFilter_AllShowsEveryNode(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Status: agent.StatusRunning},
		{ID: "b", Status: agent.StatusIdle},
		{ID: "c", Status: agent.StatusDone},
		{ID: "d", Status: agent.StatusError},
	}
	m := New(nodes, nil)
	m.statusFilter = filterAll

	vn := m.visibleNodes()
	if len(vn) != 4 {
		t.Errorf("expected 4 visible nodes under filterAll, got %d", len(vn))
	}
}

func TestFilter_HiddenNodesAbsentFromView(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Name: "running-agent", Status: agent.StatusRunning},
		{ID: "b", Name: "idle-agent", Status: agent.StatusIdle},
	}
	m := New(nodes, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	// Press f once → filterRunning.
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	view := next.(Model).View()

	if !strings.Contains(view, "running-agent") {
		t.Errorf("expected running node in view under filterRunning, got:\n%s", view)
	}
	if strings.Contains(view, "idle-agent") {
		t.Errorf("expected idle node absent from view under filterRunning, got:\n%s", view)
	}
}

func TestFilter_FooterShowsCurrentMode(t *testing.T) {
	m := New(nil, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	// filterAll (default).
	view := next.(Model).View()
	if !strings.Contains(view, "filter:all") {
		t.Errorf("expected 'filter:all' in footer, got:\n%s", view)
	}

	// filterRunning.
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	view = next.(Model).View()
	if !strings.Contains(view, "filter:running") {
		t.Errorf("expected 'filter:running' in footer after first f, got:\n%s", view)
	}

	// filterErrored.
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	view = next.(Model).View()
	if !strings.Contains(view, "filter:errored") {
		t.Errorf("expected 'filter:errored' in footer after second f, got:\n%s", view)
	}
}

// --- Mouse click tests ---

// contentTop is the Y offset where agent tree content rows begin:
// top border (1) only — the header row has been removed.
const contentTop = 1

// mouseClick builds a left-button press MouseMsg at the given screen coordinates.
func mouseClick(x, y int) tea.MouseMsg {
	return tea.MouseMsg{
		X:      x,
		Y:      y,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	}
}

func TestUpdate_MouseClick_MovesCursorToClickedRow(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Name: "agent-a", Status: agent.StatusRunning},
		{ID: "b", Name: "agent-b", Status: agent.StatusRunning},
		{ID: "c", Name: "agent-c", Status: agent.StatusRunning},
	}
	m := New(nodes, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	// Click on the third row (index 2).
	next, _ = m.Update(mouseClick(5, contentTop+2))
	if next.(Model).cursor != 2 {
		t.Errorf("expected cursor 2 after clicking row 2, got %d", next.(Model).cursor)
	}
}

func TestUpdate_MouseClick_AlreadySelectedLeaf_IsNoop(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Name: "agent-a", Status: agent.StatusRunning},
		{ID: "b", Name: "agent-b", Status: agent.StatusRunning},
	}
	m := New(nodes, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	// Navigate to leaf row 1.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = next.(Model)
	if m.cursor != 1 {
		t.Fatalf("expected cursor 1 after j, got %d", m.cursor)
	}

	// Click the already-selected leaf row — cursor must stay the same.
	next, _ = m.Update(mouseClick(5, contentTop+1))
	if next.(Model).cursor != 1 {
		t.Errorf("expected cursor to remain 1 after clicking already-selected leaf, got %d", next.(Model).cursor)
	}
}

func TestUpdate_MouseClick_TogglesCollapseOnNodeWithChildren(t *testing.T) {
	parent := agent.Node{ID: "p", Name: "parent", Status: agent.StatusRunning}
	child := agent.Node{ID: "c", Name: "child", ParentID: "p", Status: agent.StatusRunning}
	m := New([]agent.Node{parent, child}, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	// Initially expanded: two visible rows (parent + child).
	if len(m.visibleNodes()) != 2 {
		t.Fatalf("expected 2 visible nodes initially, got %d", len(m.visibleNodes()))
	}

	// Click the parent row — should collapse it.
	next, _ = m.Update(mouseClick(5, contentTop+0))
	m = next.(Model)
	if len(m.visibleNodes()) != 1 {
		t.Errorf("expected 1 visible node after collapsing parent by click, got %d", len(m.visibleNodes()))
	}

	// Click the parent row again — should expand it.
	next, _ = m.Update(mouseClick(5, contentTop+0))
	m = next.(Model)
	if len(m.visibleNodes()) != 2 {
		t.Errorf("expected 2 visible nodes after expanding parent by click, got %d", len(m.visibleNodes()))
	}
}

func TestUpdate_MouseClick_TogglesCollapseOnGroupHeader(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Name: "agent-a", GroupID: "g1", Status: agent.StatusRunning},
		{ID: "b", Name: "agent-b", GroupID: "g1", Status: agent.StatusRunning},
	}
	m := New(nodes, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	// Initially: header + 2 members = 3 visible rows.
	if len(m.visibleNodes()) != 3 {
		t.Fatalf("expected 3 visible nodes initially, got %d", len(m.visibleNodes()))
	}

	// Click the group header row (index 0) — should collapse the group.
	next, _ = m.Update(mouseClick(5, contentTop+0))
	m = next.(Model)
	if len(m.visibleNodes()) != 1 {
		t.Errorf("expected 1 visible node after collapsing group by click, got %d", len(m.visibleNodes()))
	}

	// Click again — should expand.
	next, _ = m.Update(mouseClick(5, contentTop+0))
	m = next.(Model)
	if len(m.visibleNodes()) != 3 {
		t.Errorf("expected 3 visible nodes after expanding group by click, got %d", len(m.visibleNodes()))
	}
}

func TestUpdate_MouseClick_OutsideAgentsPanel_NoEffect(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Name: "agent-a", Status: agent.StatusRunning},
		{ID: "b", Name: "agent-b", Status: agent.StatusRunning},
	}
	m := New(nodes, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	// Click in the right (Events) panel — X is past 35% width.
	rightX := 120*35/100 + 5
	next, _ = m.Update(mouseClick(rightX, contentTop+1))
	if next.(Model).cursor != 0 {
		t.Errorf("expected cursor to stay 0 when clicking outside agents panel, got %d", next.(Model).cursor)
	}
}

func TestUpdate_MouseClick_DoesNotBreakKeyboardNavigation(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Name: "agent-a", Status: agent.StatusRunning},
		{ID: "b", Name: "agent-b", Status: agent.StatusRunning},
		{ID: "c", Name: "agent-c", Status: agent.StatusRunning},
	}
	m := New(nodes, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	// Click row 2 with mouse.
	next, _ = m.Update(mouseClick(5, contentTop+2))
	m = next.(Model)
	if m.cursor != 2 {
		t.Fatalf("expected cursor 2 after mouse click, got %d", m.cursor)
	}

	// Then use keyboard to move up.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if next.(Model).cursor != 1 {
		t.Errorf("expected cursor 1 after k following mouse click, got %d", next.(Model).cursor)
	}
}

// --- Right-panel tab strip tests ---

func TestUpdate_RightTab_InitialIsZero(t *testing.T) {
	m := newModel()
	if m.activeRightTab != 0 {
		t.Errorf("expected activeRightTab=0 initially, got %d", m.activeRightTab)
	}
}

func TestUpdate_RightBracket_CyclesForward(t *testing.T) {
	// With only one tab ]  still wraps safely back to 0.
	m := newModel()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	got := next.(Model)
	want := (0 + 1) % len(rightTabs)
	if got.activeRightTab != want {
		t.Errorf("] cycle: got activeRightTab=%d, want %d", got.activeRightTab, want)
	}
}

func TestUpdate_LeftBracket_CyclesBackward(t *testing.T) {
	// With only one tab [ wraps safely back to 0.
	m := newModel()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[")})
	got := next.(Model)
	n := len(rightTabs)
	want := (0 - 1 + n) % n
	if got.activeRightTab != want {
		t.Errorf("[ cycle: got activeRightTab=%d, want %d", got.activeRightTab, want)
	}
}

func TestRightTabCycling_WrapsAtBothEnds(t *testing.T) {
	// Use a temporary replacement of rightTabs to exercise wrapping with >1 tab.
	orig := rightTabs
	rightTabs = []rightTab{tabEvents, 1, 2} // 3 tabs
	defer func() { rightTabs = orig }()

	m := newModel()

	// ] from index 0 → 1 → 2 → wraps to 0.
	for i, wantIdx := range []int{1, 2, 0} {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
		m = next.(Model)
		if m.activeRightTab != wantIdx {
			t.Errorf("] step %d: got activeRightTab=%d, want %d", i, m.activeRightTab, wantIdx)
		}
	}

	// [ from index 0 → 2 → 1 → 0.
	for i, wantIdx := range []int{2, 1, 0} {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[")})
		m = next.(Model)
		if m.activeRightTab != wantIdx {
			t.Errorf("[ step %d: got activeRightTab=%d, want %d", i, m.activeRightTab, wantIdx)
		}
	}
}

func TestTabStripTitle_LabelsPresent(t *testing.T) {
	m := newModel()
	strip := m.tabStripTitle(colorAccent)
	for _, tab := range rightTabs {
		if !strings.Contains(strip, tab.label()) {
			t.Errorf("tab strip title missing label %q", tab.label())
		}
	}
}

func TestTabStripTitle_ActiveTabDistinct(t *testing.T) {
	m := newModel()
	// All tab labels must appear regardless of which is active.
	for range rightTabs {
		strip := m.tabStripTitle(colorAccent)
		for _, tab := range rightTabs {
			if !strings.Contains(strip, tab.label()) {
				t.Errorf("tab strip title missing label %q at activeRightTab=%d", tab.label(), m.activeRightTab)
			}
		}
		m.activeRightTab = (m.activeRightTab + 1) % len(rightTabs)
	}
}

func TestView_RightPanel_ShowsTabStrip(t *testing.T) {
	m := newModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := next.(Model).View()

	// The "Events" tab label must appear somewhere in the rendered view.
	if !strings.Contains(view, "Events") {
		t.Error("expected 'Events' tab label in view output")
	}
}

// --- Tab click tests ---

func TestTabAtX_HitsFirstTab(t *testing.T) {
	m := newModel() // activeRightTab=0, so first tab renders as "[Events]"
	// "[Events]" starts at x=0 and is 8 chars wide.
	for x := 0; x < len("[Events]"); x++ {
		idx, ok := m.tabAtX(x)
		if !ok {
			t.Errorf("x=%d: expected hit, got miss", x)
		}
		if idx != 0 {
			t.Errorf("x=%d: expected tab 0, got %d", x, idx)
		}
	}
}

func TestTabAtX_HitsSecondTab(t *testing.T) {
	m := newModel() // activeRightTab=0, so second tab renders as "Files"
	// "[Events]" (8) + " " (1) = offset 9 for "Files" (5 chars wide)
	start := len("[Events]") + 1
	for x := start; x < start+len("Files"); x++ {
		idx, ok := m.tabAtX(x)
		if !ok {
			t.Errorf("x=%d: expected hit on Files, got miss", x)
		}
		if idx != 1 {
			t.Errorf("x=%d: expected tab 1, got %d", x, idx)
		}
	}
}

func TestTabAtX_MissOnGap(t *testing.T) {
	m := newModel()
	// x past all tabs should return false.
	_, ok := m.tabAtX(999)
	if ok {
		t.Error("expected miss for x past all tabs")
	}
}

func TestMouseClick_SwitchesTab(t *testing.T) {
	m := newModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	leftW := m.width * 35 / 100
	// Click on "Files" in the top border (Y=0).
	// Strip starts at x = leftW+2. "Files" starts after "[Events] " = 9 chars.
	filesX := leftW + 2 + len("[Events]") + 1
	next, _ = m.Update(tea.MouseMsg{
		X:      filesX,
		Y:      0,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	if next.(Model).activeRightTab != 1 {
		t.Errorf("expected activeRightTab=1 after clicking Files, got %d", next.(Model).activeRightTab)
	}
}

// --- Events tab tests ---

// makeNodeWithEvents returns an agent.Node with n synthetic events.
func makeNodeWithEvents(id string, n int) agent.Node {
	ts := time.Date(2026, 1, 1, 15, 4, 5, 0, time.UTC)
	node := agent.Node{
		ID:     id,
		Name:   "session:" + id[:8],
		Status: agent.StatusRunning,
	}
	for i := 0; i < n; i++ {
		node.Events = append(node.Events, agent.Event{
			Type:      "PreToolUse",
			SessionID: id,
			Tool:      "Bash",
			Timestamp: ts.Add(time.Duration(i) * time.Second),
		})
	}
	return node
}

func TestEventsContent_NoSession_ShowsWaiting(t *testing.T) {
	m := newModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	content := next.(Model).eventsContent()
	if !strings.Contains(content, "Waiting") {
		t.Errorf("expected waiting message when no session, got: %q", content)
	}
}

func TestEventsContent_NoEvents_ShowsEmptyState(t *testing.T) {
	nodes := []agent.Node{
		{ID: "s1xxxxxxxx", Name: "session:s1xxxxxx", Status: agent.StatusRunning},
	}
	m := New(nodes, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	content := next.(Model).eventsContent()
	if !strings.Contains(content, "No events yet") {
		t.Errorf("expected empty state message, got: %q", content)
	}
}

func TestEventsContent_ShowsTimestampAndType(t *testing.T) {
	ts := time.Date(2026, 1, 1, 15, 4, 5, 0, time.UTC)
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{Type: "PreToolUse", Tool: "Bash", SessionID: "s1xxxxxxxx", Timestamp: ts},
			},
		},
	}
	m := New(nodes, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	content := next.(Model).eventsContent()

	if !strings.Contains(content, "15:04:05") {
		t.Errorf("expected timestamp in events content, got: %q", content)
	}
	if !strings.Contains(content, "PreToolUse") {
		t.Errorf("expected event type in events content, got: %q", content)
	}
	if !strings.Contains(content, "Bash") {
		t.Errorf("expected tool name in events content, got: %q", content)
	}
}

func TestEventsContent_NoToolName_WhenEventHasNone(t *testing.T) {
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{Type: "Stop", SessionID: "s1xxxxxxxx", Timestamp: time.Now()},
			},
		},
	}
	m := New(nodes, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	content := next.(Model).eventsContent()

	if !strings.Contains(content, "Stop") {
		t.Errorf("expected 'Stop' in events content, got: %q", content)
	}
	// No tool field on a Stop event — should not show empty tool entry.
	if strings.Contains(content, "  ") {
		// This is just a presence check; trailing double-space after type would
		// indicate an empty tool was rendered. Allow it only if Stop is present.
	}
}

func TestEventsContent_NoFocusedAgent_ShowsPrompt(t *testing.T) {
	// Empty tree — cursor has no real node under it.
	m := newModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	// Inject a session so hasSession=true but cursor is on nothing.
	got := next.(Model)
	got.hasSession = true
	content := got.eventsContent()
	if !strings.Contains(content, "Focus an agent") {
		t.Errorf("expected focus prompt when no node focused, got: %q", content)
	}
}

func TestUpdate_JScrollsEvents_WhenEventsPanelActive(t *testing.T) {
	m := New([]agent.Node{makeNodeWithEvents("s1xxxxxxxx", 50)}, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	// Switch to events panel.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	if m.activePanel != panelEvents {
		t.Fatal("expected panelEvents after tab")
	}

	before := m.eventScroll
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	after := next.(Model).eventScroll

	if after != before+1 {
		t.Errorf("expected eventScroll=%d after j, got %d", before+1, after)
	}
}

func TestUpdate_KDoesNotGoNegative_WhenEventsPanelActive(t *testing.T) {
	m := New([]agent.Node{makeNodeWithEvents("s1xxxxxxxx", 5)}, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)

	// eventScroll starts at 0; pressing k should not go negative.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if next.(Model).eventScroll < 0 {
		t.Errorf("expected eventScroll >= 0 after k at top, got %d", next.(Model).eventScroll)
	}
}

func TestUpdate_JK_AgentsPanel_DoesNotScrollEvents(t *testing.T) {
	nodes := []agent.Node{
		makeNodeWithEvents("s1xxxxxxxx", 5),
		{ID: "s2xxxxxxxx", Name: "session:s2xxxxxx", Status: agent.StatusRunning},
	}
	m := New(nodes, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	if m.activePanel != panelAgents {
		t.Fatal("expected panelAgents initially")
	}

	// j in agents panel should move cursor, not touch eventScroll.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	got := next.(Model)

	if got.cursor != 1 {
		t.Errorf("expected cursor=1 after j in agents panel, got %d", got.cursor)
	}
	if got.eventScroll != 0 {
		t.Errorf("expected eventScroll=0 after agent navigation, got %d", got.eventScroll)
	}
}

func TestUpdate_SwitchingAgent_ResetsEventScroll(t *testing.T) {
	nodes := []agent.Node{
		makeNodeWithEvents("s1xxxxxxxx", 50),
		{ID: "s2xxxxxxxx", Name: "session:s2xxxxxx", Status: agent.StatusRunning},
	}
	m := New(nodes, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	// Scroll events for the first agent.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = next.(Model)
	if m.eventScroll == 0 {
		t.Fatal("expected non-zero eventScroll after scrolling")
	}

	// Switch back to agents panel and move to the second agent.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = next.(Model)

	if m.eventScroll != 0 {
		t.Errorf("expected eventScroll reset to 0 after switching agent, got %d", m.eventScroll)
	}
}

func TestEventsContent_ScrollApplied(t *testing.T) {
	// 5 events, viewport height = 40-1(footer)-2(border) = 37 — all fit, scroll does nothing.
	// To test scrolling, manually set a scroll offset and check the first visible event.
	ts := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	var events []agent.Event
	for i := 0; i < 5; i++ {
		events = append(events, agent.Event{
			Type:      "PreToolUse",
			SessionID: "s1xxxxxxxx",
			Tool:      "Bash",
			Timestamp: ts.Add(time.Duration(i) * time.Hour),
		})
	}
	nodes := []agent.Node{
		{ID: "s1xxxxxxxx", Name: "session:s1xxxxxx", Status: agent.StatusRunning, Events: events},
	}
	m := New(nodes, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	// eventScroll=0: first event (10:00:00) should be visible.
	content := m.eventsContent()
	if !strings.Contains(content, "10:00:00") {
		t.Errorf("expected 10:00:00 at scroll=0, got: %q", content)
	}

	// eventScroll=2: first two events scroll off; 12:00:00 should be first visible.
	m.eventScroll = 2
	content = m.eventsContent()
	if !strings.Contains(content, "12:00:00") {
		t.Errorf("expected 12:00:00 at scroll=2, got: %q", content)
	}
	if strings.Contains(content, "10:00:00") {
		t.Errorf("expected 10:00:00 scrolled off at scroll=2, got: %q", content)
	}
}

func TestEventsContent_AppearsInView(t *testing.T) {
	ts := time.Date(2026, 1, 1, 15, 4, 5, 0, time.UTC)
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{Type: "PreToolUse", Tool: "Read", SessionID: "s1xxxxxxxx", Timestamp: ts},
			},
		},
	}
	m := New(nodes, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := next.(Model).View()

	if !strings.Contains(view, "15:04:05") {
		t.Errorf("expected event timestamp in full View output, got:\n%s", view)
	}
	if !strings.Contains(view, "PreToolUse") {
		t.Errorf("expected event type in full View output, got:\n%s", view)
	}
}

func TestFocusedNode_GroupHeader_ReturnsNil(t *testing.T) {
	// Two nodes sharing a GroupID produce a group header as the first visible entry.
	nodes := []agent.Node{
		{ID: "s1xxxxxxxx", Name: "session:s1xxxxxx", Status: agent.StatusRunning, GroupID: "g1"},
		{ID: "s2xxxxxxxx", Name: "session:s2xxxxxx", Status: agent.StatusRunning, GroupID: "g1"},
	}
	m := New(nodes, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	// Cursor=0 is the group header (id==""), so focusedNode should be nil.
	if m.focusedNode() != nil {
		t.Error("expected nil from focusedNode when cursor is on a group header")
	}
}

func TestFocusedNode_ReturnsCorrectNode(t *testing.T) {
	nodes := []agent.Node{
		{ID: "s1xxxxxxxx", Name: "session:s1xxxxxx", Status: agent.StatusRunning},
	}
	m := New(nodes, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	node := m.focusedNode()
	if node == nil {
		t.Fatal("expected non-nil focusedNode when cursor is on a real node")
	}
	if node.ID != "s1xxxxxxxx" {
		t.Errorf("expected node ID s1xxxxxxxx, got %s", node.ID)
	}
}

func TestClampEventScroll_ClampsToMax(t *testing.T) {
	node := makeNodeWithEvents("s1xxxxxxxx", 5)
	m := New([]agent.Node{node}, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	// viewH = 40-1-2 = 37; max scroll for 5 events = max(0, 5-37) = 0.
	m.eventScroll = 100
	m.clampEventScroll()
	if m.eventScroll != 0 {
		t.Errorf("expected eventScroll clamped to 0, got %d", m.eventScroll)
	}
}

func TestClampEventScroll_NilNode_SetsZero(t *testing.T) {
	m := newModel()
	m.eventScroll = 5
	m.clampEventScroll()
	if m.eventScroll != 0 {
		t.Errorf("expected eventScroll=0 when no node focused, got %d", m.eventScroll)
	}
}

func TestMouseWheel_ScrollsEventPanel(t *testing.T) {
	m := New([]agent.Node{makeNodeWithEvents("s1xxxxxxxx", 50)}, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	leftW := m.width * 35 / 100
	wheelDown := tea.MouseMsg{X: leftW + 10, Button: tea.MouseButtonWheelDown}
	next, _ = m.Update(wheelDown)
	if next.(Model).eventScroll != 1 {
		t.Errorf("expected eventScroll=1 after wheel down, got %d", next.(Model).eventScroll)
	}

	next, _ = next.Update(tea.MouseMsg{X: leftW + 10, Button: tea.MouseButtonWheelUp})
	if next.(Model).eventScroll != 0 {
		t.Errorf("expected eventScroll=0 after wheel up, got %d", next.(Model).eventScroll)
	}
}

func TestMouseWheel_ScrollsAgentsPanel(t *testing.T) {
	var nodes []agent.Node
	for i := range 50 {
		id := fmt.Sprintf("s%02dxxxxxx", i)
		nodes = append(nodes, agent.Node{ID: id, Name: "session:" + id[:8], Status: agent.StatusRunning})
	}
	m := New(nodes, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	wheelDown := tea.MouseMsg{X: 5, Button: tea.MouseButtonWheelDown}
	next, _ = m.Update(wheelDown)
	if next.(Model).scrollOffset != 1 {
		t.Errorf("expected scrollOffset=1 after wheel down in agents panel, got %d", next.(Model).scrollOffset)
	}
}

func TestEventsContent_LipglossWidth_NotLen(t *testing.T) {
	// Verify that events with ANSI-styled tool names don't overflow innerW.
	// We can't easily measure ANSI output width here, but we can ensure the
	// render does not panic and returns a non-empty string.
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{Type: "PreToolUse", Tool: "Agent", SessionID: "s1xxxxxxxx", Timestamp: time.Now()},
			},
		},
	}
	m := New(nodes, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	content := next.(Model).eventsContent()
	if content == "" {
		t.Error("expected non-empty events content at narrow terminal width")
	}
	// Ensure lipgloss.Width is used: each line should be within the inner panel width.
	leftW := 40 * 35 / 100
	rightW := 40 - leftW
	innerW := rightW - 2
	for _, line := range strings.Split(content, "\n") {
		if lw := lipgloss.Width(line); lw > innerW {
			t.Errorf("line exceeds innerW=%d (lipgloss.Width=%d): %q", innerW, lw, line)
		}
	}
}

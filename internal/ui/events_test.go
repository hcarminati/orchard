package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hcarminati/orchard/internal/agent"
)

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

	if !strings.Contains(content, "Jan 01 15:04:05") {
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
}

func TestEventsContent_NoFocusedAgent_ShowsPrompt(t *testing.T) {
	m := newModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	got := next.(Model)
	got.hasSession = true
	content := got.eventsContent()
	if !strings.Contains(content, "Focus an agent") {
		t.Errorf("expected focus prompt when no node focused, got: %q", content)
	}
}

func TestUpdate_JMovesCursor_WhenEventsPanelActive(t *testing.T) {
	m := New([]agent.Node{makeNodeWithEvents("s1xxxxxxxx", 50)}, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	if m.activePanel != panelEvents {
		t.Fatal("expected panelEvents after tab")
	}

	// Move cursor up one so there is room to move down.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = next.(Model)

	before := m.eventCursor
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	after := next.(Model).eventCursor

	if after != before+1 {
		t.Errorf("expected eventCursor=%d after j, got %d", before+1, after)
	}
}

func TestUpdate_KDoesNotGoNegative_WhenEventsPanelActive(t *testing.T) {
	m := New([]agent.Node{makeNodeWithEvents("s1xxxxxxxx", 5)}, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)

	// Press k many times to reach the top.
	for i := 0; i < 10; i++ {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
		m = next.(Model)
	}
	if m.eventCursor < 0 {
		t.Errorf("expected eventCursor >= 0 after repeated k, got %d", m.eventCursor)
	}
	if m.eventCursor != 0 {
		t.Errorf("expected eventCursor=0 at top, got %d", m.eventCursor)
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

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = next.(Model)
	if m.eventScroll == 0 {
		t.Fatal("expected non-zero eventScroll after scrolling")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = next.(Model)

	if m.eventScroll != 0 {
		t.Errorf("expected eventScroll reset to 0 after switching agent, got %d", m.eventScroll)
	}
}

func TestEventsContent_ScrollApplied(t *testing.T) {
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
	// height=6 → viewH=3; with 5 events maxStart=2, so scrolling is meaningful.
	m := New(nodes, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 6})
	m = next.(Model)

	m.eventScroll = 0
	content := m.eventsContent()
	if !strings.Contains(content, "Jan 01 10:00:00") {
		t.Errorf("expected Jan 01 10:00:00 at scroll=0, got: %q", content)
	}

	m.eventScroll = 2
	content = m.eventsContent()
	if !strings.Contains(content, "Jan 01 12:00:00") {
		t.Errorf("expected Jan 01 12:00:00 at scroll=2, got: %q", content)
	}
	if strings.Contains(content, "Jan 01 10:00:00") {
		t.Errorf("expected Jan 01 10:00:00 scrolled off at scroll=2, got: %q", content)
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

	if !strings.Contains(view, "Jan 01 15:04:05") {
		t.Errorf("expected event timestamp in full View output, got:\n%s", view)
	}
	if !strings.Contains(view, "PreToolUse") {
		t.Errorf("expected event type in full View output, got:\n%s", view)
	}
}

func TestFocusedNode_GroupHeader_ReturnsNil(t *testing.T) {
	nodes := []agent.Node{
		{ID: "s1xxxxxxxx", Name: "session:s1xxxxxx", Status: agent.StatusRunning, GroupID: "g1"},
		{ID: "s2xxxxxxxx", Name: "session:s2xxxxxx", Status: agent.StatusRunning, GroupID: "g1"},
	}
	m := New(nodes, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

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

func TestEventsContent_LipglossWidth_NotLen(t *testing.T) {
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
	leftW := 40 * 35 / 100
	rightW := 40 - leftW
	innerW := rightW - 2
	for _, line := range strings.Split(content, "\n") {
		if lw := lipgloss.Width(line); lw > innerW {
			t.Errorf("line exceeds innerW=%d (lipgloss.Width=%d): %q", innerW, lw, line)
		}
	}
}

func TestEventsContent_CollapsedToolEvent_ShowsTruncatedInput(t *testing.T) {
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{
					Type:      "PreToolUse",
					Tool:      "Bash",
					Input:     `{"cmd":"ls -la"}`,
					SessionID: "s1xxxxxxxx",
					Timestamp: time.Now(),
				},
			},
		},
	}
	m := New(nodes, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	content := next.(Model).eventsContent()

	if !strings.Contains(content, "PreToolUse") {
		t.Errorf("expected event type in collapsed view, got: %q", content)
	}
	if !strings.Contains(content, "Bash") {
		t.Errorf("expected tool name in collapsed view, got: %q", content)
	}
	if !strings.Contains(content, "ls -la") {
		t.Errorf("expected truncated input in collapsed view, got: %q", content)
	}
}

func TestEventsContent_ExpandedToolEvent_ShowsFullInputAndOutput(t *testing.T) {
	input := `{"cmd":"ls -la /home/user"}`
	output := "total 24\ndrwxr-xr-x 3 user user 4096 Jan 1 15:04 ."
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{
					Type:      "PostToolUse",
					Tool:      "Bash",
					Input:     input,
					Response:  output,
					SessionID: "s1xxxxxxxx",
					Timestamp: time.Now(),
				},
			},
		},
	}
	m := New(nodes, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	// Switch to events panel and press enter to expand.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)

	content := m.eventsContent()
	if !strings.Contains(content, "Input:") {
		t.Errorf("expected 'Input:' label in expanded view, got: %q", content)
	}
	if !strings.Contains(content, "ls -la /home/user") {
		t.Errorf("expected full input in expanded view, got: %q", content)
	}
	if !strings.Contains(content, "Output:") {
		t.Errorf("expected 'Output:' label in expanded view, got: %q", content)
	}
	if !strings.Contains(content, "total 24") {
		t.Errorf("expected output text in expanded view, got: %q", content)
	}
}

func TestEventsContent_EnterTogglesExpand(t *testing.T) {
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{
					Type:      "PreToolUse",
					Tool:      "Read",
					Input:     `{"file_path":"/tmp/foo"}`,
					SessionID: "s1xxxxxxxx",
					Timestamp: time.Now(),
				},
			},
		},
	}
	m := New(nodes, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	// Switch to events panel.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)

	// First enter: expand.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	content := m.eventsContent()
	if !strings.Contains(content, "Input:") {
		t.Errorf("expected expanded after first enter, got: %q", content)
	}

	// Second enter: collapse.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	content = m.eventsContent()
	if strings.Contains(content, "Input:") {
		t.Errorf("expected collapsed after second enter, got: %q", content)
	}
}

func TestEventsContent_NonToolEventNotExpandable(t *testing.T) {
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{Type: "Stop", SessionID: "s1xxxxxxxx", Timestamp: time.Now()},
				{Type: "Notification", Message: "hello", SessionID: "s1xxxxxxxx", Timestamp: time.Now()},
				{Type: "SubagentStop", SessionID: "s1xxxxxxxx", Timestamp: time.Now()},
			},
		},
	}
	m := New(nodes, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	// Switch to events panel and attempt to expand each non-tool event.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)

	for range 3 {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m = next.(Model)
		content := m.eventsContent()
		if strings.Contains(content, "Input:") {
			t.Errorf("expected non-tool event to be unexpandable, got: %q", content)
		}
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
		m = next.(Model)
	}
}

func TestEventsContent_ExpandedLongInput_LineWrapped(t *testing.T) {
	// Input longer than the panel width so it must wrap.
	longInput := `{"path":"` + strings.Repeat("a", 200) + `"}`
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{
					Type:      "PreToolUse",
					Tool:      "Read",
					Input:     longInput,
					SessionID: "s1xxxxxxxx",
					Timestamp: time.Now(),
				},
			},
		},
	}
	m := New(nodes, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	m = next.(Model)

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)

	leftW := 80 * 35 / 100
	rightW := 80 - leftW
	innerW := rightW - 2

	content := m.eventsContent()
	for _, line := range strings.Split(content, "\n") {
		if lw := lipgloss.Width(line); lw > innerW {
			t.Errorf("expanded line exceeds innerW=%d (lipgloss.Width=%d): %q", innerW, lw, line)
		}
	}
	// Content must include the long input split across multiple lines (not truncated).
	if !strings.Contains(content, strings.Repeat("a", 10)) {
		t.Errorf("expected long input content in expanded view, got: %q", content)
	}
}

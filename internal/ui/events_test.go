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

func TestUpdate_JScrollsEvents_WhenEventsPanelActive(t *testing.T) {
	m := New([]agent.Node{makeNodeWithEvents("s1xxxxxxxx", 50)}, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	if m.activePanel != panelEvents {
		t.Fatal("expected panelEvents after tab")
	}

	// Scroll up one so there is room to scroll down.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = next.(Model)

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

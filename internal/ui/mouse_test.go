package ui

import (
	"fmt"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hcarminati/orchard/internal/agent"
)

// contentTop is the Y offset where agent tree content rows begin (after the top border).
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
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

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
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = next.(Model)
	if m.cursor != 1 {
		t.Fatalf("expected cursor 1 after j, got %d", m.cursor)
	}

	next, _ = m.Update(mouseClick(5, contentTop+1))
	if next.(Model).cursor != 1 {
		t.Errorf("expected cursor to remain 1 after clicking already-selected leaf, got %d", next.(Model).cursor)
	}
}

func TestUpdate_MouseClick_TogglesCollapseOnNodeWithChildren(t *testing.T) {
	parent := agent.Node{ID: "p", Name: "parent", Status: agent.StatusRunning}
	child := agent.Node{ID: "c", Name: "child", ParentID: "p", Status: agent.StatusRunning}
	m := newWithClock([]agent.Node{parent, child}, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	if len(m.visibleNodes()) != 2 {
		t.Fatalf("expected 2 visible nodes initially, got %d", len(m.visibleNodes()))
	}

	next, _ = m.Update(mouseClick(5, contentTop+0))
	m = next.(Model)
	if len(m.visibleNodes()) != 1 {
		t.Errorf("expected 1 visible node after collapsing parent by click, got %d", len(m.visibleNodes()))
	}

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
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	if len(m.visibleNodes()) != 3 {
		t.Fatalf("expected 3 visible nodes initially, got %d", len(m.visibleNodes()))
	}

	next, _ = m.Update(mouseClick(5, contentTop+0))
	m = next.(Model)
	if len(m.visibleNodes()) != 1 {
		t.Errorf("expected 1 visible node after collapsing group by click, got %d", len(m.visibleNodes()))
	}

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
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

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
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	next, _ = m.Update(mouseClick(5, contentTop+2))
	m = next.(Model)
	if m.cursor != 2 {
		t.Fatalf("expected cursor 2 after mouse click, got %d", m.cursor)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if next.(Model).cursor != 1 {
		t.Errorf("expected cursor 1 after k following mouse click, got %d", next.(Model).cursor)
	}
}

// --- Scroll wheel ---

func TestMouseWheel_ScrollsEventPanel(t *testing.T) {
	m := newWithClock([]agent.Node{makeNodeWithEvents("s1xxxxxxxx", 50)}, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	// Default position is at the bottom.
	atBottom := m.eventScroll
	leftW := m.width * 35 / 100

	// Wheel up scrolls toward older events.
	next, _ = m.Update(tea.MouseMsg{X: leftW + 10, Button: tea.MouseButtonWheelUp})
	if next.(Model).eventScroll != atBottom-1 {
		t.Errorf("expected eventScroll=%d after wheel up, got %d", atBottom-1, next.(Model).eventScroll)
	}

	// Wheel down scrolls back toward newer events.
	next, _ = next.Update(tea.MouseMsg{X: leftW + 10, Button: tea.MouseButtonWheelDown})
	if next.(Model).eventScroll != atBottom {
		t.Errorf("expected eventScroll=%d after wheel down, got %d", atBottom, next.(Model).eventScroll)
	}
}

func TestMouseWheel_ScrollsAgentsPanel(t *testing.T) {
	var nodes []agent.Node
	for i := range 50 {
		id := fmt.Sprintf("s%02dxxxxxx", i)
		nodes = append(nodes, agent.Node{ID: id, Name: "session:" + id[:8], Status: agent.StatusRunning})
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	next, _ = m.Update(tea.MouseMsg{X: 5, Button: tea.MouseButtonWheelDown})
	if next.(Model).scrollOffset != 1 {
		t.Errorf("expected scrollOffset=1 after wheel down in agents panel, got %d", next.(Model).scrollOffset)
	}
}

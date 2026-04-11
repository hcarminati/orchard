package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hcarminati/orchard/internal/agent"
)

// --- j/k navigation ---

func TestUpdate_JKNavigation(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Name: "agent-a", Status: agent.StatusRunning},
		{ID: "b", Name: "agent-b", Status: agent.StatusRunning},
		{ID: "c", Name: "agent-c", Status: agent.StatusRunning},
	}
	m := New(nodes, nil, nil)
	if m.cursor != 0 {
		t.Errorf("expected initial cursor 0, got %d", m.cursor)
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if next.(Model).cursor != 1 {
		t.Errorf("expected cursor 1 after j, got %d", next.(Model).cursor)
	}

	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if next.(Model).cursor != 2 {
		t.Errorf("expected cursor 2 after second j, got %d", next.(Model).cursor)
	}

	// j at last item: cursor should not exceed bounds.
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if next.(Model).cursor != 2 {
		t.Errorf("expected cursor to stay at 2 at boundary, got %d", next.(Model).cursor)
	}

	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if next.(Model).cursor != 1 {
		t.Errorf("expected cursor 1 after k, got %d", next.(Model).cursor)
	}
}

func TestUpdate_KAtTopBoundary(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Name: "agent-a", Status: agent.StatusRunning},
	}
	m := New(nodes, nil, nil)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if next.(Model).cursor != 0 {
		t.Errorf("expected cursor to stay at 0 at top boundary, got %d", next.(Model).cursor)
	}
}

func TestUpdate_EnterOnUngroupedNode_NoEffect(t *testing.T) {
	nodes := []agent.Node{
		{ID: "x", Name: "solo", Status: agent.StatusRunning},
	}
	m := New(nodes, nil, nil)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := next.(Model)
	if got.agents.Nodes["x"].Winner {
		t.Error("expected Winner to remain false for ungrouped node")
	}
}

// --- visibleNodes ---

func TestVisibleNodes_FlatTree(t *testing.T) {
	nodes := []agent.Node{
		{ID: "x", Name: "x", Status: agent.StatusRunning},
		{ID: "y", Name: "y", Status: agent.StatusRunning},
	}
	m := New(nodes, nil, nil)
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
	if len(vn) != 2 {
		t.Fatalf("expected 2 visible nodes after collapsing A, got %d: %+v", len(vn), vn)
	}
	if vn[0].id != "A" || vn[1].id != "D" {
		t.Errorf("expected [A, D], got [%s, %s]", vn[0].id, vn[1].id)
	}
}

// --- Collapse / expand ---

func TestUpdate_JK_NavigatesNestedTree(t *testing.T) {
	m := makeTree()
	var next tea.Model = m

	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if next.(Model).cursor != 1 {
		t.Errorf("expected cursor 1 after j, got %d", next.(Model).cursor)
	}
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if next.(Model).cursor != 2 {
		t.Errorf("expected cursor 2 after j, got %d", next.(Model).cursor)
	}
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if next.(Model).cursor != 3 {
		t.Errorf("expected cursor 3 after j, got %d", next.(Model).cursor)
	}
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if next.(Model).cursor != 3 {
		t.Errorf("expected cursor to stay at 3 at boundary, got %d", next.(Model).cursor)
	}
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if next.(Model).cursor != 2 {
		t.Errorf("expected cursor 2 after k, got %d", next.(Model).cursor)
	}
}

func TestUpdate_JK_RespectsCollapsedState(t *testing.T) {
	m := makeTree()
	m.collapsed["A"] = true
	var next tea.Model = m
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if next.(Model).cursor != 1 {
		t.Errorf("expected cursor 1 (D) after j with A collapsed, got %d", next.(Model).cursor)
	}
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if next.(Model).cursor != 1 {
		t.Errorf("expected cursor to stay at 1 at boundary, got %d", next.(Model).cursor)
	}
}

func TestUpdate_Space_TogglesCollapse(t *testing.T) {
	m := makeTree()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	got := next.(Model)
	if !got.collapsed["A"] {
		t.Error("expected A to be collapsed after space")
	}
	if len(got.visibleNodes()) != 2 {
		t.Errorf("expected 2 visible nodes after collapse, got %d", len(got.visibleNodes()))
	}

	next, _ = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	got = next.(Model)
	if got.collapsed["A"] {
		t.Error("expected A to be expanded after second space")
	}
	if len(got.visibleNodes()) != 4 {
		t.Errorf("expected 4 visible nodes after expand, got %d", len(got.visibleNodes()))
	}
}

func TestUpdate_Space_NoEffectOnLeaf(t *testing.T) {
	m := makeTree()
	var next tea.Model = m
	for i := 0; i < 3; i++ {
		next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	}
	got := next.(Model)
	if got.cursor != 3 {
		t.Fatalf("expected cursor 3 (D), got %d", got.cursor)
	}
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
	m := makeTree()
	var next tea.Model = m
	for i := 0; i < 3; i++ {
		next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	}
	for i := 0; i < 3; i++ {
		next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	}
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	got := next.(Model)

	vn := got.visibleNodes()
	if got.cursor >= len(vn) {
		t.Errorf("cursor %d >= visible len %d — cursor correction failed", got.cursor, len(vn))
	}
}

func TestUpdate_CursorCorrection_ClampedWhenOutOfBounds(t *testing.T) {
	m := makeTree()
	m.cursor = 99
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	got := next.(Model)
	vn := got.visibleNodes()
	if got.cursor >= len(vn) {
		t.Errorf("cursor %d still out of bounds (visible: %d) after space", got.cursor, len(vn))
	}
}

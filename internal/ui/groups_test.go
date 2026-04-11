package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hcarminati/orchard/internal/agent"
)

func TestView_GroupLabel_ShowsCorrectCount(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Name: "agent-a", GroupID: "g1", Status: agent.StatusRunning},
		{ID: "b", Name: "agent-b", GroupID: "g1", Status: agent.StatusRunning},
		{ID: "c", Name: "agent-c", GroupID: "g1", Status: agent.StatusRunning},
	}
	m := New(nodes, nil, nil)
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
	m := New(nodes, nil, nil)
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
	m := New(nodes, nil, nil)
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
	m := New(nodes, nil, nil)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := next.(Model)

	if !got.agents.Nodes["a"].Winner {
		t.Error("expected node 'a' to be marked as winner")
	}
	if got.agents.Nodes["b"].Winner {
		t.Error("expected node 'b' to not be winner after 'a' is chosen")
	}

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
	m := New(nodes, nil, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := next.(Model).View()

	if strings.Contains(view, "parallel") {
		t.Errorf("expected no 'parallel' label for ungrouped node, got:\n%s", view)
	}
	if !strings.Contains(view, "solo-agent") {
		t.Errorf("expected node name 'solo-agent' in view, got:\n%s", view)
	}
}

func TestVisibleNodes_GroupHeaderInVisibleNodes(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Name: "agent-a", GroupID: "g1", Status: agent.StatusRunning},
		{ID: "b", Name: "agent-b", GroupID: "g1", Status: agent.StatusRunning},
	}
	m := New(nodes, nil, nil)
	vn := m.visibleNodes()
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
	m := New(nodes, nil, nil)
	m.collapsedGroups["g1"] = true
	vn := m.visibleNodes()
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
	m := New(nodes, nil, nil)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	got := next.(Model)
	if !got.collapsedGroups["g1"] {
		t.Error("expected group g1 to be collapsed after space on header")
	}
	if len(got.visibleNodes()) != 1 {
		t.Errorf("expected 1 visible node (header only), got %d", len(got.visibleNodes()))
	}

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
	m := New(nodes, nil, nil)
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

func TestFooter_SpaceHint_ShownForGroupHeader(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Name: "agent-a", GroupID: "g1", Status: agent.StatusRunning},
		{ID: "b", Name: "agent-b", GroupID: "g1", Status: agent.StatusRunning},
	}
	m := New(nodes, nil, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := next.(Model).View()
	if !strings.Contains(view, "expand/collapse") {
		t.Errorf("expected 'expand/collapse' hint when cursor is on group header, got:\n%s", view)
	}
}

func TestFooter_MarkWinnerHint_OnlyWhenCursorInGroup(t *testing.T) {
	grouped := []agent.Node{
		{ID: "a", Name: "agent-a", GroupID: "g1", Status: agent.StatusRunning},
		{ID: "b", Name: "agent-b", GroupID: "g1", Status: agent.StatusRunning},
		{ID: "c", Name: "solo", Status: agent.StatusRunning},
	}
	m := New(grouped, nil, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	// cursor=0 is the group header — no "mark winner" hint.
	view := next.(Model).View()
	if strings.Contains(view, "mark winner") {
		t.Errorf("expected no 'mark winner' hint on group header, got:\n%s", view)
	}

	// Navigate to "a" (grouped member) → hint shown.
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	view = next.(Model).View()
	if !strings.Contains(view, "mark winner") {
		t.Errorf("expected 'mark winner' hint on grouped node, got:\n%s", view)
	}

	// Navigate to "c" (ungrouped) → no hint.
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	view = next.(Model).View()
	if strings.Contains(view, "mark winner") {
		t.Errorf("expected no 'mark winner' hint on ungrouped node, got:\n%s", view)
	}
}

package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hcarminati/orchard/internal/agent"
)

func TestUpdate_FilterCyclesMode(t *testing.T) {
	m := newWithClock(nil, nil, nil, time.Time{})
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
	if got.statusFilter != filterHidden {
		t.Errorf("after third f: expected filterHidden, got %d", got.statusFilter)
	}

	next, _ = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	got = next.(Model)
	if got.statusFilter != filterAll {
		t.Errorf("after fourth f: expected filterAll (wrap), got %d", got.statusFilter)
	}
}

func TestUpdate_FilterResetsCursorToZero(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Name: "agent-a", Status: agent.StatusRunning},
		{ID: "b", Name: "agent-b", Status: agent.StatusRunning},
		{ID: "c", Name: "agent-c", Status: agent.StatusRunning},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if next.(Model).cursor != 2 {
		t.Fatalf("expected cursor 2 before filter change, got %d", next.(Model).cursor)
	}

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
	m := newWithClock(nodes, nil, nil, time.Time{})
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
	m := newWithClock(nodes, nil, nil, time.Time{})
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
	m := newWithClock(nodes, nil, nil, time.Time{})
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
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
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
	m := newWithClock(nil, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	view := next.(Model).View()
	if !strings.Contains(view, "filter:all") {
		t.Errorf("expected 'filter:all' in footer, got:\n%s", view)
	}

	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	view = next.(Model).View()
	if !strings.Contains(view, "filter:running") {
		t.Errorf("expected 'filter:running' in footer after first f, got:\n%s", view)
	}

	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	view = next.(Model).View()
	if !strings.Contains(view, "filter:errored") {
		t.Errorf("expected 'filter:errored' in footer after second f, got:\n%s", view)
	}

	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	view = next.(Model).View()
	if !strings.Contains(view, "filter:hidden") {
		t.Errorf("expected 'filter:hidden' in footer after third f, got:\n%s", view)
	}
}

func TestHide_ConfirmPromptAppearsOnD(t *testing.T) {
	nodes := []agent.Node{
		{ID: "sess-abc123", Name: "session:sessabc1", Status: agent.StatusDone},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = next.(Model)

	if m.confirmHide == "" {
		t.Fatal("expected confirmHide to be set after pressing d on a top-level done session")
	}
	view := m.View()
	if !strings.Contains(view, "Hide session:") {
		t.Errorf("expected confirmation prompt in footer, got:\n%s", view)
	}
}

func TestHide_YConfirmsHide(t *testing.T) {
	nodes := []agent.Node{
		{ID: "sess-abc123", Name: "session:sessabc1", Status: agent.StatusDone},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = next.(Model)

	if !m.hiddenSessions["sess-abc123"] {
		t.Error("expected session to be in hiddenSessions after y confirmation")
	}
	if m.confirmHide != "" {
		t.Error("expected confirmHide to be cleared after confirmation")
	}
	vn := m.visibleNodes()
	for _, v := range vn {
		if v.id == "sess-abc123" {
			t.Error("expected hidden session to be absent from visibleNodes")
		}
	}
}

func TestHide_NonYCancels(t *testing.T) {
	nodes := []agent.Node{
		{ID: "sess-abc123", Name: "session:sessabc1", Status: agent.StatusDone},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)

	if m.hiddenSessions["sess-abc123"] {
		t.Error("expected session to NOT be hidden after esc cancel")
	}
	if m.confirmHide != "" {
		t.Error("expected confirmHide to be cleared after cancel")
	}
}

func TestHide_RunningSessionShowsStatusMsg(t *testing.T) {
	nodes := []agent.Node{
		{ID: "sess-run", Name: "session:sess-run", Status: agent.StatusRunning},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = next.(Model)

	if m.confirmHide != "" {
		t.Error("expected no confirmation prompt for running session")
	}
	if m.hideStatusMsg == "" {
		t.Error("expected status message for cannot-hide-running case")
	}
	view := m.View()
	if !strings.Contains(view, "Cannot hide active session") {
		t.Errorf("expected status message in footer, got:\n%s", view)
	}
}

func TestHide_RestoreViaR(t *testing.T) {
	nodes := []agent.Node{
		{ID: "sess-abc123", Name: "session:sessabc1", Status: agent.StatusDone},
	}
	hidden := map[string]bool{"sess-abc123": true}
	m := newWithClock(nodes, nil, hidden, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	// Cycle to hidden filter view.
	for i := 0; i < 3; i++ {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
		m = next.(Model)
	}
	if m.statusFilter != filterHidden {
		t.Fatalf("expected filterHidden, got %d", m.statusFilter)
	}

	// The hidden session should be visible in this view.
	vn := m.visibleNodes()
	if len(vn) == 0 || vn[0].id != "sess-abc123" {
		t.Fatal("expected hidden session visible in filterHidden view")
	}

	// Press r to restore.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	m = next.(Model)

	if m.hiddenSessions["sess-abc123"] {
		t.Error("expected session removed from hiddenSessions after r")
	}
}

func TestHide_ChildNodeCannotBeHidden(t *testing.T) {
	nodes := []agent.Node{
		{ID: "parent", Name: "session:parent", Status: agent.StatusDone},
		{ID: "child", Name: "child-agent", ParentID: "parent", Status: agent.StatusDone},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	// Navigate to child node (j moves cursor down).
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = next.(Model)

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = next.(Model)

	if m.confirmHide != "" {
		t.Error("expected no confirmation prompt when pressing d on a child node")
	}
}

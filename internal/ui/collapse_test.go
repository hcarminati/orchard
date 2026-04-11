package ui

import (
	"testing"

	"github.com/hcarminati/orchard/internal/agent"
)

// newForStartup builds a Model via New (the real startup path), which applies
// collapse-by-default logic. expandedIDs mirrors what would be loaded from
// state.json.
func newForStartup(nodes []agent.Node, expandedIDs map[string]bool) Model {
	return New(nodes, nil, nil, expandedIDs)
}

func TestNew_CollapsesByDefault(t *testing.T) {
	nodes := []agent.Node{
		{ID: "s1", Name: "session:s1", Status: agent.StatusDone},
		{ID: "s2", Name: "session:s2", Status: agent.StatusIdle},
		{ID: "c1", Name: "child", ParentID: "s1", Status: agent.StatusDone},
	}
	m := newForStartup(nodes, nil)

	// Both root sessions should start collapsed.
	if !m.collapsed["s1"] {
		t.Error("expected s1 (Done) to be collapsed by default")
	}
	if !m.collapsed["s2"] {
		t.Error("expected s2 (Idle) to be collapsed by default")
	}
	// Children should not have their own collapsed entries (they're hidden by parent collapse).
	if m.collapsed["c1"] {
		t.Error("expected child c1 to not have an explicit collapsed entry")
	}
}

func TestNew_AutoExpandsRunningSession(t *testing.T) {
	nodes := []agent.Node{
		{ID: "run", Name: "session:run", Status: agent.StatusRunning},
		{ID: "done", Name: "session:done", Status: agent.StatusDone},
		{ID: "child", Name: "child", ParentID: "run", Status: agent.StatusRunning},
	}
	m := newForStartup(nodes, nil)

	// Running root should be auto-expanded.
	if m.collapsed["run"] {
		t.Error("expected running session to be auto-expanded")
	}
	// Done root should be collapsed.
	if !m.collapsed["done"] {
		t.Error("expected done session to be collapsed by default")
	}
}

func TestNew_AutoExpandsParentWithRunningChild(t *testing.T) {
	// A parent that is Idle but has a running child is effectively Running.
	nodes := []agent.Node{
		{ID: "parent", Name: "session:parent", Status: agent.StatusIdle},
		{ID: "child", Name: "child", ParentID: "parent", Status: agent.StatusRunning},
	}
	m := newForStartup(nodes, nil)

	if m.collapsed["parent"] {
		t.Error("expected parent (effectively Running via child) to be auto-expanded")
	}
}

func TestNew_RestoresExpandedFromState(t *testing.T) {
	nodes := []agent.Node{
		{ID: "s1", Name: "session:s1", Status: agent.StatusDone},
		{ID: "s2", Name: "session:s2", Status: agent.StatusDone},
	}
	expandedIDs := map[string]bool{"s1": true}
	m := newForStartup(nodes, expandedIDs)

	// s1 was expanded last time — keep it expanded.
	if m.collapsed["s1"] {
		t.Error("expected s1 to be expanded (restored from state)")
	}
	// s2 was not in the expanded list — keep it collapsed.
	if !m.collapsed["s2"] {
		t.Error("expected s2 to be collapsed (not in saved state)")
	}
}

func TestNew_RunningSessionExpanded_EvenIfNotInExpandedIDs(t *testing.T) {
	// Running sessions are auto-expanded regardless of saved state.
	nodes := []agent.Node{
		{ID: "run", Name: "session:run", Status: agent.StatusRunning},
	}
	m := newForStartup(nodes, map[string]bool{}) // empty expanded set
	if m.collapsed["run"] {
		t.Error("expected running session to be expanded even with empty expandedIDs")
	}
}

func TestNew_ChildrenHiddenWhenRootCollapsed(t *testing.T) {
	nodes := []agent.Node{
		{ID: "parent", Name: "session:parent", Status: agent.StatusDone},
		{ID: "child", Name: "child", ParentID: "parent", Status: agent.StatusDone},
	}
	m := newForStartup(nodes, nil)

	// parent is collapsed, so child should not appear in visible nodes.
	vn := m.visibleNodes()
	for _, n := range vn {
		if n.id == "child" {
			t.Error("child should not be visible when parent is collapsed at startup")
		}
	}
	// parent itself should still appear.
	found := false
	for _, n := range vn {
		if n.id == "parent" {
			found = true
		}
	}
	if !found {
		t.Error("parent root node should still be visible even when collapsed")
	}
}

func TestNew_NoNodes_NoCrash(t *testing.T) {
	m := newForStartup(nil, nil)
	if len(m.collapsed) != 0 {
		t.Errorf("expected empty collapsed map for empty tree, got %v", m.collapsed)
	}
}

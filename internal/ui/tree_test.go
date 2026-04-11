package ui

import (
	"testing"
	"time"

	"github.com/hcarminati/orchard/internal/agent"
)

// buildModel constructs a Model from the given nodes, suitable for exercising
// sortedRoots, effectiveStatus, and lastEventTime without a window size.
func buildModel(nodes []agent.Node) Model {
	return New(nodes, nil)
}

// event returns a synthetic agent.Event at the given time.
func event(ts time.Time) agent.Event {
	return agent.Event{Type: "PreToolUse", Tool: "Bash", Timestamp: ts}
}

var (
	t1 = time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	t2 = time.Date(2026, 1, 1, 11, 0, 0, 0, time.UTC)
	t3 = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
)

// --- sortedRoots ---

func TestSortedRoots_RunningBeforeIdleBeforeDone(t *testing.T) {
	nodes := []agent.Node{
		{ID: "done", Name: "done", Status: agent.StatusDone, Events: []agent.Event{event(t1)}},
		{ID: "idle", Name: "idle", Status: agent.StatusIdle, Events: []agent.Event{event(t2)}},
		{ID: "run", Name: "run", Status: agent.StatusRunning, Events: []agent.Event{event(t3)}},
	}
	m := buildModel(nodes)
	got := m.sortedRoots()
	want := []string{"run", "idle", "done"}
	if !sliceEqual(got, want) {
		t.Errorf("sortedRoots() = %v, want %v", got, want)
	}
}

func TestSortedRoots_ErrorAlwaysFirst(t *testing.T) {
	nodes := []agent.Node{
		{ID: "run", Name: "run", Status: agent.StatusRunning, Events: []agent.Event{event(t3)}},
		{ID: "err", Name: "err", Status: agent.StatusError, Events: []agent.Event{event(t2)}},
	}
	m := buildModel(nodes)
	got := m.sortedRoots()
	if got[0] != "err" {
		t.Errorf("expected 'err' first, got %v", got)
	}
}

func TestSortedRoots_WithinTierMostRecentFirst(t *testing.T) {
	nodes := []agent.Node{
		{ID: "old", Name: "old", Status: agent.StatusRunning, Events: []agent.Event{event(t1)}},
		{ID: "new", Name: "new", Status: agent.StatusRunning, Events: []agent.Event{event(t3)}},
		{ID: "mid", Name: "mid", Status: agent.StatusRunning, Events: []agent.Event{event(t2)}},
	}
	m := buildModel(nodes)
	got := m.sortedRoots()
	want := []string{"new", "mid", "old"}
	if !sliceEqual(got, want) {
		t.Errorf("sortedRoots() = %v, want %v", got, want)
	}
}

func TestSortedRoots_ParentWithRunningChild_SortsAsRunning(t *testing.T) {
	// Parent is Idle but child is Running — parent should sort into the Running tier.
	nodes := []agent.Node{
		{ID: "done", Name: "done", Status: agent.StatusDone, Events: []agent.Event{event(t3)}},
		{ID: "parent", Name: "parent", Status: agent.StatusIdle, Events: []agent.Event{event(t1)}},
		{ID: "child", Name: "child", ParentID: "parent", Status: agent.StatusRunning, Events: []agent.Event{event(t2)}},
	}
	m := buildModel(nodes)
	got := m.sortedRoots()
	// "parent" (effectively Running via child) should come before "done".
	// "child" is not a root, so Roots = ["done", "parent"].
	if len(got) != 2 {
		t.Fatalf("expected 2 roots, got %v", got)
	}
	if got[0] != "parent" {
		t.Errorf("expected 'parent' first (effective Running via child), got %v", got)
	}
}

func TestSortedRoots_NoEvents_SortsLast(t *testing.T) {
	// Nodes in the same tier with no events sort after those with events.
	nodes := []agent.Node{
		{ID: "no-events", Name: "no-events", Status: agent.StatusDone},
		{ID: "has-events", Name: "has-events", Status: agent.StatusDone, Events: []agent.Event{event(t1)}},
	}
	m := buildModel(nodes)
	got := m.sortedRoots()
	if got[0] != "has-events" {
		t.Errorf("expected 'has-events' first within Done tier, got %v", got)
	}
}

// --- effectiveStatus ---

func TestEffectiveStatus_RunningNode(t *testing.T) {
	m := buildModel([]agent.Node{{ID: "a", Status: agent.StatusRunning}})
	if got := m.effectiveStatus("a"); got != agent.StatusRunning {
		t.Errorf("expected Running, got %v", got)
	}
}

func TestEffectiveStatus_IdleWithRunningChild(t *testing.T) {
	nodes := []agent.Node{
		{ID: "parent", Status: agent.StatusIdle},
		{ID: "child", ParentID: "parent", Status: agent.StatusRunning},
	}
	m := buildModel(nodes)
	if got := m.effectiveStatus("parent"); got != agent.StatusRunning {
		t.Errorf("expected Running (promoted by child), got %v", got)
	}
}

func TestEffectiveStatus_ErrorNotOverriddenByRunningChild(t *testing.T) {
	nodes := []agent.Node{
		{ID: "parent", Status: agent.StatusError},
		{ID: "child", ParentID: "parent", Status: agent.StatusRunning},
	}
	m := buildModel(nodes)
	if got := m.effectiveStatus("parent"); got != agent.StatusError {
		t.Errorf("expected Error to be preserved, got %v", got)
	}
}

func TestEffectiveStatus_AllChildrenDone_ParentIdle(t *testing.T) {
	nodes := []agent.Node{
		{ID: "parent", Status: agent.StatusIdle},
		{ID: "child", ParentID: "parent", Status: agent.StatusDone},
	}
	m := buildModel(nodes)
	if got := m.effectiveStatus("parent"); got != agent.StatusIdle {
		t.Errorf("expected Idle (no running child), got %v", got)
	}
}

// --- lastEventTime ---

func TestLastEventTime_ReturnsLatestInSubtree(t *testing.T) {
	nodes := []agent.Node{
		{ID: "parent", Status: agent.StatusIdle, Events: []agent.Event{event(t1)}},
		{ID: "child", ParentID: "parent", Status: agent.StatusRunning, Events: []agent.Event{event(t3)}},
	}
	m := buildModel(nodes)
	got := m.lastEventTime("parent")
	if !got.Equal(t3) {
		t.Errorf("expected %v (child's timestamp), got %v", t3, got)
	}
}

func TestLastEventTime_NoEvents_ReturnsZero(t *testing.T) {
	m := buildModel([]agent.Node{{ID: "a", Status: agent.StatusRunning}})
	got := m.lastEventTime("a")
	if !got.IsZero() {
		t.Errorf("expected zero time, got %v", got)
	}
}

// sliceEqual compares two string slices for equality.
func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

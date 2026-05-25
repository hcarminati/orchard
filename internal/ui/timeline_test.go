package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hcarminati/orchard/internal/agent"
)

// baseTime is a fixed reference time used across timeline tests.
var baseTime = time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)

// --- nodeEndTime ---

func TestNodeEndTime_UsesLastEvent(t *testing.T) {
	n := agent.Node{
		ID:        "a",
		SpawnedAt: baseTime,
		Events: []agent.Event{
			{Timestamp: baseTime.Add(5 * time.Second)},
			{Timestamp: baseTime.Add(10 * time.Second)},
		},
	}
	nodes := []agent.Node{n}
	m := newWithClock(nodes, nil, nil, time.Time{})
	end := m.nodeEndTime("a")
	if end != baseTime.Add(10*time.Second) {
		t.Errorf("expected %v, got %v", baseTime.Add(10*time.Second), end)
	}
}

func TestNodeEndTime_FallsBackToSpawnedAt(t *testing.T) {
	n := agent.Node{
		ID:        "a",
		SpawnedAt: baseTime,
		Events:    []agent.Event{},
	}
	nodes := []agent.Node{n}
	m := newWithClock(nodes, nil, nil, time.Time{})
	end := m.nodeEndTime("a")
	if end != baseTime {
		t.Errorf("expected SpawnedAt %v, got %v", baseTime, end)
	}
}

func TestNodeEndTime_UnknownNode(t *testing.T) {
	m := newWithClock(nil, nil, nil, time.Time{})
	end := m.nodeEndTime("no-such-node")
	if !end.IsZero() {
		t.Errorf("expected zero time for unknown node, got %v", end)
	}
}

// --- subtreeEntries ---

func TestSubtreeEntries_SingleNode(t *testing.T) {
	n := agent.Node{
		ID:        "a",
		Name:      "root-a",
		SpawnedAt: baseTime,
		Status:    agent.StatusDone,
		Events:    []agent.Event{{Timestamp: baseTime.Add(5 * time.Second)}},
	}
	m := newWithClock([]agent.Node{n}, nil, nil, time.Time{})
	entries := m.subtreeEntries("a", 0)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].nodeID != "a" {
		t.Errorf("unexpected nodeID: %q", entries[0].nodeID)
	}
	if entries[0].depth != 0 {
		t.Errorf("expected depth 0, got %d", entries[0].depth)
	}
}

func TestSubtreeEntries_IncludesChildren(t *testing.T) {
	nodes := []agent.Node{
		{ID: "parent", Name: "parent", SpawnedAt: baseTime, Status: agent.StatusDone,
			Events: []agent.Event{{Timestamp: baseTime.Add(20 * time.Second)}}},
		{ID: "child", Name: "child", ParentID: "parent", SpawnedAt: baseTime.Add(5 * time.Second),
			Status: agent.StatusDone,
			Events: []agent.Event{{Timestamp: baseTime.Add(15 * time.Second)}}},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	entries := m.subtreeEntries("parent", 0)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (parent + child), got %d", len(entries))
	}
	if entries[0].nodeID != "parent" {
		t.Errorf("expected parent first, got %q", entries[0].nodeID)
	}
	if entries[1].nodeID != "child" {
		t.Errorf("expected child second, got %q", entries[1].nodeID)
	}
	if entries[1].depth != 1 {
		t.Errorf("expected child depth=1, got %d", entries[1].depth)
	}
}

// --- criticalPathNodes ---

func TestCriticalPathNodes_SingleNode(t *testing.T) {
	n := agent.Node{
		ID:        "a",
		SpawnedAt: baseTime,
		Events:    []agent.Event{{Timestamp: baseTime.Add(10 * time.Second)}},
	}
	m := newWithClock([]agent.Node{n}, nil, nil, time.Time{})
	cp := m.criticalPathNodes("a")
	if !cp["a"] {
		t.Errorf("expected single node to be on critical path")
	}
}

func TestCriticalPathNodes_LongestChain(t *testing.T) {
	// parent → child1 (5s), parent → child2 (15s)
	// Critical path = parent → child2
	nodes := []agent.Node{
		{ID: "parent", SpawnedAt: baseTime,
			Events: []agent.Event{{Timestamp: baseTime.Add(20 * time.Second)}}},
		{ID: "child1", ParentID: "parent", SpawnedAt: baseTime.Add(2 * time.Second),
			Events: []agent.Event{{Timestamp: baseTime.Add(7 * time.Second)}}},
		{ID: "child2", ParentID: "parent", SpawnedAt: baseTime.Add(2 * time.Second),
			Events: []agent.Event{{Timestamp: baseTime.Add(17 * time.Second)}}},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	cp := m.criticalPathNodes("parent")
	if !cp["parent"] {
		t.Errorf("expected parent on critical path")
	}
	if !cp["child2"] {
		t.Errorf("expected longer child (child2) on critical path")
	}
	if cp["child1"] {
		t.Errorf("expected shorter child (child1) NOT on critical path")
	}
}

// --- renderRuler ---

func TestRenderRuler_LengthMatchesBarW(t *testing.T) {
	barW := 40
	ruler := renderRuler(60*time.Second, barW)
	if len([]rune(ruler)) != barW {
		t.Errorf("expected ruler length %d, got %d", barW, len([]rune(ruler)))
	}
}

func TestRenderRuler_ContainsZero(t *testing.T) {
	ruler := renderRuler(60*time.Second, 40)
	if !strings.Contains(ruler, "0") {
		t.Errorf("expected ruler to contain '0' tick, got %q", ruler)
	}
}

// --- timelineContent ---

func TestTimelineContent_ShowsNodeNames(t *testing.T) {
	nodes := []agent.Node{
		{
			ID:        "a",
			Name:      "my-agent",
			SpawnedAt: baseTime,
			Status:    agent.StatusDone,
			Events:    []agent.Event{{Timestamp: baseTime.Add(10 * time.Second)}},
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m2 := next.(Model)
	m2.timelineMode = true
	content := m2.timelineContent()

	if !strings.Contains(content, "my-agent") {
		t.Errorf("expected node name in timeline content, got:\n%s", content)
	}
}

func TestTimelineContent_ShowsBarCharacters(t *testing.T) {
	nodes := []agent.Node{
		{
			ID:        "a",
			Name:      "agent-a",
			SpawnedAt: baseTime,
			Status:    agent.StatusDone,
			Events:    []agent.Event{{Timestamp: baseTime.Add(10 * time.Second)}},
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m2 := next.(Model)
	m2.timelineMode = true
	content := m2.timelineContent()

	// Bar characters (█ or ▓) must appear for a node with known time bounds.
	if !strings.ContainsAny(content, "█▓") {
		t.Errorf("expected bar characters in timeline content, got:\n%s", content)
	}
}

func TestTimelineContent_EmptyWhenNoSession(t *testing.T) {
	m := newWithClock(nil, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m2 := next.(Model)
	m2.timelineMode = true
	content := m2.timelineContent()

	if !strings.Contains(content, "Waiting") {
		t.Errorf("expected waiting message for empty session, got:\n%s", content)
	}
}

// --- T key toggles timeline mode ---

func TestKey_T_TogglesTimeline(t *testing.T) {
	nodes := []agent.Node{{ID: "a", Name: "agent-a", Status: agent.StatusRunning}}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m2 := next.(Model)

	if m2.timelineMode {
		t.Fatal("expected timelineMode=false initially")
	}

	next2, _ := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	m3 := next2.(Model)
	if !m3.timelineMode {
		t.Errorf("expected timelineMode=true after pressing T")
	}

	next3, _ := m3.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	m4 := next3.(Model)
	if m4.timelineMode {
		t.Errorf("expected timelineMode=false after pressing T again")
	}
}

func TestView_TimelineHeaderLabel(t *testing.T) {
	nodes := []agent.Node{{ID: "a", Name: "agent-a", Status: agent.StatusDone,
		SpawnedAt: baseTime, Events: []agent.Event{{Timestamp: baseTime.Add(5 * time.Second)}}}}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m2 := next.(Model)
	m2.timelineMode = true

	view := m2.View()
	if !strings.Contains(view, "Timeline") {
		t.Errorf("expected 'Timeline' label in header when timeline mode is active, got:\n%s", view)
	}
}

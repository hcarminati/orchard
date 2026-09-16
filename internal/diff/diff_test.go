package diff

import (
	"strings"
	"testing"
	"time"

	"github.com/hcarminati/orchard/internal/agent"
)

func makeNodes(events []agent.Event) []agent.Node {
	n := agent.NewNode("session-1")
	n.Events = events
	return []agent.Node{n}
}

func toolEvent(tool string, ts time.Time) agent.Event {
	return agent.Event{Type: "PreToolUse", Tool: tool, SessionID: "session-1", Timestamp: ts}
}

func skillEvent(skill string, ts time.Time) agent.Event {
	return agent.Event{Type: "SkillTrigger", Tool: skill, SessionID: "session-1", Timestamp: ts}
}

var t0 = time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

func TestCompare_EmptySessionsReturnZeroDiff(t *testing.T) {
	d := Compare(nil, nil)
	if d.NodeDelta != 0 {
		t.Errorf("NodeDelta: got %d, want 0", d.NodeDelta)
	}
	if d.CostDelta != 0 {
		t.Errorf("CostDelta: got %f, want 0", d.CostDelta)
	}
}

func TestCompare_NodeCountDelta(t *testing.T) {
	a := []agent.Node{agent.NewNode("s1"), agent.NewNode("s2")}
	b := []agent.Node{agent.NewNode("s1"), agent.NewNode("s2"), agent.NewNode("s3")}
	d := Compare(a, b)
	if d.NodeDelta != 1 {
		t.Errorf("NodeDelta: got %d, want 1", d.NodeDelta)
	}
}

func TestCompare_ToolsAdded(t *testing.T) {
	a := makeNodes([]agent.Event{toolEvent("Bash", t0)})
	b := makeNodes([]agent.Event{toolEvent("Bash", t0), toolEvent("Edit", t0.Add(time.Second))})

	d := Compare(a, b)
	if len(d.ToolsAdded) != 1 || d.ToolsAdded[0] != "Edit" {
		t.Errorf("ToolsAdded: got %v, want [Edit]", d.ToolsAdded)
	}
	if len(d.ToolsRemoved) != 0 {
		t.Errorf("ToolsRemoved: got %v, want empty", d.ToolsRemoved)
	}
}

func TestCompare_ToolsRemoved(t *testing.T) {
	a := makeNodes([]agent.Event{toolEvent("Bash", t0), toolEvent("Grep", t0.Add(time.Second))})
	b := makeNodes([]agent.Event{toolEvent("Bash", t0)})

	d := Compare(a, b)
	if len(d.ToolsRemoved) != 1 || d.ToolsRemoved[0] != "Grep" {
		t.Errorf("ToolsRemoved: got %v, want [Grep]", d.ToolsRemoved)
	}
}

func TestCompare_ToolCountDiff(t *testing.T) {
	a := makeNodes([]agent.Event{
		toolEvent("Bash", t0),
		toolEvent("Bash", t0.Add(time.Second)),
	})
	b := makeNodes([]agent.Event{
		toolEvent("Bash", t0),
		toolEvent("Bash", t0.Add(time.Second)),
		toolEvent("Bash", t0.Add(2*time.Second)),
	})

	d := Compare(a, b)
	if d.ToolCountDiffs["Bash"] != 1 {
		t.Errorf("ToolCountDiffs[Bash]: got %d, want 1", d.ToolCountDiffs["Bash"])
	}
}

func TestCompare_SkillsAdded(t *testing.T) {
	a := makeNodes(nil)
	b := makeNodes([]agent.Event{skillEvent("commit", t0)})

	d := Compare(a, b)
	if len(d.SkillsAdded) != 1 || d.SkillsAdded[0] != "commit" {
		t.Errorf("SkillsAdded: got %v, want [commit]", d.SkillsAdded)
	}
}

func TestCompare_SkillsRemoved(t *testing.T) {
	a := makeNodes([]agent.Event{skillEvent("commit", t0)})
	b := makeNodes(nil)

	d := Compare(a, b)
	if len(d.SkillsRemoved) != 1 || d.SkillsRemoved[0] != "commit" {
		t.Errorf("SkillsRemoved: got %v, want [commit]", d.SkillsRemoved)
	}
}

func TestCompare_DurationDelta(t *testing.T) {
	a := makeNodes([]agent.Event{
		toolEvent("Bash", t0),
		toolEvent("Edit", t0.Add(10*time.Second)),
	})
	b := makeNodes([]agent.Event{
		toolEvent("Bash", t0),
		toolEvent("Edit", t0.Add(20*time.Second)),
	})

	d := Compare(a, b)
	if d.DurationDelta != 10*time.Second {
		t.Errorf("DurationDelta: got %v, want 10s", d.DurationDelta)
	}
}

func TestCompare_CostDelta(t *testing.T) {
	n1 := agent.NewNode("s1")
	n1.Usage = agent.Usage{InputTokens: 1000, OutputTokens: 100}

	n2 := agent.NewNode("s2")
	n2.Usage = agent.Usage{InputTokens: 2000, OutputTokens: 200}

	d := Compare([]agent.Node{n1}, []agent.Node{n2})
	if d.CostDelta <= 0 {
		t.Errorf("expected positive CostDelta, got %f", d.CostDelta)
	}
}

func TestCompare_MaxDepth_FlatSessionIsZero(t *testing.T) {
	nodes := []agent.Node{agent.NewNode("s1"), agent.NewNode("s2")}
	d := Compare(nodes, nodes)
	if d.A.MaxDepth != 0 {
		t.Errorf("MaxDepth for flat session: got %d, want 0", d.A.MaxDepth)
	}
}

func TestCompare_MaxDepth_ChildNodes(t *testing.T) {
	parent := agent.NewNode("parent")
	child := agent.NewNode("child")
	child.ParentID = "parent"

	nodes := []agent.Node{parent, child}
	d := Compare(nodes, nodes)
	if d.A.MaxDepth != 1 {
		t.Errorf("MaxDepth: got %d, want 1", d.A.MaxDepth)
	}
}

func TestDiff_Format_ContainsKeyFields(t *testing.T) {
	a := makeNodes([]agent.Event{toolEvent("Bash", t0)})
	b := makeNodes([]agent.Event{toolEvent("Edit", t0.Add(5*time.Second))})

	d := Compare(a, b)
	output := d.Format()

	// Should mention agents, duration, cost, tools
	for _, want := range []string{"Agents:", "Duration:", "Cost:", "Tokens:"} {
		if !strings.Contains(output, want) {
			t.Errorf("Format() missing %q\n got: %s", want, output)
		}
	}
}

func TestSummarize_EmptyNodes(t *testing.T) {
	s := summarize(nil)
	if s.NodeCount != 0 {
		t.Errorf("NodeCount: got %d, want 0", s.NodeCount)
	}
	if s.ToolCounts == nil {
		t.Error("ToolCounts should not be nil")
	}
}

func TestFormatDur(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{45 * time.Second, "45s"},
		{60 * time.Second, "1m"},
		{90 * time.Second, "1m30s"},
	}
	for _, tc := range tests {
		got := formatDur(tc.d)
		if got != tc.want {
			t.Errorf("formatDur(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

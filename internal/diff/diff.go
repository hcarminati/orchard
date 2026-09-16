// Package diff compares two Claude Code sessions and produces a structured
// summary of what changed between them. This is a unique Orchard capability:
// understanding how an agent's behavior, tool usage, cost, and duration
// evolved between two attempts at the same task.
package diff

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hcarminati/orchard/internal/agent"
)

// SessionSummary is a compact representation of a session for diffing.
type SessionSummary struct {
	ID         string
	NodeCount  int
	Duration   time.Duration
	ToolCounts map[string]int  // tool name → call count
	Skills     []string        // skills triggered
	TotalCost  float64         // estimated USD cost
	Usage      agent.Usage
	MaxDepth   int // deepest subagent nesting level
	LoopCount  int // number of nodes that hit loop threshold
}

// Diff summarizes the delta between two sessions.
type Diff struct {
	A, B           SessionSummary
	NodeDelta      int     // B.NodeCount - A.NodeCount
	DurationDelta  time.Duration
	CostDelta      float64
	ToolsAdded     []string // tools called in B but not A
	ToolsRemoved   []string // tools called in A but not B
	ToolCountDiffs map[string]int // tool → (B count - A count)
	SkillsAdded    []string
	SkillsRemoved  []string
	DepthDelta     int
	LoopDelta      int
}

// Compare computes a Diff between two sets of agent nodes (one per session).
// The first slice (a) is the "before" session; the second (b) is "after".
func Compare(a, b []agent.Node) Diff {
	sumA := summarize(a)
	sumB := summarize(b)

	d := Diff{A: sumA, B: sumB}
	d.NodeDelta = sumB.NodeCount - sumA.NodeCount
	d.DurationDelta = sumB.Duration - sumA.Duration
	d.CostDelta = sumB.TotalCost - sumA.TotalCost
	d.DepthDelta = sumB.MaxDepth - sumA.MaxDepth
	d.LoopDelta = sumB.LoopCount - sumA.LoopCount

	// Tool diff.
	allTools := make(map[string]bool)
	for t := range sumA.ToolCounts {
		allTools[t] = true
	}
	for t := range sumB.ToolCounts {
		allTools[t] = true
	}
	d.ToolCountDiffs = make(map[string]int)
	for t := range allTools {
		ca, cb := sumA.ToolCounts[t], sumB.ToolCounts[t]
		if ca == 0 && cb > 0 {
			d.ToolsAdded = append(d.ToolsAdded, t)
		} else if ca > 0 && cb == 0 {
			d.ToolsRemoved = append(d.ToolsRemoved, t)
		}
		if ca != cb {
			d.ToolCountDiffs[t] = cb - ca
		}
	}
	sort.Strings(d.ToolsAdded)
	sort.Strings(d.ToolsRemoved)

	// Skill diff.
	skillSetA := make(map[string]bool)
	for _, s := range sumA.Skills {
		skillSetA[s] = true
	}
	skillSetB := make(map[string]bool)
	for _, s := range sumB.Skills {
		skillSetB[s] = true
	}
	for _, s := range sumB.Skills {
		if !skillSetA[s] {
			d.SkillsAdded = append(d.SkillsAdded, s)
		}
	}
	for _, s := range sumA.Skills {
		if !skillSetB[s] {
			d.SkillsRemoved = append(d.SkillsRemoved, s)
		}
	}
	sort.Strings(d.SkillsAdded)
	sort.Strings(d.SkillsRemoved)

	return d
}

// summarize computes a SessionSummary from a slice of agent nodes.
func summarize(nodes []agent.Node) SessionSummary {
	if len(nodes) == 0 {
		return SessionSummary{ToolCounts: map[string]int{}}
	}

	s := SessionSummary{
		NodeCount:  len(nodes),
		ToolCounts: make(map[string]int),
	}

	var earliest, latest time.Time
	skillSet := make(map[string]bool)

	// Build parent-to-children map for depth calculation.
	children := make(map[string][]string)
	rootIDs := make(map[string]bool)
	nodeIDs := make(map[string]bool)
	for _, n := range nodes {
		nodeIDs[n.ID] = true
	}
	for _, n := range nodes {
		if n.ParentID == "" || !nodeIDs[n.ParentID] {
			rootIDs[n.ID] = true
		} else {
			children[n.ParentID] = append(children[n.ParentID], n.ID)
		}
	}

	for _, n := range nodes {
		s.Usage.Add(n.Usage)
		if n.ConsecutiveTools >= agent.LoopThreshold {
			s.LoopCount++
		}

		for _, e := range n.Events {
			if e.Type == "PreToolUse" && e.Tool != "" && e.Tool != "Agent" {
				s.ToolCounts[e.Tool]++
			}
			if e.Type == "SkillTrigger" && e.Tool != "" {
				skillSet[e.Tool] = true
			}
			if !e.Timestamp.IsZero() {
				if earliest.IsZero() || e.Timestamp.Before(earliest) {
					earliest = e.Timestamp
				}
				if latest.IsZero() || e.Timestamp.After(latest) {
					latest = e.Timestamp
				}
			}
		}
	}

	if !earliest.IsZero() && !latest.IsZero() {
		s.Duration = latest.Sub(earliest)
	}

	for sk := range skillSet {
		s.Skills = append(s.Skills, sk)
	}
	sort.Strings(s.Skills)

	// Estimate cost: $3/1M input, $15/1M output (Sonnet 4 rates).
	s.TotalCost = (float64(s.Usage.InputTokens)/1e6)*3.0 +
		(float64(s.Usage.OutputTokens)/1e6)*15.0

	// Compute max depth via BFS/DFS.
	s.MaxDepth = maxDepth(children, rootIDs)

	return s
}

// maxDepth computes the maximum depth in the agent tree.
func maxDepth(children map[string][]string, roots map[string]bool) int {
	max := 0
	var dfs func(id string, depth int)
	dfs = func(id string, depth int) {
		if depth > max {
			max = depth
		}
		for _, child := range children[id] {
			dfs(child, depth+1)
		}
	}
	for id := range roots {
		dfs(id, 0)
	}
	return max
}

// Format returns a human-readable multi-line summary of the diff.
func (d Diff) Format() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Sessions: %s  vs  %s\n\n", d.A.ID, d.B.ID))

	// Agent count
	sb.WriteString(fmt.Sprintf("Agents:    %d → %d  (%s)\n",
		d.A.NodeCount, d.B.NodeCount, signedInt(d.NodeDelta)))

	// Duration
	sb.WriteString(fmt.Sprintf("Duration:  %s → %s  (%s)\n",
		formatDur(d.A.Duration), formatDur(d.B.Duration), signedDur(d.DurationDelta)))

	// Cost
	sb.WriteString(fmt.Sprintf("Cost:      $%.4f → $%.4f  (%s)\n",
		d.A.TotalCost, d.B.TotalCost, signedFloat(d.CostDelta, "$%.4f")))

	// Tokens
	sb.WriteString(fmt.Sprintf("Tokens:    %d → %d  (%s)\n",
		d.A.Usage.InputTokens+d.A.Usage.OutputTokens,
		d.B.Usage.InputTokens+d.B.Usage.OutputTokens,
		signedInt((d.B.Usage.InputTokens+d.B.Usage.OutputTokens)-(d.A.Usage.InputTokens+d.A.Usage.OutputTokens))))

	// Depth
	sb.WriteString(fmt.Sprintf("Max depth: %d → %d  (%s)\n",
		d.A.MaxDepth, d.B.MaxDepth, signedInt(d.DepthDelta)))

	// Loops
	if d.A.LoopCount > 0 || d.B.LoopCount > 0 {
		sb.WriteString(fmt.Sprintf("Loops:     %d → %d  (%s)\n",
			d.A.LoopCount, d.B.LoopCount, signedInt(d.LoopDelta)))
	}

	// Tool changes
	if len(d.ToolsAdded) > 0 {
		sb.WriteString(fmt.Sprintf("\nTools added:   %s\n", strings.Join(d.ToolsAdded, ", ")))
	}
	if len(d.ToolsRemoved) > 0 {
		sb.WriteString(fmt.Sprintf("Tools removed: %s\n", strings.Join(d.ToolsRemoved, ", ")))
	}

	// Notable tool count changes
	if len(d.ToolCountDiffs) > 0 {
		type kv struct{ k string; v int }
		var changes []kv
		for k, v := range d.ToolCountDiffs {
			changes = append(changes, kv{k, v})
		}
		sort.Slice(changes, func(i, j int) bool {
			if changes[i].v == changes[j].v {
				return changes[i].k < changes[j].k
			}
			return changes[i].v < changes[j].v
		})
		sb.WriteString("\nTool call changes:\n")
		for _, c := range changes {
			sb.WriteString(fmt.Sprintf("  %-20s  %s → %s  (%s)\n",
				c.k,
				fmt.Sprintf("%d", d.A.ToolCounts[c.k]),
				fmt.Sprintf("%d", d.B.ToolCounts[c.k]),
				signedInt(c.v)))
		}
	}

	// Skill changes
	if len(d.SkillsAdded) > 0 {
		sb.WriteString(fmt.Sprintf("\nSkills added:   %s\n", strings.Join(d.SkillsAdded, ", ")))
	}
	if len(d.SkillsRemoved) > 0 {
		sb.WriteString(fmt.Sprintf("Skills removed: %s\n", strings.Join(d.SkillsRemoved, ", ")))
	}

	return sb.String()
}

func signedInt(n int) string {
	if n > 0 {
		return fmt.Sprintf("+%d", n)
	}
	return fmt.Sprintf("%d", n)
}

func signedFloat(f float64, format string) string {
	if f > 0 {
		return "+" + fmt.Sprintf(format, f)
	}
	return fmt.Sprintf(format, f)
}

func signedDur(d time.Duration) string {
	if d > 0 {
		return "+" + formatDur(d)
	}
	if d < 0 {
		return "-" + formatDur(-d)
	}
	return "0s"
}

func formatDur(d time.Duration) string {
	if d == 0 {
		return "0s"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	if s == 0 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dm%02ds", m, s)
}

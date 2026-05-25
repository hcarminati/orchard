package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/hcarminati/orchard/internal/agent"
)

// timelineEntry is a single row in the timeline view, holding the display
// metadata needed to render a horizontal bar.
type timelineEntry struct {
	nodeID    string
	depth     int
	startTime time.Time
	endTime   time.Time
	status    agent.Status
	name      string
	onCritical bool // true when this node is on the critical path
}

// nodeEndTime returns the timestamp of the last event on node with the given ID,
// or SpawnedAt when the node has no events.
func (m Model) nodeEndTime(nodeID string) time.Time {
	n := m.agents.Nodes[nodeID]
	if n == nil {
		return time.Time{}
	}
	if len(n.Events) > 0 {
		return n.Events[len(n.Events)-1].Timestamp
	}
	return n.SpawnedAt
}

// subtreeEntries returns a depth-first list of timelineEntry values for nodeID
// and all its descendants.
func (m Model) subtreeEntries(nodeID string, depth int) []timelineEntry {
	n := m.agents.Nodes[nodeID]
	if n == nil {
		return nil
	}
	start := n.SpawnedAt
	if start.IsZero() && len(n.Events) > 0 {
		start = n.Events[0].Timestamp
	}
	end := m.nodeEndTime(nodeID)
	if end.IsZero() || (!start.IsZero() && end.Before(start)) {
		end = start
	}

	entries := []timelineEntry{{
		nodeID:    nodeID,
		depth:     depth,
		startTime: start,
		endTime:   end,
		status:    n.Status,
		name:      n.Name,
	}}
	for _, childID := range n.Children {
		entries = append(entries, m.subtreeEntries(childID, depth+1)...)
	}
	return entries
}

// criticalPathNodes returns the set of node IDs that form the critical path —
// the root-to-leaf path with the maximum total duration — within the subtree
// rooted at nodeID.
func (m Model) criticalPathNodes(nodeID string) map[string]bool {
	type pathResult struct {
		duration time.Duration
		path     []string
	}

	var longest func(id string) pathResult
	longest = func(id string) pathResult {
		n := m.agents.Nodes[id]
		if n == nil {
			return pathResult{}
		}
		start := n.SpawnedAt
		if start.IsZero() && len(n.Events) > 0 {
			start = n.Events[0].Timestamp
		}
		end := m.nodeEndTime(id)
		dur := end.Sub(start)
		if dur < 0 {
			dur = 0
		}
		best := pathResult{duration: dur, path: []string{id}}
		for _, childID := range n.Children {
			child := longest(childID)
			if child.duration > 0 {
				total := dur + child.duration
				if total > best.duration {
					best = pathResult{
						duration: total,
						path:     append([]string{id}, child.path...),
					}
				}
			}
		}
		return best
	}

	result := longest(nodeID)
	cp := make(map[string]bool, len(result.path))
	for _, id := range result.path {
		cp[id] = true
	}
	return cp
}

// timelineContent renders the horizontal timeline view for the currently
// focused root session (or all roots if no specific session is focused).
//
// Layout:
//
//	0s       10s      20s      30s
//	|         |         |         |
//	session:abc  ████████████████████
//	  └─ researcher    ██████████
//	  └─ implementer        ██████████████
//
// The critical path is rendered in the accent color; other bars use muted/status colors.
func (m Model) timelineContent() string {
	if !m.hasSession || len(m.agents.Nodes) == 0 {
		return lipgloss.NewStyle().Foreground(colorMuted).Padding(0, 1).Render("Waiting for session…")
	}

	innerW := max(1, m.agentsPanelW()-2)

	// Collect all entries for every root session.
	var allEntries []timelineEntry
	criticalSets := map[string]map[string]bool{}
	for _, rootID := range m.sortedRoots() {
		if m.hiddenSessions[rootID] {
			continue
		}
		entries := m.subtreeEntries(rootID, 0)
		allEntries = append(allEntries, entries...)
		criticalSets[rootID] = m.criticalPathNodes(rootID)
	}

	if len(allEntries) == 0 {
		return lipgloss.NewStyle().Foreground(colorMuted).Padding(0, 1).Render("No timeline data yet.")
	}

	// Mark critical path nodes in entries.
	for i, e := range allEntries {
		for _, cp := range criticalSets {
			if cp[e.nodeID] {
				allEntries[i].onCritical = true
				break
			}
		}
	}

	// Find global time bounds.
	var globalStart, globalEnd time.Time
	for _, e := range allEntries {
		if !e.startTime.IsZero() && (globalStart.IsZero() || e.startTime.Before(globalStart)) {
			globalStart = e.startTime
		}
		if !e.endTime.IsZero() && e.endTime.After(globalEnd) {
			globalEnd = e.endTime
		}
	}
	if globalStart.IsZero() {
		return lipgloss.NewStyle().Foreground(colorMuted).Padding(0, 1).Render("No timing data yet.")
	}
	totalDur := globalEnd.Sub(globalStart)
	if totalDur < time.Second {
		totalDur = time.Second // prevent division by zero; snap to 1s minimum
	}

	// The bar rendering area: name label + gap + bar.
	// Reserve nameW columns for the label, barW for the timeline bars.
	const nameW = 18
	barW := max(10, innerW-nameW-2)

	// Render time ruler above the bars.
	ruler := renderRuler(totalDur, barW)
	lines := []string{
		lipgloss.NewStyle().Foreground(colorMuted).Render(strings.Repeat(" ", nameW+2) + ruler),
	}

	for _, e := range allEntries {
		indent := strings.Repeat("  ", e.depth)
		connector := ""
		if e.depth > 0 {
			connector = "└─ "
		}

		// Truncate name to fit nameW (accounting for indent and connector).
		labelBudget := max(1, nameW-len(indent)-len(connector))
		label := e.name
		if len([]rune(label)) > labelBudget {
			label = string([]rune(label)[:max(1, labelBudget-1)]) + "…"
		}
		// Pad to nameW for column alignment.
		raw := indent + connector + label
		padding := max(0, nameW-len([]rune(raw)))
		nameCol := raw + strings.Repeat(" ", padding)

		// Compute bar position and width.
		barStr := renderBar(e, globalStart, totalDur, barW)

		// Assemble the full line.
		var fullLine string
		if e.onCritical {
			fullLine = lipgloss.NewStyle().Foreground(colorAccent).Render(nameCol) + "  " + barStr
		} else {
			fullLine = lipgloss.NewStyle().Foreground(colorFg).Render(nameCol) + "  " + barStr
		}
		lines = append(lines, fullLine)
	}

	// Footer: total session duration.
	footer := fmt.Sprintf("total %s", formatDuration(totalDur))
	lines = append(lines, "", lipgloss.NewStyle().Foreground(colorMuted).Render(footer))

	return strings.Join(lines, "\n")
}

// renderRuler returns a time ruler string of exactly barW characters showing
// tick marks and labels for the session duration.
func renderRuler(total time.Duration, barW int) string {
	buf := []rune(strings.Repeat(" ", barW))
	// Place 4-6 ticks spread evenly across barW.
	numTicks := 5
	if barW < 20 {
		numTicks = 3
	}
	for i := 0; i <= numTicks; i++ {
		pos := i * (barW - 1) / numTicks
		t := time.Duration(int64(total) * int64(i) / int64(numTicks))
		label := []rune(formatDuration(t))
		for j, ch := range label {
			p := pos + j
			if p < barW {
				buf[p] = ch
			}
		}
	}
	return string(buf)
}

// renderBar builds the horizontal bar string for one timeline entry.
func renderBar(e timelineEntry, globalStart time.Time, total time.Duration, barW int) string {
	buf := []rune(strings.Repeat("·", barW))

	if !e.startTime.IsZero() {
		startOff := e.startTime.Sub(globalStart)
		endOff := e.endTime.Sub(globalStart)

		startCol := int(float64(barW) * float64(startOff) / float64(total))
		endCol := int(float64(barW)*float64(endOff)/float64(total)) + 1

		startCol = max(0, min(startCol, barW))
		endCol = max(startCol+1, min(endCol, barW))

		var barChar rune
		if e.status == agent.StatusRunning {
			barChar = '█'
		} else {
			barChar = '▓'
		}
		for i := startCol; i < endCol; i++ {
			buf[i] = barChar
		}
	}

	barStr := string(buf)
	var barColor lipgloss.Color
	switch {
	case e.onCritical:
		barColor = colorAccent
	case e.status == agent.StatusRunning:
		barColor = colorGreen
	case e.status == agent.StatusError:
		barColor = colorRed
	default:
		barColor = colorMuted
	}
	return lipgloss.NewStyle().Foreground(barColor).Render(barStr)
}


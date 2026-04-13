package ui

import (
	"sort"
	"time"

	"github.com/hcarminati/orchard/internal/agent"
)

// visibleNode is a single entry in the flattened, depth-first traversal of the
// visible agent tree. Nodes whose ancestors are collapsed are excluded.
// When groupID is non-empty the entry is a virtual group header row (no real
// node behind it); id is empty in that case.
type visibleNode struct {
	id      string
	depth   int
	groupID string // non-empty only for virtual parallel-group header rows
}

// sortedRoots returns root IDs sorted by activity level then recency:
//
//  1. Error nodes (need immediate attention)
//  2. Running (effective — a parent with any running child counts as running)
//  3. Idle / waiting
//  4. Done / stopped
//
// Within each tier, the node whose subtree had the most recent event appears first.
// The whole parent+children block moves together; only root order changes.
func (m Model) sortedRoots() []string {
	out := make([]string, len(m.agents.Roots))
	copy(out, m.agents.Roots)
	sort.SliceStable(out, func(i, j int) bool {
		ri := m.sortRank(out[i])
		rj := m.sortRank(out[j])
		if ri != rj {
			return ri < rj
		}
		return m.lastEventTime(out[i]).After(m.lastEventTime(out[j]))
	})
	return out
}

// sortRank returns the sort priority for a root node (lower = higher in the list).
// Error is always 0 so broken sessions float to the top regardless of children.
func (m Model) sortRank(id string) int {
	node := m.agents.Nodes[id]
	if node == nil {
		return 10
	}
	if node.Status == agent.StatusError {
		return 0
	}
	switch m.effectiveStatus(id) {
	case agent.StatusRunning:
		return 1
	case agent.StatusIdle:
		return 2
	default:
		return 3
	}
}

// effectiveStatus returns the most active status in the subtree rooted at id.
// A parent is considered Running if any descendant is Running, even when the
// parent itself is Idle. An Error on the parent is never overridden by children.
func (m Model) effectiveStatus(id string) agent.Status {
	node := m.agents.Nodes[id]
	if node == nil {
		return agent.StatusDone
	}
	if node.Status == agent.StatusError {
		return agent.StatusError
	}
	if node.Status == agent.StatusRunning {
		return agent.StatusRunning
	}
	for _, childID := range node.Children {
		if m.effectiveStatus(childID) == agent.StatusRunning {
			return agent.StatusRunning
		}
	}
	return node.Status
}

// lastEventTime returns the timestamp of the most recent event anywhere in the
// subtree rooted at id. Returns the zero time when no events exist.
func (m Model) lastEventTime(id string) time.Time {
	node := m.agents.Nodes[id]
	if node == nil {
		return time.Time{}
	}
	var t time.Time
	if len(node.Events) > 0 {
		t = node.Events[len(node.Events)-1].Timestamp
	}
	for _, childID := range node.Children {
		if ct := m.lastEventTime(childID); ct.After(t) {
			t = ct
		}
	}
	return t
}

// nodeMatchesFilter reports whether n should be shown under the current filter.
// For filterHidden the caller handles visibility directly via hiddenSessions.
func (m Model) nodeMatchesFilter(n *agent.Node) bool {
	switch m.statusFilter {
	case filterRunning:
		return n.Status == agent.StatusRunning
	case filterErrored:
		return n.Status == agent.StatusError
	default: // filterAll, filterHidden handled by visibleNodes
		return true
	}
}

// visibleNodes returns the flat, pre-order depth-first traversal of the agent
// tree. Collapsed nodes hide their subtrees; collapsed parallel groups hide
// their members entirely. Nodes excluded by the status filter are hidden along
// with their subtrees. Parallel groups are represented by a virtual header
// row (groupID non-empty, id empty) that the cursor can land on.
//
// When statusFilter is filterHidden, only top-level sessions in hiddenSessions
// are shown (no children, no filter by status).
// In all other filters, hidden sessions and their children are suppressed.
func (m Model) visibleNodes() []visibleNode {
	var result []visibleNode
	seenGroups := map[string]bool{}

	if m.statusFilter == filterHidden {
		// Hidden view: show only the sessions the user has hidden.
		for _, id := range m.sortedRoots() {
			if m.hiddenSessions[id] {
				result = append(result, visibleNode{id: id, depth: 0})
			}
		}
		return result
	}

	var walk func(id string, depth int)
	walk = func(id string, depth int) {
		node := m.agents.Nodes[id]
		if node == nil {
			return
		}

		// Never show hidden sessions (or their children) in any non-hidden filter.
		if depth == 0 && m.hiddenSessions[id] {
			return
		}

		// Status filter: skip nodes (and their subtrees) that don't match.
		if !m.nodeMatchesFilter(node) {
			return
		}

		// Root-level grouped nodes share a virtual header row.
		if node.GroupID != "" && depth == 0 {
			if !seenGroups[node.GroupID] {
				seenGroups[node.GroupID] = true
				result = append(result, visibleNode{groupID: node.GroupID})
			}
			if m.collapsedGroups[node.GroupID] {
				return // hide all members when group is collapsed
			}
		}

		result = append(result, visibleNode{id: id, depth: depth})
		if !m.collapsed[id] {
			for _, childID := range node.Children {
				walk(childID, depth+1)
			}
		}
	}
	for _, id := range m.sortedRoots() {
		walk(id, 0)
	}
	return result
}

// cursorInGroup reports whether the currently focused node belongs to a parallel group.
func (m Model) cursorInGroup() bool {
	vn := m.visibleNodes()
	if m.cursor >= len(vn) {
		return false
	}
	n := m.agents.Nodes[vn[m.cursor].id]
	return n != nil && n.GroupID != ""
}

// cursorHasChildren reports whether the currently focused node (or group header)
// can be collapsed/expanded with space.
func (m Model) cursorHasChildren() bool {
	vn := m.visibleNodes()
	if m.cursor >= len(vn) {
		return false
	}
	entry := vn[m.cursor]
	if entry.groupID != "" {
		return true // group headers are always collapsible
	}
	n := m.agents.Nodes[entry.id]
	return n != nil && len(n.Children) > 0
}

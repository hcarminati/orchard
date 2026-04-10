package ui

import "github.com/hcarminati/orchard/internal/agent"

// visibleNode is a single entry in the flattened, depth-first traversal of the
// visible agent tree. Nodes whose ancestors are collapsed are excluded.
// When groupID is non-empty the entry is a virtual group header row (no real
// node behind it); id is empty in that case.
type visibleNode struct {
	id      string
	depth   int
	groupID string // non-empty only for virtual parallel-group header rows
}

// sortedRoots returns root IDs with StatusError nodes first, preserving
// relative order within each tier. This ensures errored agents are always
// visible at the top of the list without requiring the user to scroll.
func (m Model) sortedRoots() []string {
	out := make([]string, 0, len(m.agents.Roots))
	for _, id := range m.agents.Roots {
		if n := m.agents.Nodes[id]; n != nil && n.Status == agent.StatusError {
			out = append(out, id)
		}
	}
	for _, id := range m.agents.Roots {
		if n := m.agents.Nodes[id]; n != nil && n.Status != agent.StatusError {
			out = append(out, id)
		}
	}
	return out
}

// nodeMatchesFilter reports whether n should be shown under the current filter.
func (m Model) nodeMatchesFilter(n *agent.Node) bool {
	switch m.statusFilter {
	case filterRunning:
		return n.Status == agent.StatusRunning
	case filterErrored:
		return n.Status == agent.StatusError
	default: // filterAll
		return true
	}
}

// visibleNodes returns the flat, pre-order depth-first traversal of the agent
// tree. Collapsed nodes hide their subtrees; collapsed parallel groups hide
// their members entirely. Nodes excluded by the status filter are hidden along
// with their subtrees. Parallel groups are represented by a virtual header
// row (groupID non-empty, id empty) that the cursor can land on.
func (m Model) visibleNodes() []visibleNode {
	var result []visibleNode
	seenGroups := map[string]bool{}

	var walk func(id string, depth int)
	walk = func(id string, depth int) {
		node := m.agents.Nodes[id]
		if node == nil {
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

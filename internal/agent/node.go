// Package agent defines the agent tree data model for Orchard.
// An agent tree represents the hierarchy of Claude Code agents active in a session:
// which agents spawned which subagents, their statuses, and the events each received.
package agent

import "time"

// Status represents the current state of an agent node.
type Status int

const (
	// StatusRunning means the agent is actively processing.
	StatusRunning Status = iota
	// StatusIdle means the agent is waiting for input.
	StatusIdle
	// StatusDone means the agent has finished successfully.
	StatusDone
	// StatusError means the agent stopped with an error.
	StatusError
)

// Event is a single hook event received from Claude Code for a particular session.
type Event struct {
	// Type is the hook event name: "PreToolUse", "PostToolUse", "Stop", "SubagentStop", "Notification".
	Type string
	// SessionID identifies which session this event belongs to.
	SessionID string
	// Tool is the tool name involved, if any (present for PreToolUse / PostToolUse).
	Tool string
	// Input is the raw JSON tool input (present for PreToolUse / PostToolUse).
	Input string
	// Response is the raw tool response (present for PostToolUse).
	Response string
	// Message is the notification text (present for Notification events).
	Message string
	// Timestamp is when Orchard received the event.
	Timestamp time.Time
}

// Node represents a single agent in the hierarchy.
type Node struct {
	// ID uniquely identifies this node (typically the session_id from Claude Code).
	ID string
	// ParentID is the ID of the parent node, or empty for root-level nodes.
	ParentID string
	// Name is the human-readable label shown in the TUI.
	Name string
	// Status is the current lifecycle state of this agent.
	Status Status
	// GroupID links nodes that are parallel competing runs of the same task.
	GroupID string
	// Events holds every hook event received for this agent, in order.
	Events []Event
}

// Tree holds the complete agent hierarchy for one or more sessions.
// Nodes are stored in a map for O(1) lookup; Roots lists the IDs of top-level nodes
// so the renderer can walk the tree in display order.
type Tree struct {
	// Nodes maps node ID → node pointer.
	Nodes map[string]*Node
	// Roots contains the IDs of root-level nodes, in insertion order.
	Roots []string
}

// NewTree returns an empty, ready-to-use Tree.
func NewTree() Tree {
	return Tree{Nodes: make(map[string]*Node)}
}

// AddNode inserts n into the tree.
// If n.ParentID is empty, n is also appended to Roots.
// If a node with the same ID already exists it is silently replaced.
func (t *Tree) AddNode(n Node) {
	t.Nodes[n.ID] = &n
	if n.ParentID == "" {
		t.Roots = append(t.Roots, n.ID)
	}
}

// ApplyEvent updates the tree based on an incoming hook event.
// If the event's session is not yet in the tree, a new root node is created for it.
// The event is appended to the node's Events slice, and Status is updated
// based on the event type.
func (t *Tree) ApplyEvent(e Event) {
	node, exists := t.Nodes[e.SessionID]
	if !exists {
		name := e.SessionID
		if len(name) > 8 {
			name = "session:" + name[:8]
		}
		n := Node{
			ID:     e.SessionID,
			Name:   name,
			Status: StatusRunning,
		}
		t.AddNode(n)
		node = t.Nodes[e.SessionID]
	}

	node.Events = append(node.Events, e)

	switch e.Type {
	case "Stop", "SubagentStop":
		node.Status = StatusDone
	case "PreToolUse":
		node.Status = StatusRunning
	}
}

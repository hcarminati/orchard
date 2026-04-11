// Package agent defines the agent tree data model for Orchard.
// An agent tree represents the hierarchy of Claude Code agents active in a session:
// which agents spawned which subagents, their statuses, and the events each received.
package agent

import (
	"encoding/json"
	"time"
)

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

// Model identifies which Claude model the agent is running.
type Model string

const (
	ModelHaiku   Model = "haiku"
	ModelSonnet  Model = "sonnet"
	ModelOpus    Model = "opus"
	ModelUnknown Model = ""
)

// Event is a single hook event received from Claude Code for a particular session.
type Event struct {
	// Type is the hook event name: "PreToolUse", "PostToolUse", "Stop", "SubagentStop", "Notification", "PermissionRequest".
	Type string
	// SessionID identifies which session this event belongs to.
	SessionID string
	// ParentID is the session ID of the parent agent, if this event comes from a subagent.
	// Present when Claude Code includes parent_session_id in the hook payload.
	ParentID string
	// Tool is the tool name involved, if any (present for PreToolUse / PostToolUse).
	Tool string
	// ToolUseID is the unique identifier for this specific tool call from Claude Code.
	// Used to correlate PreToolUse[Agent] events with their child subagent nodes.
	ToolUseID string
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
	// ID uniquely identifies this node (typically the session_id from Claude Code,
	// or the tool_use_id for subagent nodes spawned via the Agent tool).
	ID string
	// ParentID is the ID of the parent node, or empty for root-level nodes.
	ParentID string
	// Children holds the IDs of direct child nodes, in insertion order.
	Children []string
	// Name is the human-readable label shown in the TUI.
	Name string
	// Model is the Claude model this agent is running (haiku, sonnet, opus).
	Model Model
	// Status is the current lifecycle state of this agent.
	Status Status
	// ErrorMsg holds the error message when Status is StatusError.
	ErrorMsg string
	// GroupID links nodes that are parallel competing runs of the same task.
	GroupID string
	// Winner is true when this node has been chosen as the best result among
	// parallel competing runs that share the same GroupID.
	Winner bool
	// Tools lists tool names called by this agent, in call order.
	Tools []string
	// Skills lists skill names attached to this agent.
	Skills []string
	// Prompt is the instruction the agent was given.
	Prompt string
	// SpawnedAt is the time the subagent was created, taken from the first
	// record in its JSONL file. Zero for root-level sessions.
	SpawnedAt time.Time
	// Events holds every hook event received for this agent, in order.
	Events []Event
}

// NewNode returns a Node with the given ID and sensible zero/default values.
func NewNode(id string) Node {
	return Node{
		ID:       id,
		Status:   StatusIdle,
		Children: []string{},
		Tools:    []string{},
		Skills:   []string{},
	}
}

// Tree holds the complete agent hierarchy for one or more sessions.
// Nodes are stored in a map for O(1) lookup; Roots lists the IDs of top-level nodes
// so the renderer can walk the tree in display order.
type Tree struct {
	// Nodes maps node ID → node pointer.
	Nodes map[string]*Node
	// Roots contains the IDs of root-level nodes, in insertion order.
	Roots []string
	// pendingSubagents maps a parent session ID to the ordered list of tool_use_id
	// placeholder node IDs that have been pre-created for Agent tool calls but have
	// not yet been claimed by an arriving subagent session.
	pendingSubagents map[string][]string
	// sessionAlias maps a real subagent session ID to the tool_use_id placeholder
	// node ID it was matched to, so subsequent events for that session are routed
	// to the correct (already-visible) node.
	sessionAlias map[string]string
}

// NewTree returns an empty, ready-to-use Tree.
func NewTree() Tree {
	return Tree{
		Nodes:            make(map[string]*Node),
		pendingSubagents: make(map[string][]string),
		sessionAlias:     make(map[string]string),
	}
}

// AddNode inserts n into the tree.
// If n.ParentID is empty, n is also appended to Roots.
// If n.ParentID refers to an existing node, n.ID is appended to the parent's Children.
// If n.ParentID is set but the parent is not yet in the tree, n is treated as a root
// so it remains visible rather than disappearing silently.
// If a node with the same ID already exists it is silently replaced.
func (t *Tree) AddNode(n Node) {
	t.Nodes[n.ID] = &n
	if n.ParentID == "" {
		t.Roots = append(t.Roots, n.ID)
	} else if parent, ok := t.Nodes[n.ParentID]; ok {
		parent.Children = append(parent.Children, n.ID)
	} else {
		// Parent not yet known — surface as root rather than orphaning the node.
		t.Roots = append(t.Roots, n.ID)
	}
}

// ApplyEvent updates the tree based on an incoming hook event.
// If the event's session is not yet in the tree, a new node is created for it.
// When the event carries a ParentID, the new node is attached as a child of
// that parent (if the parent is already in the tree). The event is appended to
// the node's Events slice, and Status is updated based on the event type.
//
// When a PreToolUse event for the Agent tool arrives, a child node is created
// immediately using the tool_use_id as its ID and subagent_type as its name.
// When Stop fires on the parent session, all Running child subagent nodes are
// marked Done since the turn has ended and they must have completed.
func (t *Tree) ApplyEvent(e Event) {
	// If this session ID has been aliased to a pre-created placeholder node
	// (because the subagent's real session ID arrived after the tool_use_id
	// placeholder was created), route all events to that placeholder node.
	nodeID := e.SessionID
	if alias, ok := t.sessionAlias[e.SessionID]; ok {
		nodeID = alias
	}

	node, exists := t.Nodes[nodeID]
	if !exists {
		// Check whether a pre-created placeholder is waiting for this subagent.
		// When a PreToolUse[Agent] fires we create a child keyed by tool_use_id and
		// enqueue it under the parent's session ID. The first live event from the
		// real subagent session claims that placeholder so only one node is shown.
		if e.ParentID != "" {
			if pending := t.pendingSubagents[e.ParentID]; len(pending) > 0 {
				placeholderID := pending[0]
				t.pendingSubagents[e.ParentID] = pending[1:]
				t.sessionAlias[e.SessionID] = placeholderID
				nodeID = placeholderID
				node = t.Nodes[placeholderID]
				exists = true
			}
		}
		if !exists {
			name := e.SessionID
			if len(name) > 8 {
				name = "session:" + name[:8]
			}
			n := Node{
				ID:       e.SessionID,
				ParentID: e.ParentID,
				Name:     name,
				Status:   StatusRunning,
			}
			t.AddNode(n)
			node = t.Nodes[e.SessionID]
		}
	}

	node.Events = append(node.Events, e)

	// StatusError is the only truly unrecoverable terminal state.
	// StatusDone can transition back to Running if a new PreToolUse arrives —
	// this happens when Orchard starts mid-session and loads the session from
	// JSONL as Done, then receives live events for the still-active session.
	if node.Status == StatusError {
		return
	}

	switch e.Type {
	case "Stop":
		// A live Stop means the turn ended and the session is waiting for the next
		// user message — not that it is permanently over. StatusDone is reserved for
		// sessions loaded from JSONL at startup (historical runs).
		node.Status = StatusIdle
		// Any subagent child nodes that are still Running must have completed
		// since the parent's turn is over. Mark them Done.
		for _, childID := range node.Children {
			if child := t.Nodes[childID]; child != nil && child.Status == StatusRunning {
				child.Status = StatusDone
			}
		}
	case "SubagentStop":
		// The subagent's session has ended — mark it done so it renders with a
		// gray dot like a completed historical session.
		node.Status = StatusDone
	case "PreToolUse":
		node.Status = StatusRunning
		// When the Agent tool is invoked, immediately create a child node so it
		// appears in the hierarchy while the subagent is running. Claude Code
		// subagents share the parent's session_id and don't fire their own hooks,
		// so tool_use_id is the only stable identifier for the child.
		if e.Tool == "Agent" && e.ToolUseID != "" {
			var input struct {
				SubagentType string `json:"subagent_type"`
			}
			name := "agent"
			if e.Input != "" {
				if err := json.Unmarshal([]byte(e.Input), &input); err == nil && input.SubagentType != "" {
					name = input.SubagentType
				}
			}
			child := Node{
				ID:       e.ToolUseID,
				ParentID: e.SessionID,
				Name:     name,
				Status:   StatusRunning,
				Children: []string{},
				Tools:    []string{},
				Skills:   []string{},
				Events:   []Event{e},
			}
			t.AddNode(child)
			// Enqueue as a pending placeholder so the first real event from
			// the subagent's own session claims this node instead of creating
			// a duplicate.
			t.pendingSubagents[e.SessionID] = append(t.pendingSubagents[e.SessionID], e.ToolUseID)
		}
	case "Error":
		node.Status = StatusError
		node.ErrorMsg = e.Message
	}
}

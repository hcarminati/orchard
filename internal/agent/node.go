package agent

import (
	"encoding/json"
	"time"
)

type Status int

const (
	StatusRunning Status = iota
	StatusIdle
	StatusDone
	StatusError
)

type Model string

const (
	ModelHaiku   Model = "haiku"
	ModelSonnet  Model = "sonnet"
	ModelOpus    Model = "opus"
	ModelUnknown Model = ""
)

type Event struct {
	Type      string
	SessionID string
	ParentID  string
	Tool      string
	ToolUseID string
	Input     string
	Response  string
	Message   string
	Timestamp time.Time
}

type Node struct {
	ID        string
	ParentID  string
	Children  []string
	Name      string
	Model     Model
	Status    Status
	ErrorMsg  string
	GroupID   string
	Winner    bool
	Tools     []string
	Skills    []string
	Prompt    string
	SpawnedAt time.Time
	Events    []Event
}

func NewNode(id string) Node {
	return Node{
		ID:       id,
		Status:   StatusIdle,
		Children: []string{},
		Tools:    []string{},
		Skills:   []string{},
	}
}

type Tree struct {
	Nodes            map[string]*Node
	Roots            []string
	pendingSubagents map[string][]string
	sessionAlias     map[string]string
	// activeDelegation maps a parent session ID to the placeholder node ID of
	// the subagent it is currently delegating to. Set when PreToolUse[Agent]
	// fires and cleared when the matching PostToolUse[Agent] (or Stop) arrives.
	// While set, non-Agent tool events arriving for the parent session are
	// re-routed to the subagent node — Claude Code fires those events under the
	// parent's session_id even though they are the subagent's own tool calls.
	activeDelegation map[string]string
}

func (t *Tree) SessionAlias(realSessionID string) (string, bool) {
	id, ok := t.sessionAlias[realSessionID]
	return id, ok
}

func NewTree() Tree {
	return Tree{
		Nodes:            make(map[string]*Node),
		pendingSubagents: make(map[string][]string),
		sessionAlias:     make(map[string]string),
		activeDelegation: make(map[string]string),
	}
}

func (t *Tree) AddNode(n Node) {
	if _, exists := t.Nodes[n.ID]; exists {
		return // idempotent: skip if already present
	}
	t.Nodes[n.ID] = &n
	if n.ParentID == "" {
		t.Roots = append(t.Roots, n.ID)
	} else if parent, ok := t.Nodes[n.ParentID]; ok {
		parent.Children = append(parent.Children, n.ID)
	} else {
		t.Roots = append(t.Roots, n.ID)
	}
}

func (t *Tree) ApplyEvent(e Event) {
	nodeID := e.SessionID
	if alias, ok := t.sessionAlias[e.SessionID]; ok {
		nodeID = alias
	}

	node, exists := t.Nodes[nodeID]
	if !exists {
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

	// While a parent is actively delegating to a subagent, Claude Code fires the
	// subagent's own tool events under the parent's session_id. Re-route them to
	// the subagent placeholder so they don't pollute the parent's event list.
	// SkillTrigger events are also re-routed since skills invoked during delegation
	// belong to the subagent's reasoning path.
	if (e.Type == "PreToolUse" && e.Tool != "Agent") || e.Type == "SkillTrigger" {
		if placeholderID := t.activeDelegation[nodeID]; placeholderID != "" {
			if sub := t.Nodes[placeholderID]; sub != nil {
				sub.Events = append(sub.Events, e)
				sub.Status = StatusRunning
				return
			}
		}
	}

	node.Events = append(node.Events, e)

	if node.Status == StatusError {
		return
	}

	switch e.Type {
	case "Stop":
		node.Status = StatusIdle
		delete(t.activeDelegation, nodeID)
		for _, childID := range node.Children {
			if child := t.Nodes[childID]; child != nil && child.Status == StatusRunning {
				child.Status = StatusDone
			}
		}
	case "SubagentStop":
		node.Status = StatusIdle
	case "PostToolUse":
		if e.Tool == "Agent" {
			delete(t.activeDelegation, nodeID)
		}
	case "SkillTrigger":
		node.Status = StatusRunning
	case "PreToolUse":
		node.Status = StatusRunning
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
				Events:   []Event{},
			}
			t.AddNode(child)
			t.pendingSubagents[e.SessionID] = append(t.pendingSubagents[e.SessionID], e.ToolUseID)
			t.activeDelegation[nodeID] = e.ToolUseID
		}
	case "Error":
		node.Status = StatusError
		node.ErrorMsg = e.Message
	}
}

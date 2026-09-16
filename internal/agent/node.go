package agent

import (
	"encoding/json"
	"strings"
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

// ParseModel normalizes a raw model ID string (e.g. "claude-sonnet-4-6") to
// one of the known Model constants. Returns ModelUnknown for empty input and
// the raw string as a Model for unrecognized model names so callers can still
// display them.
func ParseModel(s string) Model {
	lower := strings.ToLower(s)
	switch {
	case strings.Contains(lower, "haiku"):
		return ModelHaiku
	case strings.Contains(lower, "sonnet"):
		return ModelSonnet
	case strings.Contains(lower, "opus"):
		return ModelOpus
	case s == "":
		return ModelUnknown
	default:
		return Model(s)
	}
}

// Usage tracks token counts from Claude API responses. Each field corresponds
// to a field in the API's usage object. Counts are additive across messages.
type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

// Add accumulates other into u in place.
func (u *Usage) Add(other Usage) {
	u.InputTokens += other.InputTokens
	u.OutputTokens += other.OutputTokens
	u.CacheCreationInputTokens += other.CacheCreationInputTokens
	u.CacheReadInputTokens += other.CacheReadInputTokens
}

// IsZero reports whether no tokens have been recorded.
func (u Usage) IsZero() bool {
	return u == (Usage{})
}

type Event struct {
	Type      string
	SessionID string
	ParentID  string
	Tool      string
	ToolUseID string
	Input     string
	Response  string
	Message   string
	Model     Model
	Usage     Usage
	Timestamp time.Time
}

// LoopThreshold is the number of consecutive identical tool calls before a
// node is considered looping. Visible as a ⚠ badge in the Agents panel.
const LoopThreshold = 3

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
	Usage     Usage // accumulated token counts across all events on this node

	// Loop detection: tracks the last tool called and how many times in a row.
	lastTool          string
	consecutiveCount  int
	ConsecutiveTools  int // max consecutive same-tool calls observed; resets when a different tool fires
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
				ID:        e.SessionID,
				ParentID:  e.ParentID,
				Name:      name,
				Status:    StatusRunning,
				SpawnedAt: e.Timestamp,
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

	if !e.Usage.IsZero() {
		node.Usage.Add(e.Usage)
	}

	if e.Model != ModelUnknown {
		node.Model = e.Model
	}

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
		if e.Tool != "" {
			node.Skills = appendUnique(node.Skills, e.Tool)
		}
	case "PreToolUse":
		node.Status = StatusRunning
		if e.Tool != "" && e.Tool != "Agent" {
			node.Tools = appendUnique(node.Tools, e.Tool)
			// Loop detection: count consecutive identical tool calls.
			if e.Tool == node.lastTool {
				node.consecutiveCount++
			} else {
				node.lastTool = e.Tool
				node.consecutiveCount = 1
			}
			if node.consecutiveCount >= LoopThreshold {
				node.ConsecutiveTools = node.consecutiveCount
			} else if node.consecutiveCount == 1 && node.ConsecutiveTools > 0 {
				// A different tool fired — reset the public counter.
				node.ConsecutiveTools = 0
			}
		}
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
				ID:        e.ToolUseID,
				ParentID:  e.SessionID,
				Name:      name,
				Status:    StatusRunning,
				SpawnedAt: e.Timestamp,
				Children:  []string{},
				Tools:     []string{},
				Skills:    []string{},
				Events:    []Event{},
			}
			t.AddNode(child)
			t.pendingSubagents[e.SessionID] = append(t.pendingSubagents[e.SessionID], e.ToolUseID)
			t.activeDelegation[nodeID] = e.ToolUseID
		}
	case "TokenUsage":
		// Usage already accumulated above; no status change needed.
	case "Error":
		node.Status = StatusError
		node.ErrorMsg = e.Message
	}
}

// appendUnique appends s to slice only if it is not already present.
func appendUnique(slice []string, s string) []string {
	for _, v := range slice {
		if v == s {
			return slice
		}
	}
	return append(slice, s)
}

// TotalUsage returns the sum of Usage across nodeID and all its descendants.
func (t *Tree) TotalUsage(nodeID string) Usage {
	n := t.Nodes[nodeID]
	if n == nil {
		return Usage{}
	}
	total := n.Usage
	for _, childID := range n.Children {
		total.Add(t.TotalUsage(childID))
	}
	return total
}

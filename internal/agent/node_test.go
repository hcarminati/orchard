package agent

import (
	"testing"
	"time"
)

func TestNewTree(t *testing.T) {
	tree := NewTree()
	if tree.Nodes == nil {
		t.Error("expected Nodes map to be initialized, got nil")
	}
	if len(tree.Roots) != 0 {
		t.Errorf("expected empty Roots, got %d entries", len(tree.Roots))
	}
}

func TestAddNode_Root(t *testing.T) {
	tree := NewTree()
	tree.AddNode(Node{ID: "a", Name: "alpha"})

	if _, ok := tree.Nodes["a"]; !ok {
		t.Error("expected node 'a' in Nodes map")
	}
	if len(tree.Roots) != 1 || tree.Roots[0] != "a" {
		t.Errorf("expected Roots = [a], got %v", tree.Roots)
	}
}

func TestAddNode_Child(t *testing.T) {
	tree := NewTree()
	tree.AddNode(Node{ID: "parent"})
	tree.AddNode(Node{ID: "child", ParentID: "parent"})

	if _, ok := tree.Nodes["child"]; !ok {
		t.Error("expected child node in Nodes map")
	}
	// Child should not appear in Roots.
	for _, r := range tree.Roots {
		if r == "child" {
			t.Error("child node should not be in Roots")
		}
	}
}

func TestAddNode_MultipleRoots(t *testing.T) {
	tree := NewTree()
	tree.AddNode(Node{ID: "a"})
	tree.AddNode(Node{ID: "b"})

	if len(tree.Roots) != 2 {
		t.Errorf("expected 2 roots, got %d", len(tree.Roots))
	}
}

func TestApplyEvent_CreatesNodeWhenMissing(t *testing.T) {
	tree := NewTree()
	e := Event{
		Type:      "PreToolUse",
		SessionID: "session-abc",
		Tool:      "Bash",
		Timestamp: time.Now(),
	}
	tree.ApplyEvent(e)

	node, ok := tree.Nodes["session-abc"]
	if !ok {
		t.Fatal("expected node to be created for new session ID")
	}
	if node.Status != StatusRunning {
		t.Errorf("expected StatusRunning for new node, got %d", node.Status)
	}
	if len(node.Events) != 1 {
		t.Errorf("expected 1 event, got %d", len(node.Events))
	}
	// Newly created node should be a root.
	found := false
	for _, r := range tree.Roots {
		if r == "session-abc" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected new node to appear in Roots")
	}
}

func TestApplyEvent_AppendToExistingNode(t *testing.T) {
	tree := NewTree()
	tree.AddNode(Node{ID: "s1", Name: "existing", Status: StatusIdle})

	e := Event{Type: "PreToolUse", SessionID: "s1", Timestamp: time.Now()}
	tree.ApplyEvent(e)

	node := tree.Nodes["s1"]
	if len(node.Events) != 1 {
		t.Errorf("expected 1 event on existing node, got %d", len(node.Events))
	}
}

func TestApplyEvent_StopSetsIdle(t *testing.T) {
	tree := NewTree()
	tree.AddNode(Node{ID: "s1", Status: StatusRunning})
	tree.ApplyEvent(Event{Type: "Stop", SessionID: "s1", Timestamp: time.Now()})

	// Stop means the turn ended and the session is waiting for the next user message.
	// StatusDone is reserved for sessions loaded from JSONL (historical runs).
	if tree.Nodes["s1"].Status != StatusIdle {
		t.Errorf("expected StatusIdle after Stop event, got %d", tree.Nodes["s1"].Status)
	}
}

func TestApplyEvent_SubagentStopSetsIdle(t *testing.T) {
	tree := NewTree()
	tree.AddNode(Node{ID: "s1", Status: StatusRunning})
	tree.ApplyEvent(Event{Type: "SubagentStop", SessionID: "s1", Timestamp: time.Now()})

	if tree.Nodes["s1"].Status != StatusIdle {
		t.Errorf("expected StatusIdle after SubagentStop, got %d", tree.Nodes["s1"].Status)
	}
}

func TestApplyEvent_NameTruncatedForLongID(t *testing.T) {
	tree := NewTree()
	tree.ApplyEvent(Event{Type: "Notification", SessionID: "abcdefghijklmnop", Timestamp: time.Now()})

	node := tree.Nodes["abcdefghijklmnop"]
	if node == nil {
		t.Fatal("expected node to be created")
	}
	// Name should be "session:" + first 8 chars.
	if node.Name != "session:abcdefgh" {
		t.Errorf("expected name 'session:abcdefgh', got %q", node.Name)
	}
}

func TestNewNode_Defaults(t *testing.T) {
	n := NewNode("abc")
	if n.ID != "abc" {
		t.Errorf("expected ID 'abc', got %q", n.ID)
	}
	if n.Status != StatusIdle {
		t.Errorf("expected StatusIdle, got %d", n.Status)
	}
	if n.Children == nil {
		t.Error("expected Children slice initialized, got nil")
	}
	if n.Tools == nil {
		t.Error("expected Tools slice initialized, got nil")
	}
	if n.Skills == nil {
		t.Error("expected Skills slice initialized, got nil")
	}
}

func TestAddNode_BidirectionalLink(t *testing.T) {
	tree := NewTree()
	tree.AddNode(Node{ID: "parent"})
	tree.AddNode(Node{ID: "child", ParentID: "parent"})

	parent := tree.Nodes["parent"]
	if len(parent.Children) != 1 || parent.Children[0] != "child" {
		t.Errorf("expected parent.Children = [child], got %v", parent.Children)
	}
}

func TestAddNode_MultipleChildren(t *testing.T) {
	tree := NewTree()
	tree.AddNode(Node{ID: "parent"})
	tree.AddNode(Node{ID: "c1", ParentID: "parent"})
	tree.AddNode(Node{ID: "c2", ParentID: "parent"})

	parent := tree.Nodes["parent"]
	if len(parent.Children) != 2 {
		t.Errorf("expected 2 children, got %d", len(parent.Children))
	}
}

func TestGroupID_LinksSiblings(t *testing.T) {
	tree := NewTree()
	tree.AddNode(Node{ID: "a", GroupID: "g1"})
	tree.AddNode(Node{ID: "b", GroupID: "g1"})
	tree.AddNode(Node{ID: "c", GroupID: "g2"})

	var g1Members []string
	for id, node := range tree.Nodes {
		if node.GroupID == "g1" {
			g1Members = append(g1Members, id)
		}
	}
	if len(g1Members) != 2 {
		t.Errorf("expected 2 nodes in group g1, got %d", len(g1Members))
	}
}

func TestStatus_DoneReactivatedByPreToolUse(t *testing.T) {
	tree := NewTree()
	// Simulate a session loaded from JSONL at startup (StatusDone = historical).
	tree.AddNode(Node{ID: "s1", Status: StatusDone})

	// A live PreToolUse arrives for the same session — it should reactivate.
	tree.ApplyEvent(Event{Type: "PreToolUse", SessionID: "s1", Timestamp: time.Now()})

	if tree.Nodes["s1"].Status != StatusRunning {
		t.Errorf("expected StatusRunning after PreToolUse on Done node, got %d", tree.Nodes["s1"].Status)
	}
}

func TestStatus_IdleReactivatedByPreToolUse(t *testing.T) {
	tree := NewTree()
	tree.AddNode(Node{ID: "s1", Status: StatusRunning})
	tree.ApplyEvent(Event{Type: "Stop", SessionID: "s1", Timestamp: time.Now()})
	// After Stop, session is Idle (waiting for next user message).
	tree.ApplyEvent(Event{Type: "PreToolUse", SessionID: "s1", Timestamp: time.Now()})

	if tree.Nodes["s1"].Status != StatusRunning {
		t.Errorf("expected StatusRunning after PreToolUse on Idle node, got %d", tree.Nodes["s1"].Status)
	}
}

func TestStatus_NoTransitionFromError(t *testing.T) {
	tree := NewTree()
	tree.AddNode(Node{ID: "s1", Status: StatusError})
	tree.ApplyEvent(Event{Type: "PreToolUse", SessionID: "s1", Timestamp: time.Now()})

	if tree.Nodes["s1"].Status != StatusError {
		t.Errorf("expected status to stay Error, got %d", tree.Nodes["s1"].Status)
	}
}

func TestApplyEvent_ErrorSetsStatusAndMessage(t *testing.T) {
	tree := NewTree()
	tree.AddNode(Node{ID: "s1", Status: StatusRunning})
	tree.ApplyEvent(Event{Type: "Error", SessionID: "s1", Message: "context deadline exceeded", Timestamp: time.Now()})

	node := tree.Nodes["s1"]
	if node.Status != StatusError {
		t.Errorf("expected StatusError after Error event, got %d", node.Status)
	}
	if node.ErrorMsg != "context deadline exceeded" {
		t.Errorf("expected ErrorMsg 'context deadline exceeded', got %q", node.ErrorMsg)
	}
}

func TestApplyEvent_ErrorOnNewNode(t *testing.T) {
	tree := NewTree()
	tree.ApplyEvent(Event{Type: "Error", SessionID: "s-new", Message: "tool panicked", Timestamp: time.Now()})

	node := tree.Nodes["s-new"]
	if node == nil {
		t.Fatal("expected node to be created")
	}
	if node.Status != StatusError {
		t.Errorf("expected StatusError, got %d", node.Status)
	}
	if node.ErrorMsg != "tool panicked" {
		t.Errorf("expected ErrorMsg 'tool panicked', got %q", node.ErrorMsg)
	}
}

func TestNodeFields_ModelToolsSkillsPrompt(t *testing.T) {
	n := NewNode("x")
	n.Model = ModelSonnet
	n.Tools = append(n.Tools, "Bash", "Read")
	n.Skills = append(n.Skills, "commit")
	n.Prompt = "do the thing"

	if n.Model != ModelSonnet {
		t.Errorf("expected ModelSonnet, got %q", n.Model)
	}
	if len(n.Tools) != 2 || n.Tools[0] != "Bash" {
		t.Errorf("unexpected Tools: %v", n.Tools)
	}
	if len(n.Skills) != 1 || n.Skills[0] != "commit" {
		t.Errorf("unexpected Skills: %v", n.Skills)
	}
	if n.Prompt != "do the thing" {
		t.Errorf("unexpected Prompt: %q", n.Prompt)
	}
}

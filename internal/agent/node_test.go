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

func TestApplyEvent_StopSetsDone(t *testing.T) {
	tree := NewTree()
	tree.AddNode(Node{ID: "s1", Status: StatusRunning})
	tree.ApplyEvent(Event{Type: "Stop", SessionID: "s1", Timestamp: time.Now()})

	if tree.Nodes["s1"].Status != StatusDone {
		t.Errorf("expected StatusDone after Stop event, got %d", tree.Nodes["s1"].Status)
	}
}

func TestApplyEvent_SubagentStopSetsDone(t *testing.T) {
	tree := NewTree()
	tree.AddNode(Node{ID: "s1", Status: StatusRunning})
	tree.ApplyEvent(Event{Type: "SubagentStop", SessionID: "s1", Timestamp: time.Now()})

	if tree.Nodes["s1"].Status != StatusDone {
		t.Errorf("expected StatusDone after SubagentStop, got %d", tree.Nodes["s1"].Status)
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

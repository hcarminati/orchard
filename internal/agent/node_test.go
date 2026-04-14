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

func TestAddNode_Idempotent(t *testing.T) {
	tree := NewTree()
	tree.AddNode(Node{ID: "parent"})
	tree.AddNode(Node{ID: "child", ParentID: "parent"})
	// Adding the same child again must not duplicate the entry in parent.Children
	// or in tree.Nodes.
	tree.AddNode(Node{ID: "child", ParentID: "parent"})

	parent := tree.Nodes["parent"]
	if len(parent.Children) != 1 {
		t.Errorf("expected parent to have 1 child after duplicate AddNode, got %d", len(parent.Children))
	}
	if len(tree.Roots) != 1 {
		t.Errorf("expected 1 root, got %d: %v", len(tree.Roots), tree.Roots)
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

	// SubagentStop fires under the parent's session_id (not the subagent's),
	// so it returns the parent to Idle — not Done. The Stop handler's child-sweep
	// already marks the subagent placeholder Done.
	if tree.Nodes["s1"].Status != StatusIdle {
		t.Errorf("expected StatusIdle after SubagentStop, got %d", tree.Nodes["s1"].Status)
	}
}

func TestApplyEvent_WithParentID_AttachesChildToParent(t *testing.T) {
	tree := NewTree()
	tree.AddNode(Node{ID: "parent", Status: StatusRunning})

	// A new session arrives with parent_session_id pointing to "parent".
	tree.ApplyEvent(Event{
		Type:      "PreToolUse",
		SessionID: "child",
		ParentID:  "parent",
		Tool:      "Bash",
		Timestamp: time.Now(),
	})

	child := tree.Nodes["child"]
	if child == nil {
		t.Fatal("expected child node to be created")
	}
	if child.ParentID != "parent" {
		t.Errorf("expected child.ParentID = 'parent', got %q", child.ParentID)
	}

	// Child must not appear in Roots.
	for _, r := range tree.Roots {
		if r == "child" {
			t.Error("child node should not be in Roots when parent exists")
		}
	}

	// Parent's Children slice must include the child.
	parent := tree.Nodes["parent"]
	if len(parent.Children) != 1 || parent.Children[0] != "child" {
		t.Errorf("expected parent.Children = [child], got %v", parent.Children)
	}
}

func TestApplyEvent_WithParentID_ParentNotYetInTree_SurfacesAsRoot(t *testing.T) {
	tree := NewTree()

	// Child event arrives before any parent event.
	tree.ApplyEvent(Event{
		Type:      "PreToolUse",
		SessionID: "child",
		ParentID:  "unknown-parent",
		Tool:      "Bash",
		Timestamp: time.Now(),
	})

	child := tree.Nodes["child"]
	if child == nil {
		t.Fatal("expected child node to be created")
	}
	// Should be visible as a root rather than disappearing.
	found := false
	for _, r := range tree.Roots {
		if r == "child" {
			found = true
		}
	}
	if !found {
		t.Error("expected orphaned child to appear in Roots")
	}
}

func TestApplyEvent_SubagentLiveEvents_ClaimPlaceholderNode(t *testing.T) {
	tree := NewTree()

	// Parent session starts.
	tree.ApplyEvent(Event{Type: "Notification", SessionID: "parent", Timestamp: time.Now()})

	// Parent spawns a subagent via the Agent tool — placeholder node created under tool_use_id.
	tree.ApplyEvent(Event{
		Type:      "PreToolUse",
		SessionID: "parent",
		Tool:      "Agent",
		ToolUseID: "toolu_abc",
		Input:     `{"subagent_type":"general-purpose","prompt":"do something"}`,
		Timestamp: time.Now(),
	})

	placeholder := tree.Nodes["toolu_abc"]
	if placeholder == nil {
		t.Fatal("expected placeholder node at tool_use_id")
	}

	// Live hook event arrives from the subagent's real session.
	tree.ApplyEvent(Event{
		Type:      "PreToolUse",
		SessionID: "agent-real-id",
		ParentID:  "parent",
		Tool:      "Read",
		Input:     `{"file_path":"/foo.go"}`,
		Timestamp: time.Now(),
	})

	// No new node should have been created — the real session is aliased to the placeholder.
	if _, ok := tree.Nodes["agent-real-id"]; ok {
		t.Error("expected no separate node for real session ID — should reuse placeholder")
	}

	// The placeholder node should carry only its own events (not the parent's spawn event).
	if len(placeholder.Events) != 1 {
		t.Fatalf("expected 1 event on placeholder (Read only), got %d", len(placeholder.Events))
	}
	if placeholder.Events[0].Tool != "Read" {
		t.Errorf("expected first event Tool = 'Read', got %q", placeholder.Events[0].Tool)
	}

	// Parent should still have only one child (the placeholder, not a duplicate).
	parent := tree.Nodes["parent"]
	if len(parent.Children) != 1 {
		t.Errorf("expected parent to have 1 child, got %d: %v", len(parent.Children), parent.Children)
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

func TestApplyEvent_AgentTool_CreatesChildNodeImmediately(t *testing.T) {
	tree := NewTree()
	tree.AddNode(Node{ID: "parent", Status: StatusRunning})

	tree.ApplyEvent(Event{
		Type:      "PreToolUse",
		SessionID: "parent",
		Tool:      "Agent",
		ToolUseID: "toolu_001",
		Input:     `{"subagent_type":"Explore","prompt":"find stuff"}`,
		Timestamp: time.Now(),
	})

	// Child node should be created immediately using tool_use_id as its ID.
	child := tree.Nodes["toolu_001"]
	if child == nil {
		t.Fatal("expected child node to be created immediately on PreToolUse[Agent]")
	}
	if child.Name != "Explore" {
		t.Errorf("expected child Name 'Explore', got %q", child.Name)
	}
	if child.Status != StatusRunning {
		t.Errorf("expected child StatusRunning, got %d", child.Status)
	}
	if child.ParentID != "parent" {
		t.Errorf("expected child ParentID 'parent', got %q", child.ParentID)
	}

	// Child must appear in parent's Children slice.
	parent := tree.Nodes["parent"]
	if len(parent.Children) != 1 || parent.Children[0] != "toolu_001" {
		t.Errorf("expected parent.Children = [toolu_001], got %v", parent.Children)
	}
}

func TestApplyEvent_AgentTool_MultipleChildrenParallel(t *testing.T) {
	tree := NewTree()
	tree.AddNode(Node{ID: "parent", Status: StatusRunning})

	tree.ApplyEvent(Event{
		Type: "PreToolUse", SessionID: "parent", Tool: "Agent",
		ToolUseID: "toolu_A", Input: `{"subagent_type":"Explore"}`,
		Timestamp: time.Now(),
	})
	tree.ApplyEvent(Event{
		Type: "PreToolUse", SessionID: "parent", Tool: "Agent",
		ToolUseID: "toolu_B", Input: `{"subagent_type":"Plan"}`,
		Timestamp: time.Now(),
	})

	if tree.Nodes["toolu_A"] == nil || tree.Nodes["toolu_A"].Name != "Explore" {
		t.Errorf("expected toolu_A named 'Explore', got %+v", tree.Nodes["toolu_A"])
	}
	if tree.Nodes["toolu_B"] == nil || tree.Nodes["toolu_B"].Name != "Plan" {
		t.Errorf("expected toolu_B named 'Plan', got %+v", tree.Nodes["toolu_B"])
	}

	parent := tree.Nodes["parent"]
	if len(parent.Children) != 2 {
		t.Errorf("expected 2 children, got %d", len(parent.Children))
	}
}

func TestApplyEvent_AgentTool_NoSubagentType_FallsBackToAgentName(t *testing.T) {
	tree := NewTree()
	tree.AddNode(Node{ID: "parent", Status: StatusRunning})

	tree.ApplyEvent(Event{
		Type:      "PreToolUse",
		SessionID: "parent",
		Tool:      "Agent",
		ToolUseID: "toolu_X",
		Input:     `{"prompt":"do something"}`,
		Timestamp: time.Now(),
	})

	child := tree.Nodes["toolu_X"]
	if child == nil {
		t.Fatal("expected child node even without subagent_type")
	}
	if child.Name != "agent" {
		t.Errorf("expected fallback name 'agent', got %q", child.Name)
	}
}

func TestApplyEvent_AgentTool_NoToolUseID_NoChildCreated(t *testing.T) {
	tree := NewTree()
	tree.AddNode(Node{ID: "parent", Status: StatusRunning})

	// Agent call without a tool_use_id — should not crash or create orphan node.
	tree.ApplyEvent(Event{
		Type:      "PreToolUse",
		SessionID: "parent",
		Tool:      "Agent",
		ToolUseID: "",
		Input:     `{"subagent_type":"Explore"}`,
		Timestamp: time.Now(),
	})

	parent := tree.Nodes["parent"]
	if len(parent.Children) != 0 {
		t.Errorf("expected no children when tool_use_id is empty, got %d", len(parent.Children))
	}
}

func TestApplyEvent_Stop_MarksRunningChildrenDone(t *testing.T) {
	tree := NewTree()
	tree.AddNode(Node{ID: "parent", Status: StatusRunning})

	// Two subagents spawned.
	tree.ApplyEvent(Event{
		Type: "PreToolUse", SessionID: "parent", Tool: "Agent",
		ToolUseID: "toolu_1", Input: `{"subagent_type":"Explore"}`,
		Timestamp: time.Now(),
	})
	tree.ApplyEvent(Event{
		Type: "PreToolUse", SessionID: "parent", Tool: "Agent",
		ToolUseID: "toolu_2", Input: `{"subagent_type":"Plan"}`,
		Timestamp: time.Now(),
	})

	// Turn ends.
	tree.ApplyEvent(Event{Type: "Stop", SessionID: "parent", Timestamp: time.Now()})

	if tree.Nodes["toolu_1"].Status != StatusDone {
		t.Errorf("expected toolu_1 StatusDone after parent Stop, got %d", tree.Nodes["toolu_1"].Status)
	}
	if tree.Nodes["toolu_2"].Status != StatusDone {
		t.Errorf("expected toolu_2 StatusDone after parent Stop, got %d", tree.Nodes["toolu_2"].Status)
	}
}

func TestApplyEvent_NotificationDoesNotChangeStatus(t *testing.T) {
	tree := NewTree()
	tree.AddNode(Node{ID: "s1", Status: StatusRunning})
	tree.ApplyEvent(Event{Type: "Notification", SessionID: "s1", Message: "you have a message", Timestamp: time.Now()})

	node := tree.Nodes["s1"]
	if node.Status != StatusRunning {
		t.Errorf("expected StatusRunning unchanged after Notification, got %d", node.Status)
	}
	if len(node.Events) != 1 {
		t.Errorf("expected 1 event appended, got %d", len(node.Events))
	}
	if node.Events[0].Message != "you have a message" {
		t.Errorf("expected event Message preserved, got %q", node.Events[0].Message)
	}
}

func TestApplyEvent_PermissionRequestDoesNotChangeStatus(t *testing.T) {
	tree := NewTree()
	tree.AddNode(Node{ID: "s1", Status: StatusRunning})
	tree.ApplyEvent(Event{Type: "PermissionRequest", SessionID: "s1", Tool: "Bash", Timestamp: time.Now()})

	node := tree.Nodes["s1"]
	if node.Status != StatusRunning {
		t.Errorf("expected StatusRunning unchanged after PermissionRequest, got %d", node.Status)
	}
	if len(node.Events) != 1 {
		t.Errorf("expected 1 event appended, got %d", len(node.Events))
	}
}

func TestApplyEvent_AgentTool_SpawnedChildHasSpawnEvent(t *testing.T) {
	tree := NewTree()
	tree.ApplyEvent(Event{
		Type:      "PreToolUse",
		SessionID: "parent",
		Tool:      "Agent",
		ToolUseID: "tool-use-1",
		Input:     `{"subagent_type":"general-purpose","prompt":"do something"}`,
		Timestamp: time.Now(),
	})

	child := tree.Nodes["tool-use-1"]
	if child == nil {
		t.Fatal("expected child node to be created for Agent tool call")
	}
	// Child starts empty — the spawn PreToolUse belongs to the parent, not the child.
	if len(child.Events) != 0 {
		t.Fatalf("expected child to start with 0 events (spawn event stays on parent), got %d", len(child.Events))
	}
	// The spawn event must be on the parent node.
	parent := tree.Nodes["parent"]
	if parent == nil {
		t.Fatal("expected parent node to exist")
	}
	if len(parent.Events) != 1 || parent.Events[0].Tool != "Agent" {
		t.Errorf("expected parent to have 1 Agent event, got %d events", len(parent.Events))
	}
}

func TestApplyEvent_SubagentToolsReroutedFromParent(t *testing.T) {
	// Claude Code fires subagent tool events with the parent's session_id.
	// ApplyEvent should re-route them to the active subagent placeholder so
	// they don't appear in the parent's event list.
	tree := NewTree()

	// Parent spawns a subagent.
	tree.ApplyEvent(Event{
		Type:      "PreToolUse",
		SessionID: "parent",
		Tool:      "Agent",
		ToolUseID: "toolu_sub",
	})

	// Subagent's Read call arrives under the parent's session_id (Claude Code behaviour).
	tree.ApplyEvent(Event{
		Type:      "PreToolUse",
		SessionID: "parent",
		Tool:      "Read",
		Input:     `{"file_path":"/foo.go"}`,
	})

	parent := tree.Nodes["parent"]
	subagent := tree.Nodes["toolu_sub"]

	if parent == nil || subagent == nil {
		t.Fatal("expected both parent and subagent nodes to exist")
	}
	// Parent should only have the Agent spawn event.
	if len(parent.Events) != 1 || parent.Events[0].Tool != "Agent" {
		t.Errorf("expected parent to have 1 Agent event, got %d: %v", len(parent.Events), parent.Events)
	}
	// Subagent should have the re-routed Read event.
	if len(subagent.Events) != 1 || subagent.Events[0].Tool != "Read" {
		t.Errorf("expected subagent to have 1 Read event, got %d: %v", len(subagent.Events), subagent.Events)
	}
}

func TestApplyEvent_DelegationClearedOnPostToolUseAgent(t *testing.T) {
	// After PostToolUse[Agent], subsequent tool events on the parent stay in
	// the parent — the subagent turn is over.
	tree := NewTree()

	tree.ApplyEvent(Event{Type: "PreToolUse", SessionID: "parent", Tool: "Agent", ToolUseID: "toolu_sub"})
	tree.ApplyEvent(Event{Type: "PostToolUse", SessionID: "parent", Tool: "Agent"})

	// Now a Bash event on the parent belongs to the parent again.
	tree.ApplyEvent(Event{Type: "PreToolUse", SessionID: "parent", Tool: "Bash"})

	parent := tree.Nodes["parent"]
	if parent == nil {
		t.Fatal("expected parent node")
	}
	tools := make([]string, 0, len(parent.Events))
	for _, e := range parent.Events {
		tools = append(tools, e.Tool)
	}
	// Expect: Agent, PostToolUse(Agent), Bash — all three on parent.
	if len(parent.Events) != 3 {
		t.Errorf("expected 3 events on parent after delegation cleared, got %d: %v", len(parent.Events), tools)
	}
}

func TestSessionAlias_ReturnsFalseWhenNotSet(t *testing.T) {
	tree := NewTree()
	_, ok := tree.SessionAlias("no-such-id")
	if ok {
		t.Error("expected no alias for unknown session ID")
	}
}

func TestSessionAlias_ReturnsPlaceholderAfterClaim(t *testing.T) {
	tree := NewTree()
	tree.AddNode(Node{ID: "parent", Status: StatusRunning})

	// Parent spawns a subagent — placeholder is created under tool_use_id.
	tree.ApplyEvent(Event{
		Type:      "PreToolUse",
		SessionID: "parent",
		Tool:      "Agent",
		ToolUseID: "toolu_placeholder",
		Input:     `{"subagent_type":"Explore"}`,
		Timestamp: time.Now(),
	})

	// Real subagent arrives — triggers alias registration in ApplyEvent.
	tree.ApplyEvent(Event{
		Type:      "PreToolUse",
		SessionID: "real-session-id",
		ParentID:  "parent",
		Tool:      "Read",
		Timestamp: time.Now(),
	})

	got, ok := tree.SessionAlias("real-session-id")
	if !ok {
		t.Fatal("expected alias to be registered for real-session-id")
	}
	if got != "toolu_placeholder" {
		t.Errorf("expected alias 'toolu_placeholder', got %q", got)
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

func TestParseModel(t *testing.T) {
	tests := []struct {
		input string
		want  Model
	}{
		{"claude-haiku-4-5-20251001", ModelHaiku},
		{"claude-haiku-3-5", ModelHaiku},
		{"claude-sonnet-4-6", ModelSonnet},
		{"claude-sonnet-3-7", ModelSonnet},
		{"claude-opus-4-6", ModelOpus},
		{"claude-opus-4", ModelOpus},
		{"", ModelUnknown},
		// Unknown model names are passed through as-is.
		{"some-future-model", Model("some-future-model")},
		// Case-insensitive matching.
		{"CLAUDE-HAIKU", ModelHaiku},
		{"Claude-Opus-4", ModelOpus},
	}
	for _, tc := range tests {
		got := ParseModel(tc.input)
		if got != tc.want {
			t.Errorf("ParseModel(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestApplyEvent_SetsModelOnNode(t *testing.T) {
	tree := NewTree()
	tree.AddNode(Node{ID: "s1", Status: StatusRunning})

	tree.ApplyEvent(Event{
		Type:      "PreToolUse",
		SessionID: "s1",
		Tool:      "Bash",
		Model:     ModelSonnet,
		Timestamp: time.Now(),
	})

	node := tree.Nodes["s1"]
	if node.Model != ModelSonnet {
		t.Errorf("expected node.Model = ModelSonnet, got %q", node.Model)
	}
}

func TestApplyEvent_ModelUpdatesOnSubsequentEvents(t *testing.T) {
	// Model can be updated by later events — last non-unknown wins.
	tree := NewTree()
	tree.AddNode(Node{ID: "s1", Status: StatusRunning})

	tree.ApplyEvent(Event{Type: "PreToolUse", SessionID: "s1", Model: ModelHaiku, Timestamp: time.Now()})
	tree.ApplyEvent(Event{Type: "PreToolUse", SessionID: "s1", Model: ModelSonnet, Timestamp: time.Now()})

	if tree.Nodes["s1"].Model != ModelSonnet {
		t.Errorf("expected model updated to ModelSonnet, got %q", tree.Nodes["s1"].Model)
	}
}

func TestApplyEvent_ModelUnknownDoesNotOverwrite(t *testing.T) {
	// An event with no model should not clear a previously set model.
	tree := NewTree()
	tree.AddNode(Node{ID: "s1", Status: StatusRunning, Model: ModelOpus})

	tree.ApplyEvent(Event{Type: "PreToolUse", SessionID: "s1", Model: ModelUnknown, Timestamp: time.Now()})

	if tree.Nodes["s1"].Model != ModelOpus {
		t.Errorf("expected model to remain ModelOpus, got %q", tree.Nodes["s1"].Model)
	}
}

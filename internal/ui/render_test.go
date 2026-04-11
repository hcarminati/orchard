package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hcarminati/orchard/internal/agent"
)

// --- Status dot color ---

func TestStatusColor_CorrectPerStatus(t *testing.T) {
	cases := []struct {
		status agent.Status
		want   lipgloss.Color
	}{
		{agent.StatusRunning, colorGreen},
		{agent.StatusIdle, colorYellow},
		{agent.StatusDone, colorMuted},
		{agent.StatusError, colorRed},
	}
	for _, tc := range cases {
		got := statusColor(tc.status)
		if got != tc.want {
			t.Errorf("statusColor(%d) = %v, want %v", tc.status, got, tc.want)
		}
	}
}

func TestStatusColor_AllDistinct(t *testing.T) {
	statuses := []agent.Status{
		agent.StatusRunning,
		agent.StatusIdle,
		agent.StatusDone,
		agent.StatusError,
	}
	seen := map[lipgloss.Color]agent.Status{}
	for _, s := range statuses {
		c := statusColor(s)
		if c == "" {
			t.Errorf("statusColor(%d) returned empty color", s)
		}
		if prev, exists := seen[c]; exists {
			t.Errorf("statusColor(%d) and statusColor(%d) both return %v", s, prev, c)
		}
		seen[c] = s
	}
}

// --- Error surfacing ---

func TestView_ErroredNodeAtTop(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Name: "agent-ok", Status: agent.StatusRunning},
		{ID: "b", Name: "agent-err", Status: agent.StatusError, ErrorMsg: "something broke"},
	}
	m := New(nodes, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := next.(Model).View()

	posOk := strings.Index(view, "agent-ok")
	posErr := strings.Index(view, "agent-err")
	if posErr == -1 {
		t.Fatal("expected 'agent-err' in view")
	}
	if posOk == -1 {
		t.Fatal("expected 'agent-ok' in view")
	}
	if posErr >= posOk {
		t.Errorf("expected errored node to appear before non-errored node in view")
	}
}

func TestView_ErrorMessageInline(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Name: "agent-err", Status: agent.StatusError, ErrorMsg: "panic"},
	}
	m := New(nodes, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := next.(Model).View()

	if !strings.Contains(view, "panic") {
		t.Errorf("expected error message in view, got:\n%s", view)
	}
}

func TestView_NoErrorMessage_WhenErrorMsgEmpty(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Name: "agent-err", Status: agent.StatusError, ErrorMsg: ""},
	}
	m := New(nodes, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := next.(Model).View()

	if strings.Contains(view, "✗") {
		t.Errorf("expected no ✗ indicator when ErrorMsg is empty, got:\n%s", view)
	}
}

func TestSortedRoots_ErroredFirst(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Status: agent.StatusRunning},
		{ID: "b", Status: agent.StatusError},
		{ID: "c", Status: agent.StatusIdle},
		{ID: "d", Status: agent.StatusError},
	}
	m := New(nodes, nil)
	sr := m.sortedRoots()

	if len(sr) != 4 {
		t.Fatalf("expected 4 roots, got %d", len(sr))
	}
	for _, id := range sr[:2] {
		n := m.agents.Nodes[id]
		if n.Status != agent.StatusError {
			t.Errorf("expected first two sorted roots to be StatusError, got node %q with status %d", id, n.Status)
		}
	}
	for _, id := range sr[2:] {
		n := m.agents.Nodes[id]
		if n.Status == agent.StatusError {
			t.Errorf("expected non-error nodes after errored ones, got node %q with StatusError", id)
		}
	}
}

func TestView_HookError_ErroredNodeAtTop(t *testing.T) {
	ch := make(chan agent.Event, 10)
	m := New(nil, ch)

	next, _ := m.Update(hookEventMsg{event: agent.Event{Type: "PreToolUse", SessionID: "s1", Timestamp: time.Now()}})
	next, _ = next.Update(hookEventMsg{event: agent.Event{Type: "PreToolUse", SessionID: "s2", Timestamp: time.Now()}})
	next, _ = next.Update(hookEventMsg{event: agent.Event{Type: "Error", SessionID: "s2", Message: "panic", Timestamp: time.Now()}})
	next, _ = next.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	view := next.(Model).View()
	posS1 := strings.Index(view, " s1")
	posS2 := strings.Index(view, " s2")
	if posS1 == -1 || posS2 == -1 {
		t.Fatalf("expected both sessions in view, got:\n%s", view)
	}
	if posS2 >= posS1 {
		t.Errorf("expected errored session s2 to appear before s1 in view")
	}
}

// --- Tree rendering ---

func TestView_TreeRendering_ChildIndented(t *testing.T) {
	m := makeTree()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := next.(Model).View()

	posA := strings.Index(view, "agent-A")
	posB := strings.Index(view, "agent-B")
	posC := strings.Index(view, "agent-C")
	posD := strings.Index(view, "agent-D")

	for name, pos := range map[string]int{"agent-A": posA, "agent-B": posB, "agent-C": posC, "agent-D": posD} {
		if pos == -1 {
			t.Errorf("expected %q in view", name)
		}
	}
	if posA >= posB {
		t.Errorf("expected A before B in view")
	}
	if posB >= posC {
		t.Errorf("expected B before C in view")
	}
	if posC >= posD {
		t.Errorf("expected C before D in view")
	}
}

func TestView_CollapseIndicator_ShownForNodesWithChildren(t *testing.T) {
	m := makeTree()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := next.(Model).View()
	if !strings.Contains(view, "▼") {
		t.Errorf("expected expand indicator ▼ for expanded node A, got:\n%s", view)
	}

	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	view = next.(Model).View()
	if !strings.Contains(view, "▶") {
		t.Errorf("expected collapse indicator ▶ for collapsed node A, got:\n%s", view)
	}
}

func TestView_CollapsedNode_HidesChildrenFromView(t *testing.T) {
	m := makeTree()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	view := next.(Model).View()

	if strings.Contains(view, "agent-B") {
		t.Errorf("expected agent-B to be hidden after collapsing A")
	}
	if strings.Contains(view, "agent-C") {
		t.Errorf("expected agent-C to be hidden after collapsing A")
	}
	if !strings.Contains(view, "agent-A") {
		t.Errorf("expected agent-A (collapsed) to still be visible")
	}
	if !strings.Contains(view, "agent-D") {
		t.Errorf("expected agent-D (sibling root) to still be visible")
	}
}

func TestEventsPanel_SpawnContextHeader_ShownForChildNode(t *testing.T) {
	parent := agent.Node{ID: "parent-abc123", Name: "session:parent-a", Status: agent.StatusDone}
	child := agent.Node{
		ID:       "child-xyz",
		ParentID: "parent-abc123",
		Name:     "Explore",
		Prompt:   "find all Go files",
		Status:   agent.StatusDone,
	}
	m := New([]agent.Node{parent, child}, nil)
	// Navigate to child node (cursor=1 after parent).
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	view := next.(Model).View()

	if !strings.Contains(view, "Spawned by") {
		t.Errorf("expected 'Spawned by' header for child node, got:\n%s", view)
	}
	if !strings.Contains(view, "session:parent-a") {
		t.Errorf("expected parent session label in header, got:\n%s", view)
	}
	if !strings.Contains(view, "find all Go files") {
		t.Errorf("expected prompt text in header, got:\n%s", view)
	}
}

func TestEventsPanel_SpawnContextHeader_NotShownForParentNode(t *testing.T) {
	parent := agent.Node{ID: "parent-abc123", Name: "session:parent-a", Status: agent.StatusDone}
	m := New([]agent.Node{parent}, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := next.(Model).View()

	if strings.Contains(view, "Spawned by") {
		t.Errorf("expected no 'Spawned by' header for root node, got:\n%s", view)
	}
}

func TestEventsPanel_SpawnContextHeader_TruncatesLongPrompt(t *testing.T) {
	parent := agent.Node{ID: "parent-aabbccdd", Name: "session:parent-a", Status: agent.StatusDone}
	child := agent.Node{
		ID:       "child-xyz",
		ParentID: "parent-aabbccdd",
		Name:     "Explore",
		Prompt:   strings.Repeat("a", 100),
		Status:   agent.StatusDone,
	}
	m := New([]agent.Node{parent, child}, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	view := next.(Model).View()

	if !strings.Contains(view, "…") {
		t.Errorf("expected long prompt to be truncated with ellipsis, got:\n%s", view)
	}
}

func TestView_ChildNode_ShowsConnector(t *testing.T) {
	m := makeTree()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := next.(Model).View()

	// Child nodes (agent-B, agent-C) should be preceded by the └─ connector.
	if !strings.Contains(view, "└─") {
		t.Errorf("expected └─ connector for child nodes in view, got:\n%s", view)
	}
}

func TestView_AgentToolCall_ChildNamedAfterSubagentType(t *testing.T) {
	ch := make(chan agent.Event, 10)
	m := New(nil, ch)

	// Parent session appears.
	next, _ := m.Update(hookEventMsg{event: agent.Event{
		Type: "PreToolUse", SessionID: "parent-sess", Tool: "Bash",
		Timestamp: time.Now(),
	}})
	// Parent fires an Agent tool call — child node should appear immediately.
	next, _ = next.Update(hookEventMsg{event: agent.Event{
		Type:      "PreToolUse",
		SessionID: "parent-sess",
		Tool:      "Agent",
		ToolUseID: "toolu_test_001",
		Input:     `{"subagent_type":"Explore","prompt":"find files"}`,
		Timestamp: time.Now(),
	}})
	next, _ = next.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	view := next.(Model).View()
	if !strings.Contains(view, "Explore") {
		t.Errorf("expected child node to be named 'Explore' in view, got:\n%s", view)
	}
	if !strings.Contains(view, "└─") {
		t.Errorf("expected └─ connector for child node in view, got:\n%s", view)
	}
}

// --- permissionPreview ---

func TestPermissionPreview_Bash_ExtractsCommand(t *testing.T) {
	got := permissionPreview("Bash", `{"command":"go test ./...","timeout":30}`, "")
	if got != "go test ./..." {
		t.Errorf("expected 'go test ./...', got %q", got)
	}
}

func TestPermissionPreview_Bash_TruncatesLongCommand(t *testing.T) {
	cmd := strings.Repeat("x", 50)
	got := permissionPreview("Bash", `{"command":"`+cmd+`"}`, "")
	if len([]rune(got)) > 41 { // 40 + "…"
		t.Errorf("expected command truncated at 40 runes, got %q (len %d)", got, len([]rune(got)))
	}
	if !strings.HasPrefix(got, strings.Repeat("x", 40)) {
		t.Errorf("expected first 40 x's preserved, got %q", got)
	}
}

func TestPermissionPreview_Read_ExtractsFilePathWithTilde(t *testing.T) {
	got := permissionPreview("Read", `{"file_path":"/home/user/project/main.go"}`, "/home/user")
	if got != "~/project/main.go" {
		t.Errorf("expected '~/project/main.go', got %q", got)
	}
}

func TestPermissionPreview_Edit_ExtractsFilePathWithTilde(t *testing.T) {
	got := permissionPreview("Edit", `{"file_path":"/home/user/file.go","old_string":"a","new_string":"b"}`, "/home/user")
	if got != "~/file.go" {
		t.Errorf("expected '~/file.go', got %q", got)
	}
}

func TestPermissionPreview_Write_ExtractsFilePathWithTilde(t *testing.T) {
	got := permissionPreview("Write", `{"file_path":"/home/user/out.go","content":"x"}`, "/home/user")
	if got != "~/out.go" {
		t.Errorf("expected '~/out.go', got %q", got)
	}
}

func TestPermissionPreview_UnknownTool_TruncatesRawInput(t *testing.T) {
	input := `{"url":"https://example.com"}`
	got := permissionPreview("WebFetch", input, "")
	if !strings.HasPrefix(got, `{"url":`) {
		t.Errorf("expected raw JSON as fallback, got %q", got)
	}
}

func TestPermissionPreview_EmptyInput_ReturnsEmpty(t *testing.T) {
	got := permissionPreview("Bash", "", "")
	if got != "" {
		t.Errorf("expected empty string for empty input, got %q", got)
	}
}

func TestPermissionPreview_MissingField_FallsBackToRaw(t *testing.T) {
	// Bash event where the JSON has no "command" field.
	got := permissionPreview("Bash", `{"description":"do stuff"}`, "")
	if !strings.Contains(got, "description") {
		t.Errorf("expected raw input fallback when command field missing, got %q", got)
	}
}

// --- Footer hints ---

func TestFooter_SpaceHint_OnlyWhenCursorOnNodeWithChildren(t *testing.T) {
	m := makeTree()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	view := next.(Model).View()
	if !strings.Contains(view, "expand/collapse") {
		t.Errorf("expected 'expand/collapse' hint when cursor is on node with children, got:\n%s", view)
	}

	// Navigate to D (leaf, no children).
	for i := 0; i < 3; i++ {
		next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	}
	view = next.(Model).View()
	if strings.Contains(view, "expand/collapse") {
		t.Errorf("expected no 'expand/collapse' hint when cursor is on a leaf, got:\n%s", view)
	}
}

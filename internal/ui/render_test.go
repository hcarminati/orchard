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
	m := newWithClock(nodes, nil, nil, time.Time{})
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
	m := newWithClock(nodes, nil, nil, time.Time{})
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
	m := newWithClock(nodes, nil, nil, time.Time{})
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
	m := newWithClock(nodes, nil, nil, time.Time{})
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
	m := newWithClock(nil, ch, nil, time.Time{})

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
	m := newWithClock([]agent.Node{parent, child}, nil, nil, time.Time{})
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
	m := newWithClock([]agent.Node{parent}, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := next.(Model).View()

	if strings.Contains(view, "Spawned by") {
		t.Errorf("expected no 'Spawned by' header for root node, got:\n%s", view)
	}
}

func TestEventsPanel_SessionHeader_ShownForRootNode(t *testing.T) {
	parent := agent.Node{ID: "parent-abc123", Name: "session:parent-a", Status: agent.StatusDone}
	m := newWithClock([]agent.Node{parent}, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	content := next.(Model).eventsContent()

	if !strings.Contains(content, "session:parent-a") {
		t.Errorf("expected session label in events panel for root node, got:\n%s", content)
	}
	if strings.Contains(content, "Spawned by") {
		t.Errorf("expected no 'Spawned by' in root node events panel, got:\n%s", content)
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
	m := newWithClock([]agent.Node{parent, child}, nil, nil, time.Time{})
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
	m := newWithClock(nil, ch, nil, time.Time{})

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

// --- toolColor ---

func TestToolColor_KnownTools(t *testing.T) {
	cases := []struct {
		tool string
		want lipgloss.Color
	}{
		{"Bash", colorYellow},
		{"Read", colorBlue},
		{"WebFetch", colorBlue},
		{"Edit", colorCoral},
		{"Write", colorCoral},
		{"Grep", colorTeal},
		{"Glob", colorTeal},
		{"ToolSearch", colorTeal},
		{"WebSearch", colorTeal},
		{"Agent", colorAccent},
		{"TaskCreate", colorAccent},
		{"TaskUpdate", colorAccent},
		{"Skill", colorGreen},
	}
	for _, tc := range cases {
		got := toolColor(tc.tool)
		if got != tc.want {
			t.Errorf("toolColor(%q) = %q, want %q", tc.tool, got, tc.want)
		}
	}
}

func TestToolColor_MCP_ReturnsBlue(t *testing.T) {
	cases := []string{"mcp__figma__get_design", "mcp__slack__send_message", "mcp__plugin_figma_figma__whoami"}
	for _, tool := range cases {
		got := toolColor(tool)
		if got != colorBlue {
			t.Errorf("toolColor(%q) = %q, want colorBlue", tool, got)
		}
	}
}

func TestToolColor_UnknownTool_ReturnsFg(t *testing.T) {
	got := toolColor("SomeUnrecognizedTool")
	if got != colorFg {
		t.Errorf("expected colorFg for unknown tool, got %q", got)
	}
}

// --- permissionPreview ---

func TestPermissionPreview_Bash_ExtractsCommand(t *testing.T) {
	got := permissionPreview("Bash", `{"command":"go test ./...","timeout":30}`, "")
	if got != "go test ./..." {
		t.Errorf("expected 'go test ./...', got %q", got)
	}
}

func TestPermissionPreview_Bash_ReturnsFullCommand(t *testing.T) {
	cmd := strings.Repeat("x", 50)
	got := permissionPreview("Bash", `{"command":"`+cmd+`"}`, "")
	if got != cmd {
		t.Errorf("expected full command returned, got %q", got)
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

// --- Cost helpers ---

func TestFormatCost_Zero(t *testing.T) {
	if got := formatCost(0); got != "" {
		t.Errorf("expected empty string for zero cost, got %q", got)
	}
}

func TestFormatCost_NonZero(t *testing.T) {
	got := formatCost(3.14159)
	if got != "$3.14" {
		t.Errorf("expected '$3.14', got %q", got)
	}
}

func TestFormatCost_TwoDecimalPlaces(t *testing.T) {
	got := formatCost(0.5)
	if got != "$0.50" {
		t.Errorf("expected '$0.50', got %q", got)
	}
}

func TestFormatCost_AboveThreshold_NoDecimals(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{10.0, "$10"},
		{167.23, "$167"},
		{1000.99, "$1000"},
	}
	for _, tc := range cases {
		got := formatCost(tc.in)
		if got != tc.want {
			t.Errorf("formatCost(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFormatCost_NoScientificNotation(t *testing.T) {
	got := formatCost(0.000001)
	if strings.Contains(got, "e") || strings.Contains(got, "E") {
		t.Errorf("cost should not use scientific notation, got %q", got)
	}
}

// --- Token count formatting ---

func TestFormatTokenCount_Small(t *testing.T) {
	if got := formatTokenCount(891); got != "891" {
		t.Errorf("expected '891', got %q", got)
	}
}

func TestFormatTokenCount_Thousands(t *testing.T) {
	if got := formatTokenCount(2300); got != "2.3k" {
		t.Errorf("expected '2.3k', got %q", got)
	}
}

func TestFormatTokenCount_ExactThousand(t *testing.T) {
	if got := formatTokenCount(1000); got != "1k" {
		t.Errorf("expected '1k', got %q", got)
	}
}

func TestFormatTokenCount_Millions(t *testing.T) {
	if got := formatTokenCount(1_200_000); got != "1.2M" {
		t.Errorf("expected '1.2M', got %q", got)
	}
}

func TestFormatTokenCount_ExactMillion(t *testing.T) {
	if got := formatTokenCount(1_000_000); got != "1M" {
		t.Errorf("expected '1M', got %q", got)
	}
}

func TestEstimateCost_Sonnet(t *testing.T) {
	u := agent.Usage{InputTokens: 1_000_000, OutputTokens: 0}
	cost := estimateCost(u, agent.ModelSonnet)
	if cost != 3.00 {
		t.Errorf("expected $3.00 for 1M input tokens on sonnet, got %f", cost)
	}
}

func TestEstimateCost_Haiku(t *testing.T) {
	u := agent.Usage{InputTokens: 0, OutputTokens: 1_000_000}
	cost := estimateCost(u, agent.ModelHaiku)
	if cost != 4.00 {
		t.Errorf("expected $4.00 for 1M output tokens on haiku, got %f", cost)
	}
}

func TestEstimateCost_UnknownModel_ReturnsZero(t *testing.T) {
	u := agent.Usage{InputTokens: 1000, OutputTokens: 500}
	cost := estimateCost(u, agent.ModelUnknown)
	if cost != 0 {
		t.Errorf("expected $0 for unknown model, got %f", cost)
	}
}

func TestEstimateCost_ZeroUsage_ReturnsZero(t *testing.T) {
	cost := estimateCost(agent.Usage{}, agent.ModelSonnet)
	if cost != 0 {
		t.Errorf("expected $0 for zero usage, got %f", cost)
	}
}

func TestEstimateCost_CacheTokens(t *testing.T) {
	// 1M cache-read tokens on sonnet = $0.30
	u := agent.Usage{CacheReadInputTokens: 1_000_000}
	cost := estimateCost(u, agent.ModelSonnet)
	if cost != 0.30 {
		t.Errorf("expected $0.30 for 1M cache-read tokens on sonnet, got %f", cost)
	}
}

// --- Model in events panel ---

func TestTabStripTitle_ShowsModelWhenFocused(t *testing.T) {
	n := agent.Node{ID: "a", Name: "session:a", Status: agent.StatusRunning, Model: agent.ModelSonnet}
	m := newWithClock([]agent.Node{n}, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	title := next.(Model).tabStripTitle(colorAccent)
	stripped := stripANSI(title)
	if !strings.Contains(stripped, "sonnet") {
		t.Errorf("expected model 'sonnet' in tab strip title, got %q", stripped)
	}
}

func TestTabStripTitle_NoModelWhenUnknown(t *testing.T) {
	n := agent.Node{ID: "a", Name: "session:a", Status: agent.StatusRunning, Model: agent.ModelUnknown}
	m := newWithClock([]agent.Node{n}, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	title := next.(Model).tabStripTitle(colorAccent)
	stripped := stripANSI(title)
	if strings.Contains(stripped, "·") {
		t.Errorf("expected no model badge when model is unknown, got %q", stripped)
	}
}

func TestTabStripTitle_NoModelWhenNoFocus(t *testing.T) {
	m := newWithClock(nil, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	title := next.(Model).tabStripTitle(colorAccent)
	stripped := stripANSI(title)
	if strings.Contains(stripped, "·") {
		t.Errorf("expected no model badge when no node is focused, got %q", stripped)
	}
}

func TestEventsHeader_RootNode_ShowsStatus(t *testing.T) {
	n := agent.Node{ID: "abc123", Name: "session:abc123", Status: agent.StatusRunning, Model: agent.ModelSonnet}
	m := newWithClock([]agent.Node{n}, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	content := next.(Model).eventsContent()
	stripped := stripANSI(content)

	if !strings.Contains(stripped, "running") {
		t.Errorf("expected status 'running' in root node events header, got:\n%s", stripped)
	}
}

func TestEventsHeader_RootNode_ShowsSubagentCount(t *testing.T) {
	parent := agent.Node{ID: "parent-id", Name: "session:parent-i", Status: agent.StatusRunning, Model: agent.ModelSonnet}
	child1 := agent.Node{ID: "child1", ParentID: "parent-id", Name: "Explore", Status: agent.StatusDone}
	child2 := agent.Node{ID: "child2", ParentID: "parent-id", Name: "Plan", Status: agent.StatusDone}
	m := newWithClock([]agent.Node{parent, child1, child2}, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	content := next.(Model).eventsContent()
	stripped := stripANSI(content)

	if !strings.Contains(stripped, "2 subagents") {
		t.Errorf("expected '2 subagents' in root node events header, got:\n%s", stripped)
	}
}

func TestEventsHeader_SpawnLine_ShowsSpawnedBy(t *testing.T) {
	// Model is now in the tab strip, not the spawn header.
	parent := agent.Node{ID: "parent-abc123", Name: "session:parent-a", Status: agent.StatusDone}
	child := agent.Node{
		ID:       "child-xyz",
		ParentID: "parent-abc123",
		Name:     "Explore",
		Status:   agent.StatusDone,
		Model:    agent.ModelHaiku,
	}
	m := newWithClock([]agent.Node{parent, child}, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	content := next.(Model).eventsContent()
	stripped := stripANSI(content)

	if !strings.Contains(stripped, "Spawned by") {
		t.Errorf("expected 'Spawned by' in spawn context header, got:\n%s", stripped)
	}
	// Model appears in tab strip, not in events content.
	if strings.Contains(stripped, "haiku") {
		t.Errorf("expected model NOT in events content (moved to tab strip), got:\n%s", stripped)
	}
}

func TestStatusLabel(t *testing.T) {
	cases := []struct {
		status agent.Status
		want   string
	}{
		{agent.StatusRunning, "running"},
		{agent.StatusIdle, "idle"},
		{agent.StatusDone, "done"},
		{agent.StatusError, "error"},
	}
	for _, tc := range cases {
		if got := statusLabel(tc.status); got != tc.want {
			t.Errorf("statusLabel(%v) = %q, want %q", tc.status, got, tc.want)
		}
	}
}

func TestEventsHeader_NoCostInEventsContent(t *testing.T) {
	// Cost has moved to the tab strip and agents panel — events content should
	// never contain a "$" sign regardless of usage.
	n := agent.Node{
		ID:     "abc123",
		Name:   "session:abc123",
		Status: agent.StatusRunning,
		Model:  agent.ModelSonnet,
		Usage:  agent.Usage{InputTokens: 1_000_000},
	}
	m := newWithClock([]agent.Node{n}, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	content := next.(Model).eventsContent()
	stripped := stripANSI(content)

	if strings.Contains(stripped, "$") {
		t.Errorf("expected no cost in events content (cost moved to tab strip), got:\n%s", stripped)
	}
}

func TestTabStripTitle_ShowsTokenCount(t *testing.T) {
	n := agent.Node{
		ID:    "a",
		Name:  "session:a",
		Model: agent.ModelSonnet,
		Usage: agent.Usage{InputTokens: 2300, OutputTokens: 891},
	}
	m := newWithClock([]agent.Node{n}, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	title := next.(Model).tabStripTitle(colorAccent)
	stripped := stripANSI(title)

	if !strings.Contains(stripped, "↑2.3k") {
		t.Errorf("expected '↑2.3k' in tab strip title, got %q", stripped)
	}
	if !strings.Contains(stripped, "↓891") {
		t.Errorf("expected '↓891' in tab strip title, got %q", stripped)
	}
}

func TestTabStripTitle_ShowsCostWhenUsagePresent(t *testing.T) {
	n := agent.Node{
		ID:    "a",
		Name:  "session:a",
		Model: agent.ModelSonnet,
		Usage: agent.Usage{InputTokens: 1_000_000},
	}
	m := newWithClock([]agent.Node{n}, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	title := next.(Model).tabStripTitle(colorAccent)
	stripped := stripANSI(title)

	if !strings.Contains(stripped, "$") {
		t.Errorf("expected cost in tab strip title when usage is non-zero, got %q", stripped)
	}
}

func TestTabStripTitle_NoCostWhenZeroUsage(t *testing.T) {
	n := agent.Node{
		ID:    "a",
		Name:  "session:a",
		Model: agent.ModelSonnet,
		// no usage
	}
	m := newWithClock([]agent.Node{n}, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	title := next.(Model).tabStripTitle(colorAccent)
	stripped := stripANSI(title)

	if strings.Contains(stripped, "$") {
		t.Errorf("expected no cost in tab strip title when usage is zero, got %q", stripped)
	}
}

func TestAgentsPanel_ShowsTotalCost(t *testing.T) {
	n1 := agent.Node{ID: "a", Name: "session:a", Status: agent.StatusRunning, Model: agent.ModelSonnet, Usage: agent.Usage{InputTokens: 1_000_000}}
	n2 := agent.Node{ID: "b", Name: "session:b", Status: agent.StatusRunning, Model: agent.ModelHaiku, Usage: agent.Usage{OutputTokens: 1_000_000}}
	m := newWithClock([]agent.Node{n1, n2}, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	stripped := stripANSI(next.(Model).View())

	// $3.00 (sonnet input) + $4.00 (haiku output) = $7.00
	if !strings.Contains(stripped, "$7.00") {
		t.Errorf("expected '$7.00' in agents panel title, got:\n%s", stripped)
	}
	if strings.Contains(stripped, "lifetime:") {
		t.Errorf("expected no lifetime label, got:\n%s", stripped)
	}
}

func TestAgentsPanel_TotalCostIncludesHistoricalSessions(t *testing.T) {
	// Historical (done) + live session — total cost should sum both, no lifetime label.
	historical := agent.Node{
		ID:     "old",
		Name:   "hist-agent",
		Status: agent.StatusDone,
		Model:  agent.ModelSonnet,
		Usage:  agent.Usage{InputTokens: 1_000_000}, // $3.00
	}
	live := agent.Node{
		ID:     "cur",
		Name:   "live-agent",
		Status: agent.StatusRunning,
		Model:  agent.ModelHaiku,
		Usage:  agent.Usage{OutputTokens: 1_000_000}, // $4.00
	}
	m := newWithClock([]agent.Node{historical, live}, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 40})
	stripped := stripANSI(next.(Model).View())

	if !strings.Contains(stripped, "$7.00") {
		t.Errorf("expected '$7.00' (sum of all sessions) in title, got:\n%s", stripped)
	}
	if strings.Contains(stripped, "lifetime:") {
		t.Errorf("expected no lifetime label, got:\n%s", stripped)
	}
}

func TestAgentsPanel_NoCostWhenZeroUsage(t *testing.T) {
	n := agent.Node{ID: "a", Name: "session:a", Model: agent.ModelSonnet}
	m := newWithClock([]agent.Node{n}, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})

	// agentsContent is the tree body — it never contains cost (cost is in the title).
	content := next.(Model).agentsContent()
	stripped := stripANSI(content)

	if strings.Contains(stripped, "$") {
		t.Errorf("expected no cost in agents panel content when usage is zero, got:\n%s", stripped)
	}
}

func TestAgentsPanel_BudgetDisplay_NoBudget_ShowsOnlyCost(t *testing.T) {
	// No budget configured — cost display shows "$X" with no denominator.
	n := agent.Node{ID: "a", Name: "session:a", Status: agent.StatusRunning, Model: agent.ModelSonnet, Usage: agent.Usage{InputTokens: 1_000_000}}
	m := newWithClock([]agent.Node{n}, nil, nil, time.Time{})
	// budget is zero (default)
	rendered := m.renderCostWithBudget(3.00, colorMuted)
	stripped := stripANSI(rendered)

	if stripped != "$3.00" {
		t.Errorf("expected '$3.00' with no denominator, got: %q", stripped)
	}
}

func TestAgentsPanel_BudgetDisplay_UnderThreshold_ShowsFraction(t *testing.T) {
	// Budget set to $300; cost is $3 — under 80%, shown with denominator.
	n := agent.Node{ID: "a", Name: "session:a", Status: agent.StatusRunning, Model: agent.ModelSonnet, Usage: agent.Usage{InputTokens: 1_000_000}}
	m := newWithClock([]agent.Node{n}, nil, nil, time.Time{})
	m.budget = 300

	rendered := m.renderCostWithBudget(3.00, colorMuted)
	stripped := stripANSI(rendered)
	if stripped != "$3.00/$300" {
		t.Errorf("expected '$3.00/$300', got: %q", stripped)
	}
}

func TestBudgetCostColor_BelowThreshold(t *testing.T) {
	// Under 80% — should use the border color unchanged.
	got := budgetCostColor(0.50, colorMuted)
	if got != colorMuted {
		t.Errorf("expected borderColor below 80%%, got %v", got)
	}
}

func TestBudgetCostColor_AmberThreshold(t *testing.T) {
	// Exactly 80% — amber.
	got := budgetCostColor(0.80, colorMuted)
	if got != colorYellow {
		t.Errorf("expected amber (colorYellow) at 80%%, got %v", got)
	}
}

func TestBudgetCostColor_RedThreshold(t *testing.T) {
	// Exactly 100% — red.
	got := budgetCostColor(1.00, colorMuted)
	if got != colorRed {
		t.Errorf("expected red (colorRed) at 100%%, got %v", got)
	}
}

func TestBudgetCostColor_OverBudget(t *testing.T) {
	// Over 100% — still red.
	got := budgetCostColor(1.50, colorMuted)
	if got != colorRed {
		t.Errorf("expected red (colorRed) over budget, got %v", got)
	}
}

func TestAgentsPanel_BudgetDisplay_AmberThreshold(t *testing.T) {
	// Cost is exactly 80% of budget — text fraction should appear correctly.
	m := newWithClock(nil, nil, nil, time.Time{})
	m.budget = 3.75 // $3.00 / $3.75 = 80%

	stripped := stripANSI(m.renderCostWithBudget(3.00, colorMuted))
	if stripped != "$3.00/$3.75" {
		t.Errorf("expected '$3.00/$3.75', got: %q", stripped)
	}
}

func TestAgentsPanel_BudgetDisplay_RedThreshold(t *testing.T) {
	// Cost exceeds 100% of budget — text fraction should appear correctly.
	m := newWithClock(nil, nil, nil, time.Time{})
	m.budget = 2.00 // $3.00 exceeds $2.00 budget

	stripped := stripANSI(m.renderCostWithBudget(3.00, colorMuted))
	if stripped != "$3.00/$2.00" {
		t.Errorf("expected '$3.00/$2.00', got: %q", stripped)
	}
}

func TestAgentsPanel_TokenDisplay_NoMax(t *testing.T) {
	// No max_tokens configured — shows raw token count with no denominator.
	m := newWithClock(nil, nil, nil, time.Time{})
	// maxTokens is zero by default
	stripped := stripANSI(m.renderTokensWithMax(1_500_000, colorMuted))
	if stripped != "1.5M" {
		t.Errorf("expected '1.5M', got: %q", stripped)
	}
}

func TestAgentsPanel_TokenDisplay_WithMax(t *testing.T) {
	// max_tokens configured — shows "used/max".
	m := newWithClock(nil, nil, nil, time.Time{})
	m.maxTokens = 5_000_000

	stripped := stripANSI(m.renderTokensWithMax(1_500_000, colorMuted))
	if stripped != "1.5M/5M" {
		t.Errorf("expected '1.5M/5M', got: %q", stripped)
	}
}

func TestAgentsPanel_TokenDisplay_ColorThresholds(t *testing.T) {
	// Token color uses the same budgetCostColor logic as cost — re-verify at token scale.
	if got := budgetCostColor(0.79, colorMuted); got != colorMuted {
		t.Errorf("expected borderColor below 80%%, got %v", got)
	}
	if got := budgetCostColor(0.80, colorMuted); got != colorYellow {
		t.Errorf("expected amber at 80%%, got %v", got)
	}
	if got := budgetCostColor(1.00, colorMuted); got != colorRed {
		t.Errorf("expected red at 100%%, got %v", got)
	}
}

func TestAgentsPanel_ShowsTokensInHeader(t *testing.T) {
	// Token count should appear in the Agents panel title when there is usage.
	n := agent.Node{
		ID:     "a",
		Name:   "session:a",
		Status: agent.StatusRunning,
		Model:  agent.ModelSonnet,
		Usage:  agent.Usage{InputTokens: 1_000_000, OutputTokens: 500_000},
	}
	m := newWithClock([]agent.Node{n}, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	stripped := stripANSI(next.(Model).View())

	// 1M input + 500k output = 1.5M total
	if !strings.Contains(stripped, "1.5M") {
		t.Errorf("expected '1.5M' token count in title, got:\n%s", stripped)
	}
}

func TestView_TreeNodes_NoModelBadge(t *testing.T) {
	n := agent.Node{ID: "a", Name: "myagent", Status: agent.StatusRunning, Model: agent.ModelSonnet}
	m := newWithClock([]agent.Node{n}, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	content := next.(Model).agentsContent()
	stripped := stripANSI(content)

	// Model name must not appear in the agents panel tree.
	if strings.Contains(stripped, "sonnet") {
		t.Errorf("expected no model badge in agents tree, got:\n%s", stripped)
	}
}

// stripANSI removes ANSI escape codes from a string for plain-text assertions.
func stripANSI(s string) string {
	var out strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) && s[i] != 'm' {
				i++
			}
			i++ // skip 'm'
			continue
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
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

// --- shortToolName ---

func TestShortToolName_KnownTools(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Bash", "bash"},
		{"Read", "read"},
		{"Edit", "edit"},
	}
	for _, tc := range cases {
		got := shortToolName(tc.in)
		if got != tc.want {
			t.Errorf("shortToolName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestShortToolName_MCP(t *testing.T) {
	got := shortToolName("mcp__plugin_figma_figma__use_figma")
	if got != "mcp:use_figma" {
		t.Errorf("shortToolName MCP = %q, want %q", got, "mcp:use_figma")
	}
}

// --- nodePills ---

func TestNodePills_Empty(t *testing.T) {
	n := &agent.Node{Tools: []string{}, Skills: []string{}}
	if got := nodePills(n); got != "" {
		t.Errorf("nodePills on empty node = %q, want empty", got)
	}
}

func TestNodePills_ToolsOnly(t *testing.T) {
	n := &agent.Node{Tools: []string{"Bash", "Read"}, Skills: []string{}}
	got := nodePills(n)
	if !strings.Contains(got, "bash") {
		t.Errorf("expected 'bash' in pills, got: %q", got)
	}
	if !strings.Contains(got, "read") {
		t.Errorf("expected 'read' in pills, got: %q", got)
	}
}

func TestNodePills_SkillsOnly(t *testing.T) {
	n := &agent.Node{Tools: []string{}, Skills: []string{"commit", "build"}}
	got := nodePills(n)
	if !strings.Contains(got, "▸commit") {
		t.Errorf("expected '▸commit' in pills, got: %q", got)
	}
	if !strings.Contains(got, "▸build") {
		t.Errorf("expected '▸build' in pills, got: %q", got)
	}
}

func TestNodePills_ToolsAndSkills(t *testing.T) {
	n := &agent.Node{Tools: []string{"Bash"}, Skills: []string{"myskill"}}
	got := nodePills(n)
	if !strings.Contains(got, "bash") {
		t.Errorf("expected tool pill in output, got: %q", got)
	}
	if !strings.Contains(got, "▸myskill") {
		t.Errorf("expected skill pill in output, got: %q", got)
	}
}

func TestNodePills_SkillWithNamespaceStripped(t *testing.T) {
	n := &agent.Node{Tools: []string{}, Skills: []string{"figma:use-figma"}}
	got := nodePills(n)
	// The namespace prefix should be stripped: "figma:" → show "use-figma"
	if !strings.Contains(got, "▸use-figma") {
		t.Errorf("expected namespace-stripped skill pill, got: %q", got)
	}
	if strings.Contains(got, "figma:") {
		t.Errorf("expected namespace to be stripped from skill pill, got: %q", got)
	}
}

// --- Pills appear inline on tree node rows ---

func TestView_PillsAppearsInAgentsPanel(t *testing.T) {
	nodes := []agent.Node{
		{
			ID:     "a",
			Name:   "agent-a",
			Status: agent.StatusRunning,
			Tools:  []string{"Bash", "Read"},
			Skills: []string{"commit"},
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	view := next.(Model).View()

	if !strings.Contains(view, "bash") {
		t.Errorf("expected 'bash' pill in view, got:\n%s", view)
	}
	if !strings.Contains(view, "▸commit") {
		t.Errorf("expected '▸commit' skill pill in view, got:\n%s", view)
	}
}

func TestView_PillsNotShownWhenError(t *testing.T) {
	nodes := []agent.Node{
		{
			ID:       "a",
			Name:     "agent-a",
			Status:   agent.StatusError,
			ErrorMsg: "something failed",
			Tools:    []string{"Bash"},
			Skills:   []string{},
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	view := next.(Model).View()

	// Error message should appear; pills should not (error takes priority).
	if !strings.Contains(view, "something failed") {
		t.Errorf("expected error message in view, got:\n%s", view)
	}
	if strings.Contains(view, "bash") {
		t.Errorf("expected no tool pills when error is showing, got:\n%s", view)
	}
}

// --- statusSummary ---

func TestStatusSummary_AllBuckets(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Status: agent.StatusRunning},
		{ID: "b", Status: agent.StatusRunning},
		{ID: "c", Status: agent.StatusIdle},
		{ID: "d", Status: agent.StatusDone},
		{ID: "e", Status: agent.StatusError},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	parts := m.statusSummary()

	want := []string{"2 running", "1 idle", "1 done", "1 error"}
	if len(parts) != len(want) {
		t.Fatalf("expected %d parts, got %d: %v", len(want), len(parts), parts)
	}
	for i, w := range want {
		if parts[i] != w {
			t.Errorf("parts[%d] = %q, want %q", i, parts[i], w)
		}
	}
}

func TestStatusSummary_ZeroBucketsOmitted(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Status: agent.StatusRunning},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	parts := m.statusSummary()

	if len(parts) != 1 || parts[0] != "1 running" {
		t.Errorf("expected [1 running], got %v", parts)
	}
}

func TestStatusSummary_EmptyTree(t *testing.T) {
	m := newWithClock(nil, nil, nil, time.Time{})
	parts := m.statusSummary()
	if len(parts) != 0 {
		t.Errorf("expected empty summary for empty tree, got %v", parts)
	}
}

// --- Status summary appears in Agents header ---

func TestView_StatusSummaryInHeader(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Name: "agent-a", Status: agent.StatusRunning},
		{ID: "b", Name: "agent-b", Status: agent.StatusDone},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	view := next.(Model).View()

	if !strings.Contains(view, "running") {
		t.Errorf("expected 'running' in Agents header, got:\n%s", view)
	}
	if !strings.Contains(view, "done") {
		t.Errorf("expected 'done' in Agents header, got:\n%s", view)
	}
}

// --- Loop badge ---

func TestLoopBadge_BelowThreshold(t *testing.T) {
	n := &agent.Node{ConsecutiveTools: agent.LoopThreshold - 1}
	if got := loopBadge(n); got != "" {
		t.Errorf("expected empty badge below threshold, got %q", got)
	}
}

func TestLoopBadge_AtThreshold(t *testing.T) {
	n := &agent.Node{ConsecutiveTools: agent.LoopThreshold}
	got := loopBadge(n)
	if got == "" {
		t.Errorf("expected non-empty loop badge at threshold")
	}
	if !strings.Contains(got, "⚠") {
		t.Errorf("expected ⚠ in loop badge, got %q", got)
	}
}

func TestView_LoopBadgeAppearsInAgentsPanel(t *testing.T) {
	nodes := []agent.Node{
		{
			ID:               "a",
			Name:             "loopy-agent",
			Status:           agent.StatusRunning,
			ConsecutiveTools: agent.LoopThreshold,
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	view := next.(Model).View()

	if !strings.Contains(view, "⚠") {
		t.Errorf("expected loop badge ⚠ in view for looping agent, got:\n%s", view)
	}
}

func TestView_LoopBadgeSuppressesPills(t *testing.T) {
	// When an agent is looping, the loop badge takes priority over pills.
	nodes := []agent.Node{
		{
			ID:               "a",
			Name:             "agent-a",
			Status:           agent.StatusRunning,
			Tools:            []string{"Bash"},
			ConsecutiveTools: agent.LoopThreshold,
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	view := next.(Model).View()

	if !strings.Contains(view, "⚠") {
		t.Errorf("expected loop badge in view, got:\n%s", view)
	}
	// "bash" pill should not appear when the loop badge is shown (badge takes precedence).
	if strings.Contains(view, "bash") {
		t.Errorf("expected no tool pills when loop badge is shown, got:\n%s", view)
	}
}

// --- Files tab ---

func TestExtractFilePath_Valid(t *testing.T) {
	path := extractFilePath(`{"file_path":"/tmp/foo.go","content":"bar"}`)
	if path != "/tmp/foo.go" {
		t.Errorf("expected /tmp/foo.go, got %q", path)
	}
}

func TestExtractFilePath_Missing(t *testing.T) {
	path := extractFilePath(`{"content":"bar"}`)
	if path != "" {
		t.Errorf("expected empty string, got %q", path)
	}
}

func TestExtractFilePath_InvalidJSON(t *testing.T) {
	path := extractFilePath(`not-json`)
	if path != "" {
		t.Errorf("expected empty string for invalid JSON, got %q", path)
	}
}

func TestFileChangeTool_KnownTools(t *testing.T) {
	cases := []struct {
		tool string
		op   string
		ok   bool
	}{
		{"Write", "write", true},
		{"Edit", "edit", true},
		{"MultiEdit", "edit", true},
		{"NotebookEdit", "notebook", true},
		{"Bash", "", false},
		{"Read", "", false},
	}
	for _, tc := range cases {
		op, ok := fileChangeTool(tc.tool)
		if ok != tc.ok || op != tc.op {
			t.Errorf("fileChangeTool(%q) = (%q, %v), want (%q, %v)", tc.tool, op, ok, tc.op, tc.ok)
		}
	}
}

func TestAllFileChanges_CollectsFromAllNodes(t *testing.T) {
	ts := time.Now()
	nodes := []agent.Node{
		{
			ID:     "a",
			Name:   "agent-a",
			Status: agent.StatusDone,
			Events: []agent.Event{
				{Type: "PostToolUse", Tool: "Write", Input: `{"file_path":"/src/main.go"}`, Timestamp: ts},
			},
		},
		{
			ID:     "b",
			Name:   "agent-b",
			Status: agent.StatusDone,
			Events: []agent.Event{
				{Type: "PostToolUse", Tool: "Edit", Input: `{"file_path":"/src/util.go"}`, Timestamp: ts.Add(time.Second)},
			},
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	changes := m.allFileChanges()

	if len(changes) != 2 {
		t.Fatalf("expected 2 file changes, got %d", len(changes))
	}
}

func TestAllFileChanges_SkipsNonFileEvents(t *testing.T) {
	ts := time.Now()
	nodes := []agent.Node{
		{
			ID:   "a",
			Name: "agent-a",
			Events: []agent.Event{
				{Type: "PostToolUse", Tool: "Bash", Input: `{"command":"ls"}`, Timestamp: ts},
				{Type: "PreToolUse", Tool: "Write", Input: `{"file_path":"/x.go"}`, Timestamp: ts},
				{Type: "PostToolUse", Tool: "Write", Input: `{"file_path":"/y.go"}`, Timestamp: ts},
			},
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	changes := m.allFileChanges()

	// Only the PostToolUse Write should be collected; Bash and PreToolUse are skipped.
	if len(changes) != 1 {
		t.Fatalf("expected 1 file change (only PostToolUse+Write), got %d", len(changes))
	}
	if changes[0].path != "/y.go" {
		t.Errorf("expected /y.go, got %q", changes[0].path)
	}
}

func TestFilesContent_ShowsFilenames(t *testing.T) {
	ts := time.Now().Add(-30 * time.Second)
	nodes := []agent.Node{
		{
			ID:     "a",
			Name:   "agent-a",
			Status: agent.StatusDone,
			Events: []agent.Event{
				{Type: "PostToolUse", Tool: "Write", Input: `{"file_path":"/project/main.go"}`, Timestamp: ts},
			},
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m2 := next.(Model)
	m2.activeRightTab = 1 // tabFiles
	view := m2.View()

	if !strings.Contains(view, "main.go") {
		t.Errorf("expected 'main.go' in Files tab view, got:\n%s", view)
	}
}

func TestFilesContent_EmptyMessage_WhenNoChanges(t *testing.T) {
	nodes := []agent.Node{
		{ID: "a", Name: "agent-a", Status: agent.StatusRunning, Events: []agent.Event{}},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m2 := next.(Model)
	m2.activeRightTab = 1 // tabFiles
	content := m2.filesContent()

	if !strings.Contains(content, "No file writes") {
		t.Errorf("expected 'No file writes' message, got:\n%s", content)
	}
}

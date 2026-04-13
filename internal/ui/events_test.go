package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hcarminati/orchard/internal/agent"
)

func TestEventsContent_NoSession_ShowsWaiting(t *testing.T) {
	m := newModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	content := next.(Model).eventsContent()
	if !strings.Contains(content, "Waiting") {
		t.Errorf("expected waiting message when no session, got: %q", content)
	}
}

func TestEventsContent_NoEvents_ShowsEmptyState(t *testing.T) {
	nodes := []agent.Node{
		{ID: "s1xxxxxxxx", Name: "session:s1xxxxxx", Status: agent.StatusRunning},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	content := next.(Model).eventsContent()
	if !strings.Contains(content, "No events yet") {
		t.Errorf("expected empty state message, got: %q", content)
	}
}

func TestEventsContent_ShowsTimestampAndType(t *testing.T) {
	ts := time.Date(2026, 1, 1, 15, 4, 5, 0, time.UTC)
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{Type: "PreToolUse", Tool: "Bash", SessionID: "s1xxxxxxxx", Timestamp: ts},
			},
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	content := next.(Model).eventsContent()

	if !strings.Contains(content, "Jan 01 15:04:05") {
		t.Errorf("expected timestamp in events content, got: %q", content)
	}
	if !strings.Contains(content, "PreToolUse") {
		t.Errorf("expected event type in events content, got: %q", content)
	}
	if !strings.Contains(content, "Bash") {
		t.Errorf("expected tool name in events content, got: %q", content)
	}
}

func TestEventsContent_NoToolName_WhenEventHasNone(t *testing.T) {
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{Type: "Stop", SessionID: "s1xxxxxxxx", Timestamp: time.Now()},
			},
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	content := next.(Model).eventsContent()

	if !strings.Contains(content, "Stop") {
		t.Errorf("expected 'Stop' in events content, got: %q", content)
	}
}

func TestEventsContent_NoFocusedAgent_ShowsPrompt(t *testing.T) {
	m := newModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	got := next.(Model)
	got.hasSession = true
	content := got.eventsContent()
	if !strings.Contains(content, "Focus an agent") {
		t.Errorf("expected focus prompt when no node focused, got: %q", content)
	}
}

func TestUpdate_JMovesCursor_WhenEventsPanelActive(t *testing.T) {
	m := newWithClock([]agent.Node{makeNodeWithEvents("s1xxxxxxxx", 50)}, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	if m.activePanel != panelEvents {
		t.Fatal("expected panelEvents after tab")
	}

	// Move cursor up one so there is room to move down.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = next.(Model)

	before := m.eventCursor
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	after := next.(Model).eventCursor

	if after != before+1 {
		t.Errorf("expected eventCursor=%d after j, got %d", before+1, after)
	}
}

func TestUpdate_KDoesNotGoNegative_WhenEventsPanelActive(t *testing.T) {
	m := newWithClock([]agent.Node{makeNodeWithEvents("s1xxxxxxxx", 5)}, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)

	// Press k many times to reach the top.
	for i := 0; i < 10; i++ {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
		m = next.(Model)
	}
	if m.eventCursor < 0 {
		t.Errorf("expected eventCursor >= 0 after repeated k, got %d", m.eventCursor)
	}
	if m.eventCursor != 0 {
		t.Errorf("expected eventCursor=0 at top, got %d", m.eventCursor)
	}
}

func TestUpdate_JK_AgentsPanel_DoesNotScrollEvents(t *testing.T) {
	nodes := []agent.Node{
		makeNodeWithEvents("s1xxxxxxxx", 5),
		{ID: "s2xxxxxxxx", Name: "session:s2xxxxxx", Status: agent.StatusRunning},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	if m.activePanel != panelAgents {
		t.Fatal("expected panelAgents initially")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	got := next.(Model)

	if got.cursor != 1 {
		t.Errorf("expected cursor=1 after j in agents panel, got %d", got.cursor)
	}
	if got.eventScroll != 0 {
		t.Errorf("expected eventScroll=0 after agent navigation, got %d", got.eventScroll)
	}
}

func TestUpdate_SwitchingAgent_ResetsEventScroll(t *testing.T) {
	nodes := []agent.Node{
		makeNodeWithEvents("s1xxxxxxxx", 50),
		{ID: "s2xxxxxxxx", Name: "session:s2xxxxxx", Status: agent.StatusRunning},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = next.(Model)
	if m.eventScroll == 0 {
		t.Fatal("expected non-zero eventScroll after scrolling")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = next.(Model)

	if m.eventScroll != 0 {
		t.Errorf("expected eventScroll reset to 0 after switching agent, got %d", m.eventScroll)
	}
}

func TestEventsContent_ScrollApplied(t *testing.T) {
	ts := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	var events []agent.Event
	for i := 0; i < 5; i++ {
		events = append(events, agent.Event{
			Type:      "PreToolUse",
			SessionID: "s1xxxxxxxx",
			Tool:      "Bash",
			Timestamp: ts.Add(time.Duration(i) * time.Hour),
		})
	}
	nodes := []agent.Node{
		{ID: "s1xxxxxxxx", Name: "session:s1xxxxxx", Status: agent.StatusRunning, Events: events},
	}
	// height=6 → viewH=3; with 5 events maxStart=2, so scrolling is meaningful.
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 6})
	m = next.(Model)

	m.eventScroll = 0
	content := m.eventsContent()
	if !strings.Contains(content, "Jan 01 10:00:00") {
		t.Errorf("expected Jan 01 10:00:00 at scroll=0, got: %q", content)
	}

	m.eventScroll = 2
	content = m.eventsContent()
	if !strings.Contains(content, "Jan 01 12:00:00") {
		t.Errorf("expected Jan 01 12:00:00 at scroll=2, got: %q", content)
	}
	if strings.Contains(content, "Jan 01 10:00:00") {
		t.Errorf("expected Jan 01 10:00:00 scrolled off at scroll=2, got: %q", content)
	}
}

func TestEventsContent_AppearsInView(t *testing.T) {
	ts := time.Date(2026, 1, 1, 15, 4, 5, 0, time.UTC)
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{Type: "PreToolUse", Tool: "Read", SessionID: "s1xxxxxxxx", Timestamp: ts},
			},
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := next.(Model).View()

	if !strings.Contains(view, "Jan 01 15:04:05") {
		t.Errorf("expected event timestamp in full View output, got:\n%s", view)
	}
	if !strings.Contains(view, "PreToolUse") {
		t.Errorf("expected event type in full View output, got:\n%s", view)
	}
}

func TestFocusedNode_GroupHeader_ReturnsNil(t *testing.T) {
	nodes := []agent.Node{
		{ID: "s1xxxxxxxx", Name: "session:s1xxxxxx", Status: agent.StatusRunning, GroupID: "g1"},
		{ID: "s2xxxxxxxx", Name: "session:s2xxxxxx", Status: agent.StatusRunning, GroupID: "g1"},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	if m.focusedNode() != nil {
		t.Error("expected nil from focusedNode when cursor is on a group header")
	}
}

func TestFocusedNode_ReturnsCorrectNode(t *testing.T) {
	nodes := []agent.Node{
		{ID: "s1xxxxxxxx", Name: "session:s1xxxxxx", Status: agent.StatusRunning},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	node := m.focusedNode()
	if node == nil {
		t.Fatal("expected non-nil focusedNode when cursor is on a real node")
	}
	if node.ID != "s1xxxxxxxx" {
		t.Errorf("expected node ID s1xxxxxxxx, got %s", node.ID)
	}
}

func TestClampEventScroll_ClampsToMax(t *testing.T) {
	node := makeNodeWithEvents("s1xxxxxxxx", 5)
	m := newWithClock([]agent.Node{node}, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	m.eventScroll = 100
	m.clampEventScroll()
	if m.eventScroll != 0 {
		t.Errorf("expected eventScroll clamped to 0, got %d", m.eventScroll)
	}
}

func TestClampEventScroll_NilNode_SetsZero(t *testing.T) {
	m := newModel()
	m.eventScroll = 5
	m.clampEventScroll()
	if m.eventScroll != 0 {
		t.Errorf("expected eventScroll=0 when no node focused, got %d", m.eventScroll)
	}
}

func TestEventsContent_LipglossWidth_NotLen(t *testing.T) {
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{Type: "PreToolUse", Tool: "Agent", SessionID: "s1xxxxxxxx", Timestamp: time.Now()},
			},
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	content := next.(Model).eventsContent()
	if content == "" {
		t.Error("expected non-empty events content at narrow terminal width")
	}
	leftW := 40 * 35 / 100
	rightW := 40 - leftW
	innerW := rightW - 2
	for _, line := range strings.Split(content, "\n") {
		if lw := lipgloss.Width(line); lw > innerW {
			t.Errorf("line exceeds innerW=%d (lipgloss.Width=%d): %q", innerW, lw, line)
		}
	}
}

func TestEventsContent_CollapsedToolEvent_ShowsTruncatedInput(t *testing.T) {
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{
					Type:      "PreToolUse",
					Tool:      "Bash",
					Input:     `{"cmd":"ls -la"}`,
					SessionID: "s1xxxxxxxx",
					Timestamp: time.Now(),
				},
			},
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	content := next.(Model).eventsContent()

	if !strings.Contains(content, "PreToolUse") {
		t.Errorf("expected event type in collapsed view, got: %q", content)
	}
	if !strings.Contains(content, "Bash") {
		t.Errorf("expected tool name in collapsed view, got: %q", content)
	}
	if !strings.Contains(content, "ls -la") {
		t.Errorf("expected truncated input in collapsed view, got: %q", content)
	}
}

func TestEventsContent_ExpandedToolEvent_ShowsFullInputAndOutput(t *testing.T) {
	input := `{"cmd":"ls -la /home/user"}`
	output := "total 24\ndrwxr-xr-x 3 user user 4096 Jan 1 15:04 ."
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{
					Type:      "PostToolUse",
					Tool:      "Bash",
					Input:     input,
					Response:  output,
					SessionID: "s1xxxxxxxx",
					Timestamp: time.Now(),
				},
			},
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	// Click the event row to expand inline (cursor is already on the only event).
	leftW := 120 * 35 / 100
	next, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: leftW + 1, Y: 3})
	m = next.(Model)

	content := m.eventsContent()
	if !strings.Contains(content, "Input:") {
		t.Errorf("expected 'Input:' label in expanded view, got: %q", content)
	}
	if !strings.Contains(content, "ls -la /home/user") {
		t.Errorf("expected full input in expanded view, got: %q", content)
	}
	if !strings.Contains(content, "Output:") {
		t.Errorf("expected 'Output:' label in expanded view, got: %q", content)
	}
	if !strings.Contains(content, "total 24") {
		t.Errorf("expected output text in expanded view, got: %q", content)
	}
}

func TestEventsContent_ClickTogglesExpand(t *testing.T) {
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{
					Type:      "PreToolUse",
					Tool:      "Read",
					Input:     `{"file_path":"/tmp/foo"}`,
					SessionID: "s1xxxxxxxx",
					Timestamp: time.Now(),
				},
			},
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	leftW := 120 * 35 / 100
	click := tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: leftW + 1, Y: 3}

	// First click: expand.
	next, _ = m.Update(click)
	m = next.(Model)
	content := m.eventsContent()
	if !strings.Contains(content, "Input:") {
		t.Errorf("expected expanded after first click, got: %q", content)
	}

	// Reset double-click timer so the next click is treated as a single click.
	m.lastClickTime = time.Time{}

	// Second click: collapse.
	next, _ = m.Update(click)
	m = next.(Model)
	content = m.eventsContent()
	if strings.Contains(content, "Input:") {
		t.Errorf("expected collapsed after second click, got: %q", content)
	}
}

func TestEventsContent_NonToolEvent_ClickDoesNotExpandInline(t *testing.T) {
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{Type: "Stop", SessionID: "s1xxxxxxxx", Timestamp: time.Now()},
				{Type: "SubagentStop", SessionID: "s1xxxxxxxx", Timestamp: time.Now()},
			},
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	leftW := 120 * 35 / 100
	// Click each event row — non-tool events should not produce inline Input: content.
	for row := 1; row <= 2; row++ {
		next, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: leftW + 1, Y: row})
		m = next.(Model)
		content := m.eventsContent()
		if strings.Contains(content, "Input:") {
			t.Errorf("expected non-tool event row %d to have no inline expansion, got: %q", row, content)
		}
	}
}

func TestEventsContent_NotificationShowsArrowAndMessage(t *testing.T) {
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{Type: "Notification", Message: "Claude needs your attention", SessionID: "s1xxxxxxxx", Timestamp: time.Now()},
			},
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	content := next.(Model).eventsContent()

	if !strings.Contains(content, "Notification") {
		t.Errorf("expected 'Notification' in events content, got: %q", content)
	}
	if !strings.Contains(content, "►") {
		t.Errorf("expected '►' separator in Notification row, got: %q", content)
	}
	if !strings.Contains(content, "Claude needs your attention") {
		t.Errorf("expected notification message in events content, got: %q", content)
	}
}

func TestEventsContent_NotificationLongMessage_TruncatedAt40(t *testing.T) {
	msg := strings.Repeat("x", 50)
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{Type: "Notification", Message: msg, SessionID: "s1xxxxxxxx", Timestamp: time.Now()},
			},
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	// Wide terminal so panel width doesn't interfere with the 40-char truncation check.
	next, _ := m.Update(tea.WindowSizeMsg{Width: 300, Height: 40})
	content := next.(Model).eventsContent()

	// 50 x's truncated to 40 + "…" — the full 50-char string must not appear.
	if strings.Contains(content, msg) {
		t.Errorf("expected message truncated at 40 chars, but full message appeared: %q", content)
	}
	if !strings.Contains(content, strings.Repeat("x", 40)) {
		t.Errorf("expected first 40 chars of message in content, got: %q", content)
	}
}

func TestEventsContent_NotificationEmptyMessage_ShowsType(t *testing.T) {
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{Type: "Notification", Message: "", SessionID: "s1xxxxxxxx", Timestamp: time.Now()},
			},
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	content := next.(Model).eventsContent()

	if !strings.Contains(content, "Notification") {
		t.Errorf("expected 'Notification' in events content, got: %q", content)
	}
	// No inline message means no ► separator.
	if strings.Contains(content, "►") {
		t.Errorf("expected no '►' separator when message is empty, got: %q", content)
	}
}

// --- PermissionRequest absorption ---

func TestIsAbsorbedPermission_MatchingPair(t *testing.T) {
	ts := time.Now()
	events := []agent.Event{
		{Type: "PreToolUse", Tool: "Bash", Input: `{"command":"go test ./..."}`, Timestamp: ts},
		{Type: "PermissionRequest", Tool: "Bash", Input: `{"command":"go test ./..."}`, Timestamp: ts.Add(100 * time.Millisecond)},
	}
	if !isAbsorbedPermission(events, 1) {
		t.Error("expected PermissionRequest to be absorbed into matching PreToolUse")
	}
}

func TestIsAbsorbedPermission_DifferentTool(t *testing.T) {
	ts := time.Now()
	events := []agent.Event{
		{Type: "PreToolUse", Tool: "Read", Input: `{"file_path":"/tmp/a"}`, Timestamp: ts},
		{Type: "PermissionRequest", Tool: "Bash", Input: `{"file_path":"/tmp/a"}`, Timestamp: ts.Add(100 * time.Millisecond)},
	}
	if isAbsorbedPermission(events, 1) {
		t.Error("expected non-absorption when tool names differ")
	}
}

func TestIsAbsorbedPermission_DifferentInput(t *testing.T) {
	ts := time.Now()
	events := []agent.Event{
		{Type: "PreToolUse", Tool: "Bash", Input: `{"command":"ls"}`, Timestamp: ts},
		{Type: "PermissionRequest", Tool: "Bash", Input: `{"command":"rm -rf /"}`, Timestamp: ts.Add(100 * time.Millisecond)},
	}
	if isAbsorbedPermission(events, 1) {
		t.Error("expected non-absorption when inputs differ")
	}
}

func TestIsAbsorbedPermission_TooOld(t *testing.T) {
	ts := time.Now()
	events := []agent.Event{
		{Type: "PreToolUse", Tool: "Bash", Input: `{"command":"go test"}`, Timestamp: ts},
		{Type: "PermissionRequest", Tool: "Bash", Input: `{"command":"go test"}`, Timestamp: ts.Add(2 * time.Second)},
	}
	if isAbsorbedPermission(events, 1) {
		t.Error("expected non-absorption when timestamp gap > 1 second")
	}
}

func TestIsAbsorbedPermission_NotFirstEvent(t *testing.T) {
	ts := time.Now()
	events := []agent.Event{
		{Type: "PermissionRequest", Tool: "Bash", Input: `{"command":"x"}`, Timestamp: ts},
	}
	if isAbsorbedPermission(events, 0) {
		t.Error("expected non-absorption for the first event (no preceding PreToolUse)")
	}
}

func TestEventsContent_AbsorbedPermission_HidesPermissionRow(t *testing.T) {
	ts := time.Now()
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{Type: "PreToolUse", Tool: "Bash", Input: `{"command":"go test ./..."}`, SessionID: "s1xxxxxxxx", Timestamp: ts},
				{Type: "PermissionRequest", Tool: "Bash", Input: `{"command":"go test ./..."}`, SessionID: "s1xxxxxxxx", Timestamp: ts.Add(50 * time.Millisecond)},
			},
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	content := next.(Model).eventsContent()

	// Only one event row should appear — the PermissionRequest row is absorbed.
	// Total lines = 2 header lines (session label + divider) + 1 event row.
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines (2 header + 1 event row), got %d lines:\n%s", len(lines), content)
	}
	// The PreToolUse row should show ⚠ instead of ▶.
	if !strings.Contains(content, "⚠") {
		t.Errorf("expected ⚠ on PreToolUse row when permission is absorbed, got: %q", content)
	}
	if strings.Contains(content, "PermissionRequest") {
		t.Errorf("expected PermissionRequest label to be hidden, got: %q", content)
	}
}

func TestEventsContent_AbsorbedPermission_ShowsPreview(t *testing.T) {
	ts := time.Now()
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{Type: "PreToolUse", Tool: "Bash", Input: `{"command":"go test ./..."}`, SessionID: "s1xxxxxxxx", Timestamp: ts},
				{Type: "PermissionRequest", Tool: "Bash", Input: `{"command":"go test ./..."}`, SessionID: "s1xxxxxxxx", Timestamp: ts.Add(50 * time.Millisecond)},
			},
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 40})
	content := next.(Model).eventsContent()

	if !strings.Contains(content, "go test ./...") {
		t.Errorf("expected command preview on collapsed absorbed row, got: %q", content)
	}
}

func TestEventsContent_NonMatchingPermission_ShowsBothRows(t *testing.T) {
	ts := time.Now()
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{Type: "PreToolUse", Tool: "Bash", Input: `{"command":"ls"}`, SessionID: "s1xxxxxxxx", Timestamp: ts},
				{Type: "PermissionRequest", Tool: "Bash", Input: `{"command":"rm -rf /"}`, SessionID: "s1xxxxxxxx", Timestamp: ts.Add(50 * time.Millisecond)},
			},
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	content := next.(Model).eventsContent()

	if !strings.Contains(content, "PreToolUse") {
		t.Errorf("expected PreToolUse row, got: %q", content)
	}
	if !strings.Contains(content, "PermissionRequest") {
		t.Errorf("expected standalone PermissionRequest row when inputs differ, got: %q", content)
	}
}

func TestEventsContent_Notification_Expandable(t *testing.T) {
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{Type: "Notification", Message: "something happened", SessionID: "s1xxxxxxxx", Timestamp: time.Now()},
			},
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	// Click the row to expand inline.
	leftW := 120 * 35 / 100
	next, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: leftW + 1, Y: 3})
	m = next.(Model)

	content := m.eventsContent()
	if !strings.Contains(content, "Message:") {
		t.Errorf("expected 'Message:' label in expanded Notification, got: %q", content)
	}
	if !strings.Contains(content, "something happened") {
		t.Errorf("expected full message text in expanded Notification, got: %q", content)
	}
}

func TestEventsContent_Notification_ExpandedShowsDownArrow(t *testing.T) {
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{Type: "Notification", Message: "hello", SessionID: "s1xxxxxxxx", Timestamp: time.Now()},
			},
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	// Collapsed — should show ► not ▼.
	content := m.eventsContent()
	if !strings.Contains(content, "►") {
		t.Errorf("expected '►' when collapsed, got: %q", content)
	}

	// Click to expand inline.
	leftW := 120 * 35 / 100
	next, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: leftW + 1, Y: 3})
	m = next.(Model)

	// Expanded — ► replaced by ▼, message no longer inline.
	content = m.eventsContent()
	if !strings.Contains(content, "▼") {
		t.Errorf("expected '▼' when expanded, got: %q", content)
	}
	if strings.Contains(content, "►") {
		t.Errorf("expected '►' to be absent when expanded, got: %q", content)
	}
}

func TestEventsContent_Notification_EnterOpensModal(t *testing.T) {
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{Type: "Notification", Message: "ping", SessionID: "s1xxxxxxxx", Timestamp: time.Now()},
			},
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)

	if m.modalOpen {
		t.Fatal("expected modal closed before enter")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if !m.modalOpen {
		t.Error("expected modal open after enter in events panel")
	}
	// eventsContent is still intact (not expanded inline).
	if strings.Contains(m.eventsContent(), "Message:") {
		t.Error("expected no inline expansion — modal handles full content")
	}
}

func TestEventsContent_PermissionRequestShowsWarningAndTool(t *testing.T) {
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{Type: "PermissionRequest", Tool: "Bash", SessionID: "s1xxxxxxxx", Timestamp: time.Now()},
			},
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	content := next.(Model).eventsContent()

	if !strings.Contains(content, "PermissionRequest") {
		t.Errorf("expected 'PermissionRequest' in events content, got: %q", content)
	}
	if !strings.Contains(content, "⚠") {
		t.Errorf("expected '⚠' warning indicator in PermissionRequest row, got: %q", content)
	}
	if !strings.Contains(content, "Bash") {
		t.Errorf("expected tool name after warning indicator, got: %q", content)
	}
}

func TestEventsContent_PermissionRequestNoTool_FallsBackToMessage(t *testing.T) {
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{Type: "PermissionRequest", Tool: "", Message: "approve network access", SessionID: "s1xxxxxxxx", Timestamp: time.Now()},
			},
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	content := next.(Model).eventsContent()

	if !strings.Contains(content, "⚠") {
		t.Errorf("expected '⚠' in PermissionRequest row, got: %q", content)
	}
	if !strings.Contains(content, "approve network access") {
		t.Errorf("expected message as fallback description, got: %q", content)
	}
}

func TestEventsContent_PermissionRequest_Expandable(t *testing.T) {
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{
					Type:      "PermissionRequest",
					Tool:      "Bash",
					Input:     `{"command":"go test ./..."}`,
					SessionID: "s1xxxxxxxx",
					Timestamp: time.Now(),
				},
			},
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	// Click the row to expand inline.
	leftW := 120 * 35 / 100
	next, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: leftW + 1, Y: 3})
	m = next.(Model)

	content := m.eventsContent()
	if !strings.Contains(content, "Input:") {
		t.Errorf("expected PermissionRequest to be expandable (show Input:), got: %q", content)
	}
	if !strings.Contains(content, "go test ./...") {
		t.Errorf("expected full command in expanded view, got: %q", content)
	}
}

func TestEventsContent_PermissionRequest_Bash_ShowsCommand(t *testing.T) {
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{
					Type:      "PermissionRequest",
					Tool:      "Bash",
					Input:     `{"command":"go test ./..."}`,
					SessionID: "s1xxxxxxxx",
					Timestamp: time.Now(),
				},
			},
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	content := next.(Model).eventsContent()

	if !strings.Contains(content, "►") {
		t.Errorf("expected '►' separator in collapsed PermissionRequest row, got: %q", content)
	}
	if !strings.Contains(content, "go test ./...") {
		t.Errorf("expected command preview in collapsed row, got: %q", content)
	}
}

func TestEventsContent_PermissionRequest_Read_ShowsFilePathWithTilde(t *testing.T) {
	home := os.Getenv("HOME")
	if home == "" {
		t.Skip("HOME not set")
	}
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{
					Type:      "PermissionRequest",
					Tool:      "Read",
					Input:     `{"file_path":"` + home + `/orchard/main.go"}`,
					SessionID: "s1xxxxxxxx",
					Timestamp: time.Now(),
				},
			},
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 40})
	content := next.(Model).eventsContent()

	if !strings.Contains(content, "~/orchard/main.go") {
		t.Errorf("expected file_path with ~ prefix in collapsed row, got: %q", content)
	}
}

func TestEventsContent_PermissionRequest_Write_ShowsFilePath(t *testing.T) {
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{
					Type:      "PermissionRequest",
					Tool:      "Write",
					Input:     `{"file_path":"/tmp/output.go","content":"package main"}`,
					SessionID: "s1xxxxxxxx",
					Timestamp: time.Now(),
				},
			},
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	content := next.(Model).eventsContent()

	if !strings.Contains(content, "/tmp/output.go") {
		t.Errorf("expected file_path in collapsed row for Write, got: %q", content)
	}
}

func TestEventsContent_PermissionRequest_UnknownTool_ShowsTruncatedInput(t *testing.T) {
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{
					Type:      "PermissionRequest",
					Tool:      "WebFetch",
					Input:     `{"url":"https://example.com","prompt":"summarize"}`,
					SessionID: "s1xxxxxxxx",
					Timestamp: time.Now(),
				},
			},
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 300, Height: 40})
	content := next.(Model).eventsContent()

	if !strings.Contains(content, "►") {
		t.Errorf("expected '►' separator for unknown tool PermissionRequest, got: %q", content)
	}
	// Raw JSON truncated to 40 chars — starts with {"url":
	if !strings.Contains(content, `{"url":`) {
		t.Errorf("expected truncated raw input for unknown tool, got: %q", content)
	}
}

func TestEventsContent_ExpandedLongInput_LineWrapped(t *testing.T) {
	// Input longer than the panel width so it must wrap.
	longInput := `{"path":"` + strings.Repeat("a", 200) + `"}`
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{
					Type:      "PreToolUse",
					Tool:      "Read",
					Input:     longInput,
					SessionID: "s1xxxxxxxx",
					Timestamp: time.Now(),
				},
			},
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	m = next.(Model)

	// Click the row to expand inline (cursor is on the only event).
	leftW := 80 * 35 / 100
	next, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: leftW + 1, Y: 3})
	m = next.(Model)

	rightW := 80 - leftW
	innerW := rightW - 2

	content := m.eventsContent()
	for _, line := range strings.Split(content, "\n") {
		if lw := lipgloss.Width(line); lw > innerW {
			t.Errorf("expanded line exceeds innerW=%d (lipgloss.Width=%d): %q", innerW, lw, line)
		}
	}
	// Content must include the long input split across multiple lines (not truncated).
	if !strings.Contains(content, strings.Repeat("a", 10)) {
		t.Errorf("expected long input content in expanded view, got: %q", content)
	}
}

// --- Detail modal ---

func makeModalModel(eventType, tool, input, response, message string) Model {
	e := agent.Event{
		Type:      eventType,
		Tool:      tool,
		Input:     input,
		Response:  response,
		Message:   message,
		SessionID: "s1xxxxxxxx",
		Timestamp: time.Now(),
	}
	nodes := []agent.Node{
		{ID: "s1xxxxxxxx", Name: "session:s1xxxxxx", Status: agent.StatusRunning, Events: []agent.Event{e}},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return next.(Model)
}

func TestModal_EnterOpensModal(t *testing.T) {
	m := makeModalModel("PreToolUse", "Bash", `{"command":"ls"}`, "", "")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	if m.modalOpen {
		t.Fatal("expected modal closed before enter")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if !m.modalOpen {
		t.Error("expected modalOpen after enter in events panel")
	}
}

func TestModal_EnterOpensForAnyEventType(t *testing.T) {
	for _, evType := range []string{"Stop", "SubagentStop", "Notification", "PermissionRequest", "PostToolUse"} {
		m := makeModalModel(evType, "", "", "", "msg")
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		m = next.(Model)
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m = next.(Model)
		if !m.modalOpen {
			t.Errorf("expected modal open after enter for event type %q", evType)
		}
	}
}

func TestModal_ClosesOnEsc(t *testing.T) {
	m := makeModalModel("PreToolUse", "Bash", `{"command":"ls"}`, "", "")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if !m.modalOpen {
		t.Fatal("expected modal open")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.modalOpen {
		t.Error("expected modal closed after esc")
	}
}

func TestModal_ClosesOnQ_DoesNotQuit(t *testing.T) {
	m := makeModalModel("PreToolUse", "Bash", `{"command":"ls"}`, "", "")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)

	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	m = model.(Model)
	if m.modalOpen {
		t.Error("expected modal closed after q")
	}
	if cmd != nil {
		t.Error("expected nil cmd (no quit) when q closes modal")
	}
}

func TestModal_QQuitsWhenClosed(t *testing.T) {
	m := makeModalModel("PreToolUse", "Bash", `{"command":"ls"}`, "", "")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Error("expected quit cmd when modal is closed and q is pressed")
	}
}

func TestModal_NavigatesRight(t *testing.T) {
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{Type: "PreToolUse", Tool: "Bash", Input: `{"command":"a"}`, SessionID: "s1xxxxxxxx", Timestamp: time.Now()},
				{Type: "PreToolUse", Tool: "Read", Input: `{"command":"b"}`, SessionID: "s1xxxxxxxx", Timestamp: time.Now()},
			},
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	// Move cursor to first event.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = next.(Model)
	if m.eventCursor != 0 {
		t.Fatalf("expected eventCursor=0, got %d", m.eventCursor)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)

	before := m.eventCursor
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = next.(Model)
	if m.eventCursor != before+1 {
		t.Errorf("expected eventCursor=%d after →, got %d", before+1, m.eventCursor)
	}
	if m.modalScroll != 0 {
		t.Error("expected modalScroll reset to 0 on navigation")
	}
}

func TestModal_NavigatesLeft(t *testing.T) {
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{Type: "PreToolUse", Tool: "Bash", Input: `{"command":"a"}`, SessionID: "s1xxxxxxxx", Timestamp: time.Now()},
				{Type: "PreToolUse", Tool: "Read", Input: `{"command":"b"}`, SessionID: "s1xxxxxxxx", Timestamp: time.Now()},
			},
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	// Cursor starts at last event (index 1); open modal then navigate left.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)

	before := m.eventCursor
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = next.(Model)
	if m.eventCursor != before-1 {
		t.Errorf("expected eventCursor=%d after ←, got %d", before-1, m.eventCursor)
	}
}

func TestModal_ScrollDownAndUp(t *testing.T) {
	m := makeModalModel("PreToolUse", "Bash", `{"command":"ls"}`, "", "")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = next.(Model)
	if m.modalScroll != 1 {
		t.Errorf("expected modalScroll=1 after j, got %d", m.modalScroll)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = next.(Model)
	if m.modalScroll != 0 {
		t.Errorf("expected modalScroll=0 after k, got %d", m.modalScroll)
	}
	// k at top should not go negative.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = next.(Model)
	if m.modalScroll < 0 {
		t.Errorf("expected modalScroll >= 0, got %d", m.modalScroll)
	}
}

func TestModal_PositionCounter(t *testing.T) {
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{Type: "PreToolUse", Tool: "Bash", Input: `{"command":"a"}`, SessionID: "s1xxxxxxxx", Timestamp: time.Now()},
				{Type: "PreToolUse", Tool: "Read", Input: `{"command":"b"}`, SessionID: "s1xxxxxxxx", Timestamp: time.Now()},
				{Type: "PreToolUse", Tool: "Edit", Input: `{"command":"c"}`, SessionID: "s1xxxxxxxx", Timestamp: time.Now()},
			},
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	// Cursor at last event (index 2).
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)

	cur, tot := m.modalEventPosition()
	if tot != 3 {
		t.Errorf("expected total=3, got %d", tot)
	}
	if cur != 3 {
		t.Errorf("expected current=3 (last event), got %d", cur)
	}
}

func TestModal_PermissionRequestShowsWarningHeader(t *testing.T) {
	m := newWithClock([]agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{Type: "PermissionRequest", Tool: "Bash", Input: `{"command":"rm -rf /"}`, SessionID: "s1xxxxxxxx", Timestamp: time.Now()},
			},
		},
	}, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)

	node := m.focusedNode()
	if node == nil {
		t.Fatal("expected focused node")
	}
	content := strings.Join(m.buildModalContent(node.Events[0], node, "", 80), "\n")
	if !strings.Contains(content, "⚠") {
		t.Errorf("expected ⚠ in PermissionRequest modal content, got: %q", content)
	}
	if !strings.Contains(content, "Awaiting approval") {
		t.Errorf("expected 'Awaiting approval' header, got: %q", content)
	}
}

func TestModal_StopShowsDuration(t *testing.T) {
	start := time.Now().Add(-5 * time.Second)
	stop := time.Now()
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{Type: "PreToolUse", Tool: "Bash", Input: `{"command":"ls"}`, SessionID: "s1xxxxxxxx", Timestamp: start},
				{Type: "Stop", SessionID: "s1xxxxxxxx", Timestamp: stop},
			},
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	node := m.focusedNode()
	if node == nil {
		t.Fatal("expected focused node")
	}
	stopEvent := node.Events[1]
	content := strings.Join(m.buildModalContent(stopEvent, node, "", 80), "\n")
	if !strings.Contains(content, "Duration:") {
		t.Errorf("expected 'Duration:' in Stop modal content, got: %q", content)
	}
}

func TestModal_FullContentNotTruncated(t *testing.T) {
	longVal := strings.Repeat("x", 100)
	input := `{"command":"` + longVal + `"}`
	m := makeModalModel("PreToolUse", "Bash", input, "", "")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)

	node := m.focusedNode()
	if node == nil {
		t.Fatal("expected focused node")
	}
	content := strings.Join(m.buildModalContent(node.Events[0], node, "", 80), "\n")
	// The value is wrapped across lines — verify no ellipsis truncation and that
	// more consecutive x's appear than the old 60-rune truncation limit would allow.
	if strings.Contains(content, "x…") {
		t.Errorf("expected no truncation in modal content, got: %q", content)
	}
	if !strings.Contains(content, strings.Repeat("x", 61)) {
		t.Errorf("expected >60 consecutive x's in modal content (full value), got: %q", content)
	}
}

// --- Edit/Write diff view ---

func TestModal_EditDiff_ShowsFilePathAndDiffBlock(t *testing.T) {
	input := `{"file_path":"/tmp/foo.go","old_string":"old line","new_string":"new line","replace_all":false}`
	lines := formatInputAsDiff(input, "", 80)
	joined := strings.Join(lines, "\n")

	if !strings.Contains(joined, "file_path: /tmp/foo.go") {
		t.Errorf("expected file_path in diff output, got: %q", joined)
	}
	if !strings.Contains(joined, "replace_all: false") {
		t.Errorf("expected replace_all in diff output, got: %q", joined)
	}
	if !strings.Contains(joined, "- old line") {
		t.Errorf("expected removed line with '- ' prefix, got: %q", joined)
	}
	if !strings.Contains(joined, "+ new line") {
		t.Errorf("expected added line with '+ ' prefix, got: %q", joined)
	}
	// old_string and new_string keys must not appear as raw key-value pairs.
	if strings.Contains(joined, "old_string:") {
		t.Errorf("expected old_string key to be absent (replaced by diff), got: %q", joined)
	}
	if strings.Contains(joined, "new_string:") {
		t.Errorf("expected new_string key to be absent (replaced by diff), got: %q", joined)
	}
}

func TestModal_EditDiff_PureInsertion_OnlyPlusLines(t *testing.T) {
	input := `{"file_path":"/tmp/new.go","old_string":"","new_string":"line one\nline two"}`
	lines := formatInputAsDiff(input, "", 80)
	joined := strings.Join(lines, "\n")

	if strings.Contains(joined, "- ") {
		t.Errorf("expected no removal lines for pure insertion, got: %q", joined)
	}
	if !strings.Contains(joined, "+ line one") {
		t.Errorf("expected added lines, got: %q", joined)
	}
}

func TestModal_EditDiff_PureDeletion_OnlyMinusLines(t *testing.T) {
	input := `{"file_path":"/tmp/old.go","old_string":"gone","new_string":""}`
	lines := formatInputAsDiff(input, "", 80)
	joined := strings.Join(lines, "\n")

	if !strings.Contains(joined, "- gone") {
		t.Errorf("expected removal line, got: %q", joined)
	}
	if strings.Contains(joined, "+ ") {
		t.Errorf("expected no addition lines for pure deletion, got: %q", joined)
	}
}

func TestModal_EditDiff_CapsAt20Lines(t *testing.T) {
	// Build an old_string with 30 lines.
	var oldLines []string
	for i := 0; i < 30; i++ {
		oldLines = append(oldLines, fmt.Sprintf("line %d", i))
	}
	old := strings.Join(oldLines, "\n")
	input := `{"file_path":"/tmp/f.go","old_string":` + jsonString(old) + `,"new_string":""}`
	lines := formatInputAsDiff(input, "", 80)
	joined := strings.Join(lines, "\n")

	// Must contain the overflow indicator.
	if !strings.Contains(joined, "more lines") {
		t.Errorf("expected overflow indicator for 30-line diff, got: %q", joined)
	}
}

func TestModal_WriteDiff_UsesContentField(t *testing.T) {
	input := `{"file_path":"/tmp/new.go","content":"package main\n\nfunc main() {}"}`
	lines := formatInputAsDiff(input, "", 80)
	joined := strings.Join(lines, "\n")

	if !strings.Contains(joined, "+ package main") {
		t.Errorf("expected Write content shown as added lines, got: %q", joined)
	}
	if strings.Contains(joined, "- ") {
		t.Errorf("expected no removal lines for Write, got: %q", joined)
	}
}

func TestModal_EditDiff_HomeDirReplaced(t *testing.T) {
	home := os.Getenv("HOME")
	if home == "" {
		t.Skip("HOME not set")
	}
	input := `{"file_path":"` + home + `/project/main.go","old_string":"x","new_string":"y"}`
	lines := formatInputAsDiff(input, home, 80)
	joined := strings.Join(lines, "\n")

	if !strings.Contains(joined, "~/project/main.go") {
		t.Errorf("expected ~ prefix in file_path, got: %q", joined)
	}
}

func TestModal_NonEditTool_UsesKeyValue(t *testing.T) {
	// Bash events should still use the standard key-value display, not the diff.
	m := makeModalModel("PreToolUse", "Bash", `{"command":"ls -la"}`, "", "")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)

	node := m.focusedNode()
	if node == nil {
		t.Fatal("expected focused node")
	}
	content := strings.Join(m.buildModalContent(node.Events[0], node, "", 80), "\n")
	if !strings.Contains(content, "Input:") {
		t.Errorf("expected 'Input:' label for non-Edit tool, got: %q", content)
	}
	if strings.Contains(content, "─────") {
		t.Errorf("expected no diff divider for non-Edit tool, got: %q", content)
	}
}

// jsonString encodes s as a JSON string literal.
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func TestModal_DoubleClickOpens(t *testing.T) {
	m := makeModalModel("PreToolUse", "Bash", `{"command":"ls"}`, "", "")
	leftW := 120 * 35 / 100
	click := tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: leftW + 1, Y: 3}

	// First click: select / expand inline.
	next, _ := m.Update(click)
	m = next.(Model)
	if m.modalOpen {
		t.Fatal("expected modal closed after first click")
	}

	// Second click within 400ms: open modal.
	next, _ = m.Update(click)
	m = next.(Model)
	if !m.modalOpen {
		t.Error("expected modal open after double-click")
	}
}

// SkillTrigger tests

func TestEventsContent_SkillTrigger_ShowsSkillIcon(t *testing.T) {
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{
					Type:      "SkillTrigger",
					Tool:      "build",
					Input:     `{"skill":"build","args":""}`,
					SessionID: "s1xxxxxxxx",
					Timestamp: time.Now(),
				},
			},
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	content := next.(Model).eventsContent()

	if !strings.Contains(content, "SkillTrigger") {
		t.Errorf("expected 'SkillTrigger' event type in content, got: %q", content)
	}
	if !strings.Contains(content, "⚡") {
		t.Errorf("expected '⚡' icon in skill trigger row, got: %q", content)
	}
	if !strings.Contains(content, "build") {
		t.Errorf("expected skill name 'build' in skill trigger row, got: %q", content)
	}
}

func TestEventsContent_SkillTrigger_NotExpandable(t *testing.T) {
	nodes := []agent.Node{
		{
			ID:     "s1xxxxxxxx",
			Name:   "session:s1xxxxxx",
			Status: agent.StatusRunning,
			Events: []agent.Event{
				{
					Type:      "SkillTrigger",
					Tool:      "check",
					Input:     `{"skill":"check","args":""}`,
					SessionID: "s1xxxxxxxx",
					Timestamp: time.Now(),
				},
			},
		},
	}
	m := newWithClock(nodes, nil, nil, time.Time{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	content := m.eventsContent()

	// No expand/collapse arrows — skill triggers are not expandable.
	if strings.Contains(content, "▶") {
		t.Errorf("expected no '▶' on SkillTrigger row, got: %q", content)
	}
	if strings.Contains(content, "▼") {
		t.Errorf("expected no '▼' on SkillTrigger row, got: %q", content)
	}

	// Clicking should not produce an Input: block.
	leftW := 120 * 35 / 100
	click := tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: leftW + 1, Y: 3}
	next, _ = m.Update(click)
	content = next.(Model).eventsContent()
	if strings.Contains(content, "Input:") {
		t.Errorf("expected SkillTrigger to be non-expandable (no 'Input:'), got: %q", content)
	}
}

func TestApplyEvent_SkillTrigger_SetsRunning(t *testing.T) {
	tree := agent.NewTree()
	tree.AddNode(agent.NewNode("sess1"))

	tree.ApplyEvent(agent.Event{
		Type:      "SkillTrigger",
		SessionID: "sess1",
		Tool:      "build",
	})

	node := tree.Nodes["sess1"]
	if node.Status != agent.StatusRunning {
		t.Errorf("expected StatusRunning after SkillTrigger, got %v", node.Status)
	}
	if len(node.Events) != 1 {
		t.Fatalf("expected 1 event stored, got %d", len(node.Events))
	}
	if node.Events[0].Type != "SkillTrigger" {
		t.Errorf("expected stored event type 'SkillTrigger', got %q", node.Events[0].Type)
	}
	if node.Events[0].Tool != "build" {
		t.Errorf("expected stored event tool 'build', got %q", node.Events[0].Tool)
	}
}

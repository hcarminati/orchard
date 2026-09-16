package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestHelpOverlay_OpenOnQuestionMark verifies ? toggles helpOpen.
func TestHelpOverlay_OpenOnQuestionMark(t *testing.T) {
	m := newModel()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	got := next.(Model)
	if !got.helpOpen {
		t.Error("expected helpOpen=true after pressing ?")
	}
}

// TestHelpOverlay_ClosedByEsc verifies esc closes the help overlay.
func TestHelpOverlay_ClosedByEsc(t *testing.T) {
	m := newModel()
	m.helpOpen = true
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if next.(Model).helpOpen {
		t.Error("expected helpOpen=false after esc")
	}
}

// TestHelpOverlay_ClosedByQ verifies q closes the help overlay.
func TestHelpOverlay_ClosedByQ(t *testing.T) {
	m := newModel()
	m.helpOpen = true
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if next.(Model).helpOpen {
		t.Error("expected helpOpen=false after q")
	}
}

// TestHelpOverlay_ClosedByQuestionMark verifies ? closes the help overlay when open.
func TestHelpOverlay_ClosedByQuestionMark(t *testing.T) {
	m := newModel()
	m.helpOpen = true
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	if next.(Model).helpOpen {
		t.Error("expected helpOpen=false after second ?")
	}
}

// TestHelpOverlay_InterceptsKeys verifies that other keys are absorbed when help is open.
func TestHelpOverlay_InterceptsKeys(t *testing.T) {
	m := newModel()
	m.helpOpen = true
	// Press 'j' — should not change cursor.
	before := m.cursor
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if next.(Model).cursor != before {
		t.Error("cursor should not move while help overlay is open")
	}
}

// TestRenderHelpOverlay_ContainsSections verifies the overlay contains key section names.
func TestRenderHelpOverlay_ContainsSections(t *testing.T) {
	m := newModel()
	m.width = 120
	m.height = 40
	overlay := m.renderHelpOverlay(39)

	for _, section := range []string{"Navigation", "Events", "Tree", "Session Management"} {
		if !strings.Contains(overlay, section) {
			t.Errorf("help overlay missing section %q", section)
		}
	}
}

// TestRenderHelpOverlay_ContainsBindings verifies key bindings are shown.
func TestRenderHelpOverlay_ContainsBindings(t *testing.T) {
	m := newModel()
	m.width = 120
	m.height = 40
	overlay := m.renderHelpOverlay(39)

	for _, key := range []string{"j / ↓", "tab", "enter", "q / ctrl+c"} {
		if !strings.Contains(overlay, key) {
			t.Errorf("help overlay missing binding %q", key)
		}
	}
}

// TestRenderHelpOverlay_ContainsCurrentState verifies the live state section.
func TestRenderHelpOverlay_ContainsCurrentState(t *testing.T) {
	m := newModel()
	m.width = 120
	m.height = 40
	overlay := m.renderHelpOverlay(39)

	if !strings.Contains(overlay, "Current State") {
		t.Error("help overlay missing 'Current State' section")
	}
	if !strings.Contains(overlay, "filter:") {
		t.Error("help overlay missing filter state")
	}
	if !strings.Contains(overlay, "panel:") {
		t.Error("help overlay missing panel state")
	}
}

// TestView_HelpOverlay_ShowsInView verifies the help overlay appears in View output.
func TestView_HelpOverlay_ShowsInView(t *testing.T) {
	m := newModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)
	m.helpOpen = true

	view := m.View()
	if !strings.Contains(view, "Help") {
		t.Error("expected Help overlay in View output")
	}
	if !strings.Contains(view, "Navigation") {
		t.Error("expected Navigation section in View output")
	}
}

// TestRenderHelpFooterBar_ContainsHint verifies the help footer bar content.
func TestRenderHelpFooterBar_ContainsHint(t *testing.T) {
	m := newModel()
	m.width = 120
	bar := m.renderHelpFooterBar()
	if !strings.Contains(bar, "close help") {
		t.Errorf("expected 'close help' in help footer bar, got: %s", bar)
	}
}

// TestFooter_ShowsHelpBinding verifies the normal footer shows ? help.
func TestFooter_ShowsHelpBinding(t *testing.T) {
	m := newModel()
	m.width = 120
	footer := m.renderFooter()
	if !strings.Contains(footer, "?") {
		t.Errorf("expected '?' in footer, got: %s", footer)
	}
}

// TestMCPTab_Exists verifies the MCP tab is registered.
func TestMCPTab_Exists(t *testing.T) {
	found := false
	for _, tab := range rightTabs {
		if tab == tabMCP {
			found = true
			break
		}
	}
	if !found {
		t.Error("tabMCP not present in rightTabs")
	}
}

// TestMCPTab_Label verifies the MCP tab label.
func TestMCPTab_Label(t *testing.T) {
	if got := tabMCP.label(); got != "MCP" {
		t.Errorf("tabMCP.label() = %q, want %q", got, "MCP")
	}
}

// TestMCPContent_NoServers verifies mcpContent shows a message when no servers are configured.
// Since we can't easily mock the home directory in the ui package test, we simply verify
// the function returns a non-empty string without panicking.
func TestMCPContent_ReturnsString(t *testing.T) {
	m := newModel()
	m.width = 120
	m.height = 40
	content := m.mcpContent()
	if content == "" {
		t.Error("mcpContent should return non-empty string")
	}
}

// TestHelpSections_AllPresent verifies every section has at least one binding.
func TestHelpSections_AllPresent(t *testing.T) {
	sections := helpSections()
	if len(sections) == 0 {
		t.Error("helpSections should return at least one section")
	}
	for _, s := range sections {
		if s.title == "" {
			t.Error("section has empty title")
		}
		if len(s.bindings) == 0 {
			t.Errorf("section %q has no bindings", s.title)
		}
	}
}

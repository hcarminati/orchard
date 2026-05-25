package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hcarminati/orchard/internal/agent"
)

// makeModelV08 creates a test Model with v0.8 config options set.
func makeModelV08(nodes []agent.Node, watchdogMinutes int, costAlert float64) Model {
	m := newWithClock(nodes, nil, nil, time.Time{})
	m.watchdogMinutes = watchdogMinutes
	m.costAlert = costAlert
	return m
}

// ---- watchdog badge tests ----

func TestWatchdogBadge_Zero(t *testing.T) {
	m := makeModelV08(nil, 1, 0)
	badge := m.watchdogBadge("sess1")
	if badge != "" {
		t.Errorf("expected empty badge for zero fires, got %q", badge)
	}
}

func TestWatchdogBadge_OneFire(t *testing.T) {
	m := makeModelV08(nil, 1, 0)
	m.watchdogFired["sess1"] = 1
	badge := m.watchdogBadge("sess1")
	// Should contain ⏱ (rendered with ANSI colors, just check the symbol is present)
	if !strings.Contains(badge, "⏱") {
		t.Errorf("expected ⏱ badge for one fire, got %q", badge)
	}
	// One fire should NOT contain two ⏱
	plain := strings.ReplaceAll(badge, "\033[", "")
	if strings.Count(plain, "⏱") >= 2 {
		t.Errorf("expected single ⏱ for one fire, got %q", badge)
	}
}

func TestWatchdogBadge_TwoFires(t *testing.T) {
	m := makeModelV08(nil, 1, 0)
	m.watchdogFired["sess1"] = 2
	badge := m.watchdogBadge("sess1")
	if !strings.Contains(badge, "⏱") {
		t.Errorf("expected ⏱⏱ badge for two fires, got %q", badge)
	}
	plain := strings.ReplaceAll(badge, "\033[", "")
	if strings.Count(plain, "⏱") < 2 {
		t.Errorf("expected double ⏱ for two fires, got %q", badge)
	}
}

func TestWatchdogBadge_MoreThanTwo(t *testing.T) {
	m := makeModelV08(nil, 1, 0)
	m.watchdogFired["sess1"] = 5
	badge := m.watchdogBadge("sess1")
	// Should still show double badge for any count ≥ 2
	if !strings.Contains(badge, "⏱") {
		t.Errorf("expected ⏱⏱ badge for count>2, got %q", badge)
	}
}

// ---- search tests ----

func TestSearch_OpenClose(t *testing.T) {
	m := newWithClock(nil, nil, nil, time.Time{})
	m.width = 120
	m.height = 40
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = m2.(Model)

	// Open search with /
	m3, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m2Model := m3.(Model)
	if !m2Model.searchOpen {
		t.Error("expected searchOpen after pressing /")
	}

	// Close with esc
	m4, _ := m2Model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m3Model := m4.(Model)
	if m3Model.searchOpen {
		t.Error("expected searchOpen=false after esc")
	}
	if m3Model.searchQuery != "" {
		t.Errorf("expected empty searchQuery after esc, got %q", m3Model.searchQuery)
	}
}

func TestSearch_TypeQuery(t *testing.T) {
	m := newWithClock(nil, nil, nil, time.Time{})
	m.width = 120
	m.height = 40
	m.searchOpen = true

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	m2Model := m2.(Model)
	m3, _ := m2Model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m3Model := m3.(Model)
	m4, _ := m3Model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	m4Model := m4.(Model)
	if m4Model.searchQuery != "bas" {
		t.Errorf("expected searchQuery=bas, got %q", m4Model.searchQuery)
	}
}

func TestSearch_Backspace(t *testing.T) {
	m := newWithClock(nil, nil, nil, time.Time{})
	m.searchOpen = true
	m.searchQuery = "bash"

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m2Model := m2.(Model)
	if m2Model.searchQuery != "bas" {
		t.Errorf("expected searchQuery=bas after backspace, got %q", m2Model.searchQuery)
	}
}

func TestSearch_FooterShowsSearchBar(t *testing.T) {
	m := newWithClock(nil, nil, nil, time.Time{})
	m.width = 120
	m.height = 40
	m.searchOpen = true
	m.searchQuery = "bash"

	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = m2.(Model)
	m.searchOpen = true
	m.searchQuery = "bash"
	footer := m.renderFooter()
	if !strings.Contains(footer, "bash") {
		t.Errorf("expected footer to show search query 'bash', got %q", footer)
	}
	if !strings.Contains(footer, "/") {
		t.Errorf("expected footer to show / prefix in search bar")
	}
}

// ---- cost alert tests ----

func TestCostAlertBanner_NoAlert(t *testing.T) {
	m := makeModelV08(nil, 0, 0)
	// costAlert = 0 means disabled
	banner := m.renderCostAlertBanner()
	if banner != "" {
		t.Errorf("expected no banner when costAlert=0, got %q", banner)
	}
}

func TestCostAlertBanner_NoCostExceeded(t *testing.T) {
	n := agent.Node{ID: "sess1", Name: "root", Status: agent.StatusRunning}
	m := makeModelV08([]agent.Node{n}, 0, 100.0)
	m.width = 120
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = m2.(Model)
	m.costAlert = 100.0
	// No usage on node, so cost=0, no alert
	banner := m.renderCostAlertBanner()
	if banner != "" {
		t.Errorf("expected no banner when cost not exceeded, got %q", banner)
	}
}

func TestCostAlertBanner_Dismissed(t *testing.T) {
	n := agent.Node{
		ID: "sess1", Name: "root", Status: agent.StatusRunning,
		Usage: agent.Usage{InputTokens: 1_000_000, OutputTokens: 100_000},
		Model: agent.ModelOpus,
	}
	m := makeModelV08([]agent.Node{n}, 0, 1.0)
	m.width = 120
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = m2.(Model)
	m.costAlert = 1.0
	m.costAlertDismiss["sess1"] = true
	banner := m.renderCostAlertBanner()
	if banner != "" {
		t.Errorf("expected no banner when dismissed, got %q", banner)
	}
}

// ---- config store tests ----

func TestConfig_WatchdogAndCostAlertDefaults(t *testing.T) {
	// Test that new config fields parse correctly via direct construction.
	// (Full file-parse tests are in config package.)
	m := makeModelV08(nil, 5, 3.5)
	if m.watchdogMinutes != 5 {
		t.Errorf("expected watchdogMinutes=5, got %d", m.watchdogMinutes)
	}
	if m.costAlert != 3.5 {
		t.Errorf("expected costAlert=3.5, got %f", m.costAlert)
	}
}

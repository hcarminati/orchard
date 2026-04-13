package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestUpdate_RightTab_InitialIsZero(t *testing.T) {
	m := newModel()
	if m.activeRightTab != 0 {
		t.Errorf("expected activeRightTab=0 initially, got %d", m.activeRightTab)
	}
}

func TestUpdate_RightBracket_CyclesForward(t *testing.T) {
	m := newModel()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	got := next.(Model)
	want := (0 + 1) % len(rightTabs)
	if got.activeRightTab != want {
		t.Errorf("] cycle: got activeRightTab=%d, want %d", got.activeRightTab, want)
	}
}

func TestUpdate_LeftBracket_CyclesBackward(t *testing.T) {
	m := newModel()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[")})
	got := next.(Model)
	n := len(rightTabs)
	want := (0 - 1 + n) % n
	if got.activeRightTab != want {
		t.Errorf("[ cycle: got activeRightTab=%d, want %d", got.activeRightTab, want)
	}
}

func TestRightTabCycling_WrapsAtBothEnds(t *testing.T) {
	orig := rightTabs
	rightTabs = []rightTab{tabEvents, 1, 2}
	defer func() { rightTabs = orig }()

	m := newModel()

	for i, wantIdx := range []int{1, 2, 0} {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
		m = next.(Model)
		if m.activeRightTab != wantIdx {
			t.Errorf("] step %d: got activeRightTab=%d, want %d", i, m.activeRightTab, wantIdx)
		}
	}

	for i, wantIdx := range []int{2, 1, 0} {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[")})
		m = next.(Model)
		if m.activeRightTab != wantIdx {
			t.Errorf("[ step %d: got activeRightTab=%d, want %d", i, m.activeRightTab, wantIdx)
		}
	}
}

func TestTabStripTitle_LabelsPresent(t *testing.T) {
	m := newModel()
	strip := m.tabStripTitle(colorAccent)
	for _, tab := range rightTabs {
		if !strings.Contains(strip, tab.label()) {
			t.Errorf("tab strip title missing label %q", tab.label())
		}
	}
}

func TestTabStripTitle_ActiveTabDistinct(t *testing.T) {
	m := newModel()
	for range rightTabs {
		strip := m.tabStripTitle(colorAccent)
		for _, tab := range rightTabs {
			if !strings.Contains(strip, tab.label()) {
				t.Errorf("tab strip title missing label %q at activeRightTab=%d", tab.label(), m.activeRightTab)
			}
		}
		m.activeRightTab = (m.activeRightTab + 1) % len(rightTabs)
	}
}

func TestView_RightPanel_ShowsTabStrip(t *testing.T) {
	m := newModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := next.(Model).View()

	if !strings.Contains(view, "Events") {
		t.Error("expected 'Events' tab label in view output")
	}
}

func TestTabAtX_HitsFirstTab(t *testing.T) {
	m := newModel()
	for x := 0; x < len("[Events]"); x++ {
		idx, ok := m.tabAtX(x)
		if !ok {
			t.Errorf("x=%d: expected hit, got miss", x)
		}
		if idx != 0 {
			t.Errorf("x=%d: expected tab 0, got %d", x, idx)
		}
	}
}

func TestTabAtX_HitsSecondTab(t *testing.T) {
	m := newModel()
	start := len("[Events]") + 1
	for x := start; x < start+len("Files"); x++ {
		idx, ok := m.tabAtX(x)
		if !ok {
			t.Errorf("x=%d: expected hit on Files, got miss", x)
		}
		if idx != 1 {
			t.Errorf("x=%d: expected tab 1, got %d", x, idx)
		}
	}
}

func TestTabAtX_MissOnGap(t *testing.T) {
	m := newModel()
	_, ok := m.tabAtX(999)
	if ok {
		t.Error("expected miss for x past all tabs")
	}
}

func TestMouseClick_SwitchesTab(t *testing.T) {
	m := newModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	leftW := m.width * 35 / 100
	filesX := leftW + 2 + len("[Events]") + 1
	next, _ = m.Update(tea.MouseMsg{
		X:      filesX,
		Y:      0,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	if next.(Model).activeRightTab != 1 {
		t.Errorf("expected activeRightTab=1 after clicking Files, got %d", next.(Model).activeRightTab)
	}
}

package cmd

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestTUIModelViewRendersTwoPaneHeaders(t *testing.T) {
	m := newTUIModel()
	m.setSize(80, 24)

	view := m.View().Content
	lines := strings.Split(view, "\n")
	if len(lines) < 3 {
		t.Fatalf("expected at least 3 lines in view, got %d", len(lines))
	}

	if !strings.Contains(lines[0], "collab viewer") {
		t.Fatalf("view missing topbar title on first line: %q", view)
	}
	if !strings.Contains(view, "Tasks") {
		t.Fatalf("view missing Tasks header: %q", view)
	}
	if !strings.Contains(view, "Conversations") {
		t.Fatalf("view missing Conversations header: %q", view)
	}
	if !strings.Contains(lines[1], "│") {
		t.Fatalf("header should contain pane separator: %q", view)
	}
}

func TestTUIModelViewUsesContinuousVerticalDivider(t *testing.T) {
	m := newTUIModel()
	m.setSize(60, 10)

	view := m.View().Content
	lines := strings.Split(view, "\n")
	if len(lines) < 4 {
		t.Fatalf("expected enough lines for topbar+header+body, got %d", len(lines))
	}

	headerLine := lines[1]
	headerRunes := []rune(headerLine)
	dividerIdx := -1
	for i, r := range headerRunes {
		if r == '│' {
			dividerIdx = i
			break
		}
	}
	if dividerIdx == -1 {
		t.Fatalf("expected divider glyph in header line, got %q", headerLine)
	}

	for i := 2; i < len(lines); i++ {
		lineRunes := []rune(lines[i])
		if dividerIdx >= len(lineRunes) {
			t.Fatalf("line %d too short for divider index %d: %q", i, dividerIdx, lines[i])
		}
		if lineRunes[dividerIdx] != '│' && lineRunes[dividerIdx] != '┼' {
			t.Fatalf("line %d missing continuous divider at column %d: %q", i, dividerIdx, lines[i])
		}
	}
}

func TestTUIModelWindowResizeUpdatesDimensions(t *testing.T) {
	m := newTUIModel()

	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	resized, ok := next.(*tuiModel)
	if !ok {
		t.Fatalf("expected *tuiModel from update, got %T", next)
	}

	if resized.width != 120 || resized.height != 40 {
		t.Fatalf("dimensions = (%d,%d), want (120,40)", resized.width, resized.height)
	}

	left, right := resized.paneWidths(120)
	if left <= 0 || right <= 0 {
		t.Fatalf("pane widths must be positive, got left=%d right=%d", left, right)
	}
	if left+right+tuiPaneSeparatorWidth != 120 {
		t.Fatalf("pane widths should fill full width: left=%d right=%d sep=%d", left, right, tuiPaneSeparatorWidth)
	}
}

func TestTUIModelQuitKeysReturnQuitCommand(t *testing.T) {
	m := newTUIModel()

	quitKeys := []tea.KeyPressMsg{
		{Code: 'q', Text: "q"},
		{Code: tea.KeyEscape},
		{Code: 'c', Mod: tea.ModCtrl},
	}

	for _, key := range quitKeys {
		_, cmd := m.Update(key)
		if cmd == nil {
			t.Fatalf("key %q should return quit command", key.String())
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatalf("key %q should trigger tea.QuitMsg", key.String())
		}
	}
}

func TestTUIModelNonQuitKeyDoesNotExit(t *testing.T) {
	m := newTUIModel()

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if cmd != nil {
		t.Fatalf("non-quit key should not return command, got %T", cmd)
	}
}

func TestTUIModelViewHandlesTinyTerminalGracefully(t *testing.T) {
	m := newTUIModel()
	m.setSize(10, 3)

	view := m.View().Content
	if strings.TrimSpace(view) == "" {
		t.Fatalf("expected non-empty view for tiny terminal")
	}
}

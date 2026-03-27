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
	if len(lines) < 6 {
		t.Fatalf("expected framed layout lines, got %d", len(lines))
	}

	if !strings.Contains(lines[0], "collab viewer") {
		t.Fatalf("view missing topbar title on first line: %q", view)
	}
	if !strings.Contains(view, "Tasks") {
		t.Fatalf("view missing Tasks header: %q", view)
	}
	if !strings.Contains(view, "Threads") {
		t.Fatalf("view missing Threads header: %q", view)
	}
	if !strings.Contains(view, "Details") {
		t.Fatalf("view missing Details header: %q", view)
	}
	if !strings.Contains(lines[2], "│") {
		t.Fatalf("header should contain pane separator: %q", view)
	}
	if !strings.Contains(lines[1], "┌") || !strings.Contains(lines[1], "┬") || !strings.Contains(lines[1], "┐") {
		t.Fatalf("missing top frame border: %q", lines[1])
	}
	bottom := lines[len(lines)-2]
	if !strings.Contains(bottom, "└") || !strings.Contains(bottom, "┴") || !strings.Contains(bottom, "┘") {
		t.Fatalf("missing bottom frame border: %q", bottom)
	}

	if !strings.Contains(lines[len(lines)-1], "q quit") {
		t.Fatalf("footer hint row missing expected quit hint: %q", lines[len(lines)-1])
	}
}

func TestTUIModelViewRendersFooterHints(t *testing.T) {
	m := newTUIModel()
	m.setSize(90, 12)

	view := m.View().Content
	lines := strings.Split(view, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least topbar+footer, got %d lines", len(lines))
	}

	footer := lines[len(lines)-1]
	if !strings.Contains(footer, "↑/↓ select") {
		t.Fatalf("footer missing selection hint: %q", footer)
	}
	if !strings.Contains(footer, "PgUp/PgDn scroll") {
		t.Fatalf("footer missing scroll hint: %q", footer)
	}
	if !strings.Contains(footer, "q quit") {
		t.Fatalf("footer missing quit hint: %q", footer)
	}
}

func TestTUIModelViewUsesContinuousVerticalDivider(t *testing.T) {
	m := newTUIModel()
	m.setSize(60, 10)

	view := m.View().Content
	lines := strings.Split(view, "\n")
	if len(lines) < 6 {
		t.Fatalf("expected enough lines for topbar+header+body, got %d", len(lines))
	}

	headerLine := lines[2]
	headerRunes := []rune(headerLine)
	dividerIdx := -1
	for i, r := range headerRunes {
		if r == '│' {
			// first divider is left frame border, second is center split
			if dividerIdx == -1 {
				dividerIdx = i
				continue
			}
			dividerIdx = i
			break
		}
	}
	if dividerIdx == -1 {
		t.Fatalf("expected divider glyph in header line, got %q", headerLine)
	}

	for i := 1; i < len(lines)-1; i++ {
		lineRunes := []rune(lines[i])
		if dividerIdx >= len(lineRunes) {
			t.Fatalf("line %d too short for divider index %d: %q", i, dividerIdx, lines[i])
		}
		if lineRunes[dividerIdx] != '│' && lineRunes[dividerIdx] != '┼' && lineRunes[dividerIdx] != '┬' && lineRunes[dividerIdx] != '┴' {
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

	tasks, threads, details := resized.paneWidths(120)
	if tasks <= 0 || threads <= 0 || details <= 0 {
		t.Fatalf("pane widths must be positive, got tasks=%d threads=%d details=%d", tasks, threads, details)
	}
	if tasks+threads+details+tuiPaneSeparatorWidth != 120 {
		t.Fatalf("pane widths should fill full width: tasks=%d threads=%d details=%d sep=%d", tasks, threads, details, tuiPaneSeparatorWidth)
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

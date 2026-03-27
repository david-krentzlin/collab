package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/david-krentzlin/collab/internal/store"
)

func TestTUIModelTimelineModeShowsAllIndexedMessagesInSeqOrder(t *testing.T) {
	root := t.TempDir()
	s := initTaskStore(t, root, "timeline-task")

	seq1 := createThreadMessage(t, s, 0, "alice", "root-a")
	_ = createThreadMessage(t, s, seq1, "bob", "reply-a1")
	_ = createThreadMessage(t, s, seq1, "carol", "reply-a2")
	_ = createThreadMessage(t, s, 0, "alice", "root-b")
	_ = createThreadMessage(t, s, 4, "bob", "reply-b1")

	m := newTUIModelForBase(filepath.Join(root, store.CollabDir))
	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks: %v", err)
	}
	m.selectedTaskIdx = findTaskIndex(t, m, "timeline-task")
	m.renderMode = renderModeTimeline

	if err := m.refreshSelectedConversation(); err != nil {
		t.Fatalf("refresh selected conversation: %v", err)
	}

	content := m.convoViewport.GetContent()
	for _, seq := range []string{"#1", "#2", "#3", "#4", "#5"} {
		if !strings.Contains(content, seq) {
			t.Fatalf("timeline content missing %s: %q", seq, content)
		}
	}

	pos1 := strings.Index(content, "#1")
	pos2 := strings.Index(content, "#2")
	pos3 := strings.Index(content, "#3")
	pos4 := strings.Index(content, "#4")
	pos5 := strings.Index(content, "#5")
	if !(pos1 < pos2 && pos2 < pos3 && pos3 < pos4 && pos4 < pos5) {
		t.Fatalf("timeline lines not in seq order: %q", content)
	}
}

func TestTUIModelThreadedModeIntegrityCountersReflectSkippedParses(t *testing.T) {
	root := t.TempDir()
	s := seedTaskMessages(t, root, "integrity-task", 3)

	entries, err := s.List(0, "")
	if err != nil {
		t.Fatalf("list seeded entries: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("seeded entries = %d, want 3", len(entries))
	}
	if err := os.WriteFile(entries[1].Path, []byte("broken\n"), 0o644); err != nil {
		t.Fatalf("corrupt message file: %v", err)
	}

	m := newTUIModelForBase(filepath.Join(root, store.CollabDir))
	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks: %v", err)
	}
	m.selectedTaskIdx = findTaskIndex(t, m, "integrity-task")
	m.renderMode = renderModeThreaded

	if err := m.refreshSelectedConversation(); err != nil {
		t.Fatalf("refresh selected conversation: %v", err)
	}

	if m.conversationIndexedCount != 3 {
		t.Fatalf("indexed count = %d, want 3", m.conversationIndexedCount)
	}
	if m.conversationSkippedCount != 1 {
		t.Fatalf("skipped count = %d, want 1", m.conversationSkippedCount)
	}
	if m.conversationShownCount != 2 {
		t.Fatalf("shown count = %d, want 2", m.conversationShownCount)
	}
}

func TestTUIModelWheelRoutingByPaneCoordinates(t *testing.T) {
	root := t.TempDir()
	seedTaskMessages(t, root, "wheel-routing", 40)

	m := newTUIModelForBase(filepath.Join(root, store.CollabDir))
	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks: %v", err)
	}
	m.selectedTaskIdx = findTaskIndex(t, m, "wheel-routing")
	m.setSize(90, 14)
	if err := m.refreshSelectedConversation(); err != nil {
		t.Fatalf("refresh selected conversation: %v", err)
	}
	m.convoViewport.GotoBottom()

	leftX := 2
	bodyY := m.convoViewport.YPosition + 1
	startOffset := m.convoViewport.YOffset()
	_, _ = m.Update(tea.MouseWheelMsg{X: leftX, Y: bodyY, Button: tea.MouseWheelUp})
	if got := m.convoViewport.YOffset(); got != startOffset {
		t.Fatalf("wheel over left pane should not scroll convo, got offset %d want %d", got, startOffset)
	}
	if !m.autoFollow {
		t.Fatalf("wheel over left pane should not change autoFollow")
	}

	tasksWidth, _, _ := m.paneWidths(m.width)
	rightX := tasksWidth + 2
	_, _ = m.Update(tea.MouseWheelMsg{X: rightX, Y: bodyY, Button: tea.MouseWheelUp})
	if got := m.convoViewport.YOffset(); got >= startOffset {
		t.Fatalf("wheel over right pane should scroll up, got offset %d from %d", got, startOffset)
	}
	if m.autoFollow {
		t.Fatalf("wheel up over right pane should disable autoFollow")
	}
}

func TestTUIModelToggleModeKeepsViewportStable(t *testing.T) {
	root := t.TempDir()
	seedTaskMessages(t, root, "toggle-mode", 50)

	m := newTUIModelForBase(filepath.Join(root, store.CollabDir))
	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks: %v", err)
	}
	m.selectedTaskIdx = findTaskIndex(t, m, "toggle-mode")
	m.setSize(90, 14)
	m.autoFollow = false
	m.renderMode = renderModeTimeline
	if err := m.refreshSelectedConversation(); err != nil {
		t.Fatalf("refresh selected conversation: %v", err)
	}
	m.convoViewport.SetYOffset(5)

	next, _ := m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	updated := next.(*tuiModel)
	if updated.renderMode != renderModeThreaded {
		t.Fatalf("render mode after toggle = %q, want %q", updated.renderMode, renderModeThreaded)
	}
	if updated.convoViewport.YOffset() < 0 {
		t.Fatalf("viewport offset should remain sane, got %d", updated.convoViewport.YOffset())
	}
}

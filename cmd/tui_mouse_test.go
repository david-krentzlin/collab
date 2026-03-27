package cmd

import (
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/david-krentzlin/collab/internal/message"
	"github.com/david-krentzlin/collab/internal/store"
)

func TestTUIModelViewEnablesMouseMode(t *testing.T) {
	m := newTUIModel()
	m.setSize(80, 20)

	v := m.View()
	if v.MouseMode == tea.MouseModeNone {
		t.Fatalf("expected mouse mode enabled, got MouseModeNone")
	}
}

func TestTUIModelMouseWheelUpScrollsConversationAndDisablesFollow(t *testing.T) {
	root := t.TempDir()
	seedTaskMessages(t, root, "a-task", 40)

	m := newTUIModelForBase(filepath.Join(root, store.CollabDir))
	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks: %v", err)
	}
	m.selectedTaskIdx = findTaskIndex(t, m, "a-task")
	m.setSize(90, 12)
	if err := m.refreshSelectedConversation(); err != nil {
		t.Fatalf("refresh selected conversation: %v", err)
	}
	m.convoViewport.GotoBottom()

	startOffset := m.convoViewport.YOffset()
	if startOffset == 0 {
		t.Fatalf("test setup invalid: expected scrollable content")
	}
	rightX := conversationPaneX(m)

	_, _ = m.Update(tea.MouseWheelMsg{X: rightX, Y: m.convoViewport.YPosition + 1, Button: tea.MouseWheelUp})

	if got := m.convoViewport.YOffset(); got >= startOffset {
		t.Fatalf("wheel up should move offset up, got %d from %d", got, startOffset)
	}
	if m.autoFollow {
		t.Fatalf("wheel up should disable autoFollow")
	}
}

func TestTUIModelMouseWheelDownScrollsConversation(t *testing.T) {
	root := t.TempDir()
	seedTaskMessages(t, root, "a-task", 40)

	m := newTUIModelForBase(filepath.Join(root, store.CollabDir))
	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks: %v", err)
	}
	m.selectedTaskIdx = findTaskIndex(t, m, "a-task")
	m.setSize(90, 12)
	if err := m.refreshSelectedConversation(); err != nil {
		t.Fatalf("refresh selected conversation: %v", err)
	}
	m.convoViewport.SetYOffset(0)
	rightX := conversationPaneX(m)

	_, _ = m.Update(tea.MouseWheelMsg{X: rightX, Y: m.convoViewport.YPosition + 1, Button: tea.MouseWheelDown})

	if got := m.convoViewport.YOffset(); got == 0 {
		t.Fatalf("wheel down should scroll down from top, got offset=%d", got)
	}
}

func TestTUIModelMouseWheelDownAtBottomReenablesFollow(t *testing.T) {
	root := t.TempDir()
	seedTaskMessages(t, root, "a-task", 40)

	m := newTUIModelForBase(filepath.Join(root, store.CollabDir))
	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks: %v", err)
	}
	m.selectedTaskIdx = findTaskIndex(t, m, "a-task")
	m.setSize(90, 12)
	if err := m.refreshSelectedConversation(); err != nil {
		t.Fatalf("refresh selected conversation: %v", err)
	}
	m.autoFollow = false
	m.convoViewport.GotoBottom()
	rightX := conversationPaneX(m)

	_, _ = m.Update(tea.MouseWheelMsg{X: rightX, Y: m.convoViewport.YPosition + 1, Button: tea.MouseWheelDown})

	if !m.autoFollow {
		t.Fatalf("wheel down at bottom should re-enable autoFollow")
	}
}

func TestTUIModelPeriodicRefreshStillRespectsFollowAfterMouseScroll(t *testing.T) {
	root := t.TempDir()
	aStore := seedTaskMessages(t, root, "a-task", 30)

	m := newTUIModelForBase(filepath.Join(root, store.CollabDir))
	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks: %v", err)
	}
	m.selectedTaskIdx = findTaskIndex(t, m, "a-task")
	m.setSize(90, 12)
	m.autoFollow = true
	if err := m.refreshSelectedConversation(); err != nil {
		t.Fatalf("refresh selected conversation: %v", err)
	}
	m.convoViewport.GotoBottom()
	rightX := conversationPaneX(m)

	_, _ = m.Update(tea.MouseWheelMsg{X: rightX, Y: m.convoViewport.YPosition + 1, Button: tea.MouseWheelUp})
	if m.autoFollow {
		t.Fatalf("expected autoFollow disabled after wheel up")
	}

	if _, err := aStore.CreateMessage(&message.Message{
		From:    "alice",
		To:      "bob",
		Type:    message.Info,
		TS:      message.Now(),
		Summary: "after-wheel-up",
		Status:  message.Open,
		Body:    "body",
	}); err != nil {
		t.Fatalf("append message after wheel up: %v", err)
	}

	next, _ := m.Update(tuiTickMsg{})
	updated := next.(*tuiModel)
	if updated.convoViewport.AtBottom() {
		t.Fatalf("expected not at bottom when autoFollow=false after wheel up")
	}

	for i := 0; i < 20 && !updated.convoViewport.AtBottom(); i++ {
		next, _ = updated.Update(tea.MouseWheelMsg{X: conversationPaneX(updated), Y: updated.convoViewport.YPosition + 1, Button: tea.MouseWheelDown})
		updated = next.(*tuiModel)
	}
	if !updated.convoViewport.AtBottom() {
		t.Fatalf("expected to reach bottom after repeated wheel down")
	}
	if !updated.autoFollow {
		t.Fatalf("expected autoFollow re-enabled after wheel down to bottom")
	}

	if _, err := aStore.CreateMessage(&message.Message{
		From:    "alice",
		To:      "bob",
		Type:    message.Info,
		TS:      message.Now(),
		Summary: "after-wheel-down",
		Status:  message.Open,
		Body:    "body",
	}); err != nil {
		t.Fatalf("append message after wheel down: %v", err)
	}

	next, _ = updated.Update(tuiTickMsg{})
	updated = next.(*tuiModel)
	if !updated.convoViewport.AtBottom() {
		t.Fatalf("expected at bottom when autoFollow=true after wheel down")
	}
}

func conversationPaneX(m *tuiModel) int {
	tasksWidth, _, _ := m.paneWidths(m.width)
	return tasksWidth + 2
}

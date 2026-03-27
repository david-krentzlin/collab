package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/david-krentzlin/collab/internal/message"
	"github.com/david-krentzlin/collab/internal/store"
)

func TestTUIModelRefreshSelectedConversationRendersPlainMessages(t *testing.T) {
	root := t.TempDir()
	seedTaskMessages(t, root, "task-a", 2)

	m := newTUIModelForBase(filepath.Join(root, store.CollabDir))
	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks: %v", err)
	}
	m.selectedTaskIdx = findTaskIndex(t, m, "task-a")
	m.renderMode = renderModeTimeline
	m.setSize(100, 10)

	if err := m.refreshSelectedConversation(); err != nil {
		t.Fatalf("refresh selected conversation: %v", err)
	}

	content := m.convoViewport.GetContent()
	if !strings.Contains(content, "#1 alice -> bob [info] message-1") {
		t.Fatalf("missing first message line in viewport content: %q", content)
	}
	if !strings.Contains(content, "#2 alice -> bob [info] message-2") {
		t.Fatalf("missing second message line in viewport content: %q", content)
	}
}

func TestTUIModelViewportUpdatesSizeOnWindowResize(t *testing.T) {
	m := newTUIModel()

	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	resized, ok := next.(*tuiModel)
	if !ok {
		t.Fatalf("expected *tuiModel from update, got %T", next)
	}

	_, middle, _ := resized.paneWidths(120)
	if got := resized.convoViewport.Width(); got != middle {
		t.Fatalf("viewport width = %d, want %d", got, middle)
	}
	if got := resized.convoViewport.Height(); got != 34 {
		t.Fatalf("viewport height = %d, want %d", got, 34)
	}
}

func TestTUIModelAutoFollowDefaultsTrue(t *testing.T) {
	m := newTUIModel()
	if !m.autoFollow {
		t.Fatalf("autoFollow should default to true")
	}
}

func TestTUIModelAutoFollowKeepsBottomOnNewContent(t *testing.T) {
	root := t.TempDir()
	s := seedTaskMessages(t, root, "task-a", 25)

	m := newTUIModelForBase(filepath.Join(root, store.CollabDir))
	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks: %v", err)
	}
	m.selectedTaskIdx = findTaskIndex(t, m, "task-a")
	m.setSize(80, 6)
	m.autoFollow = true
	m.focusedPane = focusThreads
	if err := m.refreshSelectedConversation(); err != nil {
		t.Fatalf("refresh selected conversation: %v", err)
	}
	if !m.convoViewport.AtBottom() {
		t.Fatalf("expected viewport at bottom with autoFollow=true")
	}

	m.convoViewport.SetYOffset(0)
	if _, err := s.CreateMessage(&message.Message{
		From:    "alice",
		To:      "bob",
		Type:    message.Info,
		TS:      message.Now(),
		Summary: "message-26",
		Status:  message.Open,
		Body:    "body",
	}); err != nil {
		t.Fatalf("append message: %v", err)
	}

	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks after append: %v", err)
	}
	if err := m.refreshSelectedConversation(); err != nil {
		t.Fatalf("refresh selected conversation after append: %v", err)
	}
	if !m.convoViewport.AtBottom() {
		t.Fatalf("expected viewport to follow to bottom after new content")
	}
}

func TestTUIModelManualScrollUpDisablesAutoFollow(t *testing.T) {
	root := t.TempDir()
	seedTaskMessages(t, root, "task-a", 20)

	m := newTUIModelForBase(filepath.Join(root, store.CollabDir))
	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks: %v", err)
	}
	m.selectedTaskIdx = findTaskIndex(t, m, "task-a")
	m.setSize(80, 6)
	m.autoFollow = true
	m.focusedPane = focusThreads
	if err := m.refreshSelectedConversation(); err != nil {
		t.Fatalf("refresh selected conversation: %v", err)
	}

	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	if m.autoFollow {
		t.Fatalf("expected autoFollow=false after manual scroll up")
	}
}

func TestTUIModelWhenAutoFollowOffNewContentDoesNotJumpToBottom(t *testing.T) {
	root := t.TempDir()
	s := seedTaskMessages(t, root, "task-a", 25)

	m := newTUIModelForBase(filepath.Join(root, store.CollabDir))
	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks: %v", err)
	}
	m.selectedTaskIdx = findTaskIndex(t, m, "task-a")
	m.setSize(80, 6)
	m.autoFollow = false
	if err := m.refreshSelectedConversation(); err != nil {
		t.Fatalf("refresh selected conversation: %v", err)
	}
	m.convoViewport.SetYOffset(0)

	if _, err := s.CreateMessage(&message.Message{
		From:    "alice",
		To:      "bob",
		Type:    message.Info,
		TS:      message.Now(),
		Summary: "message-26",
		Status:  message.Open,
		Body:    "body",
	}); err != nil {
		t.Fatalf("append message: %v", err)
	}

	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks after append: %v", err)
	}
	if err := m.refreshSelectedConversation(); err != nil {
		t.Fatalf("refresh selected conversation after append: %v", err)
	}
	if m.convoViewport.AtBottom() {
		t.Fatalf("expected viewport to stay off-bottom when autoFollow=false")
	}
}

func TestTUIModelTaskSwitchReenablesFollowAndGoesBottom(t *testing.T) {
	root := t.TempDir()
	seedTaskMessages(t, root, "a-task", 10)
	seedTaskMessages(t, root, "b-task", 10)

	m := newTUIModelForBase(filepath.Join(root, store.CollabDir))
	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks: %v", err)
	}
	m.setSize(80, 6)
	m.selectedTaskIdx = findTaskIndex(t, m, "a-task")
	m.autoFollow = false
	if err := m.refreshSelectedConversation(); err != nil {
		t.Fatalf("refresh selected conversation: %v", err)
	}

	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if got := m.selectedTaskName(); got != "b-task" {
		t.Fatalf("selected task after switch = %q, want b-task", got)
	}
	if !m.autoFollow {
		t.Fatalf("expected autoFollow to be re-enabled on task switch")
	}
	if !m.convoViewport.AtBottom() {
		t.Fatalf("expected viewport to be at bottom after task switch")
	}
}

func TestTUIModelRefreshSelectedConversationHandlesCorruptMessageFileGracefully(t *testing.T) {
	root := t.TempDir()
	s := seedTaskMessages(t, root, "task-a", 2)

	entries, err := s.List(0, "")
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) < 2 {
		t.Fatalf("expected at least 2 entries, got %d", len(entries))
	}
	if err := os.WriteFile(entries[0].Path, []byte("invalid-frontmatter\n"), 0o644); err != nil {
		t.Fatalf("corrupt first message file: %v", err)
	}

	m := newTUIModelForBase(filepath.Join(root, store.CollabDir))
	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks: %v", err)
	}
	m.selectedTaskIdx = findTaskIndex(t, m, "task-a")
	m.setSize(140, 8)

	if err := m.refreshSelectedConversation(); err != nil {
		t.Fatalf("refresh selected conversation with corrupt message: %v", err)
	}

	content := strings.TrimSpace(m.convoViewport.GetContent())
	if content == "" {
		t.Fatalf("expected non-empty viewport content with one valid message remaining")
	}
	if !strings.Contains(content, "message-2") {
		t.Fatalf("expected valid message summary in viewport content, got %q", content)
	}
}

func newTUIModelForBase(collabBasePath string) *tuiModel {
	return &tuiModel{
		lastSeenByTask:   make(map[string]int),
		collabBasePath:   collabBasePath,
		convoViewport:    viewport.New(),
		detailsViewport:  viewport.New(),
		autoFollow:       true,
		renderMode:       renderModeThreaded,
		focusedPane:      focusTasks,
		messageBySeq:     make(map[int]*message.Message),
		collapsedThreads: make(map[int]bool),
	}
}

func findTaskIndex(t *testing.T, m *tuiModel, taskName string) int {
	t.Helper()
	for i, task := range m.tasks {
		if task.name == taskName {
			return i
		}
	}
	t.Fatalf("task %q not found in %+v", taskName, m.tasks)
	return -1
}

func seedTaskMessages(t *testing.T, root, task string, count int) *store.Store {
	t.Helper()

	s := store.FindTask(root, task)
	if err := s.Init([]string{"alice", "bob"}); err != nil {
		t.Fatalf("init task store %q: %v", task, err)
	}

	for i := 1; i <= count; i++ {
		if _, err := s.CreateMessage(&message.Message{
			From:    "alice",
			To:      "bob",
			Type:    message.Info,
			TS:      message.Now(),
			Summary: fmt.Sprintf("message-%d", i),
			Status:  message.Open,
			Body:    "body",
		}); err != nil {
			t.Fatalf("create message %d for %q: %v", i, task, err)
		}
	}

	return s
}

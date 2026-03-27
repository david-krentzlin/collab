package cmd

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/david-krentzlin/collab/internal/message"
	"github.com/david-krentzlin/collab/internal/store"
)

func TestTUIModelThreadedFormattingShowsParentChildStructure(t *testing.T) {
	root := t.TempDir()
	s := initTaskStore(t, root, "thread-task")

	rootSeq := createThreadMessage(t, s, 0, "alice", "root")
	_ = createThreadMessage(t, s, rootSeq, "bob", "child-a")
	_ = createThreadMessage(t, s, rootSeq, "carol", "child-b")

	m := newTUIModelForBase(filepath.Join(root, store.CollabDir))
	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks: %v", err)
	}
	m.selectedTaskIdx = findTaskIndex(t, m, "thread-task")
	m.renderMode = renderModeThreaded

	if err := m.refreshSelectedConversation(); err != nil {
		t.Fatalf("refresh conversation: %v", err)
	}

	content := m.convoViewport.GetContent()
	if !strings.Contains(content, "#1 alice [reply] root") {
		t.Fatalf("missing root line: %q", content)
	}
	if !strings.Contains(content, "│ ├─ #2 bob [reply] child-a") {
		t.Fatalf("missing sibling child-a connector: %q", content)
	}
	if !strings.Contains(content, "│ └─ #3 carol [reply] child-b") {
		t.Fatalf("missing sibling child-b connector: %q", content)
	}
}

func TestTUIModelThreadedFormattingShowsNestedReplyDepth(t *testing.T) {
	root := t.TempDir()
	s := initTaskStore(t, root, "depth-task")

	seq1 := createThreadMessage(t, s, 0, "alice", "root")
	seq2 := createThreadMessage(t, s, seq1, "bob", "level-1")
	_ = createThreadMessage(t, s, seq2, "carol", "level-2")

	m := newTUIModelForBase(filepath.Join(root, store.CollabDir))
	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks: %v", err)
	}
	m.selectedTaskIdx = findTaskIndex(t, m, "depth-task")
	m.renderMode = renderModeThreaded

	if err := m.refreshSelectedConversation(); err != nil {
		t.Fatalf("refresh conversation: %v", err)
	}

	content := m.convoViewport.GetContent()
	if !strings.Contains(content, "│ └─ #2 bob [reply] level-1") {
		t.Fatalf("missing level-1 nested connector: %q", content)
	}
	if !strings.Contains(content, "│    └─ #3 carol [reply] level-2") {
		t.Fatalf("missing level-2 nested indentation: %q", content)
	}
}

func TestTUIModelThreadedFormattingClearlyDenotesSpeaker(t *testing.T) {
	root := t.TempDir()
	s := initTaskStore(t, root, "speaker-task")

	seq1 := createThreadMessage(t, s, 0, "alice", "from alice")
	_ = createThreadMessage(t, s, seq1, "bob", "from bob")
	_ = createThreadMessage(t, s, seq1, "carol", "from carol")

	m := newTUIModelForBase(filepath.Join(root, store.CollabDir))
	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks: %v", err)
	}
	m.selectedTaskIdx = findTaskIndex(t, m, "speaker-task")
	m.renderMode = renderModeThreaded

	if err := m.refreshSelectedConversation(); err != nil {
		t.Fatalf("refresh conversation: %v", err)
	}

	content := m.convoViewport.GetContent()
	for _, speaker := range []string{"alice", "bob", "carol"} {
		if !strings.Contains(content, speaker) {
			t.Fatalf("expected speaker %q in content: %q", speaker, content)
		}
	}
}

func TestTUIModelThreadedFormattingHandlesOrphanRepliesGracefully(t *testing.T) {
	root := t.TempDir()
	s := initTaskStore(t, root, "orphan-task")

	_ = createThreadMessage(t, s, 0, "alice", "root")
	_ = createThreadMessage(t, s, 999, "bob", "orphan")

	m := newTUIModelForBase(filepath.Join(root, store.CollabDir))
	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks: %v", err)
	}
	m.selectedTaskIdx = findTaskIndex(t, m, "orphan-task")
	m.renderMode = renderModeThreaded

	if err := m.refreshSelectedConversation(); err != nil {
		t.Fatalf("refresh conversation: %v", err)
	}

	content := m.convoViewport.GetContent()
	if !strings.Contains(content, "Orphans:") {
		t.Fatalf("expected explicit orphan section in content: %q", content)
	}
	if !strings.Contains(content, "? #2 bob [reply] orphan") {
		t.Fatalf("expected orphan message entry in content: %q", content)
	}
}

func TestTUIModelPeriodicRefreshUpdatesTaskUnreadMarker(t *testing.T) {
	root := t.TempDir()
	_ = seedTaskMessages(t, root, "a-task", 1)
	bStore := seedTaskMessages(t, root, "b-task", 1)

	m := newTUIModelForBase(filepath.Join(root, store.CollabDir))
	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks: %v", err)
	}
	m.selectedTaskIdx = findTaskIndex(t, m, "a-task")

	for i := range m.tasks {
		m.lastSeenByTask[m.tasks[i].name] = m.tasks[i].maxSeq
		m.tasks[i].hasUnread = false
	}

	if _, err := bStore.CreateMessage(&message.Message{
		From:    "alice",
		To:      "bob",
		Type:    message.Info,
		TS:      message.Now(),
		Summary: "b-new",
		Status:  message.Open,
		Body:    "body",
	}); err != nil {
		t.Fatalf("append message to b-task: %v", err)
	}

	next, _ := m.Update(tuiTickMsg{})
	updated := next.(*tuiModel)
	bIdx := findTaskIndex(t, updated, "b-task")
	if !updated.tasks[bIdx].hasUnread {
		t.Fatalf("expected unread marker on b-task after tick refresh")
	}
	if !strings.Contains(updated.taskLine(bIdx), "●") {
		t.Fatalf("expected unread glyph in b-task line, got %q", updated.taskLine(bIdx))
	}
}

func TestTUIModelPeriodicRefreshUpdatesSelectedConversationContent(t *testing.T) {
	root := t.TempDir()
	aStore := seedTaskMessages(t, root, "a-task", 1)

	m := newTUIModelForBase(filepath.Join(root, store.CollabDir))
	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks: %v", err)
	}
	m.selectedTaskIdx = findTaskIndex(t, m, "a-task")
	m.setSize(80, 8)

	if _, err := aStore.CreateMessage(&message.Message{
		From:    "alice",
		To:      "bob",
		Type:    message.Info,
		TS:      message.Now(),
		Summary: "selected-new",
		Status:  message.Open,
		Body:    "body",
	}); err != nil {
		t.Fatalf("append message to selected task: %v", err)
	}

	next, _ := m.Update(tuiTickMsg{})
	updated := next.(*tuiModel)
	if !strings.Contains(updated.convoViewport.GetContent(), "selected-new") {
		t.Fatalf("expected selected conversation to include new summary, got %q", updated.convoViewport.GetContent())
	}
}

func TestTUIModelPeriodicRefreshRespectsAutoFollowTrue(t *testing.T) {
	root := t.TempDir()
	aStore := seedTaskMessages(t, root, "a-task", 20)

	m := newTUIModelForBase(filepath.Join(root, store.CollabDir))
	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks: %v", err)
	}
	m.selectedTaskIdx = findTaskIndex(t, m, "a-task")
	m.setSize(80, 6)
	m.autoFollow = true
	if err := m.refreshSelectedConversation(); err != nil {
		t.Fatalf("refresh selected conversation: %v", err)
	}
	m.convoViewport.SetYOffset(0)

	if _, err := aStore.CreateMessage(&message.Message{
		From:    "alice",
		To:      "bob",
		Type:    message.Info,
		TS:      message.Now(),
		Summary: "follow-new",
		Status:  message.Open,
		Body:    "body",
	}); err != nil {
		t.Fatalf("append message: %v", err)
	}

	next, _ := m.Update(tuiTickMsg{})
	updated := next.(*tuiModel)
	if !updated.convoViewport.AtBottom() {
		t.Fatalf("expected viewport at bottom when autoFollow=true on periodic refresh")
	}
}

func TestTUIModelPeriodicRefreshRespectsAutoFollowFalse(t *testing.T) {
	root := t.TempDir()
	aStore := seedTaskMessages(t, root, "a-task", 20)

	m := newTUIModelForBase(filepath.Join(root, store.CollabDir))
	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks: %v", err)
	}
	m.selectedTaskIdx = findTaskIndex(t, m, "a-task")
	m.setSize(80, 6)
	m.autoFollow = false
	if err := m.refreshSelectedConversation(); err != nil {
		t.Fatalf("refresh selected conversation: %v", err)
	}
	m.convoViewport.SetYOffset(0)

	if _, err := aStore.CreateMessage(&message.Message{
		From:    "alice",
		To:      "bob",
		Type:    message.Info,
		TS:      message.Now(),
		Summary: "nofollow-new",
		Status:  message.Open,
		Body:    "body",
	}); err != nil {
		t.Fatalf("append message: %v", err)
	}

	next, _ := m.Update(tuiTickMsg{})
	updated := next.(*tuiModel)
	if updated.convoViewport.AtBottom() {
		t.Fatalf("expected viewport to stay off-bottom when autoFollow=false on periodic refresh")
	}
}

func TestTUIModelTickCommandReschedulesItself(t *testing.T) {
	m := newTUIModelForBase(filepath.Join(t.TempDir(), store.CollabDir))

	next, cmd := m.Update(tuiTickMsg{})
	if next == nil {
		t.Fatalf("expected model returned on tick update")
	}
	if cmd == nil {
		t.Fatalf("expected tick update to schedule another command")
	}
}

func initTaskStore(t *testing.T, root, task string) *store.Store {
	t.Helper()
	s := store.FindTask(root, task)
	if err := s.Init([]string{"alice", "bob", "carol"}); err != nil {
		t.Fatalf("init task store %q: %v", task, err)
	}
	return s
}

func createThreadMessage(t *testing.T, s *store.Store, re int, from, summary string) int {
	t.Helper()

	msg := &message.Message{
		From:    from,
		To:      "alice",
		Type:    message.Reply,
		Re:      re,
		TS:      message.Now(),
		Summary: summary,
		Status:  message.Open,
		Body:    fmt.Sprintf("body-%s", summary),
	}
	if _, err := s.CreateMessage(msg); err != nil {
		t.Fatalf("create thread message %q: %v", summary, err)
	}
	return msg.Seq
}

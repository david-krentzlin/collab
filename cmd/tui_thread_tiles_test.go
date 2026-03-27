package cmd

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/david-krentzlin/collab/internal/store"
)

func TestTUIModelThreadTilesRenderBoxAndChildren(t *testing.T) {
	root := t.TempDir()
	s := initTaskStore(t, root, "tile-task")
	rootSeq := createThreadMessage(t, s, 0, "alice", "root")
	_ = createThreadMessage(t, s, rootSeq, "bob", "child")

	m := newTUIModelForBase(filepath.Join(root, store.CollabDir))
	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks: %v", err)
	}
	m.selectedTaskIdx = findTaskIndex(t, m, "tile-task")
	m.renderMode = renderModeThreaded
	if err := m.refreshSelectedConversation(); err != nil {
		t.Fatalf("refresh selected conversation: %v", err)
	}

	content := m.convoViewport.GetContent()
	if !strings.Contains(content, "┌ "+iconExpand+" "+iconThread+" #1") {
		t.Fatalf("expected thread tile header, got: %q", content)
	}
	if !strings.Contains(content, "│ "+iconMessage+" "+iconOpen+" bob") {
		t.Fatalf("expected child message under tile, got: %q", content)
	}
	if !strings.Contains(content, "└") {
		t.Fatalf("expected tile bottom marker, got: %q", content)
	}
}

func TestTUIModelToggleThreadCollapseHidesAndShowsChildren(t *testing.T) {
	root := t.TempDir()
	s := initTaskStore(t, root, "collapse-task")
	rootSeq := createThreadMessage(t, s, 0, "alice", "root")
	_ = createThreadMessage(t, s, rootSeq, "bob", "child")

	m := newTUIModelForBase(filepath.Join(root, store.CollabDir))
	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks: %v", err)
	}
	m.selectedTaskIdx = findTaskIndex(t, m, "collapse-task")
	m.renderMode = renderModeThreaded
	m.focusedPane = focusThreads
	if err := m.refreshSelectedConversation(); err != nil {
		t.Fatalf("refresh selected conversation: %v", err)
	}

	if !strings.Contains(m.convoViewport.GetContent(), "child") {
		t.Fatalf("expected expanded thread to show child line")
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(*tuiModel)
	if strings.Contains(m.convoViewport.GetContent(), "child") {
		t.Fatalf("expected collapsed thread to hide child line")
	}
	if !strings.Contains(m.convoViewport.GetContent(), iconCollapse) {
		t.Fatalf("expected collapsed marker in tile header")
	}

	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(*tuiModel)
	if !strings.Contains(m.convoViewport.GetContent(), "child") {
		t.Fatalf("expected expanded thread to show child line again")
	}
}

func TestTUIModelDetailsPaneFormatsMetadataNicely(t *testing.T) {
	root := t.TempDir()
	seedTaskMessages(t, root, "meta-task", 1)

	m := newTUIModelForBase(filepath.Join(root, store.CollabDir))
	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks: %v", err)
	}
	m.selectedTaskIdx = findTaskIndex(t, m, "meta-task")
	if err := m.refreshSelectedConversation(); err != nil {
		t.Fatalf("refresh selected conversation: %v", err)
	}

	details := m.detailsViewport.GetContent()
	for _, want := range []string{"┌ Message #1", "From", "To", "Type", "Status", "Summary", "├ Body", "└ End"} {
		if !strings.Contains(details, want) {
			t.Fatalf("details metadata missing %q in: %q", want, details)
		}
	}
}

func TestTUIModelThreadTileHeaderShowsSummaryAndFollowUpCount(t *testing.T) {
	root := t.TempDir()
	s := initTaskStore(t, root, "header-count")
	rootSeq := createThreadMessage(t, s, 0, "alice", "root summary")
	_ = createThreadMessage(t, s, rootSeq, "bob", "reply-1")
	_ = createThreadMessage(t, s, rootSeq, "carol", "reply-2")

	m := newTUIModelForBase(filepath.Join(root, store.CollabDir))
	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks: %v", err)
	}
	m.selectedTaskIdx = findTaskIndex(t, m, "header-count")
	m.renderMode = renderModeThreaded
	if err := m.refreshSelectedConversation(); err != nil {
		t.Fatalf("refresh selected conversation: %v", err)
	}

	line := strings.Split(m.convoViewport.GetContent(), "\n")[0]
	if !strings.Contains(line, iconThread+" #1") {
		t.Fatalf("header missing thread number: %q", line)
	}
	if !strings.Contains(line, "root summary") {
		t.Fatalf("header missing summary: %q", line)
	}
	if !strings.Contains(line, "2 follow-ups") {
		t.Fatalf("header missing follow-up count: %q", line)
	}
}

func TestTUIModelThreadMessageRowsShowAgentSummaryTimeAndStatus(t *testing.T) {
	root := t.TempDir()
	s := initTaskStore(t, root, "msg-format")
	rootSeq := createThreadMessage(t, s, 0, "alice", "root")
	_ = createThreadMessage(t, s, rootSeq, "bob", "child summary")

	m := newTUIModelForBase(filepath.Join(root, store.CollabDir))
	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks: %v", err)
	}
	m.selectedTaskIdx = findTaskIndex(t, m, "msg-format")
	m.renderMode = renderModeThreaded
	if err := m.refreshSelectedConversation(); err != nil {
		t.Fatalf("refresh selected conversation: %v", err)
	}

	content := m.convoViewport.GetContent()
	if !strings.Contains(content, "│ "+iconMessage+" "+iconOpen+" bob") {
		t.Fatalf("message row missing agent and status icon: %q", content)
	}
	if !strings.Contains(content, "child summary") {
		t.Fatalf("message row missing summary: %q", content)
	}
	if !strings.Contains(content, "·") {
		t.Fatalf("message row missing time separator: %q", content)
	}
}

func TestTUIModelSelectingMessageMarksReferencedMessage(t *testing.T) {
	root := t.TempDir()
	s := initTaskStore(t, root, "referred-mark")
	rootSeq := createThreadMessage(t, s, 0, "alice", "root")
	_ = createThreadMessage(t, s, rootSeq, "bob", "child-1")
	_ = createThreadMessage(t, s, rootSeq, "carol", "child-2")

	m := newTUIModelForBase(filepath.Join(root, store.CollabDir))
	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks: %v", err)
	}
	m.selectedTaskIdx = findTaskIndex(t, m, "referred-mark")
	m.renderMode = renderModeThreaded
	m.focusedPane = focusThreads
	if err := m.refreshSelectedConversation(); err != nil {
		t.Fatalf("refresh selected conversation: %v", err)
	}

	// move to first child row (#2, re #1)
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = next.(*tuiModel)
	content := m.convoViewport.GetContent()
	if !strings.Contains(content, iconReference+" ┌") {
		t.Fatalf("expected referenced root message marker in content: %q", content)
	}
}

func TestTUIModelAgentLabelsColorizedOnlyWhenEnabled(t *testing.T) {
	m := newTUIModelForBase(filepath.Join(t.TempDir(), store.CollabDir))
	m.colorEnabled = true
	colored := m.agentLabel("alice")
	if !strings.Contains(colored, "\x1b[") {
		t.Fatalf("expected ANSI style sequence when color enabled, got %q", colored)
	}

	m.colorEnabled = false
	plain := m.agentLabel("alice")
	if strings.Contains(plain, "\x1b[") {
		t.Fatalf("expected plain label without ANSI when color disabled, got %q", plain)
	}
}

func TestTUIModelThreadTilesHaveBlankMarginBetweenThreads(t *testing.T) {
	root := t.TempDir()
	s := initTaskStore(t, root, "margin-task")
	_ = createThreadMessage(t, s, 0, "alice", "root-1")
	_ = createThreadMessage(t, s, 0, "bob", "root-2")

	m := newTUIModelForBase(filepath.Join(root, store.CollabDir))
	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks: %v", err)
	}
	m.selectedTaskIdx = findTaskIndex(t, m, "margin-task")
	m.renderMode = renderModeThreaded
	if err := m.refreshSelectedConversation(); err != nil {
		t.Fatalf("refresh selected conversation: %v", err)
	}

	content := m.convoViewport.GetContent()
	if !strings.Contains(content, "└\n\n\n┌") {
		t.Fatalf("expected visible blank margin between thread tiles, got: %q", content)
	}
}

func TestTUIModelThreadRowStylingUsesMutedBackgroundAndHeaderAccent(t *testing.T) {
	m := newTUIModelForBase(filepath.Join(t.TempDir(), store.CollabDir))
	m.colorEnabled = true

	msgRow := m.styleThreadCell("row", conversationRow{rootSeq: 1, kind: "message"}, false, false)
	headerRow := m.styleThreadCell("header", conversationRow{rootSeq: 1, kind: "thread-header"}, false, false)

	if !strings.Contains(msgRow, "48;2;42;45;56") {
		t.Fatalf("expected muted dark background in message row style, got %q", msgRow)
	}
	if !strings.Contains(headerRow, "48;2;42;45;56") {
		t.Fatalf("expected muted dark background in header row style, got %q", headerRow)
	}
	if msgRow == headerRow {
		t.Fatalf("expected header style to differ from message style")
	}
}

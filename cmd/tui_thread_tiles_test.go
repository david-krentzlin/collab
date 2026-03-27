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
	if !strings.Contains(content, "  ┌") {
		t.Fatalf("expected thread tile with horizontal margin, got: %q", content)
	}
	if !strings.Contains(content, iconThread+" #1") {
		t.Fatalf("expected thread tile header, got: %q", content)
	}
	if !strings.Contains(content, iconOpen) || !strings.Contains(content, "bob") {
		t.Fatalf("expected child message under tile, got: %q", content)
	}
	if !strings.Contains(content, "└") || !strings.Contains(content, "┐") {
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

	line := ""
	for _, candidate := range strings.Split(m.convoViewport.GetContent(), "\n") {
		if strings.Contains(candidate, iconThread+" #1") {
			line = candidate
			break
		}
	}
	if line == "" {
		t.Fatalf("missing visible thread header line in content: %q", m.convoViewport.GetContent())
	}
	if !strings.Contains(line, iconThread+" #1") {
		t.Fatalf("header missing thread number: %q", line)
	}
	if !strings.Contains(line, "root summary") {
		t.Fatalf("header missing summary: %q", line)
	}
	if !strings.Contains(line, "2 replies") {
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
	if !strings.Contains(content, iconOpen) || !strings.Contains(content, "bob") {
		t.Fatalf("message row missing agent and status icon: %q", content)
	}
	if !strings.Contains(content, "│") {
		t.Fatalf("message row should be rendered as a bordered cell line: %q", content)
	}
	if !strings.Contains(content, "child summary") {
		t.Fatalf("message row missing summary: %q", content)
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
	if !strings.Contains(content, iconReference) || !strings.Contains(content, iconThread+" #1") {
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
	firstBottom := strings.Index(content, "└")
	secondTop := strings.LastIndex(content, "┌")
	if firstBottom < 0 || secondTop <= firstBottom {
		t.Fatalf("expected two thread tiles in content, got: %q", content)
	}
	between := content[firstBottom:secondTop]
	if strings.Count(between, "\n") < 1 {
		t.Fatalf("expected visible blank margin between thread tiles, got: %q", content)
	}
}

func TestTUIModelSelectedThreadUsesStrongLeftBorder(t *testing.T) {
	root := t.TempDir()
	s := initTaskStore(t, root, "selected-border")
	_ = createThreadMessage(t, s, 0, "alice", "root-1")
	_ = createThreadMessage(t, s, 0, "bob", "root-2")

	m := newTUIModelForBase(filepath.Join(root, store.CollabDir))
	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks: %v", err)
	}
	m.selectedTaskIdx = findTaskIndex(t, m, "selected-border")
	m.renderMode = renderModeThreaded
	if err := m.refreshSelectedConversation(); err != nil {
		t.Fatalf("refresh selected conversation: %v", err)
	}

	content := m.convoViewport.GetContent()
	if !strings.Contains(content, "┃") {
		t.Fatalf("expected selected thread to use highlighted left border glyph, got: %q", content)
	}
	if !strings.Contains(content, "│") {
		t.Fatalf("expected non-selected rows to keep standard border glyph, got: %q", content)
	}
}

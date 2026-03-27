package cmd

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/david-krentzlin/collab/internal/message"
	"github.com/david-krentzlin/collab/internal/store"
)

func TestTUIModelViewRendersThreePaneHeaders(t *testing.T) {
	m := newTUIModel()
	m.setSize(100, 16)

	view := m.View().Content
	if !strings.Contains(view, "Tasks") || !strings.Contains(view, "Threads") || !strings.Contains(view, "Details") {
		t.Fatalf("expected three pane headers in view, got:\n%s", view)
	}
	if !strings.Contains(view, "▶ Tasks") {
		t.Fatalf("expected active focus marker on tasks header, got:\n%s", view)
	}
}

func TestTUIModelPaneWidthsFillTerminal(t *testing.T) {
	m := newTUIModel()
	tasks, threads, details := m.paneWidths(120)
	if tasks+threads+details+tuiPaneSeparatorWidth != 120 {
		t.Fatalf("pane widths should fill full width: tasks=%d threads=%d details=%d sep=%d", tasks, threads, details, tuiPaneSeparatorWidth)
	}
}

func TestTUIModelTabCyclesFocusAcrossPanes(t *testing.T) {
	m := newTUIModel()
	if m.focusedPane != focusTasks {
		t.Fatalf("initial focus = %q, want %q", m.focusedPane, focusTasks)
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = next.(*tuiModel)
	if m.focusedPane != focusThreads {
		t.Fatalf("focus after first tab = %q, want %q", m.focusedPane, focusThreads)
	}

	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = next.(*tuiModel)
	if m.focusedPane != focusDetails {
		t.Fatalf("focus after second tab = %q, want %q", m.focusedPane, focusDetails)
	}

	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = next.(*tuiModel)
	if m.focusedPane != focusTasks {
		t.Fatalf("focus after third tab = %q, want %q", m.focusedPane, focusTasks)
	}
}

func TestTUIModelFocusHighlightTracksActivePane(t *testing.T) {
	m := newTUIModel()
	m.setSize(100, 16)

	view := m.View().Content
	if !strings.Contains(view, "▶ Tasks") {
		t.Fatalf("expected tasks focus marker in initial view")
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = next.(*tuiModel)
	view = m.View().Content
	if !strings.Contains(view, "▶ Threads") {
		t.Fatalf("expected threads focus marker after tab")
	}
}

func TestTUIModelUpDownOnTasksChangesSelectedTask(t *testing.T) {
	root := t.TempDir()
	_ = seedTaskMessages(t, root, "a-task", 1)
	_ = seedTaskMessages(t, root, "b-task", 1)

	m := newTUIModelForBase(filepath.Join(root, store.CollabDir))
	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks: %v", err)
	}
	m.focusedPane = focusTasks
	start := m.selectedTaskIdx

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = next.(*tuiModel)
	if m.selectedTaskIdx == start {
		t.Fatalf("expected selected task to change on down in tasks focus")
	}
}

func TestTUIModelThreadsPaneShowsThreadedSummaryRows(t *testing.T) {
	root := t.TempDir()
	s := initTaskStore(t, root, "thread-pane")
	rootSeq := createThreadMessage(t, s, 0, "alice", "root")
	_ = createThreadMessage(t, s, rootSeq, "bob", "reply")

	m := newTUIModelForBase(filepath.Join(root, store.CollabDir))
	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks: %v", err)
	}
	m.selectedTaskIdx = findTaskIndex(t, m, "thread-pane")
	m.renderMode = renderModeThreaded
	if err := m.refreshSelectedConversation(); err != nil {
		t.Fatalf("refresh selected conversation: %v", err)
	}

	content := m.convoViewport.GetContent()
	if !strings.Contains(content, "┌ [-] Thread #1") || !strings.Contains(content, "│ └─ #2 bob [reply] reply") {
		t.Fatalf("expected threaded summary rows in middle pane content, got: %q", content)
	}
}

func TestTUIModelUpDownOnThreadsChangesSelectedMessageAndDetails(t *testing.T) {
	root := t.TempDir()
	s := initTaskStore(t, root, "thread-select")
	rootSeq := createThreadMessage(t, s, 0, "alice", "root")
	_ = createThreadMessage(t, s, rootSeq, "bob", "reply")

	m := newTUIModelForBase(filepath.Join(root, store.CollabDir))
	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks: %v", err)
	}
	m.selectedTaskIdx = findTaskIndex(t, m, "thread-select")
	m.renderMode = renderModeThreaded
	m.focusedPane = focusThreads
	if err := m.refreshSelectedConversation(); err != nil {
		t.Fatalf("refresh selected conversation: %v", err)
	}

	startSeq := m.selectedMessageSeq
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = next.(*tuiModel)
	if m.selectedMessageSeq == startSeq {
		t.Fatalf("expected selected message seq to change on down in threads focus")
	}
	if !strings.Contains(m.detailsViewport.GetContent(), "Summary reply") {
		t.Fatalf("expected details pane to reflect newly selected thread row, got: %q", m.detailsViewport.GetContent())
	}
}

func TestTUIModelUpDownOnDetailsScrollsDetailsViewport(t *testing.T) {
	root := t.TempDir()
	s := store.FindTask(root, "details-scroll")
	if err := s.Init([]string{"alice", "bob"}); err != nil {
		t.Fatalf("init store: %v", err)
	}
	body := strings.Repeat("line\n", 40)
	if _, err := s.CreateMessage(&message.Message{From: "alice", To: "bob", Type: message.Info, TS: message.Now(), Summary: "long", Status: message.Open, Body: body}); err != nil {
		t.Fatalf("create long message: %v", err)
	}

	m := newTUIModelForBase(filepath.Join(root, store.CollabDir))
	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks: %v", err)
	}
	m.selectedTaskIdx = findTaskIndex(t, m, "details-scroll")
	m.setSize(100, 14)
	m.focusedPane = focusDetails
	if err := m.refreshSelectedConversation(); err != nil {
		t.Fatalf("refresh selected conversation: %v", err)
	}

	start := m.detailsViewport.YOffset()
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = next.(*tuiModel)
	if m.detailsViewport.YOffset() == start {
		t.Fatalf("expected details viewport to scroll on down when details focused")
	}
}

func TestTUIModelPeriodicRefreshKeepsSelectionsStableByIdentity(t *testing.T) {
	root := t.TempDir()
	a := seedTaskMessages(t, root, "a-task", 2)
	_ = seedTaskMessages(t, root, "b-task", 2)

	m := newTUIModelForBase(filepath.Join(root, store.CollabDir))
	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks: %v", err)
	}
	m.selectedTaskIdx = findTaskIndex(t, m, "a-task")
	m.renderMode = renderModeTimeline
	if err := m.refreshSelectedConversation(); err != nil {
		t.Fatalf("refresh selected conversation: %v", err)
	}
	selectedTask := m.selectedTaskName()
	selectedSeq := m.selectedMessageSeq

	if _, err := a.CreateMessage(&message.Message{From: "alice", To: "bob", Type: message.Info, TS: message.Now(), Summary: "new", Status: message.Open, Body: "body"}); err != nil {
		t.Fatalf("append message: %v", err)
	}

	next, _ := m.Update(tuiTickMsg{})
	m = next.(*tuiModel)
	if m.selectedTaskName() != selectedTask {
		t.Fatalf("selected task changed after refresh: %q -> %q", selectedTask, m.selectedTaskName())
	}
	if m.selectedMessageSeq != selectedSeq {
		t.Fatalf("selected message seq changed after refresh: %d -> %d", selectedSeq, m.selectedMessageSeq)
	}
}

func TestTUIModelIntegrityCountersStillExposedInFooter(t *testing.T) {
	root := t.TempDir()
	_ = seedTaskMessages(t, root, "counter-task", 3)

	m := newTUIModelForBase(filepath.Join(root, store.CollabDir))
	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks: %v", err)
	}
	m.selectedTaskIdx = findTaskIndex(t, m, "counter-task")
	m.setSize(180, 14)
	if err := m.refreshSelectedConversation(); err != nil {
		t.Fatalf("refresh selected conversation: %v", err)
	}

	view := m.View().Content
	if !strings.Contains(view, "shown:") || !strings.Contains(view, "skipped:") || !strings.Contains(view, "mode:") {
		t.Fatalf("expected integrity counters in footer, got: %q", view)
	}
}

func TestTUIModelPeriodicRefreshUpdatesThreadsAndDetails(t *testing.T) {
	root := t.TempDir()
	a := seedTaskMessages(t, root, "update-task", 1)

	m := newTUIModelForBase(filepath.Join(root, store.CollabDir))
	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks: %v", err)
	}
	m.selectedTaskIdx = findTaskIndex(t, m, "update-task")
	m.renderMode = renderModeTimeline
	if err := m.refreshSelectedConversation(); err != nil {
		t.Fatalf("refresh selected conversation: %v", err)
	}

	if _, err := a.CreateMessage(&message.Message{From: "alice", To: "bob", Type: message.Info, TS: message.Now(), Summary: "new-mid", Status: message.Open, Body: fmt.Sprintf("body-%d", 2)}); err != nil {
		t.Fatalf("append message: %v", err)
	}

	next, _ := m.Update(tuiTickMsg{})
	m = next.(*tuiModel)
	if !strings.Contains(m.convoViewport.GetContent(), "new-mid") {
		t.Fatalf("threads pane content missing new message summary after refresh")
	}
	if strings.TrimSpace(m.detailsViewport.GetContent()) == "" {
		t.Fatalf("details pane should remain populated after refresh")
	}
}

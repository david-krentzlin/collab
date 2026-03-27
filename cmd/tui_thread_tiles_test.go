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
	if !strings.Contains(content, "┌ [-] Thread #1") {
		t.Fatalf("expected thread tile header, got: %q", content)
	}
	if !strings.Contains(content, "│ └─ #2") {
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
	if !strings.Contains(m.convoViewport.GetContent(), "[+]") {
		t.Fatalf("expected collapsed marker [+] in tile header")
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

package cmd

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/david-krentzlin/collab/internal/store"
)

func TestTUIModelRefreshTasksDiscoversKnownTasks(t *testing.T) {
	base := filepath.Join(t.TempDir(), store.CollabDir)
	createTaskIndexFile(t, base, "default", []int{1})
	createTaskIndexFile(t, base, "feature-a", []int{1, 2})
	createTaskIndexFile(t, base, "feature-b", []int{1, 2, 3})

	m := newTUIModel()
	m.collabBasePath = base

	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks: %v", err)
	}

	if len(m.tasks) != 3 {
		t.Fatalf("task count = %d, want 3", len(m.tasks))
	}

	names := []string{m.tasks[0].name, m.tasks[1].name, m.tasks[2].name}
	want := []string{"default", "feature-a", "feature-b"}
	if !slices.Equal(names, want) {
		t.Fatalf("task names = %v, want %v", names, want)
	}
}

func TestTUIModelRefreshTasksAddsNewTaskOnSubsequentRefresh(t *testing.T) {
	base := filepath.Join(t.TempDir(), store.CollabDir)
	createTaskIndexFile(t, base, "default", []int{1})

	m := newTUIModel()
	m.collabBasePath = base

	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("first refresh tasks: %v", err)
	}
	if len(m.tasks) != 1 {
		t.Fatalf("task count after first refresh = %d, want 1", len(m.tasks))
	}

	createTaskIndexFile(t, base, "feature-x", []int{1})

	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("second refresh tasks: %v", err)
	}
	if len(m.tasks) != 2 {
		t.Fatalf("task count after second refresh = %d, want 2", len(m.tasks))
	}

	if got := []string{m.tasks[0].name, m.tasks[1].name}; !slices.Contains(got, "feature-x") {
		t.Fatalf("expected feature-x in tasks, got %v", got)
	}
}

func TestTUIModelTaskLineShowsMessageCount(t *testing.T) {
	m := newTUIModel()
	m.tasks = []taskItem{{name: "task-a", messageCount: 3}}

	line := m.taskLine(0)
	if !strings.Contains(line, "(3)") {
		t.Fatalf("task line should include message count, got %q", line)
	}
}

func TestTUIModelRenderTaskPaneRowShowsFolderAndCount(t *testing.T) {
	m := newTUIModel()
	m.tasks = []taskItem{{name: "task-a", messageCount: 9}}

	row := m.renderTaskPaneRow(0, 24)
	if !strings.Contains(row, iconTask) {
		t.Fatalf("task row should include folder icon, got %q", row)
	}
	if !strings.Contains(row, "(9)") {
		t.Fatalf("task row should include count, got %q", row)
	}
}

func TestTUIModelRenderTaskPaneRowShowsUnreadMarker(t *testing.T) {
	m := newTUIModel()
	m.tasks = []taskItem{{name: "task-a", messageCount: 1, hasUnread: true}}

	row := m.renderTaskPaneRow(0, 24)
	if !strings.Contains(row, "●") {
		t.Fatalf("task row should include unread marker, got %q", row)
	}
}

func TestTUIModelMarksSelectedTaskWithStar(t *testing.T) {
	m := newTUIModel()
	m.tasks = []taskItem{{name: "task-a"}, {name: "task-b"}}
	m.selectedTaskIdx = 1

	selected := m.taskLine(1)
	nonSelected := m.taskLine(0)

	if !strings.HasPrefix(selected, "*") {
		t.Fatalf("selected task should be prefixed with '*', got %q", selected)
	}
	if strings.HasPrefix(nonSelected, "*") {
		t.Fatalf("non-selected task should not be prefixed with '*', got %q", nonSelected)
	}
}

func TestTUIModelUnreadMarkerWhenNewDataArrives(t *testing.T) {
	base := filepath.Join(t.TempDir(), store.CollabDir)
	createTaskIndexFile(t, base, "task-a", []int{1, 2, 3})

	m := newTUIModel()
	m.collabBasePath = base
	m.lastSeenByTask["task-a"] = 2

	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("refresh tasks: %v", err)
	}

	if len(m.tasks) != 1 {
		t.Fatalf("task count = %d, want 1", len(m.tasks))
	}
	if !m.tasks[0].hasUnread {
		t.Fatalf("expected task to be marked unread")
	}
	if !strings.Contains(m.taskLine(0), "●") {
		t.Fatalf("task line should include unread marker, got %q", m.taskLine(0))
	}
}

func TestTUIModelSelectingTaskClearsUnreadForThatTask(t *testing.T) {
	m := newTUIModel()
	m.tasks = []taskItem{{name: "task-a", maxSeq: 3, hasUnread: true}}
	m.selectedTaskIdx = 0

	m.markSelectedTaskSeen()

	if got := m.lastSeenByTask["task-a"]; got != 3 {
		t.Fatalf("last seen for task-a = %d, want 3", got)
	}
	if m.tasks[0].hasUnread {
		t.Fatalf("expected unread marker to clear for selected task")
	}
}

func TestTUIModelRefreshPreservesSelectionByTaskName(t *testing.T) {
	base := filepath.Join(t.TempDir(), store.CollabDir)
	createTaskIndexFile(t, base, "default", []int{1})
	createTaskIndexFile(t, base, "feature-b", []int{1, 2})

	m := newTUIModel()
	m.collabBasePath = base

	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("first refresh tasks: %v", err)
	}
	m.selectedTaskIdx = 1 // feature-b in lexical order: default, feature-b

	createTaskIndexFile(t, base, "a-task", []int{1})

	if err := m.refreshTasksFromDisk(); err != nil {
		t.Fatalf("second refresh tasks: %v", err)
	}

	if len(m.tasks) != 3 {
		t.Fatalf("task count after second refresh = %d, want 3", len(m.tasks))
	}
	if selected := m.tasks[m.selectedTaskIdx].name; selected != "feature-b" {
		t.Fatalf("selected task after refresh = %q, want feature-b", selected)
	}
}

func createTaskIndexFile(t *testing.T, base, task string, seqs []int) {
	t.Helper()

	taskDir := filepath.Join(base, task)
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatalf("create task dir %q: %v", task, err)
	}

	indexPath := filepath.Join(taskDir, store.IndexFile)
	var lines []string
	for _, seq := range seqs {
		lines = append(lines, `{"seq":`+itoa(seq)+`}`)
	}
	content := strings.Join(lines, "\n")
	if content != "" {
		content += "\n"
	}
	if err := os.WriteFile(indexPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write index file for %q: %v", task, err)
	}
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	buf := make([]byte, 0, 12)
	for v > 0 {
		buf = append(buf, byte('0'+v%10))
		v /= 10
	}
	if neg {
		buf = append(buf, '-')
	}
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}

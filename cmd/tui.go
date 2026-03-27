package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/david-krentzlin/collab/internal/message"
	"github.com/david-krentzlin/collab/internal/render"
	"github.com/david-krentzlin/collab/internal/store"
	"github.com/spf13/cobra"
)

const (
	tuiPaneSeparatorWidth = 1
	tuiRefreshInterval    = time.Second
)

type tuiTickMsg struct{}

type taskItem struct {
	name         string
	messageCount int
	maxSeq       int
	hasUnread    bool
}

type tuiModel struct {
	width           int
	height          int
	tasks           []taskItem
	selectedTaskIdx int
	lastSeenByTask  map[string]int
	collabBasePath  string
	convoViewport   viewport.Model
	autoFollow      bool
}

func newTUIModel() *tuiModel {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}

	m := &tuiModel{
		lastSeenByTask: make(map[string]int),
		collabBasePath: store.Find(cwd).Base,
		convoViewport:  viewport.New(),
		autoFollow:     true,
	}
	_ = m.refreshTasksFromDisk()
	_ = m.refreshSelectedConversation()

	return m
}

func (m *tuiModel) Init() tea.Cmd {
	return nextTUITickCmd()
}

func (m *tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.setSize(msg.Width, msg.Height)
		return m, nil
	case tuiTickMsg:
		_ = m.refreshTasksFromDisk()
		_ = m.refreshSelectedConversation()
		return m, nextTUITickCmd()
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			m.moveTaskSelection(-1)
			return m, nil
		case "down", "j":
			m.moveTaskSelection(1)
			return m, nil
		case "pgup", "b":
			m.convoViewport.PageUp()
			m.autoFollow = false
			return m, nil
		case "pgdown", "f":
			m.convoViewport.PageDown()
			if m.convoViewport.AtBottom() {
				m.autoFollow = true
			}
			return m, nil
		}
	}

	return m, nil
}

func (m *tuiModel) View() tea.View {
	leftWidth, rightWidth := m.paneWidths(m.width)

	headerStyleLeft := lipgloss.NewStyle().Width(leftWidth).MaxWidth(leftWidth)
	headerStyleRight := lipgloss.NewStyle().Width(rightWidth).MaxWidth(rightWidth)

	header := headerStyleLeft.Render("Tasks") + "|" + headerStyleRight.Render("Conversations")

	dividerWidth := m.width
	if dividerWidth <= 0 {
		dividerWidth = lipgloss.Width(header)
	}
	divider := strings.Repeat("-", dividerWidth)

	lineCount := m.height
	if lineCount < 2 {
		lineCount = 2
	}

	lines := make([]string, 0, lineCount)
	lines = append(lines, header, divider)
	bodyRows := max(lineCount-2, 0)
	for row := 0; row < bodyRows; row++ {
		leftContent := ""
		if row < len(m.tasks) {
			leftContent = m.taskLine(row)
		}

		rightContent := ""
		rightLines := strings.Split(m.convoViewport.View(), "\n")
		if row < len(rightLines) {
			rightContent = rightLines[row]
		}

		leftRendered := headerStyleLeft.Render(leftContent)
		rightRendered := headerStyleRight.Render(rightContent)
		lines = append(lines, leftRendered+"|"+rightRendered)
	}

	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	return v
}

func (m *tuiModel) refreshTasksFromDisk() error {
	selectedTaskName := ""
	if m.selectedTaskIdx >= 0 && m.selectedTaskIdx < len(m.tasks) {
		selectedTaskName = m.tasks[m.selectedTaskIdx].name
	}

	entries, err := os.ReadDir(m.collabBasePath)
	if err != nil {
		if os.IsNotExist(err) {
			m.tasks = nil
			m.selectedTaskIdx = 0
			return nil
		}
		return fmt.Errorf("read collab tasks: %w", err)
	}

	tasks := make([]taskItem, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		taskName := entry.Name()
		taskDir := filepath.Join(m.collabBasePath, taskName)
		messageCount, maxSeq := readTaskIndexStats(taskDir)
		tasks = append(tasks, taskItem{
			name:         taskName,
			messageCount: messageCount,
			maxSeq:       maxSeq,
			hasUnread:    maxSeq > m.lastSeenByTask[taskName],
		})
	}

	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].name < tasks[j].name
	})
	m.tasks = tasks

	if len(m.tasks) == 0 {
		m.selectedTaskIdx = 0
		_ = m.refreshSelectedConversation()
		return nil
	}

	if selectedTaskName == "" {
		if m.selectedTaskIdx < 0 || m.selectedTaskIdx >= len(m.tasks) {
			m.selectedTaskIdx = 0
		}
		_ = m.refreshSelectedConversation()
		return nil
	}

	for i, task := range m.tasks {
		if task.name == selectedTaskName {
			m.selectedTaskIdx = i
			_ = m.refreshSelectedConversation()
			return nil
		}
	}

	if m.selectedTaskIdx < 0 || m.selectedTaskIdx >= len(m.tasks) {
		m.selectedTaskIdx = 0
	}
	_ = m.refreshSelectedConversation()

	return nil
}

func (m *tuiModel) taskLine(i int) string {
	if i < 0 || i >= len(m.tasks) {
		return ""
	}

	task := m.tasks[i]
	selectionMark := " "
	if i == m.selectedTaskIdx {
		selectionMark = "*"
	}

	unreadMark := ""
	if task.hasUnread {
		unreadMark = " ●"
	}

	return fmt.Sprintf("%s%s (%d)%s", selectionMark, task.name, task.messageCount, unreadMark)
}

func (m *tuiModel) markSelectedTaskSeen() {
	if m.selectedTaskIdx < 0 || m.selectedTaskIdx >= len(m.tasks) {
		return
	}
	if m.lastSeenByTask == nil {
		m.lastSeenByTask = make(map[string]int)
	}

	task := &m.tasks[m.selectedTaskIdx]
	m.lastSeenByTask[task.name] = task.maxSeq
	task.hasUnread = false
}

func (m *tuiModel) selectedTaskName() string {
	if m.selectedTaskIdx < 0 || m.selectedTaskIdx >= len(m.tasks) {
		return ""
	}
	return m.tasks[m.selectedTaskIdx].name
}

func (m *tuiModel) refreshSelectedConversation() error {
	taskName := m.selectedTaskName()
	if taskName == "" {
		m.convoViewport.SetContent("(no task selected)")
		if m.autoFollow {
			m.convoViewport.GotoBottom()
		}
		return nil
	}

	s := &store.Store{
		Root: filepath.Join(m.collabBasePath, taskName),
		Base: m.collabBasePath,
		Task: taskName,
	}

	entries, err := s.List(0, "")
	if err != nil {
		m.convoViewport.SetContent("(no messages)")
		if m.autoFollow {
			m.convoViewport.GotoBottom()
		}
		return nil
	}

	msgs := make([]*message.Message, 0, len(entries))
	for _, entry := range entries {
		msg, err := s.ReadMessageAtPath(entry.Path)
		if err != nil {
			continue
		}
		msgs = append(msgs, msg)
	}

	if len(msgs) == 0 {
		m.convoViewport.SetContent("(no messages)")
	} else {
		m.convoViewport.SetContent(formatThreadedConversation(msgs))
	}

	if m.autoFollow {
		m.convoViewport.GotoBottom()
	}

	return nil
}

func formatThreadedConversation(msgs []*message.Message) string {
	threads, orphans := render.BuildThreads(msgs)
	lines := make([]string, 0, len(msgs)+1)

	for i, thread := range threads {
		appendThreadLines(&lines, thread.Root, "", true, true)
		if i < len(threads)-1 {
			lines = append(lines, "")
		}
	}

	if len(orphans) > 0 {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, "Orphans:")
		for _, orphan := range orphans {
			lines = append(lines, "  ? "+formatThreadMessage(orphan.Message))
		}
	}

	if len(lines) == 0 {
		return "(no messages)"
	}

	return strings.Join(lines, "\n")
}

func appendThreadLines(lines *[]string, node *render.Node, prefix string, isLast bool, isRoot bool) {
	if node == nil || node.Message == nil {
		return
	}

	if isRoot {
		*lines = append(*lines, formatThreadMessage(node.Message))
	} else {
		connector := "├─ "
		if isLast {
			connector = "└─ "
		}
		*lines = append(*lines, prefix+connector+formatThreadMessage(node.Message))
	}

	childPrefix := prefix
	if isRoot {
		childPrefix = ""
	} else if isLast {
		childPrefix = prefix + "   "
	} else {
		childPrefix = prefix + "│  "
	}

	for i, child := range node.Children {
		appendThreadLines(lines, child, childPrefix, i == len(node.Children)-1, false)
	}
}

func formatThreadMessage(msg *message.Message) string {
	return fmt.Sprintf("#%d %s [%s] %s", msg.Seq, msg.From, msg.Type, msg.Summary)
}

func (m *tuiModel) moveTaskSelection(delta int) {
	if len(m.tasks) == 0 || delta == 0 {
		return
	}

	next := m.selectedTaskIdx + delta
	if next < 0 {
		next = 0
	}
	if next >= len(m.tasks) {
		next = len(m.tasks) - 1
	}
	if next == m.selectedTaskIdx {
		return
	}

	m.selectedTaskIdx = next
	m.autoFollow = true
	m.markSelectedTaskSeen()
	_ = m.refreshSelectedConversation()
}

func readTaskIndexStats(taskDir string) (messageCount, maxSeq int) {
	indexPath := filepath.Join(taskDir, store.IndexFile)
	f, err := os.Open(indexPath)
	if err != nil {
		return 0, 0
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	for {
		var record struct {
			Seq int `json:"seq"`
		}
		if err := dec.Decode(&record); err != nil {
			if err == io.EOF {
				break
			}
			return 0, 0
		}

		messageCount++
		if record.Seq > maxSeq {
			maxSeq = record.Seq
		}
	}

	return messageCount, maxSeq
}

func (m *tuiModel) setSize(width, height int) {
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
	m.width = width
	m.height = height
	m.syncViewportSize()
}

func (m *tuiModel) paneWidths(totalWidth int) (left, right int) {
	if totalWidth <= tuiPaneSeparatorWidth {
		if totalWidth <= 0 {
			return 0, 0
		}
		return totalWidth, 0
	}

	available := totalWidth - tuiPaneSeparatorWidth
	left = available / 3
	if left < 1 {
		left = 1
	}
	right = available - left
	if right < 1 {
		right = 1
		left = available - right
	}
	if left < 0 {
		left = 0
	}

	return left, right
}

func (m *tuiModel) syncViewportSize() {
	_, rightWidth := m.paneWidths(m.width)
	bodyRows := max(m.height-2, 0)
	m.convoViewport.SetWidth(rightWidth)
	m.convoViewport.SetHeight(bodyRows)
}

func nextTUITickCmd() tea.Cmd {
	return tea.Tick(tuiRefreshInterval, func(time.Time) tea.Msg {
		return tuiTickMsg{}
	})
}

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Open a terminal UI for pairing conversations",
	RunE: func(cmd *cobra.Command, args []string) error {
		program := tea.NewProgram(newTUIModel())
		_, err := program.Run()
		return err
	},
}

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
	"golang.org/x/term"
)

const (
	tuiPaneSeparatorWidth = 4
	tuiRefreshInterval    = time.Second
	tuiTopbarTitle        = "collab viewer"
	tuiFooterHints        = "↑/↓ select   Enter collapse   PgUp/PgDn scroll   t mode   q quit"
)

type tuiTickMsg struct{}

type conversationRenderMode string

const (
	renderModeTimeline conversationRenderMode = "timeline"
	renderModeThreaded conversationRenderMode = "threaded"
)

type viewerPaneFocus string

const (
	focusTasks   viewerPaneFocus = "tasks"
	focusThreads viewerPaneFocus = "threads"
	focusDetails viewerPaneFocus = "details"
)

type conversationRow struct {
	text       string
	seq        int
	rootSeq    int
	selectable bool
	kind       string
}

type taskItem struct {
	name         string
	messageCount int
	maxSeq       int
	hasUnread    bool
}

type tuiModel struct {
	width              int
	height             int
	tasks              []taskItem
	selectedTaskIdx    int
	lastSeenByTask     map[string]int
	collabBasePath     string
	convoViewport      viewport.Model
	autoFollow         bool
	colorEnabled       bool
	renderMode         conversationRenderMode
	focusedPane        viewerPaneFocus
	conversationRows   []conversationRow
	selectedRowIdx     int
	selectedMessageSeq int
	selectedThreadRoot int
	messageBySeq       map[int]*message.Message
	detailsViewport    viewport.Model
	collapsedThreads   map[int]bool

	conversationIndexedCount int
	conversationShownCount   int
	conversationSkippedCount int
}

func newTUIModel() *tuiModel {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}

	m := &tuiModel{
		lastSeenByTask:   make(map[string]int),
		collabBasePath:   store.Find(cwd).Base,
		convoViewport:    viewport.New(),
		autoFollow:       true,
		colorEnabled:     term.IsTerminal(int(os.Stdout.Fd())),
		renderMode:       renderModeThreaded,
		focusedPane:      focusTasks,
		messageBySeq:     make(map[int]*message.Message),
		detailsViewport:  viewport.New(),
		collapsedThreads: make(map[int]bool),
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
		case "enter", "space", " ":
			m.toggleSelectedThreadCollapse()
			return m, nil
		case "tab":
			m.cycleFocus()
			return m, nil
		case "t":
			m.toggleRenderMode()
			return m, nil
		case "up", "k":
			m.handleMoveUp()
			return m, nil
		case "down", "j":
			m.handleMoveDown()
			return m, nil
		case "pgup", "b":
			switch m.focusedPane {
			case focusDetails:
				m.detailsViewport.PageUp()
			case focusThreads:
				m.convoViewport.PageUp()
				m.autoFollow = false
			}
			return m, nil
		case "pgdown", "f":
			switch m.focusedPane {
			case focusDetails:
				m.detailsViewport.PageDown()
			case focusThreads:
				m.convoViewport.PageDown()
				if m.convoViewport.AtBottom() {
					m.autoFollow = true
				}
			}
			return m, nil
		}
	case tea.MouseWheelMsg:
		mouse := msg.Mouse()
		pane, ok := m.bodyPaneAt(mouse.X, mouse.Y)
		if !ok {
			return m, nil
		}

		delta := m.convoViewport.MouseWheelDelta
		if delta <= 0 {
			delta = 3
		}

		switch pane {
		case focusThreads:
			switch mouse.Button {
			case tea.MouseWheelUp:
				m.convoViewport.ScrollUp(delta)
				m.autoFollow = false
			case tea.MouseWheelDown:
				m.convoViewport.ScrollDown(delta)
				if m.convoViewport.AtBottom() {
					m.autoFollow = true
				}
			}
		case focusDetails:
			switch mouse.Button {
			case tea.MouseWheelUp:
				m.detailsViewport.ScrollUp(delta)
			case tea.MouseWheelDown:
				m.detailsViewport.ScrollDown(delta)
			}
		}

		return m, nil
	}

	return m, nil
}

func (m *tuiModel) View() tea.View {
	totalWidth := m.width
	if totalWidth <= 0 {
		totalWidth = 1
	}

	topbarStyle := lipgloss.NewStyle().Width(totalWidth).Align(lipgloss.Center)
	headerStyleLeft := lipgloss.NewStyle()
	headerStyleRight := lipgloss.NewStyle()
	selectedTaskStyle := lipgloss.NewStyle()
	footerStyle := lipgloss.NewStyle().Width(totalWidth).Align(lipgloss.Center)

	if m.colorEnabled {
		topbarStyle = topbarStyle.Bold(true).Foreground(lipgloss.Color("#1F2330")).Background(lipgloss.Color("#A6E3A1"))
		headerStyleLeft = headerStyleLeft.Bold(true).Foreground(lipgloss.Color("#89B4FA"))
		headerStyleRight = headerStyleRight.Bold(true).Foreground(lipgloss.Color("#89B4FA"))
		selectedTaskStyle = selectedTaskStyle.Bold(true).Foreground(lipgloss.Color("#F9E2AF"))
		footerStyle = footerStyle.Foreground(lipgloss.Color("#BAC2DE"))
	}
	footerLine := footerStyle.Render(fitCellContent(m.footerText(), totalWidth))

	lines := make([]string, 0, max(m.height, 1))
	lines = append(lines, topbarStyle.Render(tuiTopbarTitle))

	if m.height <= 1 {
		v := tea.NewView(strings.Join(lines, "\n"))
		v.AltScreen = true
		return v
	}
	if m.height == 2 {
		lines = append(lines, footerLine)
		lines = finalizeViewLines(lines, m.height, footerLine)
		v := tea.NewView(strings.Join(lines, "\n"))
		v.AltScreen = true
		return v
	}

	tasksWidth, threadsWidth, detailsWidth := m.paneWidths(totalWidth)
	cellHeaderTasks := lipgloss.NewStyle().Width(tasksWidth).MaxWidth(tasksWidth)
	cellHeaderThreads := lipgloss.NewStyle().Width(threadsWidth).MaxWidth(threadsWidth)
	cellHeaderDetails := lipgloss.NewStyle().Width(detailsWidth).MaxWidth(detailsWidth)
	bodyStyleTasks := lipgloss.NewStyle().Width(tasksWidth).MaxWidth(tasksWidth)
	bodyStyleThreads := lipgloss.NewStyle().Width(threadsWidth).MaxWidth(threadsWidth)
	bodyStyleDetails := lipgloss.NewStyle().Width(detailsWidth).MaxWidth(detailsWidth)

	topFrame := "┌" + strings.Repeat("─", tasksWidth) + "┬" + strings.Repeat("─", threadsWidth) + "┬" + strings.Repeat("─", detailsWidth) + "┐"
	lines = append(lines, topFrame)

	headerTasks := cellHeaderTasks.Render(headerStyleLeft.Render(m.paneHeaderLabel("Tasks", focusTasks)))
	headerThreads := cellHeaderThreads.Render(headerStyleRight.Render(m.paneHeaderLabel("Threads", focusThreads)))
	headerDetails := cellHeaderDetails.Render(headerStyleRight.Render(m.paneHeaderLabel("Details", focusDetails)))
	header := "│" + headerTasks + "│" + headerThreads + "│" + headerDetails + "│"
	lines = append(lines, header)

	frameRowsAvailable := m.height - 2           // topbar + footer reserved
	remainingFrameRows := frameRowsAvailable - 2 // top frame + header already emitted
	if remainingFrameRows <= 0 {
		lines = append(lines, footerLine)
		lines = finalizeViewLines(lines, m.height, footerLine)
		v := tea.NewView(strings.Join(lines, "\n"))
		v.AltScreen = true
		return v
	}

	separator := "├" + strings.Repeat("─", tasksWidth) + "┼" + strings.Repeat("─", threadsWidth) + "┼" + strings.Repeat("─", detailsWidth) + "┤"
	lines = append(lines, separator)
	remainingFrameRows--

	bodyRows := max(remainingFrameRows-1, 0) // reserve bottom frame
	threadLines := strings.Split(m.convoViewport.View(), "\n")
	detailLines := strings.Split(m.detailsViewport.View(), "\n")
	for row := 0; row < bodyRows; row++ {
		taskContent := ""
		if row < len(m.tasks) {
			taskContent = m.taskLine(row)
		}

		threadContent := ""
		if row < len(threadLines) {
			threadContent = threadLines[row]
		}

		detailContent := ""
		if row < len(detailLines) {
			detailContent = detailLines[row]
		}

		taskCell := fitCellContent(taskContent, tasksWidth)
		if row < len(m.tasks) && row == m.selectedTaskIdx {
			taskCell = selectedTaskStyle.Render(taskCell)
		}
		tasksRendered := bodyStyleTasks.Render(taskCell)

		threadCell := fitCellContent(threadContent, threadsWidth)
		if row < len(m.conversationRows) && row == m.selectedRowIdx {
			threadCell = selectedTaskStyle.Render(threadCell)
		}
		threadsRendered := bodyStyleThreads.Render(threadCell)

		detailsRendered := bodyStyleDetails.Render(fitCellContent(detailContent, detailsWidth))
		lines = append(lines, "│"+tasksRendered+"│"+threadsRendered+"│"+detailsRendered+"│")
	}

	bottomFrame := "└" + strings.Repeat("─", tasksWidth) + "┴" + strings.Repeat("─", threadsWidth) + "┴" + strings.Repeat("─", detailsWidth) + "┘"
	lines = append(lines, bottomFrame)
	lines = append(lines, footerLine)
	lines = finalizeViewLines(lines, m.height, footerLine)

	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
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
		m.resetConversationStats()
		m.conversationRows = nil
		m.selectedRowIdx = 0
		m.selectedMessageSeq = 0
		m.selectedThreadRoot = 0
		m.messageBySeq = map[int]*message.Message{}
		m.convoViewport.SetContent("(no task selected)")
		m.detailsViewport.SetContent("(no message selected)")
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
		m.resetConversationStats()
		m.conversationRows = nil
		m.selectedRowIdx = 0
		m.selectedMessageSeq = 0
		m.selectedThreadRoot = 0
		m.messageBySeq = map[int]*message.Message{}
		m.convoViewport.SetContent("(no messages)")
		m.detailsViewport.SetContent("(no message selected)")
		if m.autoFollow {
			m.convoViewport.GotoBottom()
		}
		return nil
	}

	m.conversationIndexedCount = len(entries)
	m.conversationSkippedCount = 0
	m.conversationShownCount = 0

	msgs := make([]*message.Message, 0, len(entries))
	messageBySeq := make(map[int]*message.Message, len(entries))
	for _, entry := range entries {
		msg, err := s.ReadMessageAtPath(entry.Path)
		if err != nil {
			m.conversationSkippedCount++
			continue
		}
		msgs = append(msgs, msg)
		messageBySeq[msg.Seq] = msg
	}
	m.conversationShownCount = len(msgs)
	m.messageBySeq = messageBySeq

	var rows []conversationRow
	switch m.renderMode {
	case renderModeThreaded:
		rows = m.buildThreadedRows(msgs)
	default:
		rows = m.buildTimelineRows(msgs)
	}
	m.conversationRows = rows

	if len(rows) == 0 {
		m.convoViewport.SetContent("(no messages)")
		m.selectedRowIdx = 0
		m.selectedMessageSeq = 0
		m.detailsViewport.SetContent("(no message selected)")
	} else {
		m.restoreConversationSelection()
		if m.selectedRowIdx < 0 || m.selectedRowIdx >= len(rows) {
			m.selectedRowIdx = firstSelectableConversationRow(rows)
		}
		if m.selectedRowIdx >= 0 && m.selectedRowIdx < len(rows) {
			m.selectedMessageSeq = rows[m.selectedRowIdx].seq
			m.selectedThreadRoot = rows[m.selectedRowIdx].rootSeq
		}
		m.convoViewport.SetContent(conversationRowsText(rows))
		m.refreshDetailsFromSelection()
	}

	if m.autoFollow {
		m.convoViewport.GotoBottom()
	}

	return nil
}

func (m *tuiModel) buildThreadedRows(msgs []*message.Message) []conversationRow {
	threads, orphans := render.BuildThreads(msgs)
	rows := make([]conversationRow, 0, len(msgs)+1)

	for _, thread := range threads {
		if thread == nil || thread.Root == nil || thread.Root.Message == nil {
			continue
		}
		root := thread.Root.Message
		rootSeq := root.Seq
		collapsed := m.collapsedThreads[rootSeq]
		state := "[-]"
		if collapsed {
			state = "[+]"
		}

		count := len(render.FlattenThread(thread))
		header := fmt.Sprintf("┌ %s Thread #%d %s [%s] %s (%d msgs)", state, rootSeq, m.agentLabel(root.From), root.Type, root.Summary, count)
		rows = append(rows, conversationRow{text: header, seq: rootSeq, rootSeq: rootSeq, selectable: true, kind: "thread-header"})

		if !collapsed {
			for i, child := range thread.Root.Children {
				appendThreadChildRows(&rows, child, "", i == len(thread.Root.Children)-1, rootSeq, m)
			}
		}

		rows = append(rows, conversationRow{text: "└", seq: 0, rootSeq: rootSeq, selectable: false, kind: "thread-bottom"})
		rows = append(rows, conversationRow{text: "", seq: 0, rootSeq: 0, selectable: false, kind: "spacer"})
	}

	if len(orphans) > 0 {
		rows = append(rows, conversationRow{text: "Orphans:", seq: 0, rootSeq: 0, selectable: false, kind: "orphans-header"})
		for _, orphan := range orphans {
			if orphan == nil || orphan.Message == nil {
				continue
			}
			rows = append(rows, conversationRow{
				text:       "  ? " + m.formatThreadMessage(orphan.Message),
				seq:        orphan.Message.Seq,
				rootSeq:    orphan.Message.Seq,
				selectable: true,
				kind:       "orphan",
			})
		}
	}

	for len(rows) > 0 && rows[len(rows)-1].kind == "spacer" {
		rows = rows[:len(rows)-1]
	}

	return rows
}

func appendThreadChildRows(rows *[]conversationRow, node *render.Node, prefix string, isLast bool, rootSeq int, m *tuiModel) {
	if node == nil || node.Message == nil {
		return
	}

	connector := "├─ "
	if isLast {
		connector = "└─ "
	}
	line := fmt.Sprintf("│ %s%s%s", prefix, connector, m.formatThreadMessage(node.Message))
	*rows = append(*rows, conversationRow{text: line, seq: node.Message.Seq, rootSeq: rootSeq, selectable: true, kind: "message"})

	childPrefix := prefix
	if isLast {
		childPrefix += "   "
	} else {
		childPrefix += "│  "
	}

	for i, child := range node.Children {
		appendThreadChildRows(rows, child, childPrefix, i == len(node.Children)-1, rootSeq, m)
	}
}

func (m *tuiModel) formatThreadMessage(msg *message.Message) string {
	return fmt.Sprintf("#%d %s [%s] %s", msg.Seq, m.agentLabel(msg.From), msg.Type, msg.Summary)
}

func (m *tuiModel) buildTimelineRows(msgs []*message.Message) []conversationRow {
	sorted := make([]*message.Message, len(msgs))
	copy(sorted, msgs)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Seq < sorted[j].Seq
	})

	rows := make([]conversationRow, 0, len(sorted))
	for _, msg := range sorted {
		rows = append(rows, conversationRow{
			text:       fmt.Sprintf("#%d %s -> %s [%s] %s", msg.Seq, m.agentLabel(msg.From), m.agentLabel(msg.To), msg.Type, msg.Summary),
			seq:        msg.Seq,
			rootSeq:    msg.Seq,
			selectable: true,
			kind:       "message",
		})
	}

	return rows
}

func (m *tuiModel) resetConversationStats() {
	m.conversationIndexedCount = 0
	m.conversationShownCount = 0
	m.conversationSkippedCount = 0
}

var threadAgentPalette = []string{"#89B4FA", "#F38BA8", "#A6E3A1", "#FAB387", "#CBA6F7", "#74C7EC"}

func (m *tuiModel) agentLabel(name string) string {
	if !m.colorEnabled || strings.TrimSpace(name) == "" {
		return name
	}

	sum := 0
	for _, r := range name {
		sum += int(r)
	}
	color := threadAgentPalette[sum%len(threadAgentPalette)]
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(color)).Render(name)
}

func (m *tuiModel) paneHeaderLabel(title string, pane viewerPaneFocus) string {
	prefix := "  "
	if m.focusedPane == pane {
		prefix = "▶ "
	}
	return prefix + title
}

func (m *tuiModel) footerText() string {
	return fmt.Sprintf("%s   mode:%s   shown:%d/%d skipped:%d",
		tuiFooterHints,
		m.renderMode,
		m.conversationShownCount,
		m.conversationIndexedCount,
		m.conversationSkippedCount,
	)
}

func (m *tuiModel) toggleRenderMode() {
	prevOffset := m.convoViewport.YOffset()
	if m.renderMode == renderModeThreaded {
		m.renderMode = renderModeTimeline
	} else {
		m.renderMode = renderModeThreaded
	}

	_ = m.refreshSelectedConversation()
	if !m.autoFollow {
		m.convoViewport.SetYOffset(prevOffset)
	}
}

func (m *tuiModel) cycleFocus() {
	switch m.focusedPane {
	case focusTasks:
		m.focusedPane = focusThreads
	case focusThreads:
		m.focusedPane = focusDetails
	default:
		m.focusedPane = focusTasks
	}
}

func (m *tuiModel) handleMoveUp() {
	switch m.focusedPane {
	case focusTasks:
		m.moveTaskSelection(-1)
	case focusThreads:
		m.moveConversationSelection(-1)
	case focusDetails:
		m.detailsViewport.ScrollUp(1)
	}
}

func (m *tuiModel) handleMoveDown() {
	switch m.focusedPane {
	case focusTasks:
		m.moveTaskSelection(1)
	case focusThreads:
		m.moveConversationSelection(1)
	case focusDetails:
		m.detailsViewport.ScrollDown(1)
	}
}

func (m *tuiModel) bodyPaneAt(x, y int) (viewerPaneFocus, bool) {
	if x < 0 || y < 0 {
		return "", false
	}

	bodyStartY := m.convoViewport.YPosition
	bodyEndY := bodyStartY + m.convoViewport.Height() - 1
	if y < bodyStartY || y > bodyEndY {
		return "", false
	}

	tasksWidth, threadsWidth, detailsWidth := m.paneWidths(m.width)
	tasksStartX := 1
	tasksEndX := tasksStartX + tasksWidth - 1
	threadsStartX := tasksEndX + 2
	threadsEndX := threadsStartX + threadsWidth - 1
	detailsStartX := threadsEndX + 2
	detailsEndX := detailsStartX + detailsWidth - 1

	switch {
	case x >= tasksStartX && x <= tasksEndX:
		return focusTasks, true
	case x >= threadsStartX && x <= threadsEndX:
		return focusThreads, true
	case x >= detailsStartX && x <= detailsEndX:
		return focusDetails, true
	default:
		return "", false
	}
}

func firstSelectableConversationRow(rows []conversationRow) int {
	for i, row := range rows {
		if row.selectable {
			return i
		}
	}
	return 0
}

func conversationRowsText(rows []conversationRow) string {
	if len(rows) == 0 {
		return "(no messages)"
	}
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, row.text)
	}
	return strings.Join(lines, "\n")
}

func (m *tuiModel) restoreConversationSelection() {
	if len(m.conversationRows) == 0 {
		m.selectedRowIdx = 0
		m.selectedMessageSeq = 0
		m.selectedThreadRoot = 0
		return
	}

	if m.selectedMessageSeq != 0 {
		for i, row := range m.conversationRows {
			if row.selectable && row.seq == m.selectedMessageSeq {
				m.selectedRowIdx = i
				m.selectedThreadRoot = row.rootSeq
				return
			}
		}
	}

	if m.selectedThreadRoot != 0 {
		for i, row := range m.conversationRows {
			if row.selectable && row.rootSeq == m.selectedThreadRoot && row.kind == "thread-header" {
				m.selectedRowIdx = i
				m.selectedMessageSeq = row.seq
				return
			}
		}
	}

	m.selectedRowIdx = firstSelectableConversationRow(m.conversationRows)
	if m.selectedRowIdx >= 0 && m.selectedRowIdx < len(m.conversationRows) {
		m.selectedMessageSeq = m.conversationRows[m.selectedRowIdx].seq
		m.selectedThreadRoot = m.conversationRows[m.selectedRowIdx].rootSeq
	}
}

func (m *tuiModel) refreshDetailsFromSelection() {
	msg, ok := m.messageBySeq[m.selectedMessageSeq]
	if !ok || msg == nil {
		m.detailsViewport.SetContent("(no message selected)")
		return
	}

	reLine := "-"
	if msg.Re > 0 {
		reLine = fmt.Sprintf("#%d", msg.Re)
	}
	body := strings.TrimRight(msg.Body, "\n")
	if strings.TrimSpace(body) == "" {
		body = "(empty body)"
	}

	metaLine := fmt.Sprintf("From %s   To %s   Type %s   Re %s", m.agentLabel(msg.From), m.agentLabel(msg.To), msg.Type, reLine)
	details := fmt.Sprintf("┌ Message #%d\n│ %s\n│ Status %s   Time %s\n│ Summary %s\n├ Body\n%s\n└ End",
		msg.Seq,
		metaLine,
		msg.Status,
		msg.TS,
		msg.Summary,
		indentLines(body, "│ "),
	)
	m.detailsViewport.SetContent(details)
	m.detailsViewport.GotoTop()
}

func (m *tuiModel) moveConversationSelection(delta int) {
	if len(m.conversationRows) == 0 || delta == 0 {
		return
	}

	idx := m.selectedRowIdx
	for {
		idx += delta
		if idx < 0 || idx >= len(m.conversationRows) {
			return
		}
		if m.conversationRows[idx].selectable {
			m.selectedRowIdx = idx
			m.selectedMessageSeq = m.conversationRows[idx].seq
			m.selectedThreadRoot = m.conversationRows[idx].rootSeq
			m.refreshDetailsFromSelection()
			m.autoFollow = false
			return
		}
	}
}

func (m *tuiModel) toggleSelectedThreadCollapse() {
	if m.focusedPane != focusThreads || len(m.conversationRows) == 0 {
		return
	}
	if m.selectedRowIdx < 0 || m.selectedRowIdx >= len(m.conversationRows) {
		return
	}

	row := m.conversationRows[m.selectedRowIdx]
	if row.rootSeq == 0 {
		return
	}

	if m.collapsedThreads == nil {
		m.collapsedThreads = make(map[int]bool)
	}
	m.collapsedThreads[row.rootSeq] = !m.collapsedThreads[row.rootSeq]
	m.selectedMessageSeq = row.rootSeq
	m.selectedThreadRoot = row.rootSeq
	_ = m.refreshSelectedConversation()
	m.autoFollow = false
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

func (m *tuiModel) paneWidths(totalWidth int) (tasks, threads, details int) {
	if totalWidth <= tuiPaneSeparatorWidth {
		if totalWidth <= 0 {
			return 0, 0, 0
		}
		return totalWidth, 0, 0
	}

	available := totalWidth - tuiPaneSeparatorWidth
	tasks = available / 4
	if tasks < 1 {
		tasks = 1
	}
	threads = (available - tasks) / 2
	if threads < 1 {
		threads = 1
	}
	details = available - tasks - threads
	if details < 1 {
		details = 1
		if threads > 1 {
			threads = available - tasks - details
		}
	}
	if threads < 0 {
		threads = 0
	}
	if details < 0 {
		details = 0
	}

	return tasks, threads, details
}

func (m *tuiModel) syncViewportSize() {
	_, threadsWidth, detailsWidth := m.paneWidths(m.width)
	bodyRows := max(m.height-6, 0)
	m.convoViewport.SetWidth(threadsWidth)
	m.convoViewport.SetHeight(bodyRows)
	m.convoViewport.MouseWheelEnabled = true
	m.convoViewport.MouseWheelDelta = 3
	m.convoViewport.YPosition = 4

	m.detailsViewport.SetWidth(detailsWidth)
	m.detailsViewport.SetHeight(bodyRows)
	m.detailsViewport.MouseWheelEnabled = true
	m.detailsViewport.MouseWheelDelta = 3
	m.detailsViewport.YPosition = 4
}

func nextTUITickCmd() tea.Cmd {
	return tea.Tick(tuiRefreshInterval, func(time.Time) tea.Msg {
		return tuiTickMsg{}
	})
}

func fitCellContent(content string, width int) string {
	if width <= 0 {
		return ""
	}

	clean := strings.ReplaceAll(content, "\n", " ")
	if lipgloss.Width(clean) <= width {
		return clean
	}

	runes := []rune(clean)
	if width == 1 {
		return "…"
	}

	return string(runes[:width-1]) + "…"
}

func indentLines(content, prefix string) string {
	parts := strings.Split(content, "\n")
	for i, part := range parts {
		parts[i] = prefix + part
	}
	return strings.Join(parts, "\n")
}

func finalizeViewLines(lines []string, height int, footerLine string) []string {
	if height > 0 && len(lines) > height {
		lines = lines[:height]
	}

	if height >= 2 {
		if len(lines) == 0 {
			lines = append(lines, "", footerLine)
		} else if len(lines) == 1 {
			lines = append(lines, footerLine)
		} else {
			lines[len(lines)-1] = footerLine
		}
	}

	return lines
}

var viewerCmd = &cobra.Command{
	Use:     "viewer",
	Aliases: []string{"tui"},
	Short:   "Open a terminal UI for pairing conversations",
	RunE: func(cmd *cobra.Command, args []string) error {
		program := tea.NewProgram(newTUIModel())
		_, err := program.Run()
		return err
	},
}

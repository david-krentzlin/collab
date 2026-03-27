package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
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
	tuiFooterHints        = "q quit   ↑/↓ select   J/K thread   g/G top/btm   {/} scroll   Enter collapse   t mode"

	iconThread    = "󰘬"
	iconExpand    = ""
	iconCollapse  = ""
	iconOpen      = ""
	iconResolved  = ""
	iconReference = "󰌷"
	iconTask      = "󰉋"
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
	followUps  int
	depth      int
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
		case "K":
			if m.focusedPane == focusThreads {
				m.moveConversationByThread(-1)
			}
			return m, nil
		case "J":
			if m.focusedPane == focusThreads {
				m.moveConversationByThread(1)
			}
			return m, nil
		case "g":
			switch m.focusedPane {
			case focusTasks:
				m.moveTaskToBoundary(true)
			case focusThreads:
				m.moveConversationToBoundary(true)
			case focusDetails:
				m.detailsViewport.GotoTop()
			}
			return m, nil
		case "G":
			switch m.focusedPane {
			case focusTasks:
				m.moveTaskToBoundary(false)
			case focusThreads:
				m.moveConversationToBoundary(false)
			case focusDetails:
				m.detailsViewport.GotoBottom()
			}
			return m, nil
		case "{", "pgup", "b":
			switch m.focusedPane {
			case focusDetails:
				m.detailsViewport.PageUp()
			case focusThreads:
				m.convoViewport.PageUp()
				m.autoFollow = false
			}
			return m, nil
		case "}", "pgdown", "f":
			switch m.focusedPane {
			case focusDetails:
				m.detailsViewport.PageDown()
			case focusThreads:
				m.autoFollow = false
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
				m.autoFollow = false
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
	footerStyle := lipgloss.NewStyle().Width(totalWidth).Align(lipgloss.Center)

	if m.colorEnabled {
		topbarStyle = topbarStyle.Bold(true).Foreground(lipgloss.Color("#CDD6F4")).Background(lipgloss.Color("#313244"))
		headerStyleLeft = headerStyleLeft.Bold(true).Foreground(lipgloss.Color("#BAC2DE"))
		headerStyleRight = headerStyleRight.Bold(true).Foreground(lipgloss.Color("#BAC2DE"))
		footerStyle = footerStyle.Foreground(lipgloss.Color("#A6ADC8"))
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
	if m.colorEnabled {
		bodyStyleTasks = bodyStyleTasks.Background(lipgloss.Color("#000000")).ColorWhitespace(true)
	}

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
		taskContent := m.renderTaskPaneRow(row, tasksWidth)

		threadContent := ""
		if row < len(threadLines) {
			threadContent = threadLines[row]
		}

		detailContent := ""
		if row < len(detailLines) {
			detailContent = detailLines[row]
		}

		tasksRendered := bodyStyleTasks.Render(taskContent)

		threadCell := threadContent
		threadsRendered := bodyStyleThreads.Render(threadCell)

		detailsRendered := bodyStyleDetails.Render(detailContent)
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

func (m *tuiModel) renderTaskPaneRow(i, width int) string {
	if width <= 0 {
		return ""
	}
	if i < 0 || i >= len(m.tasks) {
		return strings.Repeat(" ", width)
	}

	task := m.tasks[i]
	selected := i == m.selectedTaskIdx
	innerWidth := max(width-2, 1)
	countText := "(" + strconv.Itoa(task.messageCount) + ")"
	countWidth := lipgloss.Width(countText)
	if countWidth+1 > innerWidth {
		countText = ""
		countWidth = 0
	}

	leftWidth := innerWidth
	if countWidth > 0 {
		leftWidth = innerWidth - countWidth - 1
	}
	if leftWidth < 1 {
		leftWidth = 1
	}

	leftText := iconTask + " " + task.name
	if task.hasUnread {
		leftText += " ●"
	}

	rowStyle := lipgloss.NewStyle().Width(width).MaxWidth(width).Padding(0, 1)
	leftStyle := lipgloss.NewStyle().Width(leftWidth).MaxWidth(leftWidth)
	countStyle := lipgloss.NewStyle().Width(countWidth).Align(lipgloss.Right)
	if m.colorEnabled {
		rowBG := lipgloss.Color("#000000")
		rowStyle = rowStyle.Background(rowBG).Foreground(lipgloss.Color("#C7D2E4")).ColorWhitespace(true)
		leftStyle = leftStyle.Foreground(lipgloss.Color("#C7D2E4")).Background(rowBG).ColorWhitespace(true)
		countStyle = countStyle.Foreground(lipgloss.Color("#9AA8C0")).Background(rowBG).ColorWhitespace(true)
		if task.hasUnread {
			leftStyle = leftStyle.Foreground(lipgloss.Color("#E1E9F7")).Bold(true)
		}
		if selected {
			selectedBG := lipgloss.Color("#2D3C58")
			rowStyle = rowStyle.Background(selectedBG).Foreground(lipgloss.Color("#EEF3FC")).ColorWhitespace(true)
			leftStyle = leftStyle.Foreground(lipgloss.Color("#EEF3FC")).Background(selectedBG).Bold(true).ColorWhitespace(true)
			countStyle = countStyle.Foreground(lipgloss.Color("#DFE8F8")).Background(selectedBG).Bold(true).ColorWhitespace(true)
		}
	}

	leftCell := leftStyle.Render(fitCellContent(leftText, leftWidth))
	if countWidth == 0 {
		return rowStyle.Render(leftCell)
	}
	countCell := countStyle.Render(countText)
	separator := " "
	if m.colorEnabled {
		sepBG := lipgloss.Color("#000000")
		if selected {
			sepBG = lipgloss.Color("#2D3C58")
		}
		separator = lipgloss.NewStyle().Background(sepBG).ColorWhitespace(true).Render(" ")
	}
	return rowStyle.Render(lipgloss.JoinHorizontal(lipgloss.Top, leftCell, separator, countCell))
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
		m.refreshConversationViewportContent()
		if !m.autoFollow {
			m.ensureSelectedConversationVisible()
		}
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
		flattened := render.FlattenThread(thread)
		followUps := len(flattened) - 1
		if followUps < 0 {
			followUps = 0
		}
		rows = append(rows, conversationRow{
			text:       root.Summary,
			seq:        rootSeq,
			rootSeq:    rootSeq,
			followUps:  followUps,
			selectable: true,
			kind:       "thread-header",
		})

		if !collapsed {
			sort.Slice(flattened, func(i, j int) bool {
				return flattened[i].Seq < flattened[j].Seq
			})
			bySeq := make(map[int]*message.Message, len(flattened))
			for _, msg := range flattened {
				if msg != nil {
					bySeq[msg.Seq] = msg
				}
			}
			for _, msg := range flattened {
				if msg == nil || msg.Seq == rootSeq {
					continue
				}
				rows = append(rows, conversationRow{
					text:       msg.Summary,
					seq:        msg.Seq,
					rootSeq:    rootSeq,
					depth:      threadDepth(msg, rootSeq, bySeq),
					selectable: true,
					kind:       "message",
				})
			}
		}
	}

	if len(orphans) > 0 {
		rows = append(rows, conversationRow{text: "Orphans:", seq: 0, rootSeq: 0, selectable: false, kind: "orphans-header"})
		for _, orphan := range orphans {
			if orphan == nil || orphan.Message == nil {
				continue
			}
			rows = append(rows, conversationRow{
				text:       orphan.Message.Summary,
				seq:        orphan.Message.Seq,
				rootSeq:    orphan.Message.Seq,
				depth:      1,
				selectable: true,
				kind:       "orphan",
			})
		}
	}

	return rows
}

func statusIcon(status message.Status) string {
	switch status {
	case message.Resolved:
		return iconResolved
	default:
		return iconOpen
	}
}

func shortTime(ts string) string {
	if ts == "" {
		return "--:--"
	}
	parsed, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	return parsed.Format("15:04")
}

func threadDepth(msg *message.Message, rootSeq int, bySeq map[int]*message.Message) int {
	if msg == nil {
		return 1
	}
	if msg.Re <= 0 || msg.Re == rootSeq {
		return 1
	}

	depth := 1
	parent := msg.Re
	for parent > 0 && parent != rootSeq {
		depth++
		parentMsg, ok := bySeq[parent]
		if !ok || parentMsg == nil || parentMsg.Re == parent {
			break
		}
		parent = parentMsg.Re
		if depth >= 8 {
			break
		}
	}

	return depth
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

func (m *tuiModel) agentColor(name string) string {
	sum := 0
	for _, r := range name {
		sum += int(r)
	}
	return threadAgentPalette[sum%len(threadAgentPalette)]
}

func (m *tuiModel) agentLabel(name string) string {
	if !m.colorEnabled || strings.TrimSpace(name) == "" {
		return name
	}

	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.agentColor(name))).Render(name)
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

func lastSelectableConversationRow(rows []conversationRow) int {
	for i := len(rows) - 1; i >= 0; i-- {
		if rows[i].selectable {
			return i
		}
	}
	return 0
}

func (m *tuiModel) selectedMessageReferenceSeq() int {
	msg, ok := m.messageBySeq[m.selectedMessageSeq]
	if !ok || msg == nil {
		return 0
	}
	return msg.Re
}

func (m *tuiModel) conversationRowsText(width int) string {
	if len(m.conversationRows) == 0 {
		return "(no messages)"
	}
	if width <= 0 {
		width = 120
	}

	if m.renderMode == renderModeThreaded {
		return m.threadedRowsText(width)
	}
	return m.timelineRowsText(width)
}

func (m *tuiModel) timelineRowsText(width int) string {
	refSeq := m.selectedMessageReferenceSeq()
	lines := make([]string, 0, len(m.conversationRows))
	baseStyle := lipgloss.NewStyle().Width(width).MaxWidth(width)
	selectedStyle := baseStyle.Copy()
	if m.colorEnabled {
		baseStyle = baseStyle.Foreground(lipgloss.Color("#C8D0DC"))
		selectedStyle = selectedStyle.Bold(true).Foreground(lipgloss.Color("#E8EDF7")).Background(lipgloss.Color("#344157")).ColorWhitespace(true)
	}

	for i, row := range m.conversationRows {
		line := fitCellContent(row.text, width)
		if refSeq > 0 && row.seq == refSeq {
			line = fitCellContent(iconReference+" "+line, width)
		}
		if i == m.selectedRowIdx {
			lines = append(lines, selectedStyle.Render(line))
			continue
		}
		lines = append(lines, baseStyle.Render(line))
	}

	return strings.Join(lines, "\n")
}

func (m *tuiModel) threadedRowsText(width int) string {
	refSeq := m.selectedMessageReferenceSeq()
	blocks := make([]string, 0, len(m.conversationRows)+4)

	for i := 0; i < len(m.conversationRows); {
		row := m.conversationRows[i]
		switch row.kind {
		case "thread-header":
			children := make([]conversationRow, 0, 4)
			childIndexes := make([]int, 0, 4)
			j := i + 1
			for j < len(m.conversationRows) && m.conversationRows[j].rootSeq == row.rootSeq && m.conversationRows[j].kind == "message" {
				children = append(children, m.conversationRows[j])
				childIndexes = append(childIndexes, j)
				j++
			}
			blocks = append(blocks, m.renderThreadTile(row, i, children, childIndexes, width, refSeq))
			i = j
			continue
		case "orphans-header":
			headerStyle := lipgloss.NewStyle().Width(width).MaxWidth(width).MarginTop(1).Bold(true)
			if m.colorEnabled {
				headerStyle = headerStyle.Foreground(lipgloss.Color("#A8B5C9"))
			}
			blocks = append(blocks, headerStyle.Render("Orphans:"))
			i++
		case "orphan":
			line := "? #" + strconv.Itoa(row.seq) + " " + row.text
			if msg := m.messageBySeq[row.seq]; msg != nil {
				line = "? #" + strconv.Itoa(row.seq) + " " + msg.From + " [" + string(msg.Type) + "] " + msg.Summary
			}
			line = fitCellContent(line, width)
			style := lipgloss.NewStyle().Width(width).MaxWidth(width)
			if m.colorEnabled {
				style = style.Foreground(lipgloss.Color("#C8D0DC"))
			}
			if i == m.selectedRowIdx {
				if m.colorEnabled {
					style = style.Bold(true).Foreground(lipgloss.Color("#E8EDF7")).Background(lipgloss.Color("#344157")).ColorWhitespace(true)
				}
			}
			blocks = append(blocks, style.Render(line))
			i++
		default:
			line := row.text
			if refSeq > 0 && row.seq == refSeq {
				line = iconReference + " " + line
			}
			blocks = append(blocks, lipgloss.NewStyle().Width(width).MaxWidth(width).Render(fitCellContent(line, width)))
			i++
		}
	}

	return strings.Join(blocks, "\n")
}

func (m *tuiModel) renderThreadTile(header conversationRow, headerIdx int, children []conversationRow, childIndexes []int, width, refSeq int) string {
	marginX := 2
	marginY := 0
	paddingX := 1
	chrome := marginX*2 + paddingX*2 + 2
	contentWidth := width - chrome
	if contentWidth < 16 {
		paddingX = 0
		chrome = marginX*2 + 2
		contentWidth = width - chrome
	}
	if contentWidth < 1 {
		contentWidth = 1
	}

	headerMsg := m.messageBySeq[header.seq]
	headerReferenced := refSeq > 0 && header.seq == refSeq
	headerSelected := headerIdx == m.selectedRowIdx

	bodyLines := []string{m.renderThreadHeaderLine(header, headerMsg, contentWidth, headerReferenced, headerSelected)}
	for i, row := range children {
		msg := m.messageBySeq[row.seq]
		selected := childIndexes[i] == m.selectedRowIdx
		referenced := refSeq > 0 && row.seq == refSeq
		bodyLines = append(bodyLines, m.renderThreadMessageLine(row, msg, contentWidth, selected, referenced))
	}
	body := lipgloss.JoinVertical(lipgloss.Left, bodyLines...)

	border := lipgloss.NormalBorder()
	selectedThread := header.rootSeq > 0 && header.rootSeq == m.selectedThreadRoot
	if selectedThread {
		border.Left = "┃"
	}

	style := lipgloss.NewStyle().Width(contentWidth).MaxWidth(contentWidth).Padding(0, paddingX).BorderStyle(border)
	if m.colorEnabled {
		style = style.Foreground(lipgloss.Color("#D3DBE7")).Background(lipgloss.Color("#262F3F")).ColorWhitespace(true)
		if selectedThread {
			style = style.BorderForeground(lipgloss.Color("#5FA8FF"))
		} else {
			style = style.BorderForeground(lipgloss.Color("#596273"))
		}
	}

	cell := style.Render(body)
	return lipgloss.NewStyle().Margin(marginY, marginX).Render(cell)
}

func (m *tuiModel) renderThreadHeaderLine(row conversationRow, root *message.Message, width int, referenced bool, selected bool) string {
	marker := iconExpand
	if m.collapsedThreads[row.rootSeq] {
		marker = iconCollapse
	}

	seq := row.seq
	summary := row.text
	ts := ""
	if root != nil {
		seq = root.Seq
		summary = root.Summary
		ts = root.TS
	}

	plural := "reply"
	if row.followUps != 1 {
		plural = "replies"
	}
	prefix := marker + " " + iconThread + " #" + strconv.Itoa(seq) + " "
	if width < 24 {
		prefix = "#" + strconv.Itoa(seq) + " "
	}
	left := prefix + summary
	if referenced {
		left = iconReference + " " + left
	}
	metaDate := shortDate(ts)
	right := strconv.Itoa(row.followUps) + " " + plural
	if metaDate != "" && width >= 24 {
		right += "  " + metaDate
	}

	usableWidth := max(width-4, 1)
	rightWidth := min(max(usableWidth/4, 12), max(usableWidth/2, 1))
	if usableWidth < 28 {
		rightWidth = min(max(usableWidth/3, 8), max(usableWidth/2, 1))
	}
	leftWidth := max(usableWidth-rightWidth, 1)
	rightWidth = max(usableWidth-leftWidth, 0)

	leftStyle := lipgloss.NewStyle().Width(leftWidth)
	rightStyle := lipgloss.NewStyle().Width(rightWidth).Align(lipgloss.Right)
	if m.colorEnabled {
		rightStyle = rightStyle.Foreground(lipgloss.Color("#B7C5D8"))
	}

	leftCell := leftStyle.Render(fitCellContent(left, leftWidth))
	line := leftCell
	if rightWidth > 0 {
		rightCell := rightStyle.Render(fitCellContent(right, rightWidth))
		line = lipgloss.JoinHorizontal(lipgloss.Top, leftCell, rightCell)
	}
	line = lipgloss.NewStyle().Width(width).MaxWidth(width).Render(line)

	style := lipgloss.NewStyle()
	if m.colorEnabled {
		style = style.Bold(true).Foreground(lipgloss.Color("#EAF1FF")).Background(lipgloss.Color("#334257")).ColorWhitespace(true)
		if selected {
			style = style.Background(lipgloss.Color("#40618E"))
		}
		if referenced {
			style = style.Foreground(lipgloss.Color("#FDE68A"))
		}
	}

	return style.Render(line)
}

func (m *tuiModel) renderThreadMessageLine(row conversationRow, msg *message.Message, width int, selected bool, referenced bool) string {
	if msg == nil {
		return lipgloss.NewStyle().Width(width).MaxWidth(width).Render("")
	}
	usableWidth := max(width-4, 1)
	metaWidth := min(max(usableWidth/4, 14), max(usableWidth/2, 1))
	if usableWidth < 48 {
		metaWidth = min(max(usableWidth/3, 12), max(usableWidth/2, 1))
	}
	summaryWidth := max(usableWidth-metaWidth, 1)

	metaBG := ""
	summaryBG := ""
	metaFG := ""
	timeFG := ""
	summaryFG := ""
	if m.colorEnabled {
		metaBG = "#202A39"
		summaryBG = "#262F3F"
		metaFG = "#C9D1DD"
		timeFG = "#94A2B8"
		summaryFG = "#D8E0EC"
		if selected {
			metaBG = "#3C506E"
			summaryBG = "#425874"
			timeFG = "#C8D4E4"
		}
	}

	statusWidth := 2
	timeWidth := 5
	agentWidth := max(metaWidth-statusWidth-timeWidth-2, 1)

	metaFieldStyle := lipgloss.NewStyle()
	if m.colorEnabled {
		metaFieldStyle = metaFieldStyle.Background(lipgloss.Color(metaBG)).Foreground(lipgloss.Color(metaFG)).ColorWhitespace(true)
	}

	statusCell := metaFieldStyle.Copy().Width(statusWidth).MaxWidth(statusWidth).Render(fitCellContent(statusIcon(msg.Status), statusWidth))
	timeCellStyle := metaFieldStyle.Copy().Width(timeWidth).MaxWidth(timeWidth)
	if m.colorEnabled {
		timeCellStyle = timeCellStyle.Foreground(lipgloss.Color(timeFG))
	}
	timeCell := timeCellStyle.Render(fitCellContent(shortTime(msg.TS), timeWidth))

	agentCellStyle := metaFieldStyle.Copy().Width(agentWidth).MaxWidth(agentWidth)
	if m.colorEnabled && !selected && !referenced {
		agentCellStyle = agentCellStyle.Foreground(lipgloss.Color(m.agentColor(msg.From))).Bold(true)
	}
	agentCell := agentCellStyle.Render(fitCellContent(msg.From, agentWidth))

	gap := metaFieldStyle.Copy().Width(1).MaxWidth(1).Render(" ")
	metaContent := lipgloss.JoinHorizontal(lipgloss.Top, statusCell, gap, timeCell, gap, agentCell)
	metaCell := metaFieldStyle.Copy().Width(metaWidth).MaxWidth(metaWidth).Render(metaContent)

	summaryText := msg.Summary
	if referenced {
		summaryText = iconReference + " " + summaryText
	}
	summaryText = threadSummaryPrefix(row.depth) + summaryText
	guideWidth := 2
	if summaryWidth < 6 {
		guideWidth = 1
	}
	textWidth := max(summaryWidth-guideWidth, 1)

	summaryStyle := lipgloss.NewStyle().Width(summaryWidth).MaxWidth(summaryWidth)
	if m.colorEnabled {
		summaryStyle = summaryStyle.Background(lipgloss.Color(summaryBG)).Foreground(lipgloss.Color(summaryFG)).ColorWhitespace(true)
	}
	if referenced {
		summaryStyle = summaryStyle.Foreground(lipgloss.Color("#FDE68A")).Bold(true)
	}

	guideStyle := summaryStyle.Copy().Width(guideWidth).MaxWidth(guideWidth)
	textStyle := summaryStyle.Copy().Width(textWidth).MaxWidth(textWidth)
	if m.colorEnabled {
		guideStyle = guideStyle.Foreground(lipgloss.Color("#6F7D90"))
	}
	guideText := "│ "
	if guideWidth == 1 {
		guideText = "│"
	}
	summaryCell := lipgloss.JoinHorizontal(
		lipgloss.Top,
		guideStyle.Render(guideText),
		textStyle.Render(fitCellContent(summaryText, textWidth)),
	)

	line := lipgloss.JoinHorizontal(lipgloss.Top, metaCell, summaryCell)
	if !m.colorEnabled && selected {
		return lipgloss.NewStyle().Bold(true).Width(width).MaxWidth(width).Render(line)
	}
	return lipgloss.NewStyle().Width(width).MaxWidth(width).Render(line)
}

func threadSummaryPrefix(depth int) string {
	if depth <= 1 {
		return "▸ "
	}
	if depth > 8 {
		depth = 8
	}
	return strings.Repeat("  ", depth-1) + "└ "
}

func shortDate(ts string) string {
	if ts == "" {
		return ""
	}
	parsed, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ""
	}
	return parsed.Format("2006/01/02")
}

func (m *tuiModel) refreshConversationViewportContent() {
	offset := m.convoViewport.YOffset()
	m.convoViewport.SetContent(m.conversationRowsText(m.convoViewport.Width()))
	if m.autoFollow {
		m.convoViewport.GotoBottom()
		return
	}
	m.convoViewport.SetYOffset(offset)
}

func (m *tuiModel) selectedConversationLine() int {
	if len(m.conversationRows) == 0 {
		return 0
	}
	if m.selectedRowIdx < 0 {
		return 0
	}
	if m.selectedRowIdx >= len(m.conversationRows) {
		return len(m.conversationRows) - 1
	}

	if m.renderMode != renderModeThreaded {
		return m.selectedRowIdx
	}

	line := 0
	for i := 0; i < len(m.conversationRows); {
		row := m.conversationRows[i]
		switch row.kind {
		case "thread-header":
			childCount := 0
			if i == m.selectedRowIdx {
				return line + 1
			}
			j := i + 1
			for j < len(m.conversationRows) && m.conversationRows[j].rootSeq == row.rootSeq && m.conversationRows[j].kind == "message" {
				if j == m.selectedRowIdx {
					return line + 2 + childCount
				}
				childCount++
				j++
			}
			line += childCount + 3 // border top + header + children + border bottom
			i = j
		default:
			if i == m.selectedRowIdx {
				return line
			}
			line++
			i++
		}
	}

	return 0
}

func (m *tuiModel) ensureSelectedConversationVisible() {
	if m.convoViewport.Height() <= 0 || len(m.conversationRows) == 0 {
		return
	}

	selectedLine := m.selectedConversationLine()
	offset := m.convoViewport.YOffset()
	height := m.convoViewport.Height()
	if selectedLine < offset {
		m.convoViewport.SetYOffset(selectedLine)
		return
	}

	bottom := offset + height - 1
	if selectedLine > bottom {
		m.convoViewport.SetYOffset(selectedLine - height + 1)
	}
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

	m.detailsViewport.SetContent(m.renderMessageDetails(msg))
	m.detailsViewport.GotoTop()
}

func (m *tuiModel) renderMessageDetails(msg *message.Message) string {
	if msg == nil {
		return "(no message selected)"
	}

	viewWidth := m.detailsViewport.Width()
	if viewWidth <= 0 {
		viewWidth = 72
	}

	cardPaddingX := 1
	cardWidth := max(viewWidth-2, 24)
	innerWidth := max(cardWidth-2-(cardPaddingX*2), 1)
	gapWidth := 2
	leftWidth := max((innerWidth*3)/8, 12)
	rightWidth := innerWidth - leftWidth - gapWidth
	if rightWidth < 12 {
		rightWidth = max(innerWidth/2, 12)
		leftWidth = max(innerWidth-rightWidth-gapWidth, 1)
	}

	reLabel := "-"
	if msg.Re > 0 {
		reLabel = "#" + strconv.Itoa(msg.Re)
	}

	titleStyle := lipgloss.NewStyle().Width(innerWidth).Bold(true)
	dividerStyle := lipgloss.NewStyle().Width(innerWidth)
	metaCellStyle := lipgloss.NewStyle()
	metaLabelStyle := lipgloss.NewStyle()
	metaMutedStyle := lipgloss.NewStyle()
	typeValueStyle := lipgloss.NewStyle()
	timeValueStyle := lipgloss.NewStyle()
	metaRowStyle := lipgloss.NewStyle().Width(innerWidth)
	gapStyle := lipgloss.NewStyle().Width(gapWidth).Render(strings.Repeat(" ", gapWidth))
	summaryStyle := lipgloss.NewStyle().Width(innerWidth)
	bodyHeaderStyle := lipgloss.NewStyle().Width(innerWidth).Bold(true)
	bodyStyle := lipgloss.NewStyle().Width(innerWidth).BorderStyle(lipgloss.NormalBorder()).Padding(0, 1)
	cardStyle := lipgloss.NewStyle().Width(cardWidth).BorderStyle(lipgloss.NormalBorder()).Padding(0, cardPaddingX)
	statusBadgeStyle := lipgloss.NewStyle().Bold(true)
	metaRowBG := lipgloss.Color("")

	if m.colorEnabled {
		titleStyle = titleStyle.Foreground(lipgloss.Color("#E8EDF7")).Background(lipgloss.Color("#111A2B")).ColorWhitespace(true)
		dividerStyle = dividerStyle.Foreground(lipgloss.Color("#3A4660"))
		metaRowBG = lipgloss.Color("#111A2B")
		metaCellStyle = metaCellStyle.Foreground(lipgloss.Color("#C8D0DC")).Background(metaRowBG).ColorWhitespace(true)
		metaLabelStyle = metaLabelStyle.Foreground(lipgloss.Color("#A7B5CB")).Background(metaRowBG).ColorWhitespace(true)
		metaMutedStyle = metaMutedStyle.Foreground(lipgloss.Color("#A3B3C9")).Background(metaRowBG).ColorWhitespace(true)
		typeValueStyle = typeValueStyle.Foreground(lipgloss.Color("#9BC6FF")).Bold(true).Background(metaRowBG).ColorWhitespace(true)
		timeValueStyle = timeValueStyle.Foreground(lipgloss.Color("#C8D0DC")).Background(metaRowBG).ColorWhitespace(true)
		metaRowStyle = metaRowStyle.Background(metaRowBG).ColorWhitespace(true)
		gapStyle = lipgloss.NewStyle().Width(gapWidth).Background(metaRowBG).ColorWhitespace(true).Render(strings.Repeat(" ", gapWidth))
		summaryStyle = summaryStyle.Foreground(lipgloss.Color("#DCE5F3")).Background(lipgloss.Color("#1E2634")).ColorWhitespace(true)
		bodyHeaderStyle = bodyHeaderStyle.Foreground(lipgloss.Color("#D8E0EC")).Background(lipgloss.Color("#111A2B")).ColorWhitespace(true)
		bodyStyle = bodyStyle.Foreground(lipgloss.Color("#D3DBE7")).Background(lipgloss.Color("#141B2A")).BorderForeground(lipgloss.Color("#3A4660")).ColorWhitespace(true)
		cardStyle = cardStyle.Foreground(lipgloss.Color("#CED8E7")).Background(lipgloss.Color("#0E1320")).BorderForeground(lipgloss.Color("#3A4660")).ColorWhitespace(true)
		statusBadgeStyle = statusBadgeStyle.Foreground(lipgloss.Color("#EAF2FF")).Background(metaRowBG)
		switch msg.Status {
		case message.Resolved:
			statusBadgeStyle = statusBadgeStyle.Foreground(lipgloss.Color("#84DF9E"))
		default:
			statusBadgeStyle = statusBadgeStyle.Foreground(lipgloss.Color("#F4D06F"))
		}
	}

	metaKV := func(label string, value string, valueStyle lipgloss.Style, width int) string {
		if width <= 0 {
			return ""
		}
		labelText := label + ": "
		labelWidth := lipgloss.Width(labelText)
		if labelWidth >= width {
			return metaLabelStyle.Copy().Width(width).Render(fitCellContent(labelText, width))
		}
		valueWidth := width - labelWidth
		labelCell := metaLabelStyle.Copy().Width(labelWidth).Render(labelText)
		valueCell := valueStyle.Copy().Width(valueWidth).Render(fitCellContent(value, valueWidth))
		return lipgloss.JoinHorizontal(lipgloss.Top, labelCell, valueCell)
	}
	metaRow := func(left, right string) string {
		joined := lipgloss.JoinHorizontal(lipgloss.Top, left, gapStyle, right)
		return metaRowStyle.Render(joined)
	}

	agentFromStyle := metaCellStyle.Copy()
	agentToStyle := metaCellStyle.Copy()
	if m.colorEnabled {
		agentFromStyle = agentFromStyle.Foreground(lipgloss.Color(m.agentColor(msg.From))).Bold(true)
		agentToStyle = agentToStyle.Foreground(lipgloss.Color(m.agentColor(msg.To))).Bold(true)
	}

	leftTop := metaKV("From", msg.From, agentFromStyle, leftWidth)
	leftBottom := metaKV("To", msg.To, agentToStyle, leftWidth)
	rightTop := renderDetailsTypeReRow(rightWidth, string(msg.Type), reLabel, metaRowStyle, metaCellStyle, metaLabelStyle, typeValueStyle)
	statusBadge := statusBadgeStyle.Render(statusIcon(msg.Status) + " " + detailsStatusLabel(msg.Status))
	rightBottom := renderDetailsStatusTimeRow(rightWidth, statusBadge, detailsTimeLabel(msg.TS), metaRowStyle, metaCellStyle, metaLabelStyle, metaMutedStyle, timeValueStyle)

	summaryLine := summaryStyle.Render(reflowText("Summary: "+msg.Summary, innerWidth))

	bodyText := strings.TrimRight(msg.Body, "\n")
	if strings.TrimSpace(bodyText) == "" {
		bodyText = "(empty body)"
	}
	bodyTextWidth := max(innerWidth-4, 1)
	bodyBlock := bodyStyle.Render(renderDetailsBody(bodyText, bodyTextWidth, m.colorEnabled))

	details := lipgloss.JoinVertical(
		lipgloss.Left,
		titleStyle.Render("Message #"+strconv.Itoa(msg.Seq)),
		dividerStyle.Render(strings.Repeat("─", innerWidth)),
		metaRow(leftTop, rightTop),
		metaRow(leftBottom, rightBottom),
		summaryLine,
		dividerStyle.Render(strings.Repeat("─", innerWidth)),
		bodyHeaderStyle.Render("Body"),
		bodyBlock,
	)

	return cardStyle.Render(details)
}

func detailsTimeLabel(ts string) string {
	if ts == "" {
		return "-"
	}
	parsed, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	return parsed.Format("2006-01-02 15:04")
}

func detailsStatusLabel(status message.Status) string {
	s := string(status)
	if s == "" {
		return "-"
	}
	if len(s) == 1 {
		return strings.ToUpper(s)
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func reflowText(text string, width int) string {
	if width <= 0 {
		return text
	}

	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	wrapped := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			wrapped = append(wrapped, "")
			continue
		}
		chunk := ansi.Wordwrap(line, width, " ")
		wrapped = append(wrapped, strings.Split(chunk, "\n")...)
	}

	return strings.Join(wrapped, "\n")
}

func fitStyledContent(content string, width int) string {
	if width <= 0 {
		return ""
	}

	clean := strings.ReplaceAll(content, "\n", " ")
	if lipgloss.Width(clean) > width {
		if width == 1 {
			return "…"
		}
		clean = ansi.Truncate(clean, width, "…")
	}

	padding := width - lipgloss.Width(clean)
	if padding > 0 {
		clean += strings.Repeat(" ", padding)
	}

	return clean
}

func renderDetailsStatusTimeRow(width int, statusBadge, timeLabel string, metaRowStyle, metaCellStyle, metaLabelStyle, metaMutedStyle, timeValueStyle lipgloss.Style) string {
	if width <= 0 {
		return ""
	}
	if width < 24 {
		statusText := "Status: " + detailsStatusLabelFromBadge(statusBadge)
		timeText := "Time: " + timeLabel
		return metaRowStyle.Copy().Width(width).Render(fitCellContent(statusText+"  "+timeText, width))
	}

	timeLabelText := "Time: " + timeLabel
	rightWidth := max(lipgloss.Width(timeLabelText), 22)
	if rightWidth >= width {
		statusText := "Status: " + detailsStatusLabelFromBadge(statusBadge)
		return metaRowStyle.Copy().Width(width).Render(fitCellContent(statusText+"  "+timeLabelText, width))
	}

	leftWidth := width - rightWidth
	leftLabel := metaLabelStyle.Copy().Render("Status: ")
	leftCell := metaRowStyle.Copy().Width(leftWidth).Render(lipgloss.JoinHorizontal(lipgloss.Top, leftLabel, statusBadge))
	rightLabel := metaMutedStyle.Copy().Render("Time: ")
	rightValue := timeValueStyle.Copy().Render(timeLabel)
	rightCell := metaRowStyle.Copy().Width(rightWidth).Align(lipgloss.Right).Render(lipgloss.JoinHorizontal(lipgloss.Top, rightLabel, rightValue))
	return lipgloss.JoinHorizontal(lipgloss.Top, leftCell, rightCell)
}

func renderDetailsTypeReRow(width int, msgType, reLabel string, metaRowStyle, metaCellStyle, metaLabelStyle, typeValueStyle lipgloss.Style) string {
	if width <= 0 {
		return ""
	}
	if width < 16 {
		return metaRowStyle.Copy().Width(width).Render(fitCellContent("Type: "+msgType+"  Re: "+reLabel, width))
	}

	rightText := "Re: " + reLabel
	rightWidth := max(lipgloss.Width(rightText), 7)
	if rightWidth+1 >= width {
		return metaRowStyle.Copy().Width(width).Render(fitCellContent("Type: "+msgType+"  "+rightText, width))
	}

	leftWidth := width - rightWidth - 1
	leftLabel := metaLabelStyle.Copy().Render("Type: ")
	leftValueWidth := max(leftWidth-lipgloss.Width("Type: "), 1)
	leftValue := typeValueStyle.Copy().Width(leftValueWidth).Render(fitCellContent(msgType, leftValueWidth))
	leftCell := metaRowStyle.Copy().Width(leftWidth).Render(lipgloss.JoinHorizontal(lipgloss.Top, leftLabel, leftValue))
	rightCell := metaRowStyle.Copy().Width(rightWidth).Render(metaCellStyle.Copy().Render(fitCellContent(rightText, rightWidth)))
	sep := metaRowStyle.Copy().Width(1).Render(" ")
	return lipgloss.JoinHorizontal(lipgloss.Top, leftCell, sep, rightCell)
}

func detailsStatusLabelFromBadge(statusBadge string) string {
	return strings.TrimSpace(ansi.Strip(statusBadge))
}

func renderDetailsBody(bodyText string, width int, colorEnabled bool) string {
	if strings.TrimSpace(bodyText) == "" {
		return "(empty body)"
	}

	normalized := strings.ReplaceAll(bodyText, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	plainLineStyle := lipgloss.NewStyle().Width(width).MaxWidth(width)
	codeLineStyle := plainLineStyle.Copy().PaddingLeft(1)
	codeRuleStyle := lipgloss.NewStyle().Width(1).MaxWidth(1)
	if colorEnabled {
		plainLineStyle = plainLineStyle.Foreground(lipgloss.Color("#D3DBE7")).Background(lipgloss.Color("#141B2A")).ColorWhitespace(true)
		codeRuleStyle = codeRuleStyle.Foreground(lipgloss.Color("#5D7090")).Background(lipgloss.Color("#1A2438")).ColorWhitespace(true)
		codeLineStyle = codeLineStyle.Foreground(lipgloss.Color("#D7E7FF")).Background(lipgloss.Color("#1A2438")).ColorWhitespace(true)
	}

	rendered := make([]string, 0, len(lines)+4)
	inFence := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}

		if trimmed == "" {
			rendered = append(rendered, "")
			continue
		}

		isCodeLike := inFence || strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "    ")
		if isCodeLike {
			wrapped := strings.Split(ansi.Hardwrap(line, width, true), "\n")
			for _, chunk := range wrapped {
				lineWidth := max(width-1, 1)
				rule := codeRuleStyle.Render("│")
				content := codeLineStyle.Copy().Width(lineWidth).MaxWidth(lineWidth).Render(fitCellContent(strings.TrimLeft(chunk, "\t"), max(lineWidth-1, 1)))
				rendered = append(rendered, lipgloss.JoinHorizontal(lipgloss.Top, rule, content))
			}
			continue
		}

		wrapped := strings.Split(ansi.Wordwrap(line, width, " "), "\n")
		for _, chunk := range wrapped {
			rendered = append(rendered, plainLineStyle.Render(fitCellContent(chunk, width)))
		}
	}

	if len(rendered) == 0 {
		return "(empty body)"
	}

	return strings.Join(rendered, "\n")
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
			m.selectConversationRow(idx, true)
			return
		}
	}
}

func (m *tuiModel) moveConversationByThread(delta int) {
	if len(m.conversationRows) == 0 || delta == 0 {
		return
	}
	if m.renderMode != renderModeThreaded {
		m.moveConversationSelection(delta)
		return
	}
	if m.selectedRowIdx < 0 || m.selectedRowIdx >= len(m.conversationRows) {
		m.selectedRowIdx = firstSelectableConversationRow(m.conversationRows)
	}

	currentRoot := 0
	if m.selectedRowIdx >= 0 && m.selectedRowIdx < len(m.conversationRows) {
		currentRoot = m.conversationRows[m.selectedRowIdx].rootSeq
	}

	idx := m.selectedRowIdx
	for {
		idx += delta
		if idx < 0 || idx >= len(m.conversationRows) {
			return
		}
		row := m.conversationRows[idx]
		if row.kind == "thread-header" && row.selectable && row.rootSeq != currentRoot {
			m.selectConversationRow(idx, true)
			return
		}
	}
}

func (m *tuiModel) moveConversationToBoundary(top bool) {
	if len(m.conversationRows) == 0 {
		return
	}

	idx := lastSelectableConversationRow(m.conversationRows)
	if top {
		idx = firstSelectableConversationRow(m.conversationRows)
	}
	m.selectConversationRow(idx, true)
}

func (m *tuiModel) selectConversationRow(idx int, disableFollow bool) {
	if idx < 0 || idx >= len(m.conversationRows) || !m.conversationRows[idx].selectable {
		return
	}

	m.selectedRowIdx = idx
	m.selectedMessageSeq = m.conversationRows[idx].seq
	m.selectedThreadRoot = m.conversationRows[idx].rootSeq
	if disableFollow {
		m.autoFollow = false
	}
	m.refreshConversationViewportContent()
	m.ensureSelectedConversationVisible()
	m.refreshDetailsFromSelection()
}

func (m *tuiModel) toggleSelectedThreadCollapse() {
	if m.focusedPane != focusThreads || len(m.conversationRows) == 0 {
		return
	}
	if m.selectedRowIdx < 0 || m.selectedRowIdx >= len(m.conversationRows) {
		return
	}

	row := m.conversationRows[m.selectedRowIdx]
	if row.rootSeq == 0 || row.kind != "thread-header" {
		return
	}

	if m.collapsedThreads == nil {
		m.collapsedThreads = make(map[int]bool)
	}
	m.collapsedThreads[row.rootSeq] = !m.collapsedThreads[row.rootSeq]
	m.selectedMessageSeq = row.rootSeq
	m.selectedThreadRoot = row.rootSeq
	m.autoFollow = false
	_ = m.refreshSelectedConversation()
	m.ensureSelectedConversationVisible()
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

func (m *tuiModel) moveTaskToBoundary(top bool) {
	if len(m.tasks) == 0 {
		return
	}

	next := len(m.tasks) - 1
	if top {
		next = 0
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
	tasks = available / 5
	if tasks < 1 {
		tasks = 1
	}
	remaining := available - tasks
	threads = remaining / 2
	if threads < 1 {
		threads = 1
	}
	details = remaining - threads
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
	if width == 1 {
		return "…"
	}

	maxContentWidth := width - 1
	var b strings.Builder
	renderedWidth := 0
	for _, r := range clean {
		rw := lipgloss.Width(string(r))
		if rw <= 0 {
			rw = 1
		}
		if renderedWidth+rw > maxContentWidth {
			break
		}
		b.WriteRune(r)
		renderedWidth += rw
	}

	return b.String() + "…"
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

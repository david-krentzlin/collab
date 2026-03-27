package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/david-krentzlin/collab/internal/message"
)

// ANSI color codes
const (
	reset   = "\033[0m"
	bold    = "\033[1m"
	dim     = "\033[2m"
	red     = "\033[31m"
	green   = "\033[32m"
	yellow  = "\033[33m"
	blue    = "\033[34m"
	magenta = "\033[35m"
	cyan    = "\033[36m"
)

// agentColors assigns rotating colors to agents.
var agentColors = []string{blue, magenta, cyan, green, yellow}

type PlainOptions struct {
	Color    bool // use ANSI colors
	Compact  bool // summaries only, no bodies
	OpenOnly bool // hide resolved threads
}

// PlainText renders a TaskExport to a writer as indented plaintext.
func PlainText(w io.Writer, export *TaskExport, opts PlainOptions) {
	colorMap := buildColorMap(export.Agents, opts.Color)

	// Task header
	if opts.Color {
		fmt.Fprintf(w, "%s── %s ──%s\n", bold, export.Task, reset)
	} else {
		fmt.Fprintf(w, "── %s ──\n", export.Task)
	}
	if export.Goal != "" {
		fmt.Fprintf(w, "   %s\n", export.Goal)
	}
	fmt.Fprintln(w)

	threadCount := 0
	for _, t := range export.Threads {
		if opts.OpenOnly && t.Status == "resolved" {
			continue
		}
		threadCount++
		renderThread(w, t, colorMap, opts)
		fmt.Fprintln(w)
	}

	if len(export.Orphans) > 0 {
		if opts.Color {
			fmt.Fprintf(w, "%s%sOrphaned messages:%s\n", dim, red, reset)
		} else {
			fmt.Fprintln(w, "Orphaned messages:")
		}
		for _, o := range export.Orphans {
			renderNodeFlat(w, o, "", colorMap, opts)
		}
		fmt.Fprintln(w)
	}

	if threadCount == 0 && len(export.Orphans) == 0 {
		fmt.Fprintln(w, "  (no messages)")
	}
}

func renderThread(w io.Writer, t *Thread, colorMap map[string]string, opts PlainOptions) {
	// Thread header
	rootSummary := t.Root.Message.Summary
	status := t.Status
	statusStr := ""
	if opts.Color {
		if status == "resolved" {
			statusStr = fmt.Sprintf(" %s[resolved]%s", dim, reset)
		}
		fmt.Fprintf(w, "%sThread:%s %s%s\n", bold, reset, rootSummary, statusStr)
	} else {
		if status == "resolved" {
			statusStr = " [resolved]"
		}
		fmt.Fprintf(w, "Thread: %s%s\n", rootSummary, statusStr)
	}
	fmt.Fprintln(w)

	// Render tree
	renderNode(w, t.Root, "", true, true, colorMap, opts)
}

func renderNode(w io.Writer, n *Node, prefix string, isLast bool, isRoot bool, colorMap map[string]string, opts PlainOptions) {
	m := n.Message

	// Build connector
	connector := "  "
	if !isRoot {
		if isLast {
			connector = "└─ "
		} else {
			connector = "├─ "
		}
	}

	// Message header line
	agentColor := colorMap[m.From]
	typeStr := string(m.Type)
	statusMark := ""
	if m.Status == message.Resolved {
		statusMark = " ✓"
	}

	if opts.Color {
		fmt.Fprintf(w, "%s%s%s#%d%s %s%s%s [%s] %q%s\n",
			prefix, connector,
			bold, m.Seq, reset,
			agentColor, m.From, reset,
			typeStr, m.Summary, statusMark)
	} else {
		fmt.Fprintf(w, "%s%s#%d %s [%s] %q%s\n",
			prefix, connector,
			m.Seq, m.From, typeStr, m.Summary, statusMark)
	}

	// Body (unless compact mode)
	if !opts.Compact && m.Body != "" {
		bodyPrefix := prefix
		switch {
		case isRoot:
			bodyPrefix = "  "
		case prefix != "":
			if isLast {
				bodyPrefix = prefix + "   "
			} else {
				bodyPrefix = prefix + "│  "
			}
		}
		lines := strings.Split(strings.TrimRight(m.Body, "\n"), "\n")
		for _, line := range lines {
			if opts.Color {
				fmt.Fprintf(w, "%s%s│%s %s\n", bodyPrefix, dim, reset, line)
			} else {
				fmt.Fprintf(w, "%s│ %s\n", bodyPrefix, line)
			}
		}
		// Blank line after body if there are children
		if len(n.Children) > 0 {
			if opts.Color {
				fmt.Fprintf(w, "%s%s│%s\n", bodyPrefix, dim, reset)
			} else {
				fmt.Fprintf(w, "%s│\n", bodyPrefix)
			}
		}
	}

	// Render children
	childPrefix := prefix
	if isRoot {
		childPrefix = "  "
	} else if prefix != "" {
		if isLast {
			childPrefix = prefix + "   "
		} else {
			childPrefix = prefix + "│  "
		}
	}
	for i, child := range n.Children {
		isChildLast := i == len(n.Children)-1
		renderNode(w, child, childPrefix, isChildLast, false, colorMap, opts)
	}
}

func renderNodeFlat(w io.Writer, n *Node, prefix string, colorMap map[string]string, opts PlainOptions) {
	m := n.Message
	agentColor := colorMap[m.From]
	if opts.Color {
		fmt.Fprintf(w, "%s  %s#%d%s %s%s%s [%s] re:#%d %q\n",
			prefix, bold, m.Seq, reset,
			agentColor, m.From, reset,
			m.Type, m.Re, m.Summary)
	} else {
		fmt.Fprintf(w, "%s  #%d %s [%s] re:#%d %q\n",
			prefix, m.Seq, m.From, m.Type, m.Re, m.Summary)
	}
}

func buildColorMap(agents []string, useColor bool) map[string]string {
	m := make(map[string]string, len(agents))
	for i, a := range agents {
		if useColor {
			m[a] = agentColors[i%len(agentColors)]
		} else {
			m[a] = ""
		}
	}
	return m
}

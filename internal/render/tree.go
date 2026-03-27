package render

import (
	"sort"

	"github.com/david-krentzlin/collab/internal/message"
)

// Node is a message with its child replies forming a tree.
type Node struct {
	Message  *message.Message `json:"message"`
	Children []*Node          `json:"children,omitempty"`
}

// Thread is a rooted conversation tree.
type Thread struct {
	Root   *Node  `json:"root"`
	Status string `json:"status"` // "open" if any node is open, else "resolved"
}

// TaskExport is the top-level export structure for a task's conversation.
type TaskExport struct {
	Task    string    `json:"task"`
	Goal    string    `json:"goal,omitempty"`
	Agents  []string  `json:"agents"`
	Threads []*Thread `json:"threads"`
	Orphans []*Node   `json:"orphans,omitempty"`
}

// BuildThreads takes a flat list of messages and returns threaded trees.
// Messages with no `re` field (re == 0) start new threads.
// Messages whose `re` target exists join that thread as children.
// Messages whose `re` target is missing become orphans.
func BuildThreads(msgs []*message.Message) ([]*Thread, []*Node) {
	// Sort by seq for deterministic processing
	sort.Slice(msgs, func(i, j int) bool {
		return msgs[i].Seq < msgs[j].Seq
	})

	// Index: seq -> node
	nodes := make(map[int]*Node, len(msgs))
	for _, m := range msgs {
		nodes[m.Seq] = &Node{Message: m}
	}

	var roots []*Node
	var orphans []*Node

	for _, m := range msgs {
		node := nodes[m.Seq]
		if m.Re == 0 {
			// Thread root
			roots = append(roots, node)
		} else if parent, ok := nodes[m.Re]; ok {
			// Attach to parent
			parent.Children = append(parent.Children, node)
		} else {
			// Referenced message not found — orphan
			orphans = append(orphans, node)
		}
	}

	// Build threads from roots
	threads := make([]*Thread, 0, len(roots))
	for _, root := range roots {
		status := computeThreadStatus(root)
		threads = append(threads, &Thread{
			Root:   root,
			Status: status,
		})
	}

	return threads, orphans
}

// computeThreadStatus returns "open" if any node in the tree is open.
func computeThreadStatus(n *Node) string {
	if n.Message.Status == message.Open {
		return "open"
	}
	for _, child := range n.Children {
		if computeThreadStatus(child) == "open" {
			return "open"
		}
	}
	return "resolved"
}

// FlattenThread returns all messages in a thread in seq order.
func FlattenThread(t *Thread) []*message.Message {
	var msgs []*message.Message
	var walk func(n *Node)
	walk = func(n *Node) {
		msgs = append(msgs, n.Message)
		for _, child := range n.Children {
			walk(child)
		}
	}
	walk(t.Root)
	sort.Slice(msgs, func(i, j int) bool {
		return msgs[i].Seq < msgs[j].Seq
	})
	return msgs
}

// Depth returns the nesting depth of a node within its thread.
func Depth(root *Node, targetSeq int) int {
	var find func(n *Node, d int) int
	find = func(n *Node, d int) int {
		if n.Message.Seq == targetSeq {
			return d
		}
		for _, child := range n.Children {
			if result := find(child, d+1); result >= 0 {
				return result
			}
		}
		return -1
	}
	return find(root, 0)
}

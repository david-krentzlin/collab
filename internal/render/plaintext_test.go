package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/david-krentzlin/collab/internal/message"
)

func TestPlainTextRendersTreeConnectorsForRootChildren(t *testing.T) {
	t.Parallel()

	export := &TaskExport{
		Task:   "default",
		Agents: []string{"agent-a", "agent-b", "agent-c"},
		Threads: []*Thread{
			{
				Status: "open",
				Root: &Node{Message: &message.Message{Seq: 1, From: "agent-a", Type: message.Inquiry, Summary: "root", Status: message.Open}, Children: []*Node{
					{Message: &message.Message{Seq: 2, From: "agent-b", Type: message.Reply, Summary: "child-1", Status: message.Open}},
					{Message: &message.Message{Seq: 3, From: "agent-c", Type: message.Reply, Summary: "child-2", Status: message.Open}},
				}},
			},
		},
	}

	var out bytes.Buffer
	PlainText(&out, export, PlainOptions{Color: false, Compact: true})

	text := out.String()
	if !strings.Contains(text, "├─ #2") {
		t.Fatalf("missing branch connector for first child:\n%s", text)
	}
	if !strings.Contains(text, "└─ #3") {
		t.Fatalf("missing branch connector for last child:\n%s", text)
	}
}

func TestPlainTextIndentsRootBody(t *testing.T) {
	t.Parallel()

	export := &TaskExport{
		Task:   "default",
		Agents: []string{"agent-a"},
		Threads: []*Thread{
			{
				Status: "open",
				Root:   &Node{Message: &message.Message{Seq: 1, From: "agent-a", Type: message.Info, Summary: "root", Status: message.Open, Body: "hello\n"}},
			},
		},
	}

	var out bytes.Buffer
	PlainText(&out, export, PlainOptions{Color: false, Compact: false})

	text := out.String()
	if !strings.Contains(text, "\n  │ hello\n") {
		t.Fatalf("root body line should be indented under message:\n%s", text)
	}
}

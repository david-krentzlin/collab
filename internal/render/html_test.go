package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/david-krentzlin/collab/internal/message"
)

func TestHTMLEmbedsJSONDataScriptTag(t *testing.T) {
	t.Parallel()

	export := &TaskExport{
		Task:   "auth-middleware",
		Agents: []string{"agent-a"},
		Threads: []*Thread{
			{Root: &Node{Message: &message.Message{Seq: 1, From: "agent-a", Type: message.Info, Summary: "hello", Status: message.Open}}, Status: "open"},
		},
	}

	var out bytes.Buffer
	if err := HTML(&out, export); err != nil {
		t.Fatalf("render html: %v", err)
	}

	html := out.String()
	if !strings.Contains(html, `<script type="application/json" id="collab-data">`) {
		t.Fatalf("missing application/json payload script")
	}
	if strings.Contains(html, "const DATA = `") {
		t.Fatalf("legacy template-literal payload embedding still present")
	}
	if !strings.Contains(html, "JSON.parse(document.getElementById('collab-data').textContent)") {
		t.Fatalf("missing JSON.parse for embedded payload")
	}
}

func TestHTMLDoesNotEmitRawClosingScriptFromMessageContent(t *testing.T) {
	t.Parallel()

	dangerous := "payload </script><script>alert(1)</script> and `${x}`"
	export := &TaskExport{
		Task:   "danger",
		Agents: []string{"agent-a"},
		Threads: []*Thread{
			{Root: &Node{Message: &message.Message{Seq: 1, From: "agent-a", Type: message.Info, Summary: dangerous, Status: message.Open, Body: dangerous}}, Status: "open"},
		},
	}

	var out bytes.Buffer
	if err := HTML(&out, export); err != nil {
		t.Fatalf("render html: %v", err)
	}

	html := out.String()
	if strings.Contains(html, "</script><script>alert(1)</script>") {
		t.Fatalf("raw closing script sequence leaked into HTML output")
	}
}

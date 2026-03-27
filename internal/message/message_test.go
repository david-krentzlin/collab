package message

import "testing"

func TestMarshalUnmarshalRoundTripPreservesEscapes(t *testing.T) {
	t.Parallel()

	want := &Message{
		Seq:     7,
		From:    "agent-a",
		To:      "agent-b",
		Type:    Inquiry,
		TS:      "2026-03-27T14:32:01Z",
		Summary: `He said "ship it" and path C:\tmp\plan.md`,
		Status:  Open,
		Body:    "first line\nsecond line\n",
	}

	data := want.Marshal()

	got, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("unmarshal marshaled message: %v", err)
	}

	if got.Summary != want.Summary {
		t.Fatalf("summary = %q, want %q", got.Summary, want.Summary)
	}
	if got.Body != want.Body {
		t.Fatalf("body = %q, want %q", got.Body, want.Body)
	}
}

func TestUnmarshalYAMLParsesEscapedSummary(t *testing.T) {
	t.Parallel()

	data := []byte(`---
seq: 1
from: agent-a
to: agent-b
type: info
ts: "2026-03-27T14:32:01Z"
summary: "Quote: \"yes\" and slash \\\\"
status: open
---

body
`)

	msg, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("unmarshal message: %v", err)
	}

	want := `Quote: "yes" and slash \\`
	if msg.Summary != want {
		t.Fatalf("summary = %q, want %q", msg.Summary, want)
	}
}

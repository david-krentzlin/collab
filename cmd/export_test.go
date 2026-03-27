package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/david-krentzlin/collab/internal/message"
	"github.com/david-krentzlin/collab/internal/store"
)

func TestBuildExportReturnsErrorWhenIndexedMessageIsUnreadable(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	s := store.Find(root)
	if err := s.Init([]string{"agent-a", "agent-b"}); err != nil {
		t.Fatalf("init store: %v", err)
	}

	msg := &message.Message{
		From:    "agent-a",
		To:      "agent-b",
		Type:    message.Info,
		TS:      message.Now(),
		Summary: "hello",
		Status:  message.Open,
		Body:    "body",
	}
	if _, err := s.CreateMessage(msg); err != nil {
		t.Fatalf("create message: %v", err)
	}

	entries, err := s.List(0, "")
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entry count = %d, want 1", len(entries))
	}

	if err := os.WriteFile(entries[0].Path, []byte("not frontmatter\n"), 0o644); err != nil {
		t.Fatalf("corrupt message file: %v", err)
	}

	_, err = buildExport(s)
	if err == nil {
		t.Fatalf("expected error from unreadable indexed message")
	}
	if !strings.Contains(err.Error(), "failed to read indexed messages") {
		t.Fatalf("error = %q, want failed to read indexed messages", err)
	}
}

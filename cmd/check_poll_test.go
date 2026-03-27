package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/david-krentzlin/collab/internal/message"
	"github.com/david-krentzlin/collab/internal/store"
)

func TestCheckRejectsInvalidPollCount(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	s := store.FindTask(root, store.DefaultTask)
	if err := s.Init([]string{"agent-a", "agent-b"}); err != nil {
		t.Fatalf("init store: %v", err)
	}

	resetCheckGlobalsForTest()
	checkAgent = "agent-a"
	checkPoll = 0

	err := checkCmd.RunE(checkCmd, nil)
	if err == nil {
		t.Fatalf("expected error for invalid poll count")
	}
	if !strings.Contains(err.Error(), "--poll must be >= 1") {
		t.Fatalf("error = %q, want poll count validation", err)
	}
}

func TestCheckPollFindsNewMessageDuringWaitWindow(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	s := store.FindTask(root, store.DefaultTask)
	if err := s.Init([]string{"agent-a", "agent-b"}); err != nil {
		t.Fatalf("init store: %v", err)
	}

	resetCheckGlobalsForTest()
	checkAgent = "agent-a"
	checkPoll = 5
	checkInterval = 20 * time.Millisecond

	go func() {
		time.Sleep(30 * time.Millisecond)
		_, _ = s.CreateMessage(&message.Message{
			From:    "agent-b",
			To:      "agent-a",
			Type:    message.Reply,
			TS:      message.Now(),
			Summary: "arrived while polling",
			Status:  message.Open,
			Body:    "hello",
		})
	}()

	out, err := captureStdout(func() error {
		return checkCmd.RunE(checkCmd, nil)
	})
	if err != nil {
		t.Fatalf("run check command: %v", err)
	}

	if !strings.Contains(out, "arrived while polling") {
		t.Fatalf("expected check output to include polled message, got:\n%s", out)
	}
}

func resetCheckGlobalsForTest() {
	checkSince = 0
	checkAgent = ""
	checkTask = store.DefaultTask
	checkPoll = 1
	checkInterval = 2 * time.Second
}

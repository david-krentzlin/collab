package cmd

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/david-krentzlin/collab/internal/message"
	"github.com/david-krentzlin/collab/internal/store"
)

func TestLogUsesTaskFlagToSelectConversation(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	defaultStore := store.FindTask(root, store.DefaultTask)
	if err := defaultStore.Init([]string{"agent-a", "agent-b"}); err != nil {
		t.Fatalf("init default task: %v", err)
	}
	if _, err := defaultStore.CreateMessage(&message.Message{
		From:    "agent-a",
		To:      "agent-b",
		Type:    message.Info,
		TS:      message.Now(),
		Summary: "default-task-message",
		Status:  message.Open,
		Body:    "default",
	}); err != nil {
		t.Fatalf("create default task message: %v", err)
	}

	featureStore := store.FindTask(root, "feature-x")
	if err := featureStore.Init([]string{"agent-a", "agent-b"}); err != nil {
		t.Fatalf("init feature task: %v", err)
	}
	if _, err := featureStore.CreateMessage(&message.Message{
		From:    "agent-b",
		To:      "agent-a",
		Type:    message.Info,
		TS:      message.Now(),
		Summary: "feature-task-message",
		Status:  message.Open,
		Body:    "feature",
	}); err != nil {
		t.Fatalf("create feature task message: %v", err)
	}

	resetLogGlobalsForTest()
	logTask = "feature-x"
	logNoColor = true
	logCompact = true

	out, err := captureStdout(func() error {
		return logCmd.RunE(logCmd, nil)
	})
	if err != nil {
		t.Fatalf("run log command: %v", err)
	}

	if !strings.Contains(out, "── feature-x ──") {
		t.Fatalf("log output missing selected task header:\n%s", out)
	}
	if !strings.Contains(out, "feature-task-message") {
		t.Fatalf("log output missing selected task messages:\n%s", out)
	}
	if strings.Contains(out, "default-task-message") {
		t.Fatalf("log output should not contain messages from other task:\n%s", out)
	}
}

func TestHasConversationAdvancedDetectsNewMessages(t *testing.T) {
	root := t.TempDir()
	s := store.FindTask(root, store.DefaultTask)
	if err := s.Init([]string{"agent-a", "agent-b"}); err != nil {
		t.Fatalf("init store: %v", err)
	}

	advanced, latest, err := hasConversationAdvanced(s, 0)
	if err != nil {
		t.Fatalf("check advancement in empty store: %v", err)
	}
	if advanced || latest != 0 {
		t.Fatalf("empty store advanced=%v latest=%d, want false/0", advanced, latest)
	}

	if _, err := s.CreateMessage(&message.Message{
		From:    "agent-a",
		To:      "agent-b",
		Type:    message.Info,
		TS:      message.Now(),
		Summary: "new",
		Status:  message.Open,
		Body:    "body",
	}); err != nil {
		t.Fatalf("create message: %v", err)
	}

	advanced, latest, err = hasConversationAdvanced(s, 0)
	if err != nil {
		t.Fatalf("check advancement with message: %v", err)
	}
	if !advanced || latest != 1 {
		t.Fatalf("with message advanced=%v latest=%d, want true/1", advanced, latest)
	}
}

func captureStdout(fn func() error) (string, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	defer r.Close()

	old := os.Stdout
	os.Stdout = w
	runErr := fn()
	_ = w.Close()
	os.Stdout = old

	data, readErr := io.ReadAll(r)
	if runErr != nil {
		return string(data), runErr
	}
	if readErr != nil {
		return "", readErr
	}
	return string(data), nil
}

func resetLogGlobalsForTest() {
	logCompact = false
	logOpenOnly = false
	logNoColor = false
	logTask = store.DefaultTask
	logFollow = false
	logFollowInterval = "1s"
}

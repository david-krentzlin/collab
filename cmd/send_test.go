package cmd

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/david-krentzlin/collab/internal/store"
)

func TestSendRejectsUnknownRecipient(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	s := store.Find(root)
	if err := s.Init([]string{"agent-a", "agent-b"}); err != nil {
		t.Fatalf("init store: %v", err)
	}

	resetSendGlobalsForTest()
	sendAgent = "agent-a"
	sendTo = "agent-z"
	sendType = "info"
	sendSummary = "unknown recipient"

	withTestStdin(t, "body\n", func() {
		err := sendCmd.RunE(sendCmd, nil)
		if err == nil {
			t.Fatalf("expected error for unknown recipient")
		}
		if !strings.Contains(err.Error(), "unknown recipient") {
			t.Fatalf("error = %q, want unknown recipient", err)
		}
	})
}

func TestSendAllowsBroadcastRecipient(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	s := store.Find(root)
	if err := s.Init([]string{"agent-a", "agent-b"}); err != nil {
		t.Fatalf("init store: %v", err)
	}

	resetSendGlobalsForTest()
	sendAgent = "agent-a"
	sendTo = "all"
	sendType = "info"
	sendSummary = "broadcast"

	withTestStdin(t, "body\n", func() {
		if err := sendCmd.RunE(sendCmd, nil); err != nil {
			t.Fatalf("send broadcast: %v", err)
		}
	})
}

func withTestStdin(t *testing.T, content string, fn func()) {
	t.Helper()

	f, err := os.CreateTemp(t.TempDir(), "stdin-*")
	if err != nil {
		t.Fatalf("create temp stdin: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		t.Fatalf("write temp stdin: %v", err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		f.Close()
		t.Fatalf("seek temp stdin: %v", err)
	}

	old := os.Stdin
	os.Stdin = f
	t.Cleanup(func() {
		os.Stdin = old
		_ = f.Close()
	})

	fn()
}

func resetSendGlobalsForTest() {
	sendAgent = ""
	sendTask = store.DefaultTask
	sendTo = ""
	sendType = "inquiry"
	sendRe = 0
	sendSummary = ""
}

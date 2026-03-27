package cmd

import (
	"strings"
	"testing"

	"github.com/david-krentzlin/collab/internal/store"
)

func TestSendRequiresAgentFlag(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	s := store.Find(root)
	if err := s.Init([]string{"agent-a", "agent-b"}); err != nil {
		t.Fatalf("init store: %v", err)
	}

	resetSendGlobalsForTest()
	sendTo = "agent-b"
	sendType = "info"
	sendSummary = "hello"

	withTestStdin(t, "body\n", func() {
		err := sendCmd.RunE(sendCmd, nil)
		if err == nil {
			t.Fatalf("expected missing --agent error")
		}
		if !strings.Contains(err.Error(), "--agent is required") {
			t.Fatalf("error = %q, want missing --agent", err)
		}
	})
}

func TestSendUsesAgentFlagWithoutEnvVar(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("COLLAB_AGENT", "")

	s := store.Find(root)
	if err := s.Init([]string{"agent-a", "agent-b"}); err != nil {
		t.Fatalf("init store: %v", err)
	}

	resetSendGlobalsForTest()
	sendAgent = "agent-a"
	sendTo = "agent-b"
	sendType = "info"
	sendSummary = "hello"

	withTestStdin(t, "body\n", func() {
		if err := sendCmd.RunE(sendCmd, nil); err != nil {
			t.Fatalf("send with --agent: %v", err)
		}
	})
}

func TestCheckRequiresAgentFlag(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	s := store.Find(root)
	if err := s.Init([]string{"agent-a", "agent-b"}); err != nil {
		t.Fatalf("init store: %v", err)
	}

	resetCheckGlobalsForTest()

	err := checkCmd.RunE(checkCmd, nil)
	if err == nil {
		t.Fatalf("expected missing --agent error")
	}
	if !strings.Contains(err.Error(), "--agent is required") {
		t.Fatalf("error = %q, want missing --agent", err)
	}
}

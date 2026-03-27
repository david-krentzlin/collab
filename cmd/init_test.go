package cmd

import (
	"strings"
	"testing"
)

func TestInitRejectsEmptyAgentNames(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	resetInitGlobalsForTest()
	initAgents = "agent-a,,agent-b"

	err := initCmd.RunE(initCmd, nil)
	if err == nil {
		t.Fatalf("expected error for empty agent name")
	}
	if !strings.Contains(err.Error(), "empty agent name") {
		t.Fatalf("error = %q, want empty agent name", err)
	}
}

func TestInitRejectsDuplicateAgentNames(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	resetInitGlobalsForTest()
	initAgents = "agent-a,agent-b,agent-a"

	err := initCmd.RunE(initCmd, nil)
	if err == nil {
		t.Fatalf("expected error for duplicate agent name")
	}
	if !strings.Contains(err.Error(), "duplicate agent") {
		t.Fatalf("error = %q, want duplicate agent", err)
	}
}

func resetInitGlobalsForTest() {
	initAgents = ""
}

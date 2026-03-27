package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/david-krentzlin/collab/internal/message"
	"github.com/david-krentzlin/collab/internal/store"
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

func TestInitRefusesExistingStoreWithoutForce(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	resetInitGlobalsForTest()
	initAgents = "agent-a,agent-b"
	if err := initCmd.RunE(initCmd, nil); err != nil {
		t.Fatalf("first init: %v", err)
	}

	resetInitGlobalsForTest()
	initAgents = "agent-a,agent-b"
	err := initCmd.RunE(initCmd, nil)
	if err == nil {
		t.Fatalf("expected error for re-init without --force")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Fatalf("error = %q, want guidance to use --force", err)
	}
}

func TestInitForceResetsExistingStore(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	resetInitGlobalsForTest()
	initAgents = "agent-a,agent-b"
	if err := initCmd.RunE(initCmd, nil); err != nil {
		t.Fatalf("first init: %v", err)
	}

	s := store.Find(root)
	if _, err := s.CreateMessage(&message.Message{
		From:    "agent-b",
		To:      "agent-a",
		Type:    message.Info,
		TS:      message.Now(),
		Summary: "before-reset",
		Status:  message.Open,
		Body:    "body",
	}); err != nil {
		t.Fatalf("create message: %v", err)
	}

	resetInitGlobalsForTest()
	initAgents = "agent-a"
	initForce = true
	if err := initCmd.RunE(initCmd, nil); err != nil {
		t.Fatalf("force init: %v", err)
	}

	s2 := store.Find(root)
	newMsg := &message.Message{
		From:    "agent-a",
		To:      "agent-a",
		Type:    message.Info,
		TS:      message.Now(),
		Summary: "after-reset",
		Status:  message.Open,
		Body:    "body",
	}
	if _, err := s2.CreateMessage(newMsg); err != nil {
		t.Fatalf("create message after force init: %v", err)
	}
	if newMsg.Seq != 1 {
		t.Fatalf("new message seq = %d, want 1 after reset", newMsg.Seq)
	}

	if _, err := os.Stat(filepath.Join(root, ".collab", "default", "agent-b")); !os.IsNotExist(err) {
		t.Fatalf("agent-b directory should be removed by --force reset")
	}

	matches, err := filepath.Glob(filepath.Join(root, ".collab", "default", "agent-b", "*.md"))
	if err != nil {
		t.Fatalf("glob old agent files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected no message files under removed agent-b directory, found %d", len(matches))
	}
}

func TestInitWritesAgentsAndSkillByDefault(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	resetInitGlobalsForTest()
	initAgents = "agent-a,agent-b"

	if err := initCmd.RunE(initCmd, nil); err != nil {
		t.Fatalf("init: %v", err)
	}

	agentsPath := filepath.Join(root, "AGENTS.md")
	agentsData, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	if !strings.Contains(string(agentsData), "## Collaboration via collab") {
		t.Fatalf("AGENTS.md missing collab guidance")
	}

	skillPath := filepath.Join(root, ".agents", "skills", "collab", "SKILL.md")
	skillData, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("stat installed collab skill: %v", err)
	}
	if !strings.Contains(string(skillData), "name: collab") || !strings.Contains(string(skillData), "description:") {
		t.Fatalf("installed skill missing required Agent Skills frontmatter")
	}
}

func resetInitGlobalsForTest() {
	initAgents = ""
	initTask = store.DefaultTask
	initForce = false
	initNoAgentsMD = false
	initNoInstallSkill = false
}

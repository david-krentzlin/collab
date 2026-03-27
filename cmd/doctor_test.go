package cmd

import (
	"strings"
	"testing"

	"github.com/david-krentzlin/collab/internal/store"
)

func TestDoctorPassesAfterInit(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	resetInitGlobalsForTest()
	initAgents = "agent-a,agent-b"
	if err := initCmd.RunE(initCmd, nil); err != nil {
		t.Fatalf("init: %v", err)
	}

	doctorAgent = "agent-a"
	doctorTask = store.DefaultTask
	if err := doctorCmd.RunE(doctorCmd, nil); err != nil {
		t.Fatalf("doctor: %v", err)
	}
}

func TestDoctorFailsWhenSkillMissing(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	resetInitGlobalsForTest()
	initAgents = "agent-a,agent-b"
	initNoInstallSkill = true
	if err := initCmd.RunE(initCmd, nil); err != nil {
		t.Fatalf("init: %v", err)
	}

	doctorAgent = "agent-a"
	doctorTask = store.DefaultTask
	err := doctorCmd.RunE(doctorCmd, nil)
	if err == nil {
		t.Fatalf("expected doctor failure when skill missing")
	}
	if !strings.Contains(err.Error(), "doctor checks failed") {
		t.Fatalf("error = %q, want doctor checks failed", err)
	}
}

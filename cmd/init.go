package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/david-krentzlin/collab/internal/store"
	"github.com/spf13/cobra"
)

var initAgents string
var initForce bool
var initTask string
var initNoAgentsMD bool
var initNoInstallSkill bool

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a .collab directory",
	Long:  `Creates the .collab directory with subdirectories for each agent and a sequence counter.`,
	Example: `  collab init --agents agent-a,agent-b
  collab init --agents alice,bob,carol`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if initAgents == "" {
			return fmt.Errorf("--agents is required (comma-separated list)")
		}
		agents, err := parseAndValidateAgents(initAgents)
		if err != nil {
			return err
		}

		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		s := findStoreForTask(cwd, initTask)

		exists, err := pathExists(s.Root)
		if err != nil {
			return err
		}
		if exists {
			if !initForce {
				return fmt.Errorf("task %q is already initialized at %s (use --force to reset)", s.Task, s.Root)
			}
			if err := os.RemoveAll(s.Root); err != nil {
				return fmt.Errorf("reset existing task %q: %w", s.Task, err)
			}
		}

		if err := s.Init(agents); err != nil {
			return err
		}

		workspaceRoot := workspaceRootFromStore(s)
		if !initNoInstallSkill {
			if err := ensureCollabSkill(workspaceRoot); err != nil {
				return err
			}
		}
		if !initNoAgentsMD {
			if err := ensureAgentsSnippet(workspaceRoot); err != nil {
				return err
			}
		}

		fmt.Printf("Initialized .collab/%s/ with agents: %s\n", s.Task, strings.Join(agents, ", "))
		return nil
	},
}

const agentsMDSnippet = `## Collaboration via collab

When the user asks you to pair on a feature with another agent, you must use the collab skill to do that.
First ask the user which agent-identity you shall assume, and which task context to use.

IMPORTANT: use this agent-identity consistently during the whole session. (even if the task to work on changes)
`

const collabSkillContent = `---
name: collab
description: Coordinate work between two agents using collab check/read/send/resolve. Use when the user asks to pair with another agent on the same feature and reliable message handoff is required.
---

# collab

Use collab commands to coordinate with another agent for a shared feature task.

## Workflow

1. Confirm identity and task context with the user before first message.
2. Poll using 'collab check --agent <id> --task <task>' before starting work.
3. Read only relevant messages with 'collab read <seq> --agent <id> --task <task>'.
4. Send updates with concise summaries via 'collab send --agent <id> --task <task> ...'.
5. Use '--re <seq>' when replying to keep threads linked.
6. Resolve finished threads with 'collab resolve <seq> --agent <id> --task <task>'.

## Guardrails

- Keep summaries short and specific.
- Do not change agent identity during the same session.
- Poll before and after meaningful implementation milestones.
`

func parseAndValidateAgents(raw string) ([]string, error) {
	parts := strings.Split(raw, ",")
	agents := make([]string, 0, len(parts))
	for i, agent := range parts {
		normalized := strings.TrimSpace(agent)
		if normalized == "" {
			return nil, fmt.Errorf("empty agent name at position %d", i+1)
		}
		if slices.Contains(agents, normalized) {
			return nil, fmt.Errorf("duplicate agent %q", normalized)
		}
		agents = append(agents, normalized)
	}
	return agents, nil
}

func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func ensureCollabSkill(workspaceRoot string) error {
	skillPath := filepath.Join(workspaceRoot, ".agents", "skills", "collab", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillPath), 0o755); err != nil {
		return fmt.Errorf("create collab skill dir: %w", err)
	}

	exists, err := pathExists(skillPath)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	if err := os.WriteFile(skillPath, []byte(collabSkillContent), 0o644); err != nil {
		return fmt.Errorf("write collab skill: %w", err)
	}
	return nil
}

func ensureAgentsSnippet(workspaceRoot string) error {
	agentsPath := filepath.Join(workspaceRoot, "AGENTS.md")

	exists, err := pathExists(agentsPath)
	if err != nil {
		return err
	}

	if !exists {
		if err := os.WriteFile(agentsPath, []byte(agentsMDSnippet), 0o644); err != nil {
			return fmt.Errorf("write AGENTS.md: %w", err)
		}
		return nil
	}

	data, err := os.ReadFile(agentsPath)
	if err != nil {
		return fmt.Errorf("read AGENTS.md: %w", err)
	}
	if strings.Contains(string(data), "## Collaboration via collab") {
		return nil
	}

	content := strings.TrimRight(string(data), "\n") + "\n\n" + agentsMDSnippet
	if err := os.WriteFile(agentsPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("append AGENTS.md snippet: %w", err)
	}

	return nil
}

func init() {
	initCmd.Flags().StringVar(&initAgents, "agents", "", "Comma-separated list of agent names")
	initCmd.Flags().StringVar(&initTask, "task", store.DefaultTask, "Task namespace to initialize")
	initCmd.Flags().BoolVar(&initForce, "force", false, "Reset an existing task directory before initialization")
	initCmd.Flags().BoolVar(&initNoAgentsMD, "no-agents-md", false, "Do not add collab guidance to AGENTS.md")
	initCmd.Flags().BoolVar(&initNoInstallSkill, "no-install-skill", false, "Do not install .agents/skills/collab/SKILL.md")
}

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/david-krentzlin/collab/internal/store"
)

func normalizedTask(task string) string {
	trimmed := strings.TrimSpace(task)
	if trimmed == "" {
		return store.DefaultTask
	}
	return trimmed
}

func findStoreForTask(cwd, task string) *store.Store {
	return store.FindTask(cwd, normalizedTask(task))
}

func requireKnownAgent(s *store.Store, agent string) (string, error) {
	normalized := strings.TrimSpace(agent)
	if normalized == "" {
		return "", fmt.Errorf("--agent is required")
	}

	agents, err := s.Agents()
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("task %q is not initialized at %s (run collab init --task %s --agents ...)", s.Task, s.Root, s.Task)
		}
		return "", fmt.Errorf("list task agents: %w", err)
	}

	for _, known := range agents {
		if known == normalized {
			return normalized, nil
		}
	}

	if len(agents) == 0 {
		return "", fmt.Errorf("task %q has no configured agents (run collab init --task %s --agents ...)", s.Task, s.Task)
	}

	return "", fmt.Errorf("unknown agent %q for task %q (known agents: %s)", normalized, s.Task, strings.Join(agents, ", "))
}

func workspaceRootFromStore(s *store.Store) string {
	if s.Base != "" {
		return filepath.Dir(s.Base)
	}

	collabBase := s.Root
	task := filepath.Clean(s.Task)
	if task != "" && task != "." {
		for _, part := range strings.Split(task, string(filepath.Separator)) {
			if part == "" || part == "." {
				continue
			}
			collabBase = filepath.Dir(collabBase)
		}
	}

	return filepath.Dir(collabBase)
}

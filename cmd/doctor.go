package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/david-krentzlin/collab/internal/store"
	"github.com/spf13/cobra"
)

var (
	doctorAgent string
	doctorTask  string
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Validate collab setup for an agent/task",
	Long:  `Runs setup and integrity checks to ensure collab can be used reliably.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}

		s := findStoreForTask(cwd, doctorTask)
		workspaceRoot := workspaceRootFromStore(s)

		type checkResult struct {
			ok      bool
			message string
		}
		results := make([]checkResult, 0, 6)

		if st, err := os.Stat(s.Root); err != nil {
			results = append(results, checkResult{ok: false, message: fmt.Sprintf("store path %s is not accessible: %v", s.Root, err)})
		} else if !st.IsDir() {
			results = append(results, checkResult{ok: false, message: fmt.Sprintf("store path %s is not a directory", s.Root)})
		} else {
			results = append(results, checkResult{ok: true, message: fmt.Sprintf("store path exists: %s", s.Root)})
		}

		if _, err := requireKnownAgent(s, doctorAgent); err != nil {
			results = append(results, checkResult{ok: false, message: err.Error()})
		} else {
			results = append(results, checkResult{ok: true, message: fmt.Sprintf("agent %q is configured for task %q", strings.TrimSpace(doctorAgent), s.Task)})
		}

		if _, err := os.Stat(filepath.Join(s.Root, store.SeqFile)); err != nil {
			results = append(results, checkResult{ok: false, message: fmt.Sprintf("missing %s: %v", store.SeqFile, err)})
		} else {
			results = append(results, checkResult{ok: true, message: fmt.Sprintf("%s is present", store.SeqFile)})
		}

		if _, err := os.Stat(filepath.Join(s.Root, store.IndexFile)); err != nil {
			results = append(results, checkResult{ok: false, message: fmt.Sprintf("missing %s: %v", store.IndexFile, err)})
		} else if _, err := s.List(0, ""); err != nil {
			results = append(results, checkResult{ok: false, message: fmt.Sprintf("index integrity failed: %v", err)})
		} else {
			results = append(results, checkResult{ok: true, message: fmt.Sprintf("%s integrity check passed", store.IndexFile)})
		}

		skillPath := filepath.Join(workspaceRoot, ".agents", "skills", "collab", "SKILL.md")
		if _, err := os.Stat(skillPath); err != nil {
			results = append(results, checkResult{ok: false, message: fmt.Sprintf("collab skill missing at %s: %v", skillPath, err)})
		} else {
			results = append(results, checkResult{ok: true, message: fmt.Sprintf("collab skill found: %s", skillPath)})
		}

		failed := false
		for _, result := range results {
			if result.ok {
				fmt.Printf("OK   %s\n", result.message)
			} else {
				failed = true
				fmt.Printf("FAIL %s\n", result.message)
			}
		}

		if failed {
			return fmt.Errorf("doctor checks failed")
		}

		fmt.Printf("Doctor OK for task %q\n", s.Task)
		return nil
	},
}

func init() {
	doctorCmd.Flags().StringVar(&doctorAgent, "agent", "", "Agent identity to validate (required)")
	doctorCmd.Flags().StringVar(&doctorTask, "task", store.DefaultTask, "Task namespace to validate")
}

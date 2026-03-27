package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/david-krentzlin/collab/internal/store"
	"github.com/spf13/cobra"
)

var initAgents string

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
		agents := strings.Split(initAgents, ",")
		for i := range agents {
			agents[i] = strings.TrimSpace(agents[i])
		}

		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		s := store.Find(cwd)
		if err := s.Init(agents); err != nil {
			return err
		}
		fmt.Printf("Initialized .collab/%s/ with agents: %s\n", s.Task, strings.Join(agents, ", "))
		return nil
	},
}

func init() {
	initCmd.Flags().StringVar(&initAgents, "agents", "", "Comma-separated list of agent names")
}

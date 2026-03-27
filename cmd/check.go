package cmd

import (
	"fmt"
	"os"

	"github.com/david-krentzlin/collab/internal/store"
	"github.com/spf13/cobra"
)

var checkSince int
var checkAgent string
var checkTask string

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Check for new messages from other agents",
	Long: `Lists messages from other agents since a given sequence number.
Only shows frontmatter (seq, type, from, summary) for token efficiency.
Use 'collab read <seq>' to fetch the full body of a specific message.

The caller's identity comes from the COLLAB_AGENT environment variable.
Messages from the caller are excluded from results.`,
	Example: `  collab check                  # show all messages from others
  collab check --since 4       # only messages after seq 4`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		s := findStoreForTask(cwd, checkTask)

		agent, err := requireKnownAgent(s, checkAgent)
		if err != nil {
			return err
		}

		var entries []store.MessageEntry
		entries, err = s.ListForRecipient(checkSince, agent)
		if err != nil {
			return err
		}

		if len(entries) == 0 {
			if checkSince > 0 {
				fmt.Printf("No new messages since #%d\n", checkSince)
			} else {
				fmt.Println("No messages found")
			}
			return nil
		}

		for _, e := range entries {
			re := ""
			if e.Re > 0 {
				re = fmt.Sprintf(" re:#%d", e.Re)
			}
			status := ""
			if e.Status == "resolved" {
				status = " [resolved]"
			}
			fmt.Printf("#%d [%s] from:%s%s%s %q\n", e.Seq, e.Type, e.From, re, status, e.Summary)
		}
		return nil
	},
}

func init() {
	checkCmd.Flags().StringVar(&checkAgent, "agent", "", "Agent identity for inbox filtering (required)")
	checkCmd.Flags().StringVar(&checkTask, "task", store.DefaultTask, "Task namespace for message store")
	checkCmd.Flags().IntVar(&checkSince, "since", 0, "Only show messages with seq > this value")
}

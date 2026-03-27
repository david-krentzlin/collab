package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/david-krentzlin/collab/internal/store"
	"github.com/spf13/cobra"
)

var checkSince int
var checkAgent string
var checkTask string
var checkPoll int
var checkInterval time.Duration

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Check for new messages from other agents",
	Long: `Lists messages from other agents since a given sequence number.
Only shows frontmatter (seq, type, from, summary) for token efficiency.
Use 'collab read <seq>' to fetch the full body of a specific message.

The caller identity is provided with --agent.
Messages from the caller are excluded from results.

Use --poll and --interval to wait for new messages while polling.`,
	Example: `  collab check                  # show all messages from others
  collab check --since 4       # only messages after seq 4
  collab check --poll 10 --interval 2s`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if checkPoll < 1 {
			return fmt.Errorf("--poll must be >= 1")
		}
		if checkInterval <= 0 {
			return fmt.Errorf("--interval must be > 0")
		}

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
		for attempt := 0; attempt < checkPoll; attempt++ {
			entries, err = s.ListForRecipient(checkSince, agent)
			if err != nil {
				return err
			}
			if len(entries) > 0 {
				break
			}
			if attempt < checkPoll-1 {
				time.Sleep(checkInterval)
			}
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
	checkCmd.Flags().IntVar(&checkPoll, "poll", 1, "Number of polling attempts before returning (>= 1)")
	checkCmd.Flags().DurationVar(&checkInterval, "interval", 2*time.Second, "Time between polling attempts")
}

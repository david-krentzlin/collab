package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/david-krentzlin/collab/internal/message"
	"github.com/david-krentzlin/collab/internal/store"
	"github.com/spf13/cobra"
)

var resolveCmd = &cobra.Command{
	Use:   "resolve <seq>",
	Short: "Mark a message as resolved",
	Long:  `Updates the status field of a message from 'open' to 'resolved'.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		seq, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid seq number: %s", args[0])
		}

		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		s := store.Find(cwd)

		msg, err := s.ReadMessage(seq)
		if err != nil {
			return err
		}

		if msg.Status == message.Resolved {
			fmt.Printf("#%d is already resolved\n", seq)
			return nil
		}

		msg.Status = message.Resolved
		data := msg.Marshal()

		// Find and overwrite the file
		entries, err := s.List(0, "")
		if err != nil {
			return err
		}
		for _, e := range entries {
			if e.Seq == seq {
				if err := os.WriteFile(e.Path, data, 0o644); err != nil {
					return fmt.Errorf("write: %w", err)
				}
				fmt.Printf("#%d marked as resolved\n", seq)
				return nil
			}
		}

		// Fallback: also check the message's own agent dir
		_ = strings.TrimSpace(msg.From)
		return fmt.Errorf("could not locate file for #%d", seq)
	},
}

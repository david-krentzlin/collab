package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

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
		s := findStoreForTask(cwd, resolveTask)
		if _, err := requireKnownAgent(s, resolveAgent); err != nil {
			return err
		}

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

		entry, err := s.Entry(seq)
		if err != nil {
			return err
		}

		if err := writeAtomic(entry.Path, data, 0o644); err != nil {
			return fmt.Errorf("write: %w", err)
		}
		if err := s.SetStatus(seq, message.Resolved); err != nil {
			return err
		}
		fmt.Printf("#%d marked as resolved\n", seq)
		return nil
	},
}

var (
	resolveAgent string
	resolveTask  string
)

func writeAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".resolve-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}

func init() {
	resolveCmd.Flags().StringVar(&resolveAgent, "agent", "", "Agent identity (required)")
	resolveCmd.Flags().StringVar(&resolveTask, "task", store.DefaultTask, "Task namespace for message store")
}

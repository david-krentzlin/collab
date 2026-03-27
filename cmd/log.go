package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/david-krentzlin/collab/internal/render"
	"github.com/david-krentzlin/collab/internal/store"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	logCompact        bool
	logOpenOnly       bool
	logNoColor        bool
	logTask           string
	logFollow         bool
	logFollowInterval string
)

var logCmd = &cobra.Command{
	Use:   "log",
	Short: "Display conversation as a threaded tree (for humans)",
	Long: `Renders the full conversation with threaded indentation.
Messages are grouped into threads based on re: references.
Color is auto-detected (TTY) and can be disabled with --no-color.`,
	Example: `  collab log                    # full threaded view
  collab log --compact          # summaries only
  collab log --open             # hide resolved threads
  collab log --no-color         # plain text, no ANSI
  collab log --task feature-x   # log one task conversation
  collab log --follow           # keep refreshing`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		s := findStoreForTask(cwd, logTask)

		useColor := !logNoColor && isTerminal()

		opts := render.PlainOptions{
			Color:    useColor,
			Compact:  logCompact,
			OpenOnly: logOpenOnly,
		}

		if !logFollow {
			return renderConversation(os.Stdout, s, opts)
		}

		interval, err := time.ParseDuration(logFollowInterval)
		if err != nil {
			return fmt.Errorf("parse --interval: %w", err)
		}
		if interval <= 0 {
			return fmt.Errorf("--interval must be > 0")
		}

		if err := renderConversation(os.Stdout, s, opts); err != nil {
			return err
		}

		lastSeq, err := latestConversationSeq(s)
		if err != nil {
			return err
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(sigCh)

		for {
			select {
			case <-sigCh:
				return nil
			case <-ticker.C:
				advanced, latest, err := hasConversationAdvanced(s, lastSeq)
				if err != nil {
					return err
				}
				if !advanced {
					continue
				}
				lastSeq = latest
				fmt.Fprintln(os.Stdout)
				fmt.Fprintf(os.Stdout, "--- update through #%d ---\n", lastSeq)
				if err := renderConversation(os.Stdout, s, opts); err != nil {
					return err
				}
			}
		}
	},
}

func renderConversation(w *os.File, s *store.Store, opts render.PlainOptions) error {
	export, err := buildExport(s)
	if err != nil {
		return err
	}
	render.PlainText(w, export, opts)
	return nil
}

func latestConversationSeq(s *store.Store) (int, error) {
	entries, err := s.List(0, "")
	if err != nil {
		return 0, err
	}
	maxSeq := 0
	for _, entry := range entries {
		if entry.Seq > maxSeq {
			maxSeq = entry.Seq
		}
	}
	return maxSeq, nil
}

func hasConversationAdvanced(s *store.Store, previousMaxSeq int) (bool, int, error) {
	latest, err := latestConversationSeq(s)
	if err != nil {
		return false, previousMaxSeq, err
	}
	if latest != previousMaxSeq {
		return true, latest, nil
	}
	return false, latest, nil
}

func isTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

func init() {
	logCmd.Flags().BoolVar(&logCompact, "compact", false, "Show summaries only, no message bodies")
	logCmd.Flags().BoolVar(&logOpenOnly, "open", false, "Hide resolved threads")
	logCmd.Flags().BoolVar(&logNoColor, "no-color", false, "Disable ANSI color output")
	logCmd.Flags().StringVar(&logTask, "task", store.DefaultTask, "Task namespace for conversation log")
	logCmd.Flags().BoolVar(&logFollow, "follow", false, "Refresh log output when conversation changes")
	logCmd.Flags().StringVar(&logFollowInterval, "interval", "2s", "Polling interval for --follow (e.g. 1s, 500ms)")
}

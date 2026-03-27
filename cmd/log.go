package cmd

import (
	"os"

	"github.com/david-krentzlin/collab/internal/render"
	"github.com/david-krentzlin/collab/internal/store"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	logCompact  bool
	logOpenOnly bool
	logNoColor  bool
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
  collab log --no-color         # plain text, no ANSI`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		s := store.Find(cwd)

		export, err := buildExport(s)
		if err != nil {
			return err
		}

		useColor := !logNoColor && isTerminal()

		opts := render.PlainOptions{
			Color:    useColor,
			Compact:  logCompact,
			OpenOnly: logOpenOnly,
		}
		render.PlainText(os.Stdout, export, opts)
		return nil
	},
}

func isTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

func init() {
	logCmd.Flags().BoolVar(&logCompact, "compact", false, "Show summaries only, no message bodies")
	logCmd.Flags().BoolVar(&logOpenOnly, "open", false, "Hide resolved threads")
	logCmd.Flags().BoolVar(&logNoColor, "no-color", false, "Disable ANSI color output")
}

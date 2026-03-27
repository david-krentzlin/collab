package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/david-krentzlin/collab/internal/store"
	"github.com/spf13/cobra"
)

var readRaw bool

var readCmd = &cobra.Command{
	Use:   "read <seq>",
	Short: "Read the full body of a message by sequence number",
	Long: `Fetches and displays the complete message for a given sequence number.
By default only the body is shown (what the agent needs to read).
Use --raw to include the frontmatter.`,
	Example: `  collab read 4          # show body only
  collab read 4 --raw   # show full file including frontmatter`,
	Args: cobra.ExactArgs(1),
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

		if readRaw {
			fmt.Print(string(msg.Marshal()))
		} else {
			// Print a compact header then the body
			re := ""
			if msg.Re > 0 {
				re = fmt.Sprintf(" re:#%d", msg.Re)
			}
			fmt.Printf("#%d [%s] from:%s -> %s%s\n", msg.Seq, msg.Type, msg.From, msg.To, re)
			fmt.Println("---")
			fmt.Print(msg.Body)
		}
		return nil
	},
}

func init() {
	readCmd.Flags().BoolVar(&readRaw, "raw", false, "Show full file including frontmatter")
}

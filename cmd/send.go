package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/david-krentzlin/collab/internal/message"
	"github.com/david-krentzlin/collab/internal/store"
	"github.com/spf13/cobra"
)

var (
	sendTo      string
	sendType    string
	sendRe      int
	sendSummary string
)

var sendCmd = &cobra.Command{
	Use:   "send",
	Short: "Send a message to another agent",
	Long: `Reads the message body from stdin, assigns a global sequence number,
and writes the message as a markdown file with frontmatter into the
sender's directory under .collab/.

The sender identity comes from the COLLAB_AGENT environment variable.`,
	Example: `  echo "Should we use a mutex here?" | collab send --to agent-b --type inquiry --summary "mutex question"
  cat proposal.md | collab send --to agent-a --type proposal --re 3 --summary "alternative design"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		from := os.Getenv("COLLAB_AGENT")
		if from == "" {
			return fmt.Errorf("COLLAB_AGENT env var not set — set it to this agent's name")
		}
		if sendTo == "" {
			return fmt.Errorf("--to is required")
		}
		if sendSummary == "" {
			return fmt.Errorf("--summary is required (one-line description for token-efficient scanning)")
		}

		msgType, err := message.ValidType(sendType)
		if err != nil {
			return err
		}

		// Read body from stdin
		body, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
		bodyStr := strings.TrimSpace(string(body))
		if bodyStr == "" {
			return fmt.Errorf("empty message body (pipe content to stdin)")
		}

		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		s := store.Find(cwd)
		if err := s.ValidateRecipient(sendTo); err != nil {
			return err
		}

		msg := &message.Message{
			From:    from,
			To:      sendTo,
			Type:    msgType,
			Re:      sendRe,
			TS:      message.Now(),
			Summary: sendSummary,
			Status:  message.Open,
			Body:    bodyStr,
		}

		path, err := s.CreateMessage(msg)
		if err != nil {
			return err
		}

		fmt.Printf("#%d [%s] -> %s: %s\n", msg.Seq, msgType, sendTo, sendSummary)
		fmt.Printf("wrote: %s\n", path)
		return nil
	},
}

func init() {
	sendCmd.Flags().StringVar(&sendTo, "to", "", "Recipient agent name")
	sendCmd.Flags().StringVar(&sendType, "type", "inquiry", "Message type (inquiry|reply|proposal|review|info)")
	sendCmd.Flags().IntVar(&sendRe, "re", 0, "Seq number this is in reply to")
	sendCmd.Flags().StringVar(&sendSummary, "summary", "", "One-line summary (required, used for token-efficient scanning)")
}

package cmd

import (
	"fmt"
	"os"

	"github.com/david-krentzlin/collab/internal/message"
	"github.com/david-krentzlin/collab/internal/render"
	"github.com/david-krentzlin/collab/internal/store"
	"github.com/spf13/cobra"
)

var (
	exportFormat string
	exportOutput string
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export conversation as JSON or HTML",
	Long: `Reads all messages, builds thread trees from re: references,
and exports the result as structured JSON or a self-contained HTML page.`,
	Example: `  collab export --format json
  collab export --format html > conversation.html
  collab export --format json -o conversation.json`,
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

		// Determine output writer
		w := os.Stdout
		if exportOutput != "" {
			f, err := os.Create(exportOutput)
			if err != nil {
				return fmt.Errorf("create output file: %w", err)
			}
			defer f.Close()
			w = f
		}

		switch exportFormat {
		case "json":
			return render.JSON(w, export)
		case "html":
			return render.HTML(w, export)
		default:
			return fmt.Errorf("unknown format %q (valid: json, html)", exportFormat)
		}
	},
}

func buildExport(s *store.Store) (*render.TaskExport, error) {
	// Get all messages (no filtering)
	entries, err := s.List(0, "")
	if err != nil {
		return nil, err
	}

	agents, err := s.Agents()
	if err != nil {
		return nil, err
	}

	// Read full messages
	msgs := make([]*message.Message, 0, len(entries))
	for _, e := range entries {
		msg, err := s.ReadMessageAtPath(e.Path)
		if err != nil {
			continue
		}
		msgs = append(msgs, msg)
	}

	threads, orphans := render.BuildThreads(msgs)

	taskName := s.Task
	if taskName == "" {
		taskName = store.DefaultTask
	}

	return &render.TaskExport{
		Task:    taskName,
		Agents:  agents,
		Threads: threads,
		Orphans: orphans,
	}, nil
}

func init() {
	exportCmd.Flags().StringVar(&exportFormat, "format", "json", "Output format: json, html")
	exportCmd.Flags().StringVarP(&exportOutput, "output", "o", "", "Output file (default: stdout)")
}

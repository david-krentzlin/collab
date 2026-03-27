package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "collab",
	Short: "Agent-to-agent collaboration over the filesystem",
	Long: `collab enables multiple AI agents to communicate with each other
through structured markdown files on the filesystem. Messages are
sequenced globally and stored per-agent for token-efficient polling.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(sendCmd)
	rootCmd.AddCommand(checkCmd)
	rootCmd.AddCommand(readCmd)
	rootCmd.AddCommand(resolveCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(logCmd)
	rootCmd.AddCommand(viewerCmd)
}

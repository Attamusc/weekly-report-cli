package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "weekly-report-cli",
	Short: "Generate weekly status reports from GitHub issues",
	Long: `weekly-report-cli is a CLI tool that generates weekly status reports by parsing
structured data from GitHub issue comments. It fetches GitHub issues, extracts
status report data using HTML comment markers, and generates markdown tables
with optional AI summarization.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Default behavior - show help
		cmd.Help()
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Global flags will be added here when generate command is implemented
}


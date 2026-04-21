package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/Attamusc/weekly-report-cli/internal/config"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "weekly-report-cli",
	Short: "Generate weekly status reports from GitHub issues",
	Long: `weekly-report-cli is a CLI tool that generates weekly status reports by parsing
structured data from GitHub issue comments. It fetches GitHub issues, extracts
status report data using HTML comment markers, and generates markdown tables
with optional AI summarization.`,
	SilenceErrors: true,
	SilenceUsage:  true,
	Run: func(cmd *cobra.Command, args []string) {
		// Default behavior - show help
		_ = cmd.Help()
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		if errors.Is(err, config.ErrNoRows) {
			os.Exit(2)
		}
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

func init() {
	// Global flags will be added here when generate command is implemented
}

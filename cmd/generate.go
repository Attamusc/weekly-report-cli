package cmd

import (
	"fmt"
	"os"

	"github.com/Attamusc/weekly-report-cli/internal/ai"
	"github.com/Attamusc/weekly-report-cli/internal/config"
	"github.com/Attamusc/weekly-report-cli/internal/input"
	"github.com/spf13/cobra"
)

var (
	sinceDays   int
	inputPath   string
	concurrency int
	noNotes     bool
)

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate weekly status report from GitHub issue URLs",
	Long: `Generate reads GitHub issue URLs from stdin or a file, fetches the issues
and their comments, extracts structured status reports, and outputs a markdown
table with optional AI-powered summarization.`,
	RunE: runGenerate,
}

func init() {
	rootCmd.AddCommand(generateCmd)
	
	// Add flags
	generateCmd.Flags().IntVar(&sinceDays, "since-days", 7, "Number of days to look back for updates")
	generateCmd.Flags().StringVar(&inputPath, "input", "", "Input file path (default: stdin)")
	generateCmd.Flags().IntVar(&concurrency, "concurrency", 4, "Number of concurrent workers")
	generateCmd.Flags().BoolVar(&noNotes, "no-notes", false, "Disable notes section in output")
}

func runGenerate(cmd *cobra.Command, args []string) error {
	
	// Load configuration
	cfg, err := config.FromEnvAndFlags(sinceDays, concurrency, noNotes, inputPath)
	if err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}

	// Determine input source
	var inputReader *os.File
	if inputPath == "" {
		inputReader = os.Stdin
	} else {
		inputReader, err = os.Open(inputPath)
		if err != nil {
			return fmt.Errorf("failed to open input file: %w", err)
		}
		defer inputReader.Close()
	}

	// Parse GitHub issue URLs
	issueRefs, err := input.ParseIssueLinks(inputReader)
	if err != nil {
		return fmt.Errorf("failed to parse issue links: %w", err)
	}

	if len(issueRefs) == 0 {
		fmt.Fprintf(os.Stderr, "No valid GitHub issue URLs found\n")
		os.Exit(2) // Exit code 2 for no rows produced
	}

	// Initialize summarizer based on configuration
	var summarizer ai.Summarizer
	if cfg.Models.Enabled {
		summarizer = ai.NewGHModelsClient(cfg.Models.BaseURL, cfg.Models.Model, cfg.GitHubToken)
		fmt.Printf("✓ AI summarization enabled (model: %s)\n", cfg.Models.Model)
	} else {
		summarizer = ai.NewNoopSummarizer()
		fmt.Printf("✓ AI summarization disabled\n")
	}
	_ = summarizer // TODO: Will be used in Phase 4 pipeline integration

	// TODO: Phase 3 complete - AI summarization ready
	// The following phases will implement:
	// - Status mapping and markdown rendering (Phase 4)
	// - Full pipeline integration with GitHub API and report processing

	fmt.Printf("Found %d issue(s) to process:\n", len(issueRefs))
	for _, ref := range issueRefs {
		fmt.Printf("- %s\n", ref.String())
	}
	
	fmt.Printf("\nConfiguration:\n")
	fmt.Printf("- Since days: %d\n", cfg.SinceDays)
	fmt.Printf("- Concurrency: %d\n", cfg.Concurrency) 
	fmt.Printf("- Notes enabled: %t\n", cfg.Notes)

	return nil
}
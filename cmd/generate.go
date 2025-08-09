package cmd

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/Attamusc/weekly-report-cli/internal/ai"
	"github.com/Attamusc/weekly-report-cli/internal/config"
	"github.com/Attamusc/weekly-report-cli/internal/derive"
	"github.com/Attamusc/weekly-report-cli/internal/format"
	"github.com/Attamusc/weekly-report-cli/internal/github"
	"github.com/Attamusc/weekly-report-cli/internal/input"
	"github.com/Attamusc/weekly-report-cli/internal/report"
	githubapi "github.com/google/go-github/v66/github"
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
	ctx := context.Background()
	
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

	// Initialize GitHub client
	githubClient := github.New(ctx, cfg.GitHubToken)

	// Initialize summarizer based on configuration
	var summarizer ai.Summarizer
	if cfg.Models.Enabled {
		summarizer = ai.NewGHModelsClient(cfg.Models.BaseURL, cfg.Models.Model, cfg.GitHubToken)
	} else {
		summarizer = ai.NewNoopSummarizer()
	}

	// Calculate time window
	since := time.Now().AddDate(0, 0, -cfg.SinceDays)

	// Process issues concurrently
	results := make(chan IssueResult, len(issueRefs))
	semaphore := make(chan struct{}, cfg.Concurrency)

	var wg sync.WaitGroup
	for _, ref := range issueRefs {
		wg.Add(1)
		go func(ref input.IssueRef) {
			defer wg.Done()
			semaphore <- struct{}{} // Acquire semaphore
			defer func() { <-semaphore }() // Release semaphore

			result := processIssue(ctx, githubClient, summarizer, ref, since, cfg.SinceDays)
			results <- result
		}(ref)
	}

	// Close results channel when all goroutines finish
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	var rows []format.Row
	var notes []format.Note

	for result := range results {
		if result.Err != nil {
			fmt.Fprintf(os.Stderr, "Error processing %s: %v\n", result.IssueURL, result.Err)
			continue
		}

		if result.Row != nil {
			rows = append(rows, *result.Row)
		}

		if result.Note != nil {
			notes = append(notes, *result.Note)
		}
	}

	// Generate output
	if len(rows) == 0 {
		fmt.Fprintf(os.Stderr, "No report rows generated\n")
		os.Exit(2) // Exit code 2 for no rows produced
	}

	// Output markdown table
	table := format.RenderTable(rows)
	fmt.Print(table)

	// Output notes section if enabled and there are notes
	if cfg.Notes && len(notes) > 0 {
		fmt.Print("\n")
		notesSection := format.RenderNotes(notes)
		fmt.Print(notesSection)
	}

	return nil
}

// IssueResult represents the result of processing a single issue
type IssueResult struct {
	IssueURL string
	Row      *format.Row
	Note     *format.Note
	Err      error
}

// processIssue handles the complete pipeline for a single GitHub issue
func processIssue(ctx context.Context, client *githubapi.Client, summarizer ai.Summarizer, 
	ref input.IssueRef, since time.Time, sinceDays int) IssueResult {
	
	// Fetch issue data
	issueData, err := github.FetchIssue(ctx, client, ref)
	if err != nil {
		return IssueResult{IssueURL: ref.URL, Err: fmt.Errorf("failed to fetch issue: %w", err)}
	}

	// Fetch comments since the time window
	comments, err := github.FetchCommentsSince(ctx, client, ref, since)
	if err != nil {
		return IssueResult{IssueURL: ref.URL, Err: fmt.Errorf("failed to fetch comments: %w", err)}
	}

	// Select reports within the time window
	reports := report.SelectReports(comments, since)

	if len(reports) == 0 {
		// No reports found - add note and return no row
		note := &format.Note{
			Kind:      format.NoteNoUpdatesInWindow,
			IssueURL:  ref.URL,
			SinceDays: sinceDays,
		}
		return IssueResult{IssueURL: ref.URL, Note: note}
	}

	// Use the newest report (reports are sorted newest-first)
	newestReport := reports[0]

	// Collect update texts for summarization (newest first)
	var updateTexts []string
	for _, rep := range reports {
		if rep.UpdateRaw != "" {
			updateTexts = append(updateTexts, rep.UpdateRaw)
		}
	}

	if len(updateTexts) == 0 {
		// Reports exist but no update text - add note and return no row
		note := &format.Note{
			Kind:      format.NoteNoUpdatesInWindow,
			IssueURL:  ref.URL,
			SinceDays: sinceDays,
		}
		return IssueResult{IssueURL: ref.URL, Note: note}
	}

	// Generate summary
	var summary string
	var summaryErr error
	if len(updateTexts) == 1 {
		summary, summaryErr = summarizer.Summarize(ctx, issueData.Title, ref.URL, updateTexts[0])
	} else {
		summary, summaryErr = summarizer.SummarizeMany(ctx, issueData.Title, ref.URL, updateTexts)
	}

	if summaryErr != nil {
		// Fall back to raw text if summarization fails
		if len(updateTexts) == 1 {
			summary = updateTexts[0]
		} else {
			summary = updateTexts[0] // Use newest update as fallback
		}
	}

	// Map status from trending field
	status := derive.MapTrending(newestReport.TrendingRaw)

	// Parse target date
	targetDate := derive.ParseTargetDate(newestReport.TargetDate)

	// Create table row
	row := format.NewRow(status, issueData.Title, ref.URL, targetDate, summary)

	// Add note if multiple reports were found
	var note *format.Note
	if len(reports) >= 2 {
		note = &format.Note{
			Kind:      format.NoteMultipleUpdates,
			IssueURL:  ref.URL,
			SinceDays: sinceDays,
		}
	}

	return IssueResult{
		IssueURL: ref.URL,
		Row:      &row,
		Note:     note,
	}
}
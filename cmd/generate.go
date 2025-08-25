package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
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
	sinceDays     int
	inputPath     string
	concurrency   int
	noNotes       bool
	verbose       bool
	quiet         bool
	summaryPrompt string
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
	generateCmd.Flags().BoolVar(&verbose, "verbose", false, "Enable verbose progress output")
	generateCmd.Flags().BoolVar(&quiet, "quiet", false, "Suppress all progress output")
	generateCmd.Flags().StringVar(&summaryPrompt, "summary-prompt", "", "Custom prompt for AI summarization (uses default if empty)")
}

func runGenerate(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Load configuration
	cfg, err := config.FromEnvAndFlags(sinceDays, concurrency, noNotes, verbose, quiet, inputPath, summaryPrompt)
	if err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}

	// Setup logging
	logger := setupLogger(cfg)
	ctx = context.WithValue(ctx, "logger", logger)

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
	logger.Info("Parsing GitHub URLs...")
	issueRefs, err := input.ParseIssueLinks(inputReader)
	if err != nil {
		return fmt.Errorf("failed to parse issue links: %w", err)
	}

	if len(issueRefs) == 0 {
		fmt.Fprintf(os.Stderr, "No valid GitHub issue URLs found\n")
		os.Exit(2) // Exit code 2 for no rows produced
	}

	logger.Info("Found GitHub issues", "count", len(issueRefs))

	// Initialize GitHub client
	logger.Debug("Initializing GitHub client")
	githubClient := github.New(ctx, cfg.GitHubToken)

	// Initialize summarizer based on configuration
	var summarizer ai.Summarizer
	if cfg.Models.Enabled {
		logger.Debug("AI summarization enabled", "model", cfg.Models.Model)
		summarizer = ai.NewGHModelsClient(cfg.Models.BaseURL, cfg.Models.Model, cfg.GitHubToken, cfg.Models.SystemPrompt)
	} else {
		logger.Debug("AI summarization disabled")
		summarizer = ai.NewNoopSummarizer()
	}

	// Calculate time window
	since := time.Now().AddDate(0, 0, -cfg.SinceDays)
	logger.Debug("Looking for updates since", "since", since.Format("2006-01-02"))

	// Process issues concurrently
	logger.Info("Fetching GitHub data...", "concurrency", cfg.Concurrency)
	results := make(chan IssueResult, len(issueRefs))
	semaphore := make(chan struct{}, cfg.Concurrency)

	// Progress tracking
	var completed atomic.Int32
	var wg sync.WaitGroup

	for _, ref := range issueRefs {
		wg.Add(1)
		go func(ref input.IssueRef) {
			defer wg.Done()
			semaphore <- struct{}{}        // Acquire semaphore
			defer func() { <-semaphore }() // Release semaphore

			result := processIssue(ctx, githubClient, summarizer, ref, since, cfg.SinceDays)

			// Update progress
			current := completed.Add(1)
			if !cfg.Quiet {
				logger.Info("Processing issues",
					"completed", int(current),
					"total", len(issueRefs))
			}

			results <- result
		}(ref)
	}

	// Close results channel when all goroutines finish
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	logger.Info("Collecting results...")
	var rows []format.Row
	var notes []format.Note
	var errorCount int

	for result := range results {
		if result.Err != nil {
			errorCount++
			fmt.Fprintf(os.Stderr, "Error processing %s: %v\n", result.IssueURL, result.Err)
			logger.Debug("Error processing issue", "url", result.IssueURL, "error", result.Err)
			continue
		}

		if result.Row != nil {
			rows = append(rows, *result.Row)
			logger.Debug("Added report row", "issue", result.IssueURL)
		}

		if result.Note != nil {
			notes = append(notes, *result.Note)
			logger.Debug("Added note", "issue", result.IssueURL, "kind", result.Note.Kind)
		}
	}

	if errorCount > 0 {
		logger.Info("Processing completed with errors", "errors", errorCount, "successful", len(issueRefs)-errorCount)
	} else {
		logger.Info("Processing completed successfully")
	}

	// Generate output
	if len(rows) == 0 {
		fmt.Fprintf(os.Stderr, "No report rows generated\n")
		os.Exit(2) // Exit code 2 for no rows produced
	}

	// Sort rows by target date (earliest first, then TBD)
	format.SortRowsByTargetDate(rows)

	// Output markdown table
	logger.Info("Rendering output...", "rows", len(rows))
	table := format.RenderTable(rows)
	fmt.Print(table)

	// Output notes section if enabled and there are notes
	if cfg.Notes && len(notes) > 0 {
		logger.Debug("Adding notes section", "notes", len(notes))
		fmt.Print("\n")
		notesSection := format.RenderNotes(notes)
		fmt.Print(notesSection)
	}

	logger.Info("Report generated successfully", "rows", len(rows), "notes", len(notes))

	return nil
}

// setupLogger creates a logger configured for progress output
func setupLogger(cfg *config.Config) *slog.Logger {
	if cfg.Quiet {
		// Discard all log output when quiet
		return slog.New(slog.NewTextHandler(os.NewFile(0, os.DevNull), &slog.HandlerOptions{
			Level: slog.LevelError + 1, // Higher than any log level to discard all
		}))
	}

	level := slog.LevelInfo
	if cfg.Verbose {
		level = slog.LevelDebug
	}

	// Use stderr for progress so stdout stays clean for output
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Remove time stamps for cleaner progress output
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	}))
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

	// Get logger from context
	logger, ok := ctx.Value("logger").(*slog.Logger)
	if !ok {
		logger = slog.Default()
	}

	logger.Debug("Processing issue", "url", ref.URL)

	// Fetch issue data
	logger.Debug("Fetching issue data", "issue", ref.String())
	issueData, err := github.FetchIssue(ctx, client, ref)
	if err != nil {
		logger.Debug("Failed to fetch issue data", "issue", ref.String(), "error", err)
		return IssueResult{IssueURL: ref.URL, Err: fmt.Errorf("failed to fetch issue: %w", err)}
	}

	// Fetch comments since the time window
	logger.Debug("Fetching comments", "issue", ref.String(), "since", since.Format("2006-01-02"))
	comments, err := github.FetchCommentsSince(ctx, client, ref, since)
	if err != nil {
		logger.Debug("Failed to fetch comments", "issue", ref.String(), "error", err)
		return IssueResult{IssueURL: ref.URL, Err: fmt.Errorf("failed to fetch comments: %w", err)}
	}

	// Select reports within the time window
	logger.Debug("Parsing reports", "issue", ref.String(), "comments", len(comments))
	reports := report.SelectReports(comments, since)

	if len(reports) == 0 {
		logger.Debug("No reports found in time window", "issue", ref.String())
		// No reports found - create row with NeedsUpdate status and add note
		status := derive.NeedsUpdate
		summary := fmt.Sprintf("No update provided in last %d days", sinceDays)
		row := format.NewRow(status, issueData.Title, ref.URL, nil, summary)

		note := &format.Note{
			Kind:      format.NoteNoUpdatesInWindow,
			IssueURL:  ref.URL,
			SinceDays: sinceDays,
		}
		return IssueResult{IssueURL: ref.URL, Row: &row, Note: note}
	}

	logger.Debug("Found reports", "issue", ref.String(), "count", len(reports))

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
		logger.Debug("No update text found", "issue", ref.String())
		// Reports exist but no update text - create row with NeedsUpdate status and add note
		status := derive.NeedsUpdate
		summary := fmt.Sprintf("No structured update found in last %d days", sinceDays)

		// Parse target date from newest report if available
		var targetDate *time.Time
		if len(reports) > 0 {
			targetDate = derive.ParseTargetDate(reports[0].TargetDate)
		}

		row := format.NewRow(status, issueData.Title, ref.URL, targetDate, summary)

		note := &format.Note{
			Kind:      format.NoteNoUpdatesInWindow,
			IssueURL:  ref.URL,
			SinceDays: sinceDays,
		}
		return IssueResult{IssueURL: ref.URL, Row: &row, Note: note}
	}

	// Generate summary
	logger.Debug("Generating summary", "issue", ref.String(), "updates", len(updateTexts))
	var summary string
	var summaryErr error
	if len(updateTexts) == 1 {
		summary, summaryErr = summarizer.Summarize(ctx, issueData.Title, ref.URL, updateTexts[0])
	} else {
		summary, summaryErr = summarizer.SummarizeMany(ctx, issueData.Title, ref.URL, updateTexts)
	}

	if summaryErr != nil {
		logger.Debug("AI summarization failed, using fallback", "issue", ref.String(), "error", summaryErr)
		// Fall back to raw text if summarization fails
		if len(updateTexts) == 1 {
			summary = updateTexts[0]
		} else {
			summary = updateTexts[0] // Use newest update as fallback
		}
	} else {
		logger.Debug("AI summarization succeeded", "issue", ref.String())
	}

	// Map status from trending field
	status := derive.MapTrending(newestReport.TrendingRaw)

	// Parse target date
	targetDate := derive.ParseTargetDate(newestReport.TargetDate)

	// Create table row
	logger.Debug("Creating table row", "issue", ref.String(), "status", status)
	row := format.NewRow(status, issueData.Title, ref.URL, targetDate, summary)

	// Add note if multiple reports were found
	var note *format.Note
	if len(reports) >= 2 {
		logger.Debug("Multiple reports found, adding note", "issue", ref.String(), "count", len(reports))
		note = &format.Note{
			Kind:      format.NoteMultipleUpdates,
			IssueURL:  ref.URL,
			SinceDays: sinceDays,
		}
	}

	logger.Debug("Successfully processed issue", "issue", ref.String())
	return IssueResult{
		IssueURL: ref.URL,
		Row:      &row,
		Note:     note,
	}
}

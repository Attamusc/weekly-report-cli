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
	noSentiment   bool
	verbose       bool
	quiet         bool
	summaryPrompt string

	// Project board related flags
	projectURL         string
	projectField       string
	projectFieldValues string
	projectIncludePRs  bool
	projectMaxItems    int
	projectView        string // NEW: View name
	projectViewID      string // NEW: View ID
)

// summaryCompleted is the default summary for done/closed issues that don't need AI summarization.
const summaryCompleted = "Completed"

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate weekly status report from GitHub issues",
	Long: `Generate reads GitHub issues from multiple sources and creates a weekly status report.

Input Sources:
  1. GitHub Project Board: Use --project flag with field-based filtering
  2. URL List: Provide issue URLs via stdin or --input file
  3. Mixed: Combine both project board and URL list

The tool fetches issues, extracts structured status reports from comments,
and outputs a markdown table with optional AI-powered summarization.

Examples:
  # From project board (using defaults: Status field, "In Progress,Done,Blocked")
  weekly-report-cli generate --project "org:my-org/5"

  # From project board with custom field/values
  weekly-report-cli generate \
    --project "org:my-org/5" \
    --project-field "Priority" \
    --project-field-values "High,Critical"

  # From project board using a view
  weekly-report-cli generate \
    --project "org:my-org/5" \
    --project-view "Blocked Items"

  # From project board view with additional filters
  weekly-report-cli generate \
    --project "org:my-org/5" \
    --project-view "Current Sprint" \
    --project-field "Priority" \
    --project-field-values "High"

  # From URL list (stdin)
  cat issues.txt | weekly-report-cli generate --since-days 7

  # Mixed sources
  weekly-report-cli generate \
    --project "org:my-org/5" \
    --input additional-issues.txt`,
	RunE: runGenerate,
}

func init() {
	rootCmd.AddCommand(generateCmd)

	// Add flags
	generateCmd.Flags().IntVar(&sinceDays, "since-days", 7, "Number of days to look back for updates")
	generateCmd.Flags().StringVar(&inputPath, "input", "", "Input file path (default: stdin)")
	generateCmd.Flags().IntVar(&concurrency, "concurrency", 4, "Number of concurrent workers")
	generateCmd.Flags().BoolVar(&noNotes, "no-notes", false, "Disable notes section in output")
	generateCmd.Flags().BoolVar(&noSentiment, "no-sentiment", false, "Disable AI sentiment analysis")
	generateCmd.Flags().BoolVar(&verbose, "verbose", false, "Enable verbose progress output")
	generateCmd.Flags().BoolVar(&quiet, "quiet", false, "Suppress all progress output")
	generateCmd.Flags().StringVar(&summaryPrompt, "summary-prompt", "", "Custom prompt for AI summarization (uses default if empty)")

	// Project board flags
	generateCmd.Flags().StringVar(&projectURL, "project", "", "GitHub project board URL or identifier (e.g., 'https://github.com/orgs/my-org/projects/5' or 'org:my-org/5')")
	generateCmd.Flags().StringVar(&projectField, "project-field", "Status", "Field name to filter by (default: 'Status')")
	generateCmd.Flags().StringVar(&projectFieldValues, "project-field-values", "In Progress,Done,Blocked", "Comma-separated values to match (default: 'In Progress,Done,Blocked')")
	generateCmd.Flags().BoolVar(&projectIncludePRs, "project-include-prs", false, "Include pull requests from project board (default: issues only)")
	generateCmd.Flags().IntVar(&projectMaxItems, "project-max-items", 100, "Maximum number of items to fetch from project board")
	generateCmd.Flags().StringVar(&projectView, "project-view", "", "GitHub project view name (e.g., 'Blocked Items')")
	generateCmd.Flags().StringVar(&projectViewID, "project-view-id", "", "GitHub project view ID (e.g., 'PVT_kwDOABCDEF') - takes precedence over --project-view")
}

func runGenerate(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Parse project field values
	var projectFieldValuesList []string
	if projectFieldValues != "" {
		projectFieldValuesList = input.ParseFieldValues(projectFieldValues)
	}

	// Load configuration
	cfg, err := config.FromEnvAndFlags(sinceDays, concurrency, noNotes, verbose, quiet, inputPath, summaryPrompt, projectURL, projectField, projectFieldValuesList, projectIncludePRs, projectMaxItems, projectView, projectViewID, noSentiment)
	if err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}

	// Setup logging
	logger := setupLogger(cfg)
	ctx = context.WithValue(ctx, input.LoggerContextKey{}, logger)

	// Initialize project client if needed
	var projectClient *projectClientAdapter
	if cfg.Project.URL != "" {
		logger.Debug("Initializing project client")
		projectClient = &projectClientAdapter{
			token:  cfg.GitHubToken,
			logger: logger,
		}
	}

	// Resolve issue references from all sources (project board and/or URL list)
	logger.Info("Resolving issue references...")
	resolverCfg := input.ResolverConfig{
		ProjectURL:         cfg.Project.URL,
		ProjectFieldName:   cfg.Project.FieldName,
		ProjectFieldValues: cfg.Project.FieldValues,
		ProjectIncludePRs:  cfg.Project.IncludePRs,
		ProjectMaxItems:    cfg.Project.MaxItems,
		ProjectView:        cfg.Project.ViewName,
		ProjectViewID:      cfg.Project.ViewID,
		URLListPath:        inputPath,
		UseStdin:           inputPath == "" && cfg.Project.URL == "",
	}

	issueRefs, err := input.ResolveIssueRefs(ctx, resolverCfg, projectClient)
	if err != nil {
		return fmt.Errorf("failed to resolve issue references: %w", err)
	}

	if len(issueRefs) == 0 {
		if !cfg.Quiet {
			fmt.Fprintf(os.Stderr, "No valid GitHub issue URLs found\n")
		}
		os.Exit(2) // Exit code 2 for no rows produced
	}

	logger.Info("Found GitHub issues", "count", len(issueRefs))

	// Initialize GitHub client
	logger.Debug("Initializing GitHub client")
	githubClient := github.New(ctx, cfg.GitHubToken)

	// Initialize summarizer based on configuration
	summarizer := initSummarizer(cfg, logger)

	// Calculate time window
	since := time.Now().AddDate(0, 0, -cfg.SinceDays)
	logger.Debug("Looking for updates since", "since", since.Format("2006-01-02"))

	// ========== PHASE A: Collect all issue data (parallel) ==========
	logger.Info("Collecting issue data...", "concurrency", cfg.Concurrency)
	dataResults := make(chan IssueDataResult, len(issueRefs))
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

			data, err := collectIssueData(ctx, githubClient, ref, since, cfg.SinceDays)

			// Update progress
			current := completed.Add(1)
			if !cfg.Quiet {
				logger.Info("Collecting issue data",
					"completed", int(current),
					"total", len(issueRefs))
			}

			dataResults <- IssueDataResult{Data: data, Err: err}
		}(ref)
	}

	// Close results channel when all goroutines finish
	go func() {
		wg.Wait()
		close(dataResults)
	}()

	// Collect all data
	var allData []IssueData
	var errorCount int

	for result := range dataResults {
		if result.Err != nil {
			errorCount++
			if !cfg.Quiet {
				fmt.Fprintf(os.Stderr, "Error collecting data for issue: %v\n", result.Err)
			}
			logger.Debug("Error collecting issue data", "error", result.Err)
			continue
		}
		allData = append(allData, result.Data)
	}

	if errorCount > 0 {
		logger.Info("Data collection completed with errors", "errors", errorCount, "successful", len(allData))
	} else {
		logger.Info("Data collection completed successfully", "issues", len(allData))
	}

	// ========== PHASE B: Batch summarization (single API call) ==========
	var batchResults map[string]ai.BatchResult
	if cfg.Models.Enabled {
		var err error
		batchResults, err = batchSummarize(ctx, summarizer, allData, logger)
		if err != nil {
			logger.Warn("Batch summarization failed, using fallbacks", "error", err)
			batchResults = make(map[string]ai.BatchResult) // Empty map triggers fallback
		}
	} else {
		logger.Debug("AI summarization disabled, using fallbacks")
		batchResults = make(map[string]ai.BatchResult)
	}

	// ========== PHASE C: Create final results ==========
	rows, notes := assembleGenerateResults(allData, batchResults, cfg, logger)

	// Generate output
	renderGenerateOutput(rows, notes, cfg, logger)

	return nil
}

// assembleGenerateResults creates rows and notes from collected data and batch AI results
func assembleGenerateResults(allData []IssueData, batchResults map[string]ai.BatchResult, cfg *config.Config, logger *slog.Logger) ([]format.Row, []format.Note) {
	logger.Info("Creating final results...")
	var rows []format.Row
	var notes []format.Note

	for _, data := range allData {
		var summary string

		if result, ok := batchResults[data.IssueURL]; ok {
			summary = result.Summary

			// Check for sentiment mismatch (only if sentiment is enabled)
			if cfg.Models.Sentiment && result.Sentiment != nil {
				suggestedStatus, valid := derive.ParseStatusKey(result.Sentiment.SuggestedStatus)
				if valid && suggestedStatus != data.Status {
					notes = append(notes, format.Note{
						Kind:            format.NoteSentimentMismatch,
						IssueURL:        data.IssueURL,
						ReportedStatus:  data.ReportedStatusCaption,
						SuggestedStatus: suggestedStatus.Caption,
						Explanation:     result.Sentiment.Explanation,
					})
				}
			}
		}

		if summary == "" {
			summary = data.FallbackSummary
		}

		result := createResultFromData(data, summary)
		if result.Row != nil {
			rows = append(rows, *result.Row)
			logger.Debug("Added report row", "issue", result.IssueURL)
		}

		if result.Note != nil {
			notes = append(notes, *result.Note)
			logger.Debug("Added note", "issue", result.IssueURL, "kind", result.Note.Kind)
		}
	}

	logger.Info("Results created successfully", "rows", len(rows), "notes", len(notes))
	return rows, notes
}

// renderGenerateOutput sorts, renders, and prints the report output
func renderGenerateOutput(rows []format.Row, notes []format.Note, cfg *config.Config, logger *slog.Logger) {
	if len(rows) == 0 {
		if !cfg.Quiet {
			fmt.Fprintf(os.Stderr, "No report rows generated\n")
		}
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
}

// IssueResult represents the result of processing a single issue
type IssueResult struct {
	IssueURL string
	Row      *format.Row
	Note     *format.Note
	Err      error
}

// IssueData represents collected data from an issue before AI summarization
type IssueData struct {
	IssueURL              string
	IssueTitle            string
	IssueState            string
	ClosedAt              *time.Time
	CloseReason           string
	Reports               []report.Report
	UpdateTexts           []string // Raw updates (newest first)
	Status                derive.Status
	ReportedStatusCaption string // Original status caption for sentiment comparison
	TargetDate            *time.Time
	ShouldSummarize       bool         // Whether this needs AI summarization
	FallbackSummary       string       // Used if AI fails or not needed
	Note                  *format.Note // Note to include in output
}

// IssueDataResult represents the result of collecting issue data
type IssueDataResult struct {
	Data IssueData
	Err  error
}

// createResultFromData creates an IssueResult from collected data and AI summary
func createResultFromData(data IssueData, summary string) IssueResult {
	// Use AI summary if available, otherwise use fallback
	if summary == "" {
		summary = data.FallbackSummary
	}

	// Create table row
	row := format.NewRow(data.Status, data.IssueTitle, data.IssueURL, data.TargetDate, summary)

	return IssueResult{
		IssueURL: data.IssueURL,
		Row:      &row,
		Note:     data.Note,
		Err:      nil,
	}
}

// batchSummarize summarizes all collected issue data in a single API call
func batchSummarize(ctx context.Context, summarizer ai.Summarizer, allData []IssueData, logger *slog.Logger) (map[string]ai.BatchResult, error) {
	// Collect items that need summarization
	var batchItems []ai.BatchItem
	for _, data := range allData {
		if data.ShouldSummarize && len(data.UpdateTexts) > 0 {
			batchItems = append(batchItems, ai.BatchItem{
				IssueURL:       data.IssueURL,
				IssueTitle:     data.IssueTitle,
				UpdateTexts:    data.UpdateTexts,
				ReportedStatus: data.ReportedStatusCaption,
			})
		}
	}

	if len(batchItems) == 0 {
		logger.Debug("No items need summarization")
		return map[string]ai.BatchResult{}, nil
	}

	logger.Info("Batch summarizing updates", "count", len(batchItems))
	summaries, err := summarizer.SummarizeBatch(ctx, batchItems)
	if err != nil {
		logger.Warn("Batch summarization failed", "error", err)
		return map[string]ai.BatchResult{}, err
	}

	logger.Info("Batch summarization completed", "summaries", len(summaries))
	return summaries, nil
}

// collectIssueData fetches GitHub data and extracts reports without AI summarization
func collectIssueData(ctx context.Context, client *githubapi.Client, ref input.IssueRef, since time.Time, sinceDays int) (IssueData, error) {
	// Get logger from context
	logger, ok := ctx.Value(input.LoggerContextKey{}).(*slog.Logger)
	if !ok {
		logger = slog.Default()
	}

	logger.Debug("Collecting issue data", "url", ref.URL)

	// Fetch issue data
	issueData, err := github.FetchIssue(ctx, client, ref)
	if err != nil {
		return IssueData{}, fmt.Errorf("failed to fetch issue: %w", err)
	}

	// Fetch comments since the time window
	comments, err := github.FetchCommentsSince(ctx, client, ref, since)
	if err != nil {
		return IssueData{}, fmt.Errorf("failed to fetch comments: %w", err)
	}

	// Select reports within the time window
	reports := report.SelectReports(comments, since)

	result := IssueData{
		IssueURL:    ref.URL,
		IssueTitle:  issueData.Title,
		IssueState:  issueData.State,
		ClosedAt:    issueData.ClosedAt,
		CloseReason: issueData.CloseReason,
		Reports:     reports,
	}

	// Case 1: No reports found
	if len(reports) == 0 {
		if issueData.State == github.StateClosed {
			// Issue is closed - use Done status, no AI needed
			result.Status = derive.Done
			result.ReportedStatusCaption = derive.Done.Caption
			result.TargetDate = issueData.ClosedAt
			result.ShouldSummarize = false
			result.FallbackSummary = summaryCompleted
		} else if commentBody, ok := report.SelectMostRecentComment(comments); ok {
			// Case 1 fallback: no structured reports but comments exist —
			// use the most recent comment body for AI summarization
			result.Status = derive.Unknown
			result.ReportedStatusCaption = derive.Unknown.Caption
			result.UpdateTexts = []string{commentBody}
			result.ShouldSummarize = true
			result.FallbackSummary = commentBody
			result.Note = &format.Note{
				Kind:     format.NoteUnstructuredFallback,
				IssueURL: ref.URL,
			}
		} else {
			// No comments at all - needs update
			result.Status = derive.NeedsUpdate
			result.ReportedStatusCaption = derive.NeedsUpdate.Caption
			result.ShouldSummarize = false
			result.FallbackSummary = fmt.Sprintf("No update provided in last %d days", sinceDays)
			result.Note = &format.Note{
				Kind:      format.NoteNoUpdatesInWindow,
				IssueURL:  ref.URL,
				SinceDays: sinceDays,
			}
		}
		return result, nil
	}

	// Case 2: Reports exist - collect update texts
	var updateTexts []string
	for _, rep := range reports {
		if rep.UpdateRaw != "" {
			updateTexts = append(updateTexts, rep.UpdateRaw)
		}
	}
	result.UpdateTexts = updateTexts

	// Use newest report for status and target date
	newestReport := reports[0]
	result.Status = derive.MapTrending(newestReport.TrendingRaw)
	result.ReportedStatusCaption = result.Status.Caption
	result.TargetDate = derive.ParseTargetDate(newestReport.TargetDate)

	// Case 2a: Reports exist but no update text
	if len(updateTexts) == 0 {
		if issueData.State == github.StateClosed {
			// Issue is closed - use Done status, no AI needed
			result.Status = derive.Done
			result.ReportedStatusCaption = derive.Done.Caption
			if result.TargetDate == nil {
				result.TargetDate = issueData.ClosedAt
			}
			result.ShouldSummarize = false
			result.FallbackSummary = summaryCompleted
		} else if commentBody, ok := report.SelectMostRecentComment(comments); ok {
			// Case 2a fallback: structured reports exist but have no update text —
			// keep status and target date from report, use most recent comment body
			result.UpdateTexts = []string{commentBody}
			result.ShouldSummarize = true
			result.FallbackSummary = commentBody
			result.Note = &format.Note{
				Kind:     format.NoteUnstructuredFallback,
				IssueURL: ref.URL,
			}
		} else {
			// No comments at all - needs update
			result.Status = derive.NeedsUpdate
			result.ReportedStatusCaption = derive.NeedsUpdate.Caption
			result.ShouldSummarize = false
			result.FallbackSummary = fmt.Sprintf("No structured update found in last %d days", sinceDays)
			result.Note = &format.Note{
				Kind:      format.NoteNoUpdatesInWindow,
				IssueURL:  ref.URL,
				SinceDays: sinceDays,
			}
		}
		return result, nil
	}

	// Case 2b: Reports with update text
	if result.Status == derive.Done || issueData.State == github.StateClosed {
		// Done or closed issues don't need AI summarization
		if issueData.State == github.StateClosed {
			result.Status = derive.Done
			result.ReportedStatusCaption = derive.Done.Caption
		}
		result.ShouldSummarize = false
		result.FallbackSummary = summaryCompleted
	} else {
		// Active issues need summarization
		result.ShouldSummarize = true
		result.FallbackSummary = updateTexts[0] // Use newest update as fallback
	}

	// Add note if multiple reports
	if len(reports) >= 2 {
		result.Note = &format.Note{
			Kind:      format.NoteMultipleUpdates,
			IssueURL:  ref.URL,
			SinceDays: sinceDays,
		}
	}

	return result, nil
}

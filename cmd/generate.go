package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Attamusc/weekly-report-cli/internal/ai"
	"github.com/Attamusc/weekly-report-cli/internal/config"
	"github.com/Attamusc/weekly-report-cli/internal/derive"
	"github.com/Attamusc/weekly-report-cli/internal/format"
	"github.com/Attamusc/weekly-report-cli/internal/github"
	"github.com/Attamusc/weekly-report-cli/internal/input"
	"github.com/Attamusc/weekly-report-cli/internal/projects"
	"github.com/Attamusc/weekly-report-cli/internal/report"
	githubapi "github.com/google/go-github/v66/github"
	"github.com/spf13/cobra"
)

// projectClientAdapter adapts the projects.Client to the input.ProjectClient interface
// This avoids circular dependencies between packages
type projectClientAdapter struct {
	token  string
	logger *slog.Logger
}

// FetchProjectItems implements input.ProjectClient interface
func (a *projectClientAdapter) FetchProjectItems(ctx context.Context, configInterface interface{}) ([]input.IssueRef, error) {
	// Convert the resolver config to project config
	resolverCfg, ok := configInterface.(input.ResolverConfig)
	if !ok {
		return nil, fmt.Errorf("invalid config type for project client")
	}

	// Parse project URL
	projectRef, err := projects.ParseProjectURL(resolverCfg.ProjectURL)
	if err != nil {
		return nil, fmt.Errorf("invalid project URL: %w", err)
	}

	// Create project config
	projectCfg := projects.ProjectConfig{
		Ref:      projectRef,
		ViewName: resolverCfg.ProjectView,
		ViewID:   resolverCfg.ProjectViewID,
		FieldFilters: []projects.FieldFilter{
			{
				FieldName: resolverCfg.ProjectFieldName,
				Values:    resolverCfg.ProjectFieldValues,
			},
		},
		IncludePRs: resolverCfg.ProjectIncludePRs,
		MaxItems:   resolverCfg.ProjectMaxItems,
	}

	// Create projects client and fetch items
	// Note: FetchProjectItems now handles view filter resolution and applies all filters internally
	client := projects.NewClient(a.token)
	projectItems, err := client.FetchProjectItems(ctx, projectCfg)
	if err != nil {
		return nil, err
	}

	// Extract issue refs from filtered items
	// Note: Filtering is already done in FetchProjectItems (including view filters)
	var issueRefs []input.IssueRef
	for _, item := range projectItems {
		if item.IssueRef != nil {
			issueRefs = append(issueRefs, *item.IssueRef)
		}
	}

	a.logger.Info("Project items fetched and filtered", "project", projectRef.String(), "items", len(issueRefs))

	return issueRefs, nil
}

var (
	sinceDays     int
	inputPath     string
	concurrency   int
	noNotes       bool
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
	cfg, err := config.FromEnvAndFlags(sinceDays, concurrency, noNotes, verbose, quiet, inputPath, summaryPrompt, projectURL, projectField, projectFieldValuesList, projectIncludePRs, projectMaxItems, projectView, projectViewID)
	if err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}

	// Setup logging
	logger := setupLogger(cfg)
	ctx = context.WithValue(ctx, "logger", logger)

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
			fmt.Fprintf(os.Stderr, "Error collecting data for issue: %v\n", result.Err)
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
	var summaries map[string]string
	if cfg.Models.Enabled {
		var err error
		summaries, err = batchSummarize(ctx, summarizer, allData, logger)
		if err != nil {
			logger.Warn("Batch summarization failed, using fallbacks", "error", err)
			summaries = make(map[string]string) // Empty map triggers fallback
		}
	} else {
		logger.Debug("AI summarization disabled, using fallbacks")
		summaries = make(map[string]string)
	}

	// ========== PHASE C: Create final results ==========
	logger.Info("Creating final results...")
	var rows []format.Row
	var notes []format.Note

	for _, data := range allData {
		summary := summaries[data.IssueURL]
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

// IssueData represents collected data from an issue before AI summarization
type IssueData struct {
	IssueURL        string
	IssueTitle      string
	IssueState      string
	ClosedAt        *time.Time
	CloseReason     string
	Reports         []report.Report
	UpdateTexts     []string // Raw updates (newest first)
	Status          derive.Status
	TargetDate      *time.Time
	ShouldSummarize bool         // Whether this needs AI summarization
	FallbackSummary string       // Used if AI fails or not needed
	Note            *format.Note // Note to include in output
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
func batchSummarize(ctx context.Context, summarizer ai.Summarizer, allData []IssueData, logger *slog.Logger) (map[string]string, error) {
	// Collect items that need summarization
	var batchItems []ai.BatchItem
	for _, data := range allData {
		if data.ShouldSummarize && len(data.UpdateTexts) > 0 {
			batchItems = append(batchItems, ai.BatchItem{
				IssueURL:    data.IssueURL,
				IssueTitle:  data.IssueTitle,
				UpdateTexts: data.UpdateTexts,
			})
		}
	}

	if len(batchItems) == 0 {
		logger.Debug("No items need summarization")
		return map[string]string{}, nil
	}

	logger.Info("Batch summarizing updates", "count", len(batchItems))
	summaries, err := summarizer.SummarizeBatch(ctx, batchItems)
	if err != nil {
		logger.Warn("Batch summarization failed", "error", err)
		return map[string]string{}, err
	}

	logger.Info("Batch summarization completed", "summaries", len(summaries))
	return summaries, nil
}

// collectIssueData fetches GitHub data and extracts reports without AI summarization
func collectIssueData(ctx context.Context, client *githubapi.Client, ref input.IssueRef, since time.Time, sinceDays int) (IssueData, error) {
	// Get logger from context
	logger, ok := ctx.Value("logger").(*slog.Logger)
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
		if issueData.State == "closed" {
			// Issue is closed - use Done status
			result.Status = derive.Done
			result.TargetDate = issueData.ClosedAt
			result.ShouldSummarize = true
			result.UpdateTexts = []string{issueData.CloseReason}
			result.FallbackSummary = "Issue was closed"
		} else {
			// Issue is open - needs update
			result.Status = derive.NeedsUpdate
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
	result.TargetDate = derive.ParseTargetDate(newestReport.TargetDate)

	// Case 2a: Reports exist but no update text
	if len(updateTexts) == 0 {
		if issueData.State == "closed" {
			// Issue is closed - use Done status
			result.Status = derive.Done
			if result.TargetDate == nil {
				result.TargetDate = issueData.ClosedAt
			}
			result.ShouldSummarize = true
			result.UpdateTexts = []string{issueData.CloseReason}
			result.FallbackSummary = "Issue was closed"
		} else {
			// Issue is open - needs update
			result.Status = derive.NeedsUpdate
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

	// Case 2b: Reports with update text - needs summarization
	result.ShouldSummarize = true
	result.FallbackSummary = updateTexts[0] // Use newest update as fallback

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

// summarizeWithFallback provides consistent AI summarization with fallback handling
func summarizeWithFallback(ctx context.Context, summarizer ai.Summarizer, issueTitle, issueURL, updateText string, fallbackText string, logger *slog.Logger) string {
	// Check if updateText is empty or only whitespace
	trimmed := strings.TrimSpace(updateText)
	if trimmed == "" {
		return fallbackText
	}

	summary, err := summarizer.Summarize(ctx, issueTitle, issueURL, updateText)
	if err != nil {
		logger.Debug("AI summarization failed for closing comment, using fallback", "error", err)
		return trimmed // Use trimmed original text as fallback, not the generic fallback
	}

	logger.Debug("AI summarization succeeded for closing comment")
	return summary
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

		// Check if issue is closed - if so, use Done status with closing information
		if issueData.State == "closed" {
			logger.Debug("Issue is closed, using Done status", "issue", ref.String())
			status := derive.Done

			// Use AI summarization for closing comment
			summary := summarizeWithFallback(ctx, summarizer, issueData.Title, ref.URL, issueData.CloseReason, "Issue was closed", logger)

			// Use close date as target date
			var targetDate *time.Time
			if issueData.ClosedAt != nil {
				targetDate = issueData.ClosedAt
			}

			row := format.NewRow(status, issueData.Title, ref.URL, targetDate, summary)
			return IssueResult{
				IssueURL: ref.URL,
				Row:      &row,
				Note:     nil, // No notes needed for closed issues
			}
		}

		// Issue is open - create row with NeedsUpdate status and add note
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

		// Check if issue is closed - if so, use Done status with closing information
		if issueData.State == "closed" {
			logger.Debug("Issue is closed but has reports, using Done status", "issue", ref.String())
			status := derive.Done

			// Use AI summarization for closing comment
			summary := summarizeWithFallback(ctx, summarizer, issueData.Title, ref.URL, issueData.CloseReason, "Issue was closed", logger)

			// Parse target date from newest report if available, otherwise use close date
			var targetDate *time.Time
			if len(reports) > 0 {
				targetDate = derive.ParseTargetDate(reports[0].TargetDate)
			}
			if targetDate == nil && issueData.ClosedAt != nil {
				targetDate = issueData.ClosedAt
			}

			row := format.NewRow(status, issueData.Title, ref.URL, targetDate, summary)
			return IssueResult{
				IssueURL: ref.URL,
				Row:      &row,
				Note:     nil, // No notes needed for closed issues
			}
		}

		// Issue is open - Reports exist but no update text - create row with NeedsUpdate status and add note
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

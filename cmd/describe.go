package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"

	"github.com/Attamusc/weekly-report-cli/internal/ai"
	"github.com/Attamusc/weekly-report-cli/internal/config"
	"github.com/Attamusc/weekly-report-cli/internal/format"
	"github.com/Attamusc/weekly-report-cli/internal/github"
	"github.com/Attamusc/weekly-report-cli/internal/input"
	githubapi "github.com/google/go-github/v66/github"
	"github.com/spf13/cobra"
)

var (
	// Describe-specific flags
	describeInputPath   string
	describeConcurrency int
	describeVerbose     bool
	describeQuiet       bool
	describePrompt      string
	describeFormat      string
	describeNoSummary   bool

	// Project board related flags (shared pattern with generate)
	describeProjectURL         string
	describeProjectField       string
	describeProjectFieldValues string
	describeProjectIncludePRs  bool
	describeProjectMaxItems    int
	describeProjectView        string
	describeProjectViewID      string
)

var describeCmd = &cobra.Command{
	Use:   "describe",
	Short: "Describe projects/goals from GitHub issues",
	Long: `Describe reads GitHub issues from multiple sources and generates project/goal summaries.

Input Sources:
  1. GitHub Project Board: Use --project flag with field-based filtering
  2. URL List: Provide issue URLs via stdin or --input file
  3. Mixed: Combine both project board and URL list

The tool fetches issues, analyzes the issue body/description, and outputs
summaries of project goals and objectives with optional AI-powered refinement.

Output Formats:
  - table (default): Markdown table with Initiative, Labels, Assignee, Summary
  - detailed: Full markdown sections with complete information for each issue

Examples:
  # From project board (using defaults: Status field, "In Progress,Done,Blocked")
  weekly-report-cli describe --project "org:my-org/5"

  # From project board with custom field/values
  weekly-report-cli describe \
    --project "org:my-org/5" \
    --project-field "Priority" \
    --project-field-values "High,Critical"

  # From project board using a view
  weekly-report-cli describe \
    --project "org:my-org/5" \
    --project-view "Current Sprint"

  # Detailed format output
  weekly-report-cli describe --project "org:my-org/5" --format detailed

  # From URL list (stdin)
  cat issues.txt | weekly-report-cli describe

  # Without AI summarization (raw body excerpt)
  weekly-report-cli describe --project "org:my-org/5" --no-summary

  # Mixed sources
  weekly-report-cli describe \
    --project "org:my-org/5" \
    --input additional-issues.txt`,
	RunE: runDescribe,
}

func init() {
	rootCmd.AddCommand(describeCmd)

	// Add flags
	describeCmd.Flags().StringVar(&describeInputPath, "input", "", "Input file path (default: stdin)")
	describeCmd.Flags().IntVar(&describeConcurrency, "concurrency", 4, "Number of concurrent workers")
	describeCmd.Flags().BoolVar(&describeVerbose, "verbose", false, "Enable verbose progress output")
	describeCmd.Flags().BoolVar(&describeQuiet, "quiet", false, "Suppress all progress output")
	describeCmd.Flags().StringVar(&describePrompt, "describe-prompt", "", "Custom prompt for AI description (uses default if empty)")
	describeCmd.Flags().StringVar(&describeFormat, "format", "table", "Output format: 'table' or 'detailed'")
	describeCmd.Flags().BoolVar(&describeNoSummary, "no-summary", false, "Disable AI summarization (output raw body excerpt)")

	// Project board flags
	describeCmd.Flags().StringVar(&describeProjectURL, "project", "", "GitHub project board URL or identifier (e.g., 'https://github.com/orgs/my-org/projects/5' or 'org:my-org/5')")
	describeCmd.Flags().StringVar(&describeProjectField, "project-field", "Status", "Field name to filter by (default: 'Status')")
	describeCmd.Flags().StringVar(&describeProjectFieldValues, "project-field-values", "In Progress,Done,Blocked", "Comma-separated values to match (default: 'In Progress,Done,Blocked')")
	describeCmd.Flags().BoolVar(&describeProjectIncludePRs, "project-include-prs", false, "Include pull requests from project board (default: issues only)")
	describeCmd.Flags().IntVar(&describeProjectMaxItems, "project-max-items", 100, "Maximum number of items to fetch from project board")
	describeCmd.Flags().StringVar(&describeProjectView, "project-view", "", "GitHub project view name (e.g., 'Blocked Items')")
	describeCmd.Flags().StringVar(&describeProjectViewID, "project-view-id", "", "GitHub project view ID (e.g., 'PVT_kwDOABCDEF') - takes precedence over --project-view")
}

func runDescribe(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Validate format flag
	if describeFormat != "table" && describeFormat != "detailed" {
		return fmt.Errorf("invalid format '%s': must be 'table' or 'detailed'", describeFormat)
	}

	// Parse project field values
	var projectFieldValuesList []string
	if describeProjectFieldValues != "" {
		projectFieldValuesList = input.ParseFieldValues(describeProjectFieldValues)
	}

	// Load configuration
	cfg, err := config.FromEnvAndFlags(
		0, // sinceDays not used for describe
		describeConcurrency,
		true, // noNotes - not used for describe
		describeVerbose,
		describeQuiet,
		describeInputPath,
		describePrompt,
		describeProjectURL,
		describeProjectField,
		projectFieldValuesList,
		describeProjectIncludePRs,
		describeProjectMaxItems,
		describeProjectView,
		describeProjectViewID,
		true, // noSentiment - not used for describe
	)
	if err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}

	// Override AI enabled based on --no-summary flag
	if describeNoSummary {
		cfg.Models.Enabled = false
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
		URLListPath:        describeInputPath,
		UseStdin:           describeInputPath == "" && cfg.Project.URL == "",
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

	// ========== PHASE A: Collect all issue data (parallel) ==========
	logger.Info("Collecting issue data...", "concurrency", cfg.Concurrency)
	dataResults := make(chan DescribeIssueDataResult, len(issueRefs))
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

			data, err := collectDescribeIssueData(ctx, githubClient, ref)

			// Update progress
			current := completed.Add(1)
			if !cfg.Quiet {
				logger.Info("Collecting issue data",
					"completed", int(current),
					"total", len(issueRefs))
			}

			dataResults <- DescribeIssueDataResult{Data: data, Err: err}
		}(ref)
	}

	// Close results channel when all goroutines finish
	go func() {
		wg.Wait()
		close(dataResults)
	}()

	// Collect all data
	var allData []DescribeIssueData
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

	// ========== PHASE B: Batch description (single API call) ==========
	var descriptions map[string]string
	if cfg.Models.Enabled {
		var err error
		descriptions, err = batchDescribe(ctx, summarizer, allData, logger)
		if err != nil {
			logger.Warn("Batch description failed, using fallbacks", "error", err)
			descriptions = make(map[string]string) // Empty map triggers fallback
		}
	} else {
		logger.Debug("AI description disabled, using fallbacks")
		descriptions = make(map[string]string)
	}

	// ========== PHASE C: Create final results ==========
	rows := assembleDescribeResults(allData, descriptions, logger)

	// Generate output
	renderDescribeOutput(rows, describeFormat, cfg, logger)

	return nil
}

// assembleDescribeResults creates describe rows from collected data and AI descriptions
func assembleDescribeResults(allData []DescribeIssueData, descriptions map[string]string, logger *slog.Logger) []format.DescribeRow {
	logger.Info("Creating final results...")
	var rows []format.DescribeRow

	for _, data := range allData {
		description := descriptions[data.IssueURL]
		if description == "" {
			description = data.FallbackDescription
		}

		row := format.DescribeRow{
			Title:     data.IssueTitle,
			URL:       data.IssueURL,
			Summary:   description,
			Labels:    data.Labels,
			Assignees: data.Assignees,
		}
		rows = append(rows, row)
		logger.Debug("Added describe row", "issue", data.IssueURL)
	}

	logger.Info("Results created successfully", "rows", len(rows))
	return rows
}

// renderDescribeOutput sorts, renders, and prints describe output
func renderDescribeOutput(rows []format.DescribeRow, outputFormat string, cfg *config.Config, logger *slog.Logger) {
	if len(rows) == 0 {
		if !cfg.Quiet {
			fmt.Fprintf(os.Stderr, "No describe rows generated\n")
		}
		os.Exit(2) // Exit code 2 for no rows produced
	}

	// Sort rows alphabetically by title
	format.SortDescribeRowsByTitle(rows)

	// Output based on format
	logger.Info("Rendering output...", "rows", len(rows), "format", outputFormat)
	var output string
	if outputFormat == "detailed" {
		output = format.RenderDescribeDetailed(rows)
	} else {
		output = format.RenderDescribeTable(rows)
	}
	fmt.Print(output)

	logger.Info("Describe completed successfully", "rows", len(rows))
}

// DescribeIssueData represents collected data from an issue for the describe command
type DescribeIssueData struct {
	IssueURL            string
	IssueTitle          string
	IssueBody           string
	Labels              []string
	Assignees           []string
	FallbackDescription string // Used if AI fails or is disabled
}

// DescribeIssueDataResult represents the result of collecting issue data
type DescribeIssueDataResult struct {
	Data DescribeIssueData
	Err  error
}

// collectDescribeIssueData fetches GitHub issue data for the describe command
func collectDescribeIssueData(ctx context.Context, client *githubapi.Client, ref input.IssueRef) (DescribeIssueData, error) {
	// Get logger from context
	logger, ok := ctx.Value(input.LoggerContextKey{}).(*slog.Logger)
	if !ok {
		logger = slog.Default()
	}

	logger.Debug("Collecting issue data for describe", "url", ref.URL)

	// Fetch issue data
	issueData, err := github.FetchIssue(ctx, client, ref)
	if err != nil {
		return DescribeIssueData{}, fmt.Errorf("failed to fetch issue: %w", err)
	}

	// Create fallback description (truncated body for table display)
	fallback := issueData.Body
	if len(fallback) > 500 {
		fallback = fallback[:500] + "..."
	}

	result := DescribeIssueData{
		IssueURL:            ref.URL,
		IssueTitle:          issueData.Title,
		IssueBody:           issueData.Body,
		Labels:              issueData.Labels,
		Assignees:           issueData.Assignees,
		FallbackDescription: fallback,
	}

	return result, nil
}

// batchDescribe generates descriptions for all collected issue data in a single API call
func batchDescribe(ctx context.Context, summarizer ai.Summarizer, allData []DescribeIssueData, logger *slog.Logger) (map[string]string, error) {
	// Collect items that need description
	var batchItems []ai.DescribeBatchItem
	for _, data := range allData {
		if data.IssueBody != "" {
			batchItems = append(batchItems, ai.DescribeBatchItem{
				IssueURL:   data.IssueURL,
				IssueTitle: data.IssueTitle,
				IssueBody:  data.IssueBody,
			})
		}
	}

	if len(batchItems) == 0 {
		logger.Debug("No items need description")
		return map[string]string{}, nil
	}

	logger.Info("Batch describing issues", "count", len(batchItems))
	descriptions, err := summarizer.DescribeBatch(ctx, batchItems)
	if err != nil {
		logger.Warn("Batch description failed", "error", err)
		return map[string]string{}, err
	}

	logger.Info("Batch description completed", "descriptions", len(descriptions))
	return descriptions, nil
}

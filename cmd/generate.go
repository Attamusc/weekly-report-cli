package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Attamusc/weekly-report-cli/internal/ai"
	"github.com/Attamusc/weekly-report-cli/internal/config"
	"github.com/Attamusc/weekly-report-cli/internal/format"
	"github.com/Attamusc/weekly-report-cli/internal/input"
	"github.com/Attamusc/weekly-report-cli/internal/pipeline"
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

	generateProjectFlags *projectFlags
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
	generateCmd.Flags().BoolVar(&noSentiment, "no-sentiment", false, "Disable AI sentiment analysis")
	generateCmd.Flags().BoolVar(&verbose, "verbose", false, "Enable verbose progress output")
	generateCmd.Flags().BoolVar(&quiet, "quiet", false, "Suppress all progress output")
	generateCmd.Flags().StringVar(&summaryPrompt, "summary-prompt", "", "Custom prompt for AI summarization (uses default if empty)")

	generateProjectFlags = addProjectFlags(generateCmd)
}

func runGenerate(cmd *cobra.Command, args []string) error {
	var projectFieldValuesList []string
	if generateProjectFlags.FieldValues != "" {
		projectFieldValuesList = input.ParseFieldValues(generateProjectFlags.FieldValues)
	}

	cfgInput := config.ConfigInput{
		SinceDays:          sinceDays,
		Concurrency:        concurrency,
		NoNotes:            noNotes,
		Verbose:            verbose,
		Quiet:              quiet,
		InputPath:          inputPath,
		SummaryPrompt:      summaryPrompt,
		ProjectURL:         generateProjectFlags.URL,
		ProjectField:       generateProjectFlags.Field,
		ProjectFieldValues: projectFieldValuesList,
		ProjectIncludePRs:  generateProjectFlags.IncludePRs,
		ProjectMaxItems:    generateProjectFlags.MaxItems,
		ProjectView:        generateProjectFlags.View,
		ProjectViewID:      generateProjectFlags.ViewID,
		NoSentiment:        noSentiment,
	}
	resolverCfg := input.ResolverConfig{
		ProjectURL:         generateProjectFlags.URL,
		ProjectFieldName:   generateProjectFlags.Field,
		ProjectFieldValues: projectFieldValuesList,
		ProjectIncludePRs:  generateProjectFlags.IncludePRs,
		ProjectMaxItems:    generateProjectFlags.MaxItems,
		ProjectView:        generateProjectFlags.View,
		ProjectViewID:      generateProjectFlags.ViewID,
		URLListPath:        inputPath,
		UseStdin:           inputPath == "" && generateProjectFlags.URL == "",
	}

	deps, err := setupCommand(cfgInput, resolverCfg)
	if err != nil {
		return err
	}
	ctx, cfg, logger, fetcher, summarizer, issueRefs := deps.Ctx, deps.Cfg, deps.Logger, deps.Fetcher, deps.Summarizer, deps.IssueRefs

	// Calculate time window
	since := time.Now().AddDate(0, 0, -cfg.SinceDays)
	logger.Debug("Looking for updates since", "since", since.Format("2006-01-02"))

	// ========== PHASE A: Collect all issue data (parallel) ==========
	logger.Info("Collecting issue data...", "concurrency", cfg.Concurrency)
	dataResults := make(chan pipeline.IssueDataResult, len(issueRefs))
	semaphore := make(chan struct{}, cfg.Concurrency)

	var completed atomic.Int32
	var wg sync.WaitGroup

	for _, ref := range issueRefs {
		wg.Add(1)
		go func(ref input.IssueRef) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			data, err := pipeline.CollectIssueData(ctx, fetcher, ref, since, cfg.SinceDays)

			current := completed.Add(1)
			if !cfg.Quiet {
				logger.Info("Collecting issue data",
					"completed", int(current),
					"total", len(issueRefs))
			}

			dataResults <- pipeline.IssueDataResult{Data: data, Err: err}
		}(ref)
	}

	go func() {
		wg.Wait()
		close(dataResults)
	}()

	var allData []pipeline.IssueData
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
		batchResults, err = pipeline.BatchSummarize(ctx, summarizer, allData, logger)
		if err != nil {
			logger.Warn("Batch summarization failed, using fallbacks", "error", err)
			batchResults = make(map[string]ai.BatchResult)
		}
	} else {
		logger.Debug("AI summarization disabled, using fallbacks")
		batchResults = make(map[string]ai.BatchResult)
	}

	// ========== PHASE C: Create final results ==========
	rows, notes := pipeline.AssembleGenerateResults(allData, batchResults, cfg.Models.Sentiment, logger)

	// Generate output
	return renderGenerateOutput(rows, notes, cfg, logger)
}

// renderGenerateOutput sorts, renders, and prints the report output
func renderGenerateOutput(rows []format.Row, notes []format.Note, cfg *config.Config, logger *slog.Logger) error {
	if len(rows) == 0 {
		if !cfg.Quiet {
			fmt.Fprintf(os.Stderr, "No report rows generated\n")
		}
		return config.ErrNoRows
	}

	format.SortRowsByTargetDate(rows)

	logger.Info("Rendering output...", "rows", len(rows))
	table := format.RenderTable(rows)
	fmt.Print(table)

	if cfg.Notes && len(notes) > 0 {
		logger.Debug("Adding notes section", "notes", len(notes))
		fmt.Print("\n")
		notesSection := format.RenderNotes(notes)
		fmt.Print(notesSection)
	}

	logger.Info("Report generated successfully", "rows", len(rows), "notes", len(notes))
	return nil
}

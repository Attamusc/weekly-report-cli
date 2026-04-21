package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"

	"github.com/Attamusc/weekly-report-cli/internal/ai"
	"github.com/Attamusc/weekly-report-cli/internal/config"
	"github.com/Attamusc/weekly-report-cli/internal/format"
	"github.com/Attamusc/weekly-report-cli/internal/input"
	"github.com/Attamusc/weekly-report-cli/internal/pipeline"
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

	describeProjectFlags *projectFlags
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

	describeProjectFlags = addProjectFlags(describeCmd)
}

func runDescribe(cmd *cobra.Command, args []string) error {
	// Validate format flag
	if describeFormat != "table" && describeFormat != "detailed" {
		return fmt.Errorf("invalid format '%s': must be 'table' or 'detailed'", describeFormat)
	}

	var projectFieldValuesList []string
	if describeProjectFlags.FieldValues != "" {
		projectFieldValuesList = input.ParseFieldValues(describeProjectFlags.FieldValues)
	}

	cfgInput := config.ConfigInput{
		SinceDays:          0,
		Concurrency:        describeConcurrency,
		NoNotes:            true,
		Verbose:            describeVerbose,
		Quiet:              describeQuiet,
		InputPath:          describeInputPath,
		SummaryPrompt:      describePrompt,
		ProjectURL:         describeProjectFlags.URL,
		ProjectField:       describeProjectFlags.Field,
		ProjectFieldValues: projectFieldValuesList,
		ProjectIncludePRs:  describeProjectFlags.IncludePRs,
		ProjectMaxItems:    describeProjectFlags.MaxItems,
		ProjectView:        describeProjectFlags.View,
		ProjectViewID:      describeProjectFlags.ViewID,
		NoSentiment:        true,
	}
	resolverCfg := input.ResolverConfig{
		ProjectURL:         describeProjectFlags.URL,
		ProjectFieldName:   describeProjectFlags.Field,
		ProjectFieldValues: projectFieldValuesList,
		ProjectIncludePRs:  describeProjectFlags.IncludePRs,
		ProjectMaxItems:    describeProjectFlags.MaxItems,
		ProjectView:        describeProjectFlags.View,
		ProjectViewID:      describeProjectFlags.ViewID,
		URLListPath:        describeInputPath,
		UseStdin:           describeInputPath == "" && describeProjectFlags.URL == "",
	}

	deps, err := setupCommand(cfgInput, resolverCfg)
	if err != nil {
		return err
	}

	// Override AI enabled based on --no-summary flag
	if describeNoSummary {
		deps.Cfg.Models.Enabled = false
		deps.Summarizer = ai.NewNoopSummarizer()
	}

	ctx, cfg, logger, fetcher, summarizer, issueRefs := deps.Ctx, deps.Cfg, deps.Logger, deps.Fetcher, deps.Summarizer, deps.IssueRefs

	// ========== PHASE A: Collect all issue data (parallel) ==========
	logger.Info("Collecting issue data...", "concurrency", cfg.Concurrency)
	dataResults := make(chan pipeline.DescribeIssueDataResult, len(issueRefs))
	semaphore := make(chan struct{}, cfg.Concurrency)

	var completed atomic.Int32
	var wg sync.WaitGroup

	for _, ref := range issueRefs {
		wg.Add(1)
		go func(ref input.IssueRef) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			data, err := pipeline.CollectDescribeIssueData(ctx, fetcher, ref)

			current := completed.Add(1)
			if !cfg.Quiet {
				logger.Info("Collecting issue data",
					"completed", int(current),
					"total", len(issueRefs))
			}

			dataResults <- pipeline.DescribeIssueDataResult{Data: data, Err: err}
		}(ref)
	}

	go func() {
		wg.Wait()
		close(dataResults)
	}()

	var allData []pipeline.DescribeIssueData
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
		descriptions, err = pipeline.BatchDescribe(ctx, summarizer, allData, logger)
		if err != nil {
			logger.Warn("Batch description failed, using fallbacks", "error", err)
			descriptions = make(map[string]string)
		}
	} else {
		logger.Debug("AI description disabled, using fallbacks")
		descriptions = make(map[string]string)
	}

	// ========== PHASE C: Create final results ==========
	rows := pipeline.AssembleDescribeResults(allData, descriptions, logger)

	// Generate output
	return renderDescribeOutput(rows, describeFormat, cfg, logger)
}

// renderDescribeOutput sorts, renders, and prints describe output
func renderDescribeOutput(rows []format.DescribeRow, outputFormat string, cfg *config.Config, logger *slog.Logger) error {
	if len(rows) == 0 {
		if !cfg.Quiet {
			fmt.Fprintf(os.Stderr, "No describe rows generated\n")
		}
		return config.ErrNoRows
	}

	format.SortDescribeRowsByTitle(rows)

	logger.Info("Rendering output...", "rows", len(rows), "format", outputFormat)
	var output string
	if outputFormat == "detailed" {
		output = format.RenderDescribeDetailed(rows)
	} else {
		output = format.RenderDescribeTable(rows)
	}
	fmt.Print(output)

	logger.Info("Describe completed successfully", "rows", len(rows))
	return nil
}

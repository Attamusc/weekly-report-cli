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

	"github.com/spf13/cobra"

	"github.com/Attamusc/weekly-report-cli/internal/ai"
	"github.com/Attamusc/weekly-report-cli/internal/config"
	"github.com/Attamusc/weekly-report-cli/internal/discovery"
	"github.com/Attamusc/weekly-report-cli/internal/format"
	"github.com/Attamusc/weekly-report-cli/internal/github"
	"github.com/Attamusc/weekly-report-cli/internal/input"
	"github.com/Attamusc/weekly-report-cli/internal/pipeline"
	"github.com/Attamusc/weekly-report-cli/internal/rollup"
)

var (
	highlightsUsers         string
	highlightsOrgs          string
	highlightsSinceDays     int
	highlightsConcurrency   int
	highlightsAIConcurrency int
	highlightsAITop         int
	highlightsVerbose       bool
	highlightsQuiet         bool
	highlightsPrompt        string
	highlightsNoSummary     bool
)

var highlightsCmd = &cobra.Command{
	Use:   "highlights",
	Short: "Surface notable smaller work from the past week",
	Long: `Highlights discovers issues and PRs touched by a set of GitHub users,
then scores and ranks them via a deterministic mechanical rollup. By default,
the top-scoring items are curated and summarized by AI, grouped by theme.

With --no-summary, the command prints the full mechanical rollup grouped by
author without making any AI calls — useful for quick scans or when AI is
unavailable.

Pipeline: discover → rollup → cut (top-N by score) → narrow hydrate →
  AI map (one call per survivor) → AI reduce (theme merge) → render.

Examples:
  # Basic usage (AI-curated highlights by theme)
  weekly-report-cli highlights --users "user1,user2,user3"

  # Scoped to specific orgs
  weekly-report-cli highlights --users "user1,user2" --orgs "my-org,other-org"

  # Custom time window
  weekly-report-cli highlights --users "user1,user2" --since-days 14

  # Mechanical rollup only (no AI), grouped by author
  weekly-report-cli highlights --users "user1,user2" --no-summary

  # Limit AI processing to top 20 items
  weekly-report-cli highlights --users "user1,user2" --ai-top 20

  # Increase AI concurrency for larger teams
  weekly-report-cli highlights --users "user1,user2" --ai-concurrency 10

  # With verbose logging
  weekly-report-cli highlights --users "user1,user2" --verbose`,
	RunE: runHighlights,
}

func init() {
	rootCmd.AddCommand(highlightsCmd)

	highlightsCmd.Flags().StringVar(&highlightsUsers, "users", "", "Comma-separated GitHub usernames (required)")
	highlightsCmd.Flags().StringVar(&highlightsOrgs, "orgs", "", "Comma-separated GitHub org names to scope results to")
	highlightsCmd.Flags().IntVar(&highlightsSinceDays, "since-days", 7, "Number of days to look back")
	highlightsCmd.Flags().IntVar(&highlightsConcurrency, "concurrency", 5, "Max concurrent API requests for hydration")
	highlightsCmd.Flags().IntVar(&highlightsAIConcurrency, "ai-concurrency", 5, "Max concurrent AI requests for the map step")
	highlightsCmd.Flags().IntVar(&highlightsAITop, "ai-top", 50, "Maximum number of top-scored items to send to AI")
	highlightsCmd.Flags().BoolVar(&highlightsVerbose, "verbose", false, "Enable verbose logging")
	highlightsCmd.Flags().BoolVar(&highlightsQuiet, "quiet", false, "Suppress progress output")
	highlightsCmd.Flags().StringVar(&highlightsPrompt, "summary-prompt", "", "Custom AI system prompt for highlights curation")
	highlightsCmd.Flags().BoolVar(&highlightsNoSummary, "no-summary", false, "Disable AI curation (print mechanical rollup by author)")
	_ = highlightsCmd.MarkFlagRequired("users")
}

func runHighlights(cmd *cobra.Command, args []string) error {
	// Validate flags.
	if highlightsAITop <= 0 {
		return fmt.Errorf("--ai-top must be > 0")
	}
	if highlightsAIConcurrency <= 0 {
		return fmt.Errorf("--ai-concurrency must be > 0")
	}

	users := parseUsers(highlightsUsers)
	if len(users) == 0 {
		return fmt.Errorf("no valid users provided")
	}
	orgs := parseUsers(highlightsOrgs)

	cfgInput := config.ConfigInput{
		SinceDays:   highlightsSinceDays,
		Concurrency: highlightsConcurrency,
		Verbose:     highlightsVerbose,
		Quiet:       highlightsQuiet,
		NoNotes:     true,
		NoSentiment: true,
	}

	ctx := context.Background()

	cfg, err := config.FromEnvAndFlags(cfgInput)
	if err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}

	logger := setupLogger(cfg)
	ctx = context.WithValue(ctx, input.LoggerContextKey{}, logger)

	var summarizer ai.Summarizer
	if highlightsNoSummary || !cfg.Models.Enabled {
		summarizer = ai.NewNoopSummarizer()
	} else {
		summarizer = ai.NewGHModelsClient(cfg.Models.BaseURL, cfg.Models.Model, cfg.GitHubToken, highlightsPrompt, cfg.Models.Timeout)
	}

	ghClient := github.New(ctx, cfg.GitHubToken)
	fetcher := &githubFetcher{client: ghClient}

	since := time.Now().AddDate(0, 0, -cfg.SinceDays)

	// ========== PHASE A: Discovery ==========
	logger.Info("Discovering activity", "users", len(users), "orgs", len(orgs), "since", since.Format("2006-01-02"))
	refs, err := discovery.Search(ctx, ghClient, users, orgs, since)
	if err != nil {
		return fmt.Errorf("discovery failed: %w", err)
	}
	if len(refs) == 0 {
		if !cfg.Quiet {
			fmt.Fprintln(os.Stderr, "No activity found for the specified users.")
		}
		os.Exit(2)
	}
	logger.Info("Items discovered", "count", len(refs))

	// ========== PHASE B: Rollup ==========
	logger.Info("Computing mechanical rollup", "items", len(refs))
	r := rollup.Compute(refs, since)
	logger.Info("Rollup complete", "median_score", fmt.Sprintf("%.3f", r.Median), "authors", len(r.ByAuthor))

	// ========== PHASE C: Cut ==========
	// If --no-summary or AI disabled, render the mechanical rollup and exit.
	if highlightsNoSummary || !cfg.Models.Enabled {
		output := format.RenderRollup(r)
		if output == "" {
			if !cfg.Quiet {
				fmt.Fprintln(os.Stderr, "No notable highlights found.")
			}
			os.Exit(2)
		}
		_, _ = fmt.Fprint(os.Stdout, output)
		return nil
	}

	survivors := r.Cut(highlightsAITop)
	logger.Info("Cut complete", "survivors", len(survivors), "ai_top", highlightsAITop)

	if len(survivors) == 0 {
		if !cfg.Quiet {
			fmt.Fprintln(os.Stderr, "No items survived the cut (all scores zero or below median).")
		}
		os.Exit(2)
	}

	// ========== PHASE D: Narrow hydrate ==========
	survivorRefs := make([]input.IssueRef, len(survivors))
	for i, s := range survivors {
		survivorRefs[i] = s.Ref
	}

	logger.Info("Hydrating survivors", "concurrency", cfg.Concurrency, "items", len(survivorRefs))
	hydratedData := collectHighlightDataParallel(ctx, fetcher, survivorRefs, cfg, since, logger)
	logger.Info("Hydration complete", "hydrated", len(hydratedData))

	// Build a lookup map from URL → HighlightData for the map step.
	dataByURL := make(map[string]pipeline.HighlightData, len(hydratedData))
	for _, d := range hydratedData {
		dataByURL[d.IssueURL] = d
	}

	// ========== PHASE E: AI map-reduce ==========
	logger.Info("Starting AI map step", "survivors", len(survivors), "ai_concurrency", highlightsAIConcurrency)

	highlights := aiMapHighlights(ctx, summarizer, survivors, dataByURL, highlightsAIConcurrency, cfg, logger)

	logger.Info("AI map complete", "highlights", len(highlights))

	logger.Info("Starting AI reduce step")
	merged, err := summarizer.MergeThemes(ctx, highlights)
	if err != nil {
		logger.Warn("MergeThemes failed, using pre-merge highlights", "error", err)
		merged = highlights
	} else {
		logger.Info("AI reduce complete")
	}

	// ========== PHASE F: Render ==========
	output := format.RenderHighlights(merged)
	if output == "" {
		if !cfg.Quiet {
			fmt.Fprintln(os.Stderr, "No notable highlights found.")
		}
		os.Exit(2)
	}
	_, _ = fmt.Fprint(os.Stdout, output)
	return nil
}

// aiMapHighlights runs one SummarizeHighlight call per survivor, bounded by
// aiConcurrency. Per-item errors are logged as warnings; the item falls back
// to a label-derived Highlight so partial results are always returned.
func aiMapHighlights(
	ctx context.Context,
	summarizer ai.Summarizer,
	survivors []rollup.ScoredItem,
	dataByURL map[string]pipeline.HighlightData,
	aiConcurrency int,
	cfg *config.Config,
	logger *slog.Logger,
) []ai.Highlight {
	type indexedResult struct {
		index     int
		highlight ai.Highlight
	}

	results := make(chan indexedResult, len(survivors))
	semaphore := make(chan struct{}, aiConcurrency)

	var completed atomic.Int32
	var wg sync.WaitGroup

	for i, s := range survivors {
		wg.Add(1)
		go func(idx int, scored rollup.ScoredItem) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			item := scoredItemToHighlightItem(scored, dataByURL)

			var h ai.Highlight
			if len(item.UpdateTexts) == 0 {
				// No hydrated comment bodies — skip AI entirely and emit a
				// deterministic highlight. The score formula can promote
				// items with zero comments (closed PRs, etc.) that are
				// real signal but give the AI nothing to summarize.
				logger.Info("Skipping AI for empty-context item", "url", item.IssueURL)
				h = fallbackHighlight(item)
			} else {
				start := time.Now()
				var err error
				h, err = summarizer.SummarizeHighlight(ctx, item)
				elapsed := time.Since(start)

				if cfg.Verbose {
					logger.Debug("SummarizeHighlight", "url", item.IssueURL, "latency_ms", elapsed.Milliseconds())
				}

				if err != nil {
					logger.Warn("SummarizeHighlight failed, using fallback", "url", item.IssueURL, "error", err)
					h = fallbackHighlight(item)
				}
			}

			current := completed.Add(1)
			if !cfg.Quiet {
				logger.Info("AI map progress", "completed", int(current), "total", len(survivors))
			}

			results <- indexedResult{index: idx, highlight: h}
		}(i, s)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect in original order.
	ordered := make([]ai.Highlight, len(survivors))
	for r := range results {
		ordered[r.index] = r.highlight
	}
	return ordered
}

// scoredItemToHighlightItem builds an ai.HighlightItem from a scored rollup
// item, enriching with hydrated data when available.
func scoredItemToHighlightItem(scored rollup.ScoredItem, dataByURL map[string]pipeline.HighlightData) ai.HighlightItem {
	ref := scored.Ref
	item := ai.HighlightItem{
		IssueURL:   ref.URL,
		IssueTitle: ref.Title,
		IssueState: ref.State,
		IsPR:       ref.IsPR,
		Labels:     scored.Labels,
	}
	if d, ok := dataByURL[ref.URL]; ok {
		item.UpdateTexts = d.UpdateTexts
		if item.IssueTitle == "" {
			item.IssueTitle = d.IssueTitle
		}
		if item.IssueState == "" {
			item.IssueState = d.IssueState
		}
		if len(item.Labels) == 0 {
			item.Labels = d.Labels
		}
	}
	return item
}

// defaultTheme is the fallback theme name when an item has no labels.
const defaultTheme = "General"

// fallbackHighlight builds a minimal Highlight from a HighlightItem when
// SummarizeHighlight returns an error. Mirrors NoopSummarizer.SummarizeHighlight.
func fallbackHighlight(item ai.HighlightItem) ai.Highlight {
	theme := defaultTheme
	if len(item.Labels) > 0 {
		theme = item.Labels[0]
	}
	return ai.Highlight{
		Theme:   theme,
		Title:   item.IssueTitle,
		URL:     item.IssueURL,
		Summary: item.IssueTitle,
	}
}

// parseUsers splits a comma-separated user string into trimmed, non-empty handles.
func parseUsers(raw string) []string {
	var users []string
	for _, u := range strings.Split(raw, ",") {
		u = strings.TrimSpace(u)
		if u != "" {
			users = append(users, u)
		}
	}
	return users
}

// collectHighlightDataParallel fetches lightweight highlight data in parallel.
func collectHighlightDataParallel(ctx context.Context, fetcher pipeline.IssueFetcher, refs []input.IssueRef, cfg *config.Config, since time.Time, logger *slog.Logger) []pipeline.HighlightData {
	dataResults := make(chan pipeline.HighlightDataResult, len(refs))
	semaphore := make(chan struct{}, cfg.Concurrency)

	var completed atomic.Int32
	var wg sync.WaitGroup

	for _, ref := range refs {
		wg.Add(1)
		go func(ref input.IssueRef) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			data, err := pipeline.CollectHighlightData(ctx, fetcher, ref, since)

			current := completed.Add(1)
			if !cfg.Quiet {
				logger.Info("Hydrated",
					"completed", int(current),
					"total", len(refs),
					"url", ref.URL)
			}

			dataResults <- pipeline.HighlightDataResult{Data: data, Err: err}
		}(ref)
	}

	go func() {
		wg.Wait()
		close(dataResults)
	}()

	var allData []pipeline.HighlightData
	for result := range dataResults {
		if result.Err != nil {
			logger.Debug("Error collecting highlight data", "error", result.Err)
			continue
		}
		allData = append(allData, result.Data)
	}

	return allData
}

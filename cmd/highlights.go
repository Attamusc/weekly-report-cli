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
)

var (
	highlightsUsers       string
	highlightsOrgs        string
	highlightsSinceDays   int
	highlightsConcurrency int
	highlightsVerbose     bool
	highlightsQuiet       bool
	highlightsPrompt      string
	highlightsNoSummary   bool
)

var highlightsCmd = &cobra.Command{
	Use:   "highlights",
	Short: "Surface notable smaller work from the past week",
	Long: `Highlights discovers issues and PRs touched by a set of GitHub users,
then uses AI to curate and summarize the most notable smaller work items
that don't appear on the main project board.

Output is grouped by theme (Bug Fixes, Support, Infrastructure, etc.)
and designed to complement the generate command's project board report.

Examples:
  # Basic usage
  weekly-report-cli highlights --users "user1,user2,user3"

  # Scoped to specific orgs
  weekly-report-cli highlights --users "user1,user2" --orgs "my-org,other-org"

  # Custom time window
  weekly-report-cli highlights --users "user1,user2" --since-days 14

  # Without AI curation (list all items)
  weekly-report-cli highlights --users "user1,user2" --no-summary

  # With verbose logging
  weekly-report-cli highlights --users "user1,user2" --verbose`,
	RunE: runHighlights,
}

func init() {
	rootCmd.AddCommand(highlightsCmd)

	highlightsCmd.Flags().StringVar(&highlightsUsers, "users", "", "Comma-separated GitHub usernames (required)")
	highlightsCmd.Flags().StringVar(&highlightsOrgs, "orgs", "", "Comma-separated GitHub org names to scope results to")
	highlightsCmd.Flags().IntVar(&highlightsSinceDays, "since-days", 7, "Number of days to look back")
	highlightsCmd.Flags().IntVar(&highlightsConcurrency, "concurrency", 5, "Max concurrent API requests")
	highlightsCmd.Flags().BoolVar(&highlightsVerbose, "verbose", false, "Enable verbose logging")
	highlightsCmd.Flags().BoolVar(&highlightsQuiet, "quiet", false, "Suppress progress output")
	highlightsCmd.Flags().StringVar(&highlightsPrompt, "summary-prompt", "", "Custom AI system prompt for highlights curation")
	highlightsCmd.Flags().BoolVar(&highlightsNoSummary, "no-summary", false, "Disable AI curation (list all items)")
	_ = highlightsCmd.MarkFlagRequired("users")
}

func runHighlights(cmd *cobra.Command, args []string) error {
	// Parse users
	users := parseUsers(highlightsUsers)
	if len(users) == 0 {
		return fmt.Errorf("no valid users provided")
	}

	// Parse orgs
	orgs := parseUsers(highlightsOrgs) // same comma-split logic

	// Build config
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

	// Override AI if --no-summary
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
		return nil
	}
	logger.Info("Items discovered", "count", len(refs))

	// ========== PHASE B: Collection (parallel hydration — lightweight) ==========
	logger.Info("Collecting issue data...", "concurrency", cfg.Concurrency, "items", len(refs))
	allData := collectHighlightDataParallel(ctx, fetcher, refs, cfg, since, logger)

	if len(allData) == 0 {
		if !cfg.Quiet {
			fmt.Fprintln(os.Stderr, "No issue data could be collected.")
		}
		return nil
	}
	logger.Info("Data collection complete", "items", len(allData))

	// ========== PHASE C: AI Curation ==========
	// TODO(TODO-8089d691): rewire to SummarizeHighlight+MergeThemes map-reduce pipeline.
	// The old HighlightsBatch has been deleted. Stub produces noop highlights so the
	// command compiles; the rewire todo will replace this block entirely.
	_ = summarizer // used by the rewire todo
	highlightItems := toHighlightItems(allData)
	var highlights []ai.Highlight
	noop := ai.NewNoopSummarizer()
	for _, item := range highlightItems {
		h, err := noop.SummarizeHighlight(ctx, item)
		if err != nil {
			continue
		}
		highlights = append(highlights, h)
	}

	// ========== PHASE D: Assembly ==========
	output := format.RenderHighlights(highlights)
	if output == "" {
		if !cfg.Quiet {
			fmt.Fprintln(os.Stderr, "No notable highlights found.")
		}
		return nil
	}
	_, _ = fmt.Fprint(os.Stdout, output)

	logger.Info("Highlights complete", "themes", countThemes(highlights))
	return nil
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
				logger.Info("Collected",
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

// toHighlightItems converts pipeline HighlightData to AI HighlightItems.
func toHighlightItems(data []pipeline.HighlightData) []ai.HighlightItem {
	items := make([]ai.HighlightItem, len(data))
	for i, d := range data {
		items[i] = ai.HighlightItem{
			IssueURL:    d.IssueURL,
			IssueTitle:  d.IssueTitle,
			IssueState:  d.IssueState,
			IsPR:        strings.Contains(d.IssueURL, "/pull/"),
			Labels:      d.Labels,
			UpdateTexts: d.UpdateTexts,
		}
	}
	return items
}

// countThemes counts the number of unique themes in highlights.
func countThemes(highlights []ai.Highlight) int {
	themes := make(map[string]bool)
	for _, h := range highlights {
		themes[h.Theme] = true
	}
	return len(themes)
}

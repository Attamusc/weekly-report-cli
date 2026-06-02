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
	"github.com/Attamusc/weekly-report-cli/internal/narrative"
	"github.com/Attamusc/weekly-report-cli/internal/pipeline"
	"github.com/Attamusc/weekly-report-cli/internal/rollup"
)

var (
	highlightsUsers       string
	highlightsOrgs        string
	highlightsSinceDays   int
	highlightsConcurrency int
	highlightsAITop       int
	highlightsVerbose     bool
	highlightsQuiet       bool
	highlightsPrompt      string
	highlightsNoSummary   bool
)

var highlightsCmd = &cobra.Command{
	Use:   "highlights",
	Short: "Surface notable smaller work from the past week",
	Long: `Highlights discovers issues and PRs touched by a set of GitHub users,
then scores and ranks them via a deterministic mechanical rollup. The top-
scoring items are hydrated with rich context (body, comments, timeline) and
sent in a single AI call that writes a narrative report grouped by theme.

With --no-summary, the command prints the full mechanical rollup grouped by
author without making any AI calls — pipe-friendly and instant.

Pipeline: discover → rollup → cut (top-N by score) → tier-2 hydrate →
  single WriteNarrative AI call → render narrative + rollup.

Design: the narrative gives the story; the full rollup beneath it gives the
detail. Nothing is hidden — scan narrative for context, rollup for specific
people and repos.

Examples:
  # Basic usage (AI narrative + full rollup)
  weekly-report-cli highlights --users "user1,user2,user3"

  # Scoped to specific orgs
  weekly-report-cli highlights --users "user1,user2" --orgs "my-org,other-org"

  # Custom time window
  weekly-report-cli highlights --users "user1,user2" --since-days 14

  # Mechanical rollup only (no AI), grouped by author
  weekly-report-cli highlights --users "user1,user2" --no-summary

  # Limit AI input to top 30 items
  weekly-report-cli highlights --users "user1,user2" --ai-top 30

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
	highlightsCmd.Flags().IntVar(&highlightsAITop, "ai-top", 100, "Maximum number of top-scored items to send to AI")
	highlightsCmd.Flags().BoolVar(&highlightsVerbose, "verbose", false, "Enable verbose logging")
	highlightsCmd.Flags().BoolVar(&highlightsQuiet, "quiet", false, "Suppress progress output")
	highlightsCmd.Flags().StringVar(&highlightsPrompt, "summary-prompt", "", "Custom AI system prompt for narrative curation")
	highlightsCmd.Flags().BoolVar(&highlightsNoSummary, "no-summary", false, "Disable AI curation (print mechanical rollup by author)")
	_ = highlightsCmd.MarkFlagRequired("users")
}

func runHighlights(cmd *cobra.Command, args []string) error {
	// Validate flags.
	if highlightsAITop <= 0 {
		return fmt.Errorf("--ai-top must be > 0")
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
		output := format.RenderRollup(r, nil, users, since)
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

	// ========== PHASE D: Tier-2 hydration ==========
	survivorRefs := make([]input.IssueRef, len(survivors))
	for i, s := range survivors {
		survivorRefs[i] = s.Ref
	}

	logger.Info("Hydrating survivors", "concurrency", cfg.Concurrency, "items", len(survivorRefs))
	hydratedItems := collectNarrativeItemsParallel(ctx, fetcher, survivorRefs, cfg, since, logger)
	logger.Info("Hydration complete", "hydrated", len(hydratedItems))

	// ========== PHASE E: AI narrative call ==========
	logger.Info("Starting narrative AI call", "items", len(hydratedItems))
	narr, err := summarizer.WriteNarrative(ctx, hydratedItems, r)
	if err != nil {
		logger.Warn("Narrative AI call failed; rendering rollup only", "error", err)
		narr = ai.Narrative{}
	}

	// ========== PHASE F: Render ==========
	hydratedMap := make(map[string]narrative.Item, len(hydratedItems))
	for _, item := range hydratedItems {
		hydratedMap[item.URL] = item
	}
	output := format.RenderNarrativeReport(narr, r, cfg.SinceDays, hydratedMap, users, since)
	if output == "" {
		if !cfg.Quiet {
			fmt.Fprintln(os.Stderr, "No notable highlights found.")
		}
		os.Exit(2)
	}
	_, _ = fmt.Fprint(os.Stdout, output)
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

// collectNarrativeItemsParallel fetches rich Tier-2 narrative.Item data in parallel.
func collectNarrativeItemsParallel(ctx context.Context, fetcher pipeline.IssueFetcher, refs []input.IssueRef, cfg *config.Config, since time.Time, logger *slog.Logger) []narrative.Item {
	type result struct {
		item narrative.Item
		err  error
	}
	resultCh := make(chan result, len(refs))
	semaphore := make(chan struct{}, cfg.Concurrency)

	var completed atomic.Int32
	var wg sync.WaitGroup

	for _, ref := range refs {
		wg.Add(1)
		go func(ref input.IssueRef) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			item, err := pipeline.CollectNarrativeItem(ctx, fetcher, ref, since)

			current := completed.Add(1)
			if !cfg.Quiet {
				logger.Info("Hydrated",
					"completed", int(current),
					"total", len(refs),
					"url", ref.URL)
			}

			resultCh <- result{item: item, err: err}
		}(ref)
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	var items []narrative.Item
	for res := range resultCh {
		if res.err != nil {
			logger.Debug("Error collecting narrative item", "error", res.err)
			continue
		}
		items = append(items, res.item)
	}

	return items
}

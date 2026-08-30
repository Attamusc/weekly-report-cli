package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/Attamusc/weekly-report-cli/internal/ai"
	"github.com/Attamusc/weekly-report-cli/internal/config"
	"github.com/Attamusc/weekly-report-cli/internal/github"
	"github.com/Attamusc/weekly-report-cli/internal/input"
	"github.com/Attamusc/weekly-report-cli/internal/pipeline"
	"github.com/Attamusc/weekly-report-cli/internal/projects"
	githubapi "github.com/google/go-github/v66/github"
)

// projectFlags holds project-related flag values shared across commands.
type projectFlags struct {
	URL         string
	Field       string
	FieldValues string
	IncludePRs  bool
	MaxItems    int
	View        string
	ViewID      string
}

// addProjectFlags registers project-related flags on a cobra command and returns
// the struct that will be populated when the command runs.
func addProjectFlags(cmd *cobra.Command) *projectFlags {
	pf := &projectFlags{}
	cmd.Flags().StringVar(&pf.URL, "project", "", "GitHub project board URL or identifier (e.g., 'https://github.com/orgs/my-org/projects/5' or 'org:my-org/5')")
	cmd.Flags().StringVar(&pf.Field, "project-field", "Status", "Field name to filter by (default: 'Status')")
	cmd.Flags().StringVar(&pf.FieldValues, "project-field-values", "In Progress,Done,Blocked", "Comma-separated values to match (default: 'In Progress,Done,Blocked')")
	cmd.Flags().BoolVar(&pf.IncludePRs, "project-include-prs", false, "Include pull requests from project board (default: issues only)")
	cmd.Flags().IntVar(&pf.MaxItems, "project-max-items", 100, "Maximum number of items to fetch from project board")
	cmd.Flags().StringVar(&pf.View, "project-view", "", "GitHub project view name (e.g., 'Blocked Items')")
	cmd.Flags().StringVar(&pf.ViewID, "project-view-id", "", "GitHub project view ID (e.g., 'PVT_kwDOABCDEF') - takes precedence over --project-view")
	return pf
}

// commandDeps holds initialized dependencies shared by generate and describe commands.
type commandDeps struct {
	Ctx        context.Context
	Cfg        *config.Config
	Logger     *slog.Logger
	Fetcher    pipeline.IssueFetcher
	Summarizer ai.Summarizer
	IssueRefs  []input.IssueRef
	Cleanup    func()
}

// setupCommand initializes shared dependencies from config input and resolver config.
// Returns config.ErrNoRows if no issue references are found.
func setupCommand(cfgInput config.ConfigInput, resolverCfg input.ResolverConfig) (*commandDeps, error) {
	ctx := context.Background()

	cfg, err := config.FromEnvAndFlags(cfgInput)
	if err != nil {
		return nil, fmt.Errorf("configuration error: %w", err)
	}

	logger := setupLogger(cfg)
	ctx = context.WithValue(ctx, input.LoggerContextKey{}, logger)

	var projectClient *projectClientAdapter
	if cfg.Project.URL != "" {
		logger.Debug("Initializing project client")
		projectClient = &projectClientAdapter{token: cfg.GitHubToken, logger: logger}
	}

	logger.Info("Resolving issue references...")
	issueRefs, err := input.ResolveIssueRefs(ctx, resolverCfg, projectClient)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve issue references: %w", err)
	}

	if len(issueRefs) == 0 {
		if !cfg.Quiet {
			fmt.Fprintf(os.Stderr, "No valid GitHub issue URLs found\n")
		}
		return nil, config.ErrNoRows
	}

	logger.Info("Found GitHub issues", "count", len(issueRefs))

	logger.Debug("Initializing GitHub client")
	fetcher := &githubFetcher{client: github.New(ctx, cfg.GitHubToken)}
	summarizer, cleanup := initSummarizer(ctx, cfg, logger)

	return &commandDeps{
		Ctx:        ctx,
		Cfg:        cfg,
		Logger:     logger,
		Fetcher:    fetcher,
		Summarizer: summarizer,
		IssueRefs:  issueRefs,
		Cleanup:    cleanup,
	}, nil
}

// projectClientAdapter adapts the projects.Client to the input.ProjectClient interface.
// This avoids circular dependencies between packages.
type projectClientAdapter struct {
	token  string
	logger *slog.Logger
}

// FetchProjectItems implements input.ProjectClient interface
func (a *projectClientAdapter) FetchProjectItems(ctx context.Context, resolverCfg input.ResolverConfig) ([]input.IssueRef, error) {
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
	client := projects.NewClient(a.token)
	projectItems, err := client.FetchProjectItems(ctx, projectCfg)
	if err != nil {
		return nil, err
	}

	// Extract issue refs from filtered items
	var issueRefs []input.IssueRef
	for _, item := range projectItems {
		if item.IssueRef != nil {
			ref := *item.IssueRef
			if len(item.FieldValues) > 0 {
				ref.FieldValues = make(map[string]string, len(item.FieldValues))
				for k, v := range item.FieldValues {
					ref.FieldValues[k] = v.String()
				}
			}
			issueRefs = append(issueRefs, ref)
		}
	}

	a.logger.Info("Project items fetched and filtered", "project", projectRef.String(), "items", len(issueRefs))

	return issueRefs, nil
}

// githubFetcher wraps a *github.Client to implement pipeline.IssueFetcher.
type githubFetcher struct {
	client *githubapi.Client
}

// FetchIssue implements pipeline.IssueFetcher.
func (f *githubFetcher) FetchIssue(ctx context.Context, ref input.IssueRef) (github.IssueData, error) {
	return github.FetchIssue(ctx, f.client, ref)
}

// FetchCommentsSince implements pipeline.IssueFetcher.
func (f *githubFetcher) FetchCommentsSince(ctx context.Context, ref input.IssueRef, since time.Time) ([]github.Comment, error) {
	return github.FetchCommentsSince(ctx, f.client, ref, since)
}

// initSummarizer creates and starts the configured AI summarizer.
func initSummarizer(ctx context.Context, cfg *config.Config, logger *slog.Logger) (ai.Summarizer, func()) {
	if !cfg.Copilot.Enabled {
		logger.Debug("AI summarization disabled")
		return ai.NewNoopSummarizer(), func() {}
	}

	logger.Debug("AI summarization enabled", "model", cfg.Copilot.Model)
	summarizer := ai.NewSDKCopilotSummarizer(
		cfg.Copilot.Token,
		cfg.Copilot.Model,
		cfg.Copilot.SystemPrompt,
		cfg.Copilot.Timeout,
		copilotChildEnv(os.Environ()),
	)
	startupCtx, cancel := context.WithTimeout(ctx, cfg.Copilot.Timeout)
	err := summarizer.Start(startupCtx)
	cancel()
	if err != nil {
		_ = summarizer.Cleanup()
		logger.Warn("Copilot unavailable; using non-AI fallback content")
		return ai.NewNoopSummarizer(), func() {}
	}

	var once sync.Once
	return summarizer, func() {
		once.Do(func() {
			if err := summarizer.Cleanup(); err != nil {
				logger.Warn("Failed to stop Copilot runtime", "error", err)
			}
		})
	}
}

func copilotChildEnv(environ []string) []string {
	childEnv := make([]string, 0, len(environ))
	for _, entry := range environ {
		name, _, _ := strings.Cut(entry, "=")
		switch name {
		case "COPILOT_GITHUB_TOKEN", "GH_TOKEN", "GITHUB_TOKEN":
			continue
		}
		childEnv = append(childEnv, entry)
	}
	return childEnv
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
	}))
}

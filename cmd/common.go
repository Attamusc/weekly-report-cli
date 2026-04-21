package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
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
	summarizer := initSummarizer(cfg, logger)

	return &commandDeps{
		Ctx:        ctx,
		Cfg:        cfg,
		Logger:     logger,
		Fetcher:    fetcher,
		Summarizer: summarizer,
		IssueRefs:  issueRefs,
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

// initSummarizer creates the appropriate AI summarizer based on configuration
func initSummarizer(cfg *config.Config, logger *slog.Logger) ai.Summarizer {
	if cfg.Models.Enabled {
		logger.Debug("AI summarization enabled", "model", cfg.Models.Model)
		return ai.NewGHModelsClient(cfg.Models.BaseURL, cfg.Models.Model, cfg.GitHubToken, cfg.Models.SystemPrompt, cfg.Models.Timeout)
	}
	logger.Debug("AI summarization disabled")
	return ai.NewNoopSummarizer()
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

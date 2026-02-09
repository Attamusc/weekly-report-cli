package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/Attamusc/weekly-report-cli/internal/ai"
	"github.com/Attamusc/weekly-report-cli/internal/config"
	"github.com/Attamusc/weekly-report-cli/internal/input"
	"github.com/Attamusc/weekly-report-cli/internal/projects"
)

// projectClientAdapter adapts the projects.Client to the input.ProjectClient interface.
// This avoids circular dependencies between packages.
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
	client := projects.NewClient(a.token)
	projectItems, err := client.FetchProjectItems(ctx, projectCfg)
	if err != nil {
		return nil, err
	}

	// Extract issue refs from filtered items
	var issueRefs []input.IssueRef
	for _, item := range projectItems {
		if item.IssueRef != nil {
			issueRefs = append(issueRefs, *item.IssueRef)
		}
	}

	a.logger.Info("Project items fetched and filtered", "project", projectRef.String(), "items", len(issueRefs))

	return issueRefs, nil
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

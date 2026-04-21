package pipeline

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Attamusc/weekly-report-cli/internal/ai"
	"github.com/Attamusc/weekly-report-cli/internal/format"
	"github.com/Attamusc/weekly-report-cli/internal/input"
)

// CollectDescribeIssueData fetches GitHub issue data for the describe command.
func CollectDescribeIssueData(ctx context.Context, fetcher IssueFetcher, ref input.IssueRef) (DescribeIssueData, error) {
	logger, ok := ctx.Value(input.LoggerContextKey{}).(*slog.Logger)
	if !ok {
		logger = slog.Default()
	}

	logger.Debug("Collecting issue data for describe", "url", ref.URL)

	issueData, err := fetcher.FetchIssue(ctx, ref)
	if err != nil {
		return DescribeIssueData{}, fmt.Errorf("failed to fetch issue: %w", err)
	}

	fallback := issueData.Body
	if len(fallback) > 500 {
		fallback = fallback[:500] + "..."
	}

	return DescribeIssueData{
		IssueURL:            ref.URL,
		IssueTitle:          issueData.Title,
		IssueBody:           issueData.Body,
		Labels:              issueData.Labels,
		Assignees:           issueData.Assignees,
		FallbackDescription: fallback,
	}, nil
}

// AssembleDescribeResults creates describe rows from collected data and AI descriptions.
func AssembleDescribeResults(allData []DescribeIssueData, descriptions map[string]string, logger *slog.Logger) []format.DescribeRow {
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

// BatchDescribe generates descriptions for all collected issue data in a single API call.
func BatchDescribe(ctx context.Context, summarizer ai.Summarizer, allData []DescribeIssueData, logger *slog.Logger) (map[string]string, error) {
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

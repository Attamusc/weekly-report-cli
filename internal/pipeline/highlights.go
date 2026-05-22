package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Attamusc/weekly-report-cli/internal/input"
)

// CollectHighlightData fetches lightweight data for the highlights command.
// Unlike CollectIssueData, this skips report extraction, close-reason fetching,
// and status derivation. It fetches only issue metadata and recent comments.
func CollectHighlightData(ctx context.Context, fetcher IssueFetcher, ref input.IssueRef, since time.Time) (HighlightData, error) {
	logger, ok := ctx.Value(input.LoggerContextKey{}).(*slog.Logger)
	if !ok {
		logger = slog.Default()
	}

	logger.Info("Fetching metadata", "issue", ref.String())

	issueData, err := fetcher.FetchIssue(ctx, ref)
	if err != nil {
		return HighlightData{}, fmt.Errorf("failed to fetch issue %s: %w", ref.URL, err)
	}

	logger.Info("Metadata fetched, fetching comments", "issue", ref.String(), "title", issueData.Title)

	comments, err := fetcher.FetchCommentsSince(ctx, ref, since)
	if err != nil {
		// Non-fatal: proceed with metadata only
		logger.Warn("Failed to fetch comments, proceeding without", "issue", ref.String(), "error", err)
	}

	var updateTexts []string
	for _, c := range comments {
		body := strings.TrimSpace(c.Body)
		if body != "" {
			updateTexts = append(updateTexts, body)
		}
	}

	logger.Info("Issue complete", "issue", ref.String(), "comments", len(updateTexts))

	return HighlightData{
		IssueURL:    ref.URL,
		IssueTitle:  issueData.Title,
		IssueState:  issueData.State,
		Labels:      issueData.Labels,
		UpdateTexts: updateTexts,
	}, nil
}

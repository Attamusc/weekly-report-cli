package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Attamusc/weekly-report-cli/internal/input"
	"github.com/Attamusc/weekly-report-cli/internal/narrative"
)

// truncate returns s trimmed to at most maxChars characters.
func truncate(s string, maxChars int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxChars {
		return s
	}
	return s[:maxChars]
}

// CollectNarrativeItem fetches rich Tier-2 data for one issue/PR survivor.
// Sequence: issue metadata → comments → timeline events → PR object (IsPR only).
// Truncation caps (narrative.MaxBodyChars, narrative.MaxCommentChars, etc.) are
// applied at collection time to bound token budget for the narrative AI call.
func CollectNarrativeItem(ctx context.Context, fetcher IssueFetcher, ref input.IssueRef, since time.Time) (narrative.Item, error) {
	logger, ok := ctx.Value(input.LoggerContextKey{}).(*slog.Logger)
	if !ok {
		logger = slog.Default()
	}

	logger.Info("Fetching metadata", "issue", ref.String())

	issueData, err := fetcher.FetchIssue(ctx, ref)
	if err != nil {
		return narrative.Item{}, fmt.Errorf("failed to fetch issue %s: %w", ref.URL, err)
	}

	logger.Info("Metadata fetched, fetching comments", "issue", ref.String(), "title", issueData.Title)

	rawComments, err := fetcher.FetchCommentsSince(ctx, ref, since)
	if err != nil {
		// Non-fatal: proceed with metadata only.
		logger.Warn("Failed to fetch comments, proceeding without", "issue", ref.String(), "error", err)
	}

	rawEvents, err := fetcher.FetchTimelineSince(ctx, ref, since)
	if err != nil {
		// Non-fatal: proceed without timeline.
		logger.Warn("Failed to fetch timeline, proceeding without", "issue", ref.String(), "error", err)
	}

	// Build attributed comments (chronological, capped at MaxComments most recent).
	var comments []narrative.Comment
	for _, c := range rawComments {
		body := truncate(c.Body, narrative.MaxCommentChars)
		if body == "" {
			continue
		}
		comments = append(comments, narrative.Comment{
			Author:    c.Author,
			CreatedAt: c.CreatedAt,
			Body:      body,
		})
	}
	if len(comments) > narrative.MaxComments {
		comments = comments[len(comments)-narrative.MaxComments:]
	}

	// Build narrative events (chronological, capped at MaxEvents most recent).
	var events []narrative.Event
	for _, e := range rawEvents {
		events = append(events, narrative.Event{
			Type:   e.Type,
			Actor:  e.Actor,
			At:     e.At,
			Detail: e.Detail,
		})
	}
	if len(events) > narrative.MaxEvents {
		events = events[len(events)-narrative.MaxEvents:]
	}

	item := narrative.Item{
		URL:            ref.URL,
		Title:          issueData.Title,
		Body:           truncate(issueData.Body, narrative.MaxBodyChars),
		State:          issueData.State,
		IsPR:           ref.IsPR,
		Author:         ref.AuthorLogin,
		Assignees:      issueData.Assignees,
		Labels:         issueData.Labels,
		OpenedAt:       issueData.CreatedAt,
		ClosedAt:       issueData.ClosedAt,
		RecentComments: comments,
		Events:         events,
	}

	if item.ClosedAt != nil {
		item.ClosedThisWeek = item.ClosedAt.After(since)
	}

	// For PRs, fetch the PR object to get accurate MergedAt.
	if ref.IsPR {
		prData, prErr := fetcher.FetchPullRequest(ctx, ref)
		if prErr != nil {
			logger.Warn("Failed to fetch PR data, MergedAt will be nil", "issue", ref.String(), "error", prErr)
		} else if prData != nil && prData.MergedAt != nil {
			item.MergedAt = prData.MergedAt
			item.MergedThisWeek = item.MergedAt.After(since)
		}
	}

	logger.Info("Issue complete", "issue", ref.String(), "comments", len(comments), "events", len(events))

	return item, nil
}

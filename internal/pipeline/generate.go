package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Attamusc/weekly-report-cli/internal/ai"
	"github.com/Attamusc/weekly-report-cli/internal/derive"
	"github.com/Attamusc/weekly-report-cli/internal/format"
	"github.com/Attamusc/weekly-report-cli/internal/github"
	"github.com/Attamusc/weekly-report-cli/internal/input"
	"github.com/Attamusc/weekly-report-cli/internal/report"
)

// IssueFetcher abstracts GitHub API access for issue data collection.
type IssueFetcher interface {
	FetchIssue(ctx context.Context, ref input.IssueRef) (github.IssueData, error)
	FetchCommentsSince(ctx context.Context, ref input.IssueRef, since time.Time) ([]github.Comment, error)
}

// CollectIssueData fetches GitHub data and extracts reports without AI summarization.
func CollectIssueData(ctx context.Context, fetcher IssueFetcher, ref input.IssueRef, since time.Time, sinceDays int) (IssueData, error) {
	logger, ok := ctx.Value(input.LoggerContextKey{}).(*slog.Logger)
	if !ok {
		logger = slog.Default()
	}

	logger.Debug("Collecting issue data", "url", ref.URL)

	issueData, err := fetcher.FetchIssue(ctx, ref)
	if err != nil {
		return IssueData{}, fmt.Errorf("failed to fetch issue: %w", err)
	}

	comments, err := fetcher.FetchCommentsSince(ctx, ref, since)
	if err != nil {
		return IssueData{}, fmt.Errorf("failed to fetch comments: %w", err)
	}

	reports := report.SelectReports(comments, since)

	result := IssueData{
		IssueURL:    ref.URL,
		IssueTitle:  issueData.Title,
		IssueState:  issueData.State,
		CreatedAt:   issueData.CreatedAt,
		ClosedAt:    issueData.ClosedAt,
		CloseReason: issueData.CloseReason,
		Labels:      issueData.Labels,
		Reports:     reports,
	}

	// Case 1: No structured reports found
	if len(reports) == 0 {
		semiReports := report.SelectSemiStructuredReports(comments, since)
		if len(semiReports) > 0 {
			reports = semiReports
			result.Reports = reports
			result.Note = &format.Note{
				Kind:     format.NoteSemiStructuredFallback,
				IssueURL: ref.URL,
			}
		}
	}

	if len(reports) == 0 {
		if issueData.State == github.StateClosed {
			result.Status = derive.Done
			result.ReportedStatusCaption = derive.Done.Caption
			result.TargetDate = issueData.ClosedAt
			result.ShouldSummarize = false
			result.FallbackSummary = SummaryCompleted
		} else if commentBody, ok := report.SelectMostRecentComment(comments); ok {
			result.Status = derive.Unknown
			result.ReportedStatusCaption = derive.Unknown.Caption
			result.UpdateTexts = []string{commentBody}
			result.ShouldSummarize = true
			result.FallbackSummary = commentBody
			result.Note = &format.Note{
				Kind:     format.NoteUnstructuredFallback,
				IssueURL: ref.URL,
			}
		} else {
			ApplyNoCommentFallback(&result, ref.URL, since, sinceDays,
				fmt.Sprintf("No update provided in last %d days", sinceDays))
		}

		ApplyLabelFallback(&result, ref.URL)
		return result, nil
	}

	// Case 2: Reports exist - collect update texts
	var updateTexts []string
	for _, rep := range reports {
		if rep.UpdateRaw != "" {
			updateTexts = append(updateTexts, rep.UpdateRaw)
		}
	}
	result.UpdateTexts = updateTexts

	newestReport := reports[0]
	result.Status = derive.MapTrending(newestReport.TrendingRaw)
	result.ReportedStatusCaption = result.Status.Caption
	result.TargetDate = derive.ParseTargetDate(newestReport.TargetDate)

	ApplyLabelFallback(&result, ref.URL)

	// Case 2a: Reports exist but no update text
	if len(updateTexts) == 0 {
		if issueData.State == github.StateClosed {
			result.Status = derive.Done
			result.ReportedStatusCaption = derive.Done.Caption
			if result.TargetDate == nil {
				result.TargetDate = issueData.ClosedAt
			}
			result.ShouldSummarize = false
			result.FallbackSummary = SummaryCompleted
		} else if commentBody, ok := report.SelectMostRecentComment(comments); ok {
			result.UpdateTexts = []string{commentBody}
			result.ShouldSummarize = true
			result.FallbackSummary = commentBody
			result.Note = &format.Note{
				Kind:     format.NoteUnstructuredFallback,
				IssueURL: ref.URL,
			}
		} else {
			ApplyNoCommentFallback(&result, ref.URL, since, sinceDays,
				fmt.Sprintf("No structured update found in last %d days", sinceDays))
		}
		return result, nil
	}

	// Case 2b: Reports with update text
	if result.Status == derive.Done || issueData.State == github.StateClosed {
		if issueData.State == github.StateClosed {
			result.Status = derive.Done
			result.ReportedStatusCaption = derive.Done.Caption
		}
		result.ShouldSummarize = false
		result.FallbackSummary = SummaryCompleted
	} else {
		result.ShouldSummarize = true
		result.FallbackSummary = updateTexts[0]
	}

	if len(reports) >= 2 {
		result.Note = &format.Note{
			Kind:      format.NoteMultipleUpdates,
			IssueURL:  ref.URL,
			SinceDays: sinceDays,
		}
	}

	return result, nil
}

// ApplyNoCommentFallback sets the result fields for an issue with no usable comments.
func ApplyNoCommentFallback(result *IssueData, issueURL string, since time.Time, sinceDays int, noUpdateMsg string) {
	if !result.CreatedAt.IsZero() && result.CreatedAt.After(since) {
		result.Status = derive.Shaping
		result.ReportedStatusCaption = derive.Shaping.Caption
		result.ShouldSummarize = false
		result.FallbackSummary = "New issue — still being shaped"
		result.Note = &format.Note{
			Kind:     format.NoteNewIssueShaping,
			IssueURL: issueURL,
		}
	} else {
		result.Status = derive.NeedsUpdate
		result.ReportedStatusCaption = derive.NeedsUpdate.Caption
		result.ShouldSummarize = false
		result.FallbackSummary = noUpdateMsg
		result.Note = &format.Note{
			Kind:      format.NoteNoUpdatesInWindow,
			IssueURL:  issueURL,
			SinceDays: sinceDays,
		}
	}
}

// ApplyLabelFallback checks whether the issue status is Unknown and attempts
// to derive a status from the issue labels.
func ApplyLabelFallback(result *IssueData, issueURL string) {
	if result.Status != derive.Unknown {
		return
	}
	labelStatus, ok := derive.MapLabelsToStatus(result.Labels)
	if !ok {
		return
	}
	result.Status = labelStatus
	result.ReportedStatusCaption = labelStatus.Caption
	if result.Note == nil {
		result.Note = &format.Note{
			Kind:     format.NoteLabelFallback,
			IssueURL: issueURL,
		}
	}
}

// AssembleGenerateResults creates rows and notes from collected data and batch AI results.
func AssembleGenerateResults(allData []IssueData, batchResults map[string]ai.BatchResult, sentiment bool, logger *slog.Logger) ([]format.Row, []format.Note) {
	logger.Info("Creating final results...")
	var rows []format.Row
	var notes []format.Note

	for _, data := range allData {
		var summary string

		if result, ok := batchResults[data.IssueURL]; ok {
			summary = result.Summary

			if sentiment && result.Sentiment != nil {
				suggestedStatus, valid := derive.ParseStatusKey(result.Sentiment.SuggestedStatus)
				if valid && suggestedStatus != data.Status {
					notes = append(notes, format.Note{
						Kind:            format.NoteSentimentMismatch,
						IssueURL:        data.IssueURL,
						ReportedStatus:  data.ReportedStatusCaption,
						SuggestedStatus: suggestedStatus.Caption,
						Explanation:     result.Sentiment.Explanation,
					})
				}
			}
		}

		if summary == "" {
			summary = data.FallbackSummary
		}

		result := CreateResultFromData(data, summary)
		if result.Row != nil {
			rows = append(rows, *result.Row)
			logger.Debug("Added report row", "issue", result.IssueURL)
		}
		if result.Note != nil {
			notes = append(notes, *result.Note)
			logger.Debug("Added note", "issue", result.IssueURL, "kind", result.Note.Kind)
		}
	}

	logger.Info("Results created successfully", "rows", len(rows), "notes", len(notes))
	return rows, notes
}

// CreateResultFromData creates an IssueResult from collected data and an AI summary.
func CreateResultFromData(data IssueData, summary string) IssueResult {
	if summary == "" {
		summary = data.FallbackSummary
	}

	row := format.NewRow(data.Status, data.IssueTitle, data.IssueURL, data.TargetDate, summary)
	return IssueResult{
		IssueURL: data.IssueURL,
		Row:      &row,
		Note:     data.Note,
		Err:      nil,
	}
}

// BatchSummarize summarizes all collected issue data in a single API call.
func BatchSummarize(ctx context.Context, summarizer ai.Summarizer, allData []IssueData, logger *slog.Logger) (map[string]ai.BatchResult, error) {
	var batchItems []ai.BatchItem
	for _, data := range allData {
		if data.ShouldSummarize && len(data.UpdateTexts) > 0 {
			batchItems = append(batchItems, ai.BatchItem{
				IssueURL:       data.IssueURL,
				IssueTitle:     data.IssueTitle,
				UpdateTexts:    data.UpdateTexts,
				ReportedStatus: data.ReportedStatusCaption,
			})
		}
	}

	if len(batchItems) == 0 {
		logger.Debug("No items need summarization")
		return map[string]ai.BatchResult{}, nil
	}

	logger.Info("Batch summarizing updates", "count", len(batchItems))
	summaries, err := summarizer.SummarizeBatch(ctx, batchItems)
	if err != nil {
		logger.Warn("Batch summarization failed", "error", err)
		return map[string]ai.BatchResult{}, err
	}

	logger.Info("Batch summarization completed", "summaries", len(summaries))
	return summaries, nil
}

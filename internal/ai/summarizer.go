package ai

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// SentimentResult holds the AI's assessment of whether the reported status
// matches the content of the updates.
type SentimentResult struct {
	SuggestedStatus string // Canonical status key: "on_track", "at_risk", "off_track", etc.
	Explanation     string // Brief AI explanation of the mismatch
}

// BatchResult holds both the summary and optional sentiment for a single issue.
type BatchResult struct {
	Summary   string
	Sentiment *SentimentResult // nil when sentiment is disabled or unavailable
}

// BatchItem represents a single item to summarize in a batch request
type BatchItem struct {
	IssueURL       string   // Unique identifier for matching response
	IssueTitle     string   // Issue title for context
	UpdateTexts    []string // One or more updates (newest first)
	ReportedStatus string   // The reporter's claimed status (e.g., "On Track", "Unknown")
}

// DescribeBatchItem represents a single item for project/goal description
type DescribeBatchItem struct {
	IssueURL   string // Unique identifier for matching response
	IssueTitle string // Issue title for context
	IssueBody  string // Issue body/description text
}

// HeaderItem represents a single row's data for executive summary header generation.
type HeaderItem struct {
	StatusCaption    string  // e.g., "On Track", "At Risk", "Done"
	StatusTransition *string // e.g., "At Risk→On Track" or nil if unchanged
	NewItem          bool    // true if not in previous report
	Title            string  // Initiative/epic title
	Summary          string  // The update summary text
}

// Highlight represents a single curated highlight from AI processing.
type Highlight struct {
	Theme   string // Category: "Bug Fixes", "Support & Reliability", "Infrastructure", etc.
	Title   string // Issue/PR title
	URL     string // Issue/PR URL
	Summary string // AI-written 1-line highlight
}

// HighlightItem represents input data for a single item to be considered for highlights.
type HighlightItem struct {
	IssueURL    string
	IssueTitle  string
	IssueState  string // "open", "closed"
	IsPR        bool   // true if pull request
	Labels      []string
	UpdateTexts []string // Comments/body text for context
}

// Summarizer provides AI-powered summarization of status report updates
type Summarizer interface {
	// Summarize generates a summary for a single update
	Summarize(ctx context.Context, issueTitle, issueURL, updateText string) (string, error)

	// SummarizeMany generates a summary for multiple updates (newest first)
	SummarizeMany(ctx context.Context, issueTitle, issueURL string, updates []string) (string, error)

	// SummarizeBatch generates summaries for multiple issues in a single request
	// Returns a map of issueURL -> BatchResult (summary + optional sentiment)
	SummarizeBatch(ctx context.Context, items []BatchItem) (map[string]BatchResult, error)

	// DescribeBatch generates project/goal summaries for issue descriptions
	// Returns a map of issueURL -> description summary
	DescribeBatch(ctx context.Context, items []DescribeBatchItem) (map[string]string, error)

	// GenerateHeader produces an executive summary paragraph from assembled report data.
	GenerateHeader(ctx context.Context, items []HeaderItem) (string, error)

	// HighlightsBatch curates notable items from a batch, assigns themes, and writes highlight summaries.
	// Returns only the items worth highlighting (AI filters noise).
	HighlightsBatch(ctx context.Context, items []HighlightItem) ([]Highlight, error)
}

// NoopSummarizer provides a fallback implementation that returns raw text without AI processing
type NoopSummarizer struct{}

// NewNoopSummarizer creates a new no-op summarizer
func NewNoopSummarizer() *NoopSummarizer {
	return &NoopSummarizer{}
}

// Summarize returns the trimmed raw update text
func (n *NoopSummarizer) Summarize(_ context.Context, _, _, updateText string) (string, error) {
	return strings.TrimSpace(updateText), nil
}

// SummarizeMany concatenates and returns the trimmed raw update texts
func (n *NoopSummarizer) SummarizeMany(_ context.Context, _, _ string, updates []string) (string, error) {
	return strings.TrimSpace(strings.Join(updates, " ")), nil
}

// SummarizeBatch returns raw update texts for each item
func (n *NoopSummarizer) SummarizeBatch(_ context.Context, items []BatchItem) (map[string]BatchResult, error) {
	result := make(map[string]BatchResult, len(items))
	for _, item := range items {
		var summary string
		if len(item.UpdateTexts) > 0 {
			summary = strings.TrimSpace(strings.Join(item.UpdateTexts, " "))
		}
		result[item.IssueURL] = BatchResult{Summary: summary, Sentiment: nil}
	}
	return result, nil
}

// GenerateHeader returns a simple stats string when AI is disabled.
func (n *NoopSummarizer) GenerateHeader(_ context.Context, items []HeaderItem) (string, error) {
	if len(items) == 0 {
		return "", nil
	}
	counts := make(map[string]int)
	var newCount, transitionCount int
	for _, item := range items {
		counts[item.StatusCaption]++
		if item.NewItem {
			newCount++
		}
		if item.StatusTransition != nil {
			transitionCount++
		}
	}
	var parts []string
	for status, count := range counts {
		parts = append(parts, fmt.Sprintf("%d %s", count, strings.ToLower(status)))
	}
	sort.Strings(parts)
	result := fmt.Sprintf("%d items: %s.", len(items), strings.Join(parts, ", "))
	if newCount > 0 {
		result += fmt.Sprintf(" %d new items.", newCount)
	}
	if transitionCount > 0 {
		result += fmt.Sprintf(" %d status changes.", transitionCount)
	}
	return result, nil
}

// HighlightsBatch returns all items with label-based themes when AI is disabled.
func (n *NoopSummarizer) HighlightsBatch(_ context.Context, items []HighlightItem) ([]Highlight, error) {
	var highlights []Highlight
	for _, item := range items {
		theme := "General"
		if len(item.Labels) > 0 {
			theme = item.Labels[0]
		}
		summary := item.IssueTitle
		if len(item.UpdateTexts) > 0 {
			text := strings.TrimSpace(item.UpdateTexts[0])
			if len(text) > 200 {
				text = text[:200] + "..."
			}
			if text != "" {
				summary = text
			}
		}
		highlights = append(highlights, Highlight{
			Theme:   theme,
			Title:   item.IssueTitle,
			URL:     item.IssueURL,
			Summary: summary,
		})
	}
	return highlights, nil
}

// DescribeBatch returns raw issue body text for each item (truncated for table display)
func (n *NoopSummarizer) DescribeBatch(_ context.Context, items []DescribeBatchItem) (map[string]string, error) {
	result := make(map[string]string, len(items))
	for _, item := range items {
		body := strings.TrimSpace(item.IssueBody)
		// Truncate to 500 characters for table display when AI is disabled
		if len(body) > 500 {
			body = body[:500] + "..."
		}
		result[item.IssueURL] = body
	}
	return result, nil
}

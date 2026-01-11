package ai

import (
	"context"
	"strings"
)

// BatchItem represents a single item to summarize in a batch request
type BatchItem struct {
	IssueURL    string   // Unique identifier for matching response
	IssueTitle  string   // Issue title for context
	UpdateTexts []string // One or more updates (newest first)
}

// DescribeBatchItem represents a single item for project/goal description
type DescribeBatchItem struct {
	IssueURL   string // Unique identifier for matching response
	IssueTitle string // Issue title for context
	IssueBody  string // Issue body/description text
}

// Summarizer provides AI-powered summarization of status report updates
type Summarizer interface {
	// Summarize generates a summary for a single update
	Summarize(ctx context.Context, issueTitle, issueURL, updateText string) (string, error)

	// SummarizeMany generates a summary for multiple updates (newest first)
	SummarizeMany(ctx context.Context, issueTitle, issueURL string, updates []string) (string, error)

	// SummarizeBatch generates summaries for multiple issues in a single request
	// Returns a map of issueURL -> summary
	SummarizeBatch(ctx context.Context, items []BatchItem) (map[string]string, error)

	// DescribeBatch generates project/goal summaries for issue descriptions
	// Returns a map of issueURL -> description summary
	DescribeBatch(ctx context.Context, items []DescribeBatchItem) (map[string]string, error)
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
func (n *NoopSummarizer) SummarizeBatch(_ context.Context, items []BatchItem) (map[string]string, error) {
	result := make(map[string]string, len(items))
	for _, item := range items {
		if len(item.UpdateTexts) > 0 {
			result[item.IssueURL] = strings.TrimSpace(strings.Join(item.UpdateTexts, " "))
		} else {
			result[item.IssueURL] = ""
		}
	}
	return result, nil
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

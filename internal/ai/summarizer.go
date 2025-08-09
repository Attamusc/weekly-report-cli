package ai

import (
	"context"
	"strings"
)

// Summarizer provides AI-powered summarization of status report updates
type Summarizer interface {
	// Summarize generates a summary for a single update
	Summarize(ctx context.Context, issueTitle, issueURL, updateText string) (string, error)
	
	// SummarizeMany generates a summary for multiple updates (newest first)
	SummarizeMany(ctx context.Context, issueTitle, issueURL string, updates []string) (string, error)
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
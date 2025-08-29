package cmd

import (
	"context"
	"log/slog"
	"testing"

	"github.com/Attamusc/weekly-report-cli/internal/ai"
)

func TestSummarizeWithFallback(t *testing.T) {
	ctx := context.Background()
	logger := slog.Default()

	tests := []struct {
		name         string
		summarizer   ai.Summarizer
		updateText   string
		fallbackText string
		expected     string
	}{
		{
			name:         "empty update text uses fallback",
			summarizer:   ai.NewNoopSummarizer(),
			updateText:   "",
			fallbackText: "Issue was closed",
			expected:     "Issue was closed",
		},
		{
			name:         "noop summarizer returns trimmed text",
			summarizer:   ai.NewNoopSummarizer(),
			updateText:   "  This issue is completed.  ",
			fallbackText: "Issue was closed",
			expected:     "This issue is completed.",
		},
		{
			name:         "whitespace-only update text uses fallback",
			summarizer:   ai.NewNoopSummarizer(),
			updateText:   "   ",
			fallbackText: "Issue was closed",
			expected:     "Issue was closed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := summarizeWithFallback(ctx, tt.summarizer, "Test Issue", "https://github.com/test/repo/issues/123", tt.updateText, tt.fallbackText, logger)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}
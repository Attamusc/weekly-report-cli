package ai

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Attamusc/weekly-report-cli/internal/input"
)

const (
	defaultSystemPrompt = `Refine the content in the engineering status updates to be one
	paragraph of roughly 3-5 sentences, present tense, third-person, markdown-ready, 
	no prefatory text. Attempt to not lose context when summarizing.

	When the source material contains links (GitHub issue/PR references, URLs, etc.),
	weave them naturally into the summary as inline markdown links with descriptive
	anchor text that reflects the content the link relates to. Do NOT group links in
	parentheses or use raw GitHub shorthand references as the link text.

	Bad:  "...experiments reveal more viable endpoints (github/repo#523, github/repo#524)."
	Good: "...experiments reveal [more viable endpoints to migrate](url) and [new discovery results](url)."`

	batchSystemPrompt = `You are summarizing multiple engineering status updates in a single batch.

You will receive a JSON object with an array of items, each containing:
- id: A unique identifier (the issue URL)
- issue: The issue title
- updates: One or more status updates (newest first)
- reported_status: The author's claimed status

For each item, produce:
1. A summary: 3-5 sentences, present tense, third-person, markdown-ready, no prefatory text.
   Do not lose context when summarizing.
   When the source material contains links (GitHub issue/PR references, URLs, etc.),
   weave them naturally into the summary as inline markdown links with descriptive
   anchor text that reflects the content the link relates to. Do NOT group links in
   parentheses or use raw GitHub shorthand references as the link text.
   Bad:  "...experiments reveal more viable endpoints (github/repo#523, github/repo#524)."
   Good: "...experiments reveal [more viable endpoints to migrate](url) and [new discovery results](url)."
2. A sentiment assessment: analyze whether the update content matches the reported status.
   - If the content describes blockers, delays, risks, or problems but the status is "On Track",
     suggest a more appropriate status.
   - If the content is positive but the status is "At Risk" or "Off Track", suggest a better status.
   - If the reported status is "Unknown", suggest an appropriate status based on the content.
   - If the status matches the content, set sentiment to null.

Respond ONLY with a valid JSON object. The keys are the issue URLs (from the "id" field).
Each value is an object with "summary" (string) and "sentiment" (object or null).

When sentiment is not null, it must have:
- "status": one of "on_track", "at_risk", "off_track", "not_started", "done"
- "explanation": one sentence explaining the mismatch

Example response:
{
  "https://github.com/org/repo/issues/1": {
    "summary": "The team completed the migration...",
    "sentiment": null
  },
  "https://github.com/org/repo/issues/2": {
    "summary": "Work on the API integration is ongoing...",
    "sentiment": {
      "status": "at_risk",
      "explanation": "Update mentions two unresolved blockers despite being reported as On Track."
    }
  }
}`

	describeSystemPrompt = `You are a technical writer summarizing GitHub issues for project documentation.

You will receive a JSON object with an array of items, each containing:
- id: A unique identifier (the issue URL)
- issue: The issue title
- body: The issue description/body text

For each issue, extract and summarize:
1. The main objective or goal of the project/feature
2. Key deliverables or scope items
3. Any important constraints or dependencies mentioned

Respond with ONLY a valid JSON object where:
- Keys are the item IDs (URLs) as strings
- Values are the summaries (2-4 sentences, factual, third-person present tense)

Focus on WHAT the project is about, not progress or status updates.
Keep relevant markdown links intact. Do not add any prefatory text, explanation, or markdown code fences.

Example response format:
{
  "https://github.com/org/repo/issues/1": "This project implements user authentication...",
  "https://github.com/org/repo/issues/2": "The initiative aims to refactor the payment processing module..."
}`

	headerSystemPrompt = `You are summarizing a weekly engineering status report. You will receive a JSON array of items, each with:
- status: current status (e.g., "On Track", "At Risk", "Done")
- transition: status change from previous week (e.g., "At Risk→On Track") or null
- new: whether this is a new item this week
- title: the initiative/epic name
- summary: the update text

Produce a single paragraph (2-3 sentences) highlighting the most notable changes this week.
Focus on: status transitions, new blockers, completed items, and overall trajectory.
Do NOT list every item. Be concise and executive-level.

Respond with ONLY the paragraph text, no formatting, no prefatory text.`

	maxBatchSize = 25
)

type batchRequestItem struct {
	ID             string   `json:"id"`
	Issue          string   `json:"issue"`
	Updates        []string `json:"updates"`
	ReportedStatus string   `json:"reported_status"`
}

type batchRequest struct {
	Items []batchRequestItem `json:"items"`
}

type sentimentResponseItem struct {
	Summary   string          `json:"summary"`
	Sentiment *sentimentMatch `json:"sentiment"`
}

type sentimentMatch struct {
	Status      string `json:"status"`
	Explanation string `json:"explanation"`
}

func convertNestedResponse(nested map[string]sentimentResponseItem) map[string]BatchResult {
	results := make(map[string]BatchResult, len(nested))
	for url, item := range nested {
		var sentiment *SentimentResult
		if item.Sentiment != nil {
			sentiment = &SentimentResult{SuggestedStatus: item.Sentiment.Status, Explanation: item.Sentiment.Explanation}
		}
		results[url] = BatchResult{Summary: item.Summary, Sentiment: sentiment}
	}
	return results
}

func getContextLogger(ctx context.Context) *slog.Logger {
	logger, ok := ctx.Value(input.LoggerContextKey{}).(*slog.Logger)
	if !ok {
		return slog.Default()
	}
	return logger
}

func chunkedBatch[I any, R any](ctx context.Context, items []I, logger *slog.Logger, actionName string, batchFn func(context.Context, []I) (map[string]R, error)) (map[string]R, error) {
	result := make(map[string]R)
	for i := 0; i < len(items); i += maxBatchSize {
		end := min(i+maxBatchSize, len(items))
		chunkNum := i/maxBatchSize + 1
		logger.Debug("Processing "+actionName+" chunk", "chunk", chunkNum, "items", end-i)
		chunkResults, err := batchFn(ctx, items[i:end])
		if err != nil {
			return nil, fmt.Errorf("%s chunk %d failed: %w", actionName, chunkNum, err)
		}
		for url, value := range chunkResults {
			result[url] = value
		}
	}
	return result, nil
}

type describeRequestItem struct {
	ID    string `json:"id"`
	Issue string `json:"issue"`
	Body  string `json:"body"`
}

type describeRequest struct {
	Items []describeRequestItem `json:"items"`
}

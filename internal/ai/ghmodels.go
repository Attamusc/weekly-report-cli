package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Attamusc/weekly-report-cli/internal/input"
	"github.com/Attamusc/weekly-report-cli/internal/narrative"
	"github.com/Attamusc/weekly-report-cli/internal/retry"
	"github.com/Attamusc/weekly-report-cli/internal/rollup"
)

// GHModelsClient implements Summarizer using GitHub Models API
type GHModelsClient struct {
	HTTP         *http.Client
	BaseURL      string
	Model        string
	Token        string
	SystemPrompt string
	// chatPath is the URL path suffix for the chat completions endpoint.
	// Computed from BaseURL at construction time.
	chatPath string
	// copilotIntegrationID, when non-empty, is sent as the Copilot-Integration-Id header.
	copilotIntegrationID string
}

// endpointProfileFor returns the chat path and Copilot integration ID for the given base URL.
// When baseURL contains "githubcopilot.com", the Copilot endpoint profile is used;
// otherwise, the GitHub Models endpoint profile is used.
func endpointProfileFor(baseURL string) (chatPath, integrationID string) {
	if strings.Contains(baseURL, "githubcopilot.com") {
		return "/chat/completions", "vscode-chat"
	}
	return "/inference/chat/completions", ""
}

// NewGHModelsClient creates a new GitHub Models API client
func NewGHModelsClient(baseURL, model, token, systemPrompt string, timeout time.Duration) *GHModelsClient {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	chatPath, integrationID := endpointProfileFor(baseURL)
	return &GHModelsClient{
		HTTP:                 &http.Client{Timeout: timeout},
		BaseURL:              baseURL,
		Model:                model,
		Token:                token,
		SystemPrompt:         systemPrompt,
		chatPath:             chatPath,
		copilotIntegrationID: integrationID,
	}
}

// chatCompletionRequest represents the OpenAI-compatible request format
type chatCompletionRequest struct {
	Model       string    `json:"model"`
	Messages    []message `json:"messages"`
	Temperature float64   `json:"temperature"`
}

// chatCompletionRequestJSON is like chatCompletionRequest but includes response_format.
type chatCompletionRequestJSON struct {
	Model          string            `json:"model"`
	Messages       []message         `json:"messages"`
	Temperature    float64           `json:"temperature"`
	ResponseFormat map[string]string `json:"response_format"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatCompletionResponse represents the OpenAI-compatible response format
type chatCompletionResponse struct {
	Choices []choice `json:"choices"`
}

type choice struct {
	Message message `json:"message"`
}

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

	temperature  = 1 // gpt-5o-mini only supports temperature of 1
	maxRetries   = 3
	baseDelay    = 1 * time.Second
	maxBatchSize = 25 // Maximum items per batch (count cap)
)

// getSystemPrompt returns the configured system prompt or the default if empty
func (c *GHModelsClient) getSystemPrompt() string {
	if c.SystemPrompt != "" {
		return c.SystemPrompt
	}
	return defaultSystemPrompt
}

// Summarize generates a summary for a single update using GitHub Models API
func (c *GHModelsClient) Summarize(ctx context.Context, issueTitle, issueURL, updateText string) (string, error) {
	// Get logger from context if available
	logger, ok := ctx.Value(input.LoggerContextKey{}).(*slog.Logger)
	if !ok {
		logger = slog.Default()
	}

	logger.Debug("AI summarizing single update", "model", c.Model, "issue", issueURL)
	userPrompt := fmt.Sprintf("Issue: %s (%s)\nUpdate:\n%s", issueTitle, issueURL, updateText)
	return c.callAPI(ctx, userPrompt, "")
}

// SummarizeMany generates a summary for multiple updates using GitHub Models API
func (c *GHModelsClient) SummarizeMany(ctx context.Context, issueTitle, issueURL string, updates []string) (string, error) {
	// Get logger from context if available
	logger, ok := ctx.Value(input.LoggerContextKey{}).(*slog.Logger)
	if !ok {
		logger = slog.Default()
	}

	logger.Debug("AI summarizing multiple updates", "model", c.Model, "issue", issueURL, "count", len(updates))
	userPrompt := fmt.Sprintf("Issue: %s (%s)\nUpdates (newest first):", issueTitle, issueURL)

	for i, update := range updates {
		userPrompt += fmt.Sprintf("\n%d) %s", i+1, update)
	}

	return c.callAPI(ctx, userPrompt, "")
}

// callAPI makes the actual HTTP request to GitHub Models API with retry logic
func (c *GHModelsClient) callAPI(ctx context.Context, userPrompt string, systemPromptOverride string) (string, error) {
	// Get logger from context if available
	logger, ok := ctx.Value(input.LoggerContextKey{}).(*slog.Logger)
	if !ok {
		logger = slog.Default()
	}

	request := chatCompletionRequest{
		Model:       c.Model,
		Temperature: temperature,
		Messages: []message{
			{Role: "system", Content: func() string {
				if systemPromptOverride != "" {
					return systemPromptOverride
				}
				return c.getSystemPrompt()
			}()},

			{Role: "user", Content: userPrompt},
		},
	}

	logger.Debug("Starting AI API request", "model", c.Model, "temperature", temperature, "maxRetries", maxRetries)

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Apply jittered exponential backoff
			backoff := retry.CalculateBackoff(attempt-1, int(baseDelay.Milliseconds()))
			logger.Debug("AI API retry backoff", "attempt", attempt, "delay", backoff)
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(backoff):
			}
		}

		logger.Debug("AI API request attempt", "attempt", attempt+1, "maxRetries", maxRetries)
		response, err := c.makeHTTPRequest(ctx, request)
		if err != nil {
			lastErr = err

			// Check if it's a rate limit error (429)
			if httpErr, ok := err.(*HTTPError); ok && httpErr.StatusCode == 429 {
				logger.Debug("AI API rate limited", "attempt", attempt+1, "statusCode", httpErr.StatusCode)
				// Extract retry-after header if present
				if retryAfter := httpErr.Headers.Get("Retry-After"); retryAfter != "" {
					if seconds, parseErr := strconv.Atoi(retryAfter); parseErr == nil {
						logger.Debug("AI API rate limit backoff", "retryAfter", seconds)
						select {
						case <-ctx.Done():
							return "", ctx.Err()
						case <-time.After(time.Duration(seconds) * time.Second):
						}
					}
				}
				continue // Retry on rate limit
			}

			logger.Debug("AI API request failed", "attempt", attempt+1, "error", err)
			// For other errors, return immediately
			return "", fmt.Errorf("GitHub Models API request failed: %w", err)
		}

		// Success - extract and return the response
		if len(response.Choices) == 0 {
			logger.Debug("AI API returned empty response")
			return "", fmt.Errorf("GitHub Models API returned empty response")
		}

		summary := response.Choices[0].Message.Content
		logger.Debug("AI API request succeeded", "attempt", attempt+1, "summaryLength", len(summary))
		return summary, nil
	}

	logger.Debug("AI API failed after all retries", "maxRetries", maxRetries, "lastError", lastErr)
	return "", fmt.Errorf("GitHub Models API failed after %d retries: %w", maxRetries, lastErr)
}

// makeHTTPRequest performs the actual HTTP request
func (c *GHModelsClient) makeHTTPRequest(ctx context.Context, request chatCompletionRequest) (*chatCompletionResponse, error) {
	return c.doHTTPRequest(ctx, request)
}

// makeHTTPRequestJSON performs the HTTP request for WriteNarrative (includes response_format).
func (c *GHModelsClient) makeHTTPRequestJSON(ctx context.Context, request chatCompletionRequestJSON) (*chatCompletionResponse, error) {
	return c.doHTTPRequest(ctx, request)
}

// doHTTPRequest marshals any request type and makes the POST call.
func (c *GHModelsClient) doHTTPRequest(ctx context.Context, request any) (*chatCompletionResponse, error) {
	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := c.BaseURL + c.chatPath
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("User-Agent", "weekly-report-cli/1.0")
	if c.copilotIntegrationID != "" {
		req.Header.Set("Copilot-Integration-Id", c.copilotIntegrationID)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPError{
			StatusCode: resp.StatusCode,
			Body:       string(body),
			Headers:    resp.Header,
		}
	}

	var response chatCompletionResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &response, nil
}

// HTTPError represents an HTTP error response
type HTTPError struct {
	StatusCode int
	Body       string
	Headers    http.Header
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Body)
}

// batchRequestItem represents a single item in a batch request
type batchRequestItem struct {
	ID             string   `json:"id"`
	Issue          string   `json:"issue"`
	Updates        []string `json:"updates"`
	ReportedStatus string   `json:"reported_status"`
}

// batchRequest represents the structure sent to the API for batch summarization
type batchRequest struct {
	Items []batchRequestItem `json:"items"`
}

// buildBatchPrompt creates a JSON prompt for batch summarization
func (c *GHModelsClient) buildBatchPrompt(items []BatchItem) (string, error) {
	batchReq := batchRequest{
		Items: make([]batchRequestItem, len(items)),
	}

	for i, item := range items {
		batchReq.Items[i] = batchRequestItem{
			ID:             item.IssueURL,
			Issue:          item.IssueTitle,
			Updates:        item.UpdateTexts,
			ReportedStatus: item.ReportedStatus,
		}
	}

	jsonBytes, err := json.Marshal(batchReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal batch request: %w", err)
	}

	return string(jsonBytes), nil
}

// sentimentResponseItem represents the new nested AI response format.
type sentimentResponseItem struct {
	Summary   string          `json:"summary"`
	Sentiment *sentimentMatch `json:"sentiment"`
}

// sentimentMatch represents the AI's sentiment assessment in the response JSON.
type sentimentMatch struct {
	Status      string `json:"status"`
	Explanation string `json:"explanation"`
}

// parseBatchResponse attempts to parse the API response as JSON
// Tries formats in order: nested (with sentiment), flat (legacy), markdown fallback
func (c *GHModelsClient) parseBatchResponse(response string, items []BatchItem) (map[string]BatchResult, error) {
	// Try new nested format first: {"url": {"summary": "...", "sentiment": {...}}}
	var nested map[string]sentimentResponseItem
	if err := json.Unmarshal([]byte(response), &nested); err == nil && len(nested) > 0 {
		// Verify at least one item has a "summary" field to distinguish
		// from the old flat format (which also unmarshals into this shape)
		for _, v := range nested {
			if v.Summary != "" {
				return convertNestedResponse(nested), nil
			}
			break
		}
	}

	// Fall back to old flat format: {"url": "summary text"}
	var flat map[string]string
	if err := json.Unmarshal([]byte(response), &flat); err == nil && len(flat) > 0 {
		results := make(map[string]BatchResult, len(flat))
		for url, summary := range flat {
			results[url] = BatchResult{Summary: summary, Sentiment: nil}
		}
		return results, nil
	}

	// Fall back to markdown parsing
	return c.parseMarkdownBatchResponse(response, items)
}

// convertNestedResponse converts the nested AI response to BatchResult map
func convertNestedResponse(nested map[string]sentimentResponseItem) map[string]BatchResult {
	results := make(map[string]BatchResult, len(nested))
	for url, item := range nested {
		var sentiment *SentimentResult
		if item.Sentiment != nil {
			sentiment = &SentimentResult{
				SuggestedStatus: item.Sentiment.Status,
				Explanation:     item.Sentiment.Explanation,
			}
		}
		results[url] = BatchResult{
			Summary:   item.Summary,
			Sentiment: sentiment,
		}
	}
	return results
}

// parseMarkdownBatchResponse parses a markdown-formatted batch response
// Expected format:
// ## SUMMARY <URL>
// <summary text>
//
// ## SUMMARY <URL>
// <summary text>
func (c *GHModelsClient) parseMarkdownBatchResponse(response string, items []BatchItem) (map[string]BatchResult, error) {
	result := make(map[string]BatchResult)

	// Split by "## SUMMARY" markers
	parts := strings.Split(response, "## SUMMARY")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Find the URL and extract the summary
		lines := strings.SplitN(part, "\n", 2)
		if len(lines) < 2 {
			continue
		}

		// First line should contain the URL
		urlLine := strings.TrimSpace(lines[0])
		summary := strings.TrimSpace(lines[1])

		// Try to find a matching URL from our items
		for _, item := range items {
			if strings.Contains(urlLine, item.IssueURL) {
				result[item.IssueURL] = BatchResult{Summary: summary, Sentiment: nil}
				break
			}
		}
	}

	// If we didn't parse any summaries, return error
	if len(result) == 0 {
		return nil, fmt.Errorf("failed to parse batch response in both JSON and markdown formats")
	}

	return result, nil
}

// batchConfig holds the parameters for a generic batch API call.
type batchConfig struct {
	systemPrompt string // System prompt to use during the API call
	actionName   string // "summarize" or "describe" for log messages
}

// executeBatchCall handles the common batch orchestration: swap system prompt,
// call the API, and return the raw response string. The caller is responsible
// for building the user prompt and parsing the response.
func (c *GHModelsClient) executeBatchCall(ctx context.Context, cfg batchConfig, userPrompt string) (string, error) {
	// Call API with the batch-specific system prompt
	response, err := c.callAPI(ctx, userPrompt, cfg.systemPrompt)
	if err != nil {
		return "", fmt.Errorf("%s API call failed: %w", cfg.actionName, err)
	}

	return response, nil
}

// getContextLogger retrieves the logger from context, falling back to slog.Default.
func getContextLogger(ctx context.Context) *slog.Logger {
	logger, ok := ctx.Value(input.LoggerContextKey{}).(*slog.Logger)
	if !ok {
		return slog.Default()
	}
	return logger
}

// runBatch is the generic batch orchestration: check empty, chunk if needed,
// build prompt, call API, parse response. Type parameters allow it to work
// with both BatchItem/BatchResult and DescribeBatchItem/string.
func runBatch[I any, R any](
	ctx context.Context,
	c *GHModelsClient,
	items []I,
	cfg batchConfig,
	buildPrompt func([]I) (string, error),
	parseResponse func(string) (map[string]R, error),
	selfFn func(context.Context, []I) (map[string]R, error),
) (map[string]R, error) {
	logger := getContextLogger(ctx)

	if len(items) == 0 {
		return make(map[string]R), nil
	}

	// If we have more than maxBatchSize items, chunk them
	if len(items) > maxBatchSize {
		logger.Debug("Splitting "+cfg.actionName+" batch into chunks", "totalItems", len(items), "chunkSize", maxBatchSize)
		return chunkedBatch(ctx, items, logger, cfg.actionName, selfFn)
	}

	logger.Debug("AI "+cfg.actionName+" batch", "model", c.Model, "items", len(items))

	// Build the prompt
	userPrompt, err := buildPrompt(items)
	if err != nil {
		return nil, fmt.Errorf("failed to build %s prompt: %w", cfg.actionName, err)
	}

	response, err := c.executeBatchCall(ctx, cfg, userPrompt)
	if err != nil {
		return nil, err
	}

	// Parse response
	results, err := parseResponse(response)
	if err != nil {
		logger.Debug("Failed to parse "+cfg.actionName+" response", "error", err)
		return nil, err
	}

	logger.Debug("Batch "+cfg.actionName+" succeeded", "results", len(results))
	return results, nil
}

// SummarizeBatch generates summaries for multiple issues in a single request
// Implements chunking to avoid token limits
func (c *GHModelsClient) SummarizeBatch(ctx context.Context, items []BatchItem) (map[string]BatchResult, error) {
	cfg := batchConfig{systemPrompt: batchSystemPrompt, actionName: "summarize"}
	return runBatch(ctx, c, items, cfg,
		c.buildBatchPrompt,
		func(resp string) (map[string]BatchResult, error) {
			return c.parseBatchResponse(resp, items)
		},
		c.SummarizeBatch,
	)
}

// chunkedBatch splits items into chunks and processes them sequentially.
// It works with any item and result types by accepting a batch function.
func chunkedBatch[I any, R any](ctx context.Context, items []I, logger *slog.Logger, actionName string, batchFn func(context.Context, []I) (map[string]R, error)) (map[string]R, error) {
	result := make(map[string]R)

	for i := 0; i < len(items); i += maxBatchSize {
		end := i + maxBatchSize
		if end > len(items) {
			end = len(items)
		}

		chunk := items[i:end]
		chunkNum := i/maxBatchSize + 1
		logger.Debug("Processing "+actionName+" chunk", "chunk", chunkNum, "items", len(chunk))

		chunkResults, err := batchFn(ctx, chunk)
		if err != nil {
			return nil, fmt.Errorf("%s chunk %d failed: %w", actionName, chunkNum, err)
		}

		// Merge results
		for url, val := range chunkResults {
			result[url] = val
		}
	}

	return result, nil
}

// describeRequestItem represents a single item in a describe batch request
type describeRequestItem struct {
	ID    string `json:"id"`
	Issue string `json:"issue"`
	Body  string `json:"body"`
}

// describeRequest represents the structure sent to the API for batch description
type describeRequest struct {
	Items []describeRequestItem `json:"items"`
}

// buildDescribePrompt creates a JSON prompt for batch description
func (c *GHModelsClient) buildDescribePrompt(items []DescribeBatchItem) (string, error) {
	req := describeRequest{
		Items: make([]describeRequestItem, len(items)),
	}

	for i, item := range items {
		req.Items[i] = describeRequestItem{
			ID:    item.IssueURL,
			Issue: item.IssueTitle,
			Body:  item.IssueBody,
		}
	}

	jsonBytes, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("failed to marshal describe request: %w", err)
	}

	return string(jsonBytes), nil
}

// DescribeBatch generates project/goal summaries for issue descriptions
// Implements chunking to avoid token limits
func (c *GHModelsClient) DescribeBatch(ctx context.Context, items []DescribeBatchItem) (map[string]string, error) {
	cfg := batchConfig{systemPrompt: describeSystemPrompt, actionName: "describe"}
	return runBatch(ctx, c, items, cfg,
		c.buildDescribePrompt,
		func(resp string) (map[string]string, error) {
			return c.parseDescribeResponse(resp, items)
		},
		c.DescribeBatch,
	)
}

// parseDescribeResponse attempts to parse the API response as JSON
// Returns a map of issueURL -> description
func (c *GHModelsClient) parseDescribeResponse(response string, items []DescribeBatchItem) (map[string]string, error) {
	// Try to parse as JSON first
	var jsonResponse map[string]string
	if err := json.Unmarshal([]byte(response), &jsonResponse); err == nil {
		// Successfully parsed JSON
		return jsonResponse, nil
	}

	// JSON parsing failed - try markdown fallback
	return c.parseMarkdownDescribeResponse(response, items)
}

// parseMarkdownDescribeResponse parses a markdown-formatted describe response
func (c *GHModelsClient) parseMarkdownDescribeResponse(response string, items []DescribeBatchItem) (map[string]string, error) {
	result := make(map[string]string)

	// Split by "## SUMMARY" or "## DESCRIPTION" markers
	parts := strings.Split(response, "## ")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Find the URL and extract the description
		lines := strings.SplitN(part, "\n", 2)
		if len(lines) < 2 {
			continue
		}

		// First line should contain the URL or marker
		urlLine := strings.TrimSpace(lines[0])
		description := strings.TrimSpace(lines[1])

		// Try to find a matching URL from our items
		for _, item := range items {
			if strings.Contains(urlLine, item.IssueURL) {
				result[item.IssueURL] = description
				break
			}
		}
	}

	// If we didn't parse any descriptions, return error
	if len(result) == 0 {
		return nil, fmt.Errorf("failed to parse describe response in both JSON and markdown formats")
	}

	return result, nil
}

// GenerateHeader produces an executive summary paragraph from assembled report data.
func (c *GHModelsClient) GenerateHeader(ctx context.Context, items []HeaderItem) (string, error) {
	logger := getContextLogger(ctx)
	if len(items) == 0 {
		return "", nil
	}
	logger.Debug("Generating executive summary header", "items", len(items))

	type jsonItem struct {
		Status     string  `json:"status"`
		Transition *string `json:"transition"`
		New        bool    `json:"new"`
		Title      string  `json:"title"`
		Summary    string  `json:"summary"`
	}
	jsonItems := make([]jsonItem, len(items))
	for i, item := range items {
		jsonItems[i] = jsonItem{
			Status:     item.StatusCaption,
			Transition: item.StatusTransition,
			New:        item.NewItem,
			Title:      item.Title,
			Summary:    item.Summary,
		}
	}
	jsonBytes, err := json.Marshal(jsonItems)
	if err != nil {
		return "", fmt.Errorf("failed to marshal header items: %w", err)
	}
	return c.callAPI(ctx, string(jsonBytes), headerSystemPrompt)
}

// ── WriteNarrative ───────────────────────────────────────────────────────────

// narrativeInputItem is the compact JSON shape sent to the AI for each NarrativeItem.
type narrativeInputItem struct {
	URL            string                  `json:"url"`
	Title          string                  `json:"title"`
	Body           string                  `json:"body"`
	State          string                  `json:"state"`
	IsPR           bool                    `json:"isPR"`
	Author         string                  `json:"author"`
	Assignees      []string                `json:"assignees"`
	Labels         []string                `json:"labels"`
	OpenedAt       time.Time               `json:"openedAt"`
	ClosedAt       *time.Time              `json:"closedAt,omitempty"`
	MergedAt       *time.Time              `json:"mergedAt,omitempty"`
	ClosedThisWeek bool                    `json:"closedThisWeek"`
	MergedThisWeek bool                    `json:"mergedThisWeek"`
	RecentComments []narrativeInputComment `json:"recentComments"`
	Events         []narrativeInputEvent   `json:"events"`
}

type narrativeInputComment struct {
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"createdAt"`
	Body      string    `json:"body"`
}

type narrativeInputEvent struct {
	Type   string    `json:"type"`
	Actor  string    `json:"actor"`
	At     time.Time `json:"at"`
	Detail string    `json:"detail,omitempty"`
}

// narrativeResponse is the expected JSON shape from the AI.
type narrativeResponse struct {
	Sections []narrativeResponseSection `json:"sections"`
}

type narrativeResponseSection struct {
	Heading string `json:"heading"`
	Body    string `json:"body"`
}

// citationRegex matches inline markdown links with github.com URLs.
var citationRegex = regexp.MustCompile(`\[[^\]]+\]\((https://github\.com/[^)]+)\)`)

const narrativeSystemPrompt = `You are writing a weekly engineering narrative report.

You will receive a JSON array of issues and pull requests from the past week.
Each item includes: url, title, body, state, isPR, author, assignees, labels,
openedAt, closedAt, mergedAt, closedThisWeek, mergedThisWeek, recentComments,
and events.

Write 3–5 thematic sections covering the most significant work.

Section requirements:
- Each section starts with ### <heading> then 1–2 paragraphs of prose (~80–150 words).
- Use inline [text](url) citations linking to specific items.
- Cover BOTH major initiatives AND notable day-to-day work.
- Mention people by @username when their specific work warrants it.
- SKIP routine work: entitlement adds, ownership cleanups, label-only changes, dependency bumps.
- Be specific. Avoid generic phrases: "addresses", "ensures", "enhances",
  "improves system reliability", "improves overall", "general improvements".
- Total output: ≤ 600 words across all sections.

Respond with ONLY a JSON object (no markdown fences):
{"sections": [{"heading": "...", "body": "..."}]}`

// rollupSummary builds a brief textual summary of rollup stats for use in the
// narrative system prompt. It is intentionally lightweight: no full item data.
func rollupSummary(r rollup.Rollup) string {
	totalItems := len(r.AllSorted)
	var dateRange string
	if totalItems > 0 {
		first := r.AllSorted[len(r.AllSorted)-1].Ref.UpdatedAt
		last := r.AllSorted[0].Ref.UpdatedAt
		dateRange = fmt.Sprintf("%s to %s", first.Format("2006-01-02"), last.Format("2006-01-02"))
	}
	type ac struct {
		name string
		n    int
	}
	var authors []ac
	for a, items := range r.ByAuthor {
		authors = append(authors, ac{a, len(items)})
	}
	for i := 0; i < len(authors)-1; i++ {
		for j := i + 1; j < len(authors); j++ {
			if authors[j].n > authors[i].n {
				authors[i], authors[j] = authors[j], authors[i]
			}
		}
	}
	var top []string
	for i, a := range authors {
		if i >= 5 {
			break
		}
		top = append(top, fmt.Sprintf("@%s (%d items)", a.name, a.n))
	}
	return fmt.Sprintf("Total rollup items: %d. Date range: %s. Top contributors: %s.",
		totalItems, dateRange, strings.Join(top, ", "))
}

func (c *GHModelsClient) callJSONAPI(ctx context.Context, request chatCompletionRequestJSON) (string, error) {
	var rawResponse string
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoff := retry.CalculateBackoff(attempt-1, int(baseDelay.Milliseconds()))
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(backoff):
			}
		}
		resp, err := c.makeHTTPRequestJSON(ctx, request)
		if err != nil {
			lastErr = err
			if httpErr, ok := err.(*HTTPError); ok && httpErr.StatusCode == 429 {
				if retryAfter := httpErr.Headers.Get("Retry-After"); retryAfter != "" {
					if seconds, parseErr := strconv.Atoi(retryAfter); parseErr == nil {
						select {
						case <-ctx.Done():
							return "", ctx.Err()
						case <-time.After(time.Duration(seconds) * time.Second):
						}
					}
				}
				continue
			}
			return "", fmt.Errorf("WriteNarrative API request failed: %w", err)
		}
		if len(resp.Choices) == 0 {
			return "", fmt.Errorf("WriteNarrative: empty response")
		}
		rawResponse = resp.Choices[0].Message.Content
		break
	}
	if rawResponse == "" && lastErr != nil {
		return "", fmt.Errorf("WriteNarrative failed after %d retries: %w", maxRetries, lastErr)
	}
	return rawResponse, nil
}

// WriteNarrative produces a structured narrative report from rich NarrativeItem data.
// Thin items (no body, no comments, no events) are excluded from the AI prompt.
// On AI error or unparseable response, returns Narrative{} and an error.
func (c *GHModelsClient) WriteNarrative(ctx context.Context, items []narrative.Item, r rollup.Rollup) (Narrative, error) {
	logger := getContextLogger(ctx)

	// ── Thin-input filter (premortem #7) ──
	var filtered []narrative.Item
	for _, it := range items {
		if strings.TrimSpace(it.Body) == "" && len(it.RecentComments) == 0 && len(it.Events) == 0 {
			logger.Debug("Excluding thin-input item from narrative", "url", it.URL)
			continue
		}
		filtered = append(filtered, it)
	}
	if len(filtered) == 0 {
		logger.Info("All narrative items excluded as thin-input; skipping AI call")
		return Narrative{}, nil
	}

	// ── Build URL set for citation verification ──
	inputURLs := make(map[string]bool, len(filtered))
	for _, it := range filtered {
		inputURLs[it.URL] = true
	}

	// ── Build compact JSON input ──
	inputItems := make([]narrativeInputItem, len(filtered))
	for i, it := range filtered {
		comments := make([]narrativeInputComment, len(it.RecentComments))
		for j, c := range it.RecentComments {
			comments[j] = narrativeInputComment{Author: c.Author, CreatedAt: c.CreatedAt, Body: c.Body}
		}
		events := make([]narrativeInputEvent, len(it.Events))
		for j, e := range it.Events {
			events[j] = narrativeInputEvent{Type: e.Type, Actor: e.Actor, At: e.At, Detail: e.Detail}
		}
		inputItems[i] = narrativeInputItem{
			URL: it.URL, Title: it.Title, Body: it.Body,
			State: it.State, IsPR: it.IsPR, Author: it.Author,
			Assignees: it.Assignees, Labels: it.Labels,
			OpenedAt: it.OpenedAt, ClosedAt: it.ClosedAt, MergedAt: it.MergedAt,
			ClosedThisWeek: it.ClosedThisWeek, MergedThisWeek: it.MergedThisWeek,
			RecentComments: comments, Events: events,
		}
	}

	// ── Build rollup context for system prompt ──
	promptContext := fmt.Sprintf("Context from mechanical rollup:\n%s\n\nItems to narrate (%d):\n", rollupSummary(r), len(filtered))

	itemBytes, err := json.Marshal(inputItems)
	if err != nil {
		return Narrative{}, fmt.Errorf("failed to marshal narrative items: %w", err)
	}
	userPrompt := promptContext + string(itemBytes)

	logger.Debug("WriteNarrative AI call", "model", c.Model, "items", len(filtered))

	// ── Make API call with JSON response format ──
	request := chatCompletionRequestJSON{
		Model:          c.Model,
		Temperature:    temperature,
		ResponseFormat: map[string]string{"type": "json_object"},
		Messages: []message{
			{Role: "system", Content: narrativeSystemPrompt},
			{Role: "user", Content: userPrompt},
		},
	}

	rawResponse, err := c.callJSONAPI(ctx, request)
	if err != nil {
		return Narrative{}, err
	}

	// ── Parse response ──
	var nr narrativeResponse
	if err := json.Unmarshal([]byte(rawResponse), &nr); err != nil {
		return Narrative{}, fmt.Errorf("WriteNarrative: failed to parse response: %w", err)
	}

	// ── Citation verification (premortem #1) ──
	unknownCount := 0
	for _, sec := range nr.Sections {
		matches := citationRegex.FindAllStringSubmatch(sec.Body, -1)
		for _, m := range matches {
			url := m[1]
			if !inputURLs[url] {
				logger.Warn("Narrative contains unknown citation URL", "unknown_citation_url", url)
				unknownCount++
			}
		}
	}
	logger.Info("Narrative citation verification complete", "unknown_citations", unknownCount)

	sections := make([]NarrativeSection, len(nr.Sections))
	for i, s := range nr.Sections {
		sections[i] = NarrativeSection(s)
	}
	return Narrative{Sections: sections}, nil
}

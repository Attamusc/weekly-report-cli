package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Attamusc/weekly-report-cli/internal/input"
	"github.com/Attamusc/weekly-report-cli/internal/retry"
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

	singleHighlightSystemPrompt = `You are evaluating a single engineering item to determine if it is worth highlighting.

You will receive a JSON object for one issue or pull request with: id, title, state, is_pr, labels, and recent updates.

Decide if this item is worth highlighting — bug fixes shipped, support issues resolved,
meaningful infrastructure improvements, important discussions, developer experience wins.
Skip trivial/routine items, dependabot bumps, and minor chores.

If worth highlighting:
- ASSIGN a theme from: "Bug Fixes", "Support & Reliability", "Infrastructure", "Developer Experience",
  "Documentation", "Security", "Performance", or a short custom theme if none fit.
- WRITE a 1-sentence highlight — concise, specific, present tense.

Return ONE JSON object with exactly two keys: "theme" and "summary".
Do NOT wrap the object in an array.
Example: {"theme": "Bug Fixes", "summary": "Fixes intermittent auth timeout affecting login."}

If the item is NOT worth highlighting, return: {"theme": "", "summary": ""}`

	temperature  = 1 // gpt-5o-mini only supports temperature of 1
	maxRetries   = 3
	baseDelay    = 1 * time.Second
	maxBatchSize = 25 // Maximum items per batch (count cap)
	// maxSingleHighlightChars is the character budget for UpdateTexts in a single
	// SummarizeHighlight request. One item per call, so token pressure is low.
	maxSingleHighlightChars = 3000
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

// highlightRequestItem represents a single item in a SummarizeHighlight request.
type highlightRequestItem struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	State   string   `json:"state"`
	IsPR    bool     `json:"is_pr"`
	Labels  []string `json:"labels"`
	Updates []string `json:"updates"`
}

// highlightSingleResponse is the expected response shape for SummarizeHighlight.
type highlightSingleResponse struct {
	Theme   string `json:"theme"`
	Summary string `json:"summary"`
}

// highlightMergeItem is one entry in the MergeThemes request payload.
type highlightMergeItem struct {
	URL     string `json:"url"`
	Title   string `json:"title"`
	Theme   string `json:"theme"`
	Summary string `json:"summary"`
}

// highlightMergeResponseItem is one entry in the MergeThemes response.
type highlightMergeResponseItem struct {
	URL   string `json:"url"`
	Theme string `json:"theme"`
}

const mergeThemesSystemPrompt = `You are merging theme labels across a set of engineering highlights.

You will receive a JSON array where each item has: url, title, theme, summary.

Your job: rename and consolidate themes so they are consistent and minimal.
Use themes from: "Bug Fixes", "Support & Reliability", "Infrastructure", "Developer Experience",
"Documentation", "Security", "Performance", or a short custom theme if none fit.

Return ONLY a JSON array of objects with exactly two fields: "url" and "theme".
Do NOT include title, summary, or any other field.
Example: [{"url":"https://...","theme":"Bug Fixes"}]`

// truncateHighlightItem returns a copy of item with UpdateTexts trimmed so the
// total UpdateTexts size fits within maxSingleHighlightChars. The most recent
// updates are preserved.
func truncateHighlightItem(item HighlightItem) HighlightItem {
	out := item
	out.UpdateTexts = nil
	totalLen := 0
	for i := len(item.UpdateTexts) - 1; i >= 0; i-- {
		u := item.UpdateTexts[i]
		if totalLen+len(u) > maxSingleHighlightChars {
			if totalLen < maxSingleHighlightChars {
				trunc := u[:maxSingleHighlightChars-totalLen]
				out.UpdateTexts = append([]string{trunc + "…"}, out.UpdateTexts...)
			}
			break
		}
		out.UpdateTexts = append([]string{u}, out.UpdateTexts...)
		totalLen += len(u)
	}
	return out
}

// parseSingleHighlightResponse parses the raw JSON string from a SummarizeHighlight
// API response. It accepts both shapes the model may return:
//   - Preferred: {"theme": "...", "summary": "..."}
//   - Acceptable: [{"theme": "...", "summary": "..."}] — single-element array, unwrapped.
//
// Any other shape is returned as an error so the caller's per-item fallback fires.
func parseSingleHighlightResponse(raw string) (highlightSingleResponse, error) {
	// Try object first (preferred shape).
	var single highlightSingleResponse
	if err := json.Unmarshal([]byte(raw), &single); err == nil {
		return single, nil
	}

	// Try single-element array (model wrapped the object in [])
	var arr []highlightSingleResponse
	if err := json.Unmarshal([]byte(raw), &arr); err == nil && len(arr) == 1 {
		return arr[0], nil
	}

	return highlightSingleResponse{}, fmt.Errorf("expected JSON object or single-element array, got: %s", raw)
}

// SummarizeHighlight curates a single item and returns a Highlight with theme and summary.
func (c *GHModelsClient) SummarizeHighlight(ctx context.Context, item HighlightItem) (Highlight, error) {
	logger := getContextLogger(ctx)
	logger.Debug("AI summarize highlight", "model", c.Model, "url", item.IssueURL)

	// Apply per-item safety cap on UpdateTexts.
	item = truncateHighlightItem(item)

	reqItem := highlightRequestItem{
		ID:      item.IssueURL,
		Title:   item.IssueTitle,
		State:   item.IssueState,
		IsPR:    item.IsPR,
		Labels:  item.Labels,
		Updates: item.UpdateTexts,
	}
	jsonBytes, err := json.Marshal(reqItem)
	if err != nil {
		return Highlight{}, fmt.Errorf("failed to marshal highlight item: %w", err)
	}

	// Use the system prompt override if provided, otherwise use the single-item
	// system prompt (singleHighlightSystemPrompt) which asks for one JSON object.
	sysPrompt := singleHighlightSystemPrompt
	if c.SystemPrompt != "" {
		sysPrompt = c.SystemPrompt
	}

	response, err := c.callAPI(ctx, string(jsonBytes), sysPrompt)
	if err != nil {
		return Highlight{}, fmt.Errorf("highlight API call failed: %w", err)
	}

	// Strip markdown code fences.
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	resp, err := parseSingleHighlightResponse(response)
	if err != nil {
		return Highlight{}, fmt.Errorf("failed to parse highlight response: %w", err)
	}

	return Highlight{
		Theme:   resp.Theme,
		Title:   item.IssueTitle,
		URL:     item.IssueURL,
		Summary: resp.Summary,
	}, nil
}

// MergeThemes renames and merges themes across a set of highlights. Only the
// Theme field is overwritten; Title, URL, and Summary are sourced from the input
// slice and are never read from the AI response (premortem #5).
// If the response is malformed or empty, the input is returned unchanged.
func (c *GHModelsClient) MergeThemes(ctx context.Context, in []Highlight) ([]Highlight, error) {
	logger := getContextLogger(ctx)

	if len(in) == 0 {
		return in, nil
	}

	mergeItems := make([]highlightMergeItem, len(in))
	for i, h := range in {
		mergeItems[i] = highlightMergeItem{
			URL:     h.URL,
			Title:   h.Title,
			Theme:   h.Theme,
			Summary: h.Summary,
		}
	}
	jsonBytes, err := json.Marshal(mergeItems)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal merge themes request: %w", err)
	}

	response, err := c.callAPI(ctx, string(jsonBytes), mergeThemesSystemPrompt)
	if err != nil {
		return nil, fmt.Errorf("merge themes API call failed: %w", err)
	}

	// Strip markdown code fences.
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	var respItems []highlightMergeResponseItem
	if err := json.Unmarshal([]byte(response), &respItems); err != nil || len(respItems) == 0 {
		logger.Warn("MergeThemes response malformed or empty; returning input unchanged",
			"error", err, "response", response)
		return in, nil
	}

	// Build url → new theme map. Only Theme is read from the response.
	themeByURL := make(map[string]string, len(respItems))
	for _, ri := range respItems {
		themeByURL[ri.URL] = ri.Theme
	}

	// Walk input, overwriting only Theme.
	out := make([]Highlight, len(in))
	for i, h := range in {
		out[i] = h
		if newTheme, ok := themeByURL[h.URL]; ok && newTheme != "" {
			out[i].Theme = newTheme
		}
	}
	return out, nil
}

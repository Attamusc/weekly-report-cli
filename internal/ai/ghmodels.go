package ai

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// GHModelsClient implements Summarizer using GitHub Models API
type GHModelsClient struct {
	HTTP         *http.Client
	BaseURL      string
	Model        string
	Token        string
	SystemPrompt string
}

// NewGHModelsClient creates a new GitHub Models API client
func NewGHModelsClient(baseURL, model, token, systemPrompt string, timeout time.Duration) *GHModelsClient {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	return &GHModelsClient{
		HTTP:         &http.Client{Timeout: timeout},
		BaseURL:      baseURL,
		Model:        model,
		Token:        token,
		SystemPrompt: systemPrompt,
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
	no prefatory text. Keep relevant looking links intact in markdown format. 
	Attempt to not lose context when summarizing.`

	batchSystemPrompt = `You are summarizing multiple engineering status updates in a single batch.

You will receive a JSON object with an array of items, each containing:
- id: A unique identifier (the issue URL)
- issue: The issue title
- updates: One or more status updates (newest first)
- reported_status: The author's claimed status

For each item, produce:
1. A summary: 3-5 sentences, present tense, third-person, markdown-ready, no prefatory text.
   Keep relevant links intact in markdown format. Do not lose context when summarizing.
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

	temperature    = 1 // gpt-5o-mini only supports temperature of 1
	maxRetries     = 3
	baseDelay      = 1 * time.Second
	maxBatchSize   = 25   // Maximum items per batch to avoid token limits
	maxBatchTokens = 8000 // Rough estimate of safe token limit for batch
)

// getSystemPrompt returns the configured system prompt or the default if empty
func (c *GHModelsClient) getSystemPrompt() string {
	if c.SystemPrompt != "" {
		return c.SystemPrompt
	}
	return defaultSystemPrompt
}

// loggerContextKey is the context key for the logger
type loggerContextKey struct{}

// Summarize generates a summary for a single update using GitHub Models API
func (c *GHModelsClient) Summarize(ctx context.Context, issueTitle, issueURL, updateText string) (string, error) {
	// Get logger from context if available
	logger, ok := ctx.Value(loggerContextKey{}).(*slog.Logger)
	if !ok {
		logger = slog.Default()
	}

	logger.Debug("AI summarizing single update", "model", c.Model, "issue", issueURL)
	userPrompt := fmt.Sprintf("Issue: %s (%s)\nUpdate:\n%s", issueTitle, issueURL, updateText)
	return c.callAPI(ctx, userPrompt)
}

// SummarizeMany generates a summary for multiple updates using GitHub Models API
func (c *GHModelsClient) SummarizeMany(ctx context.Context, issueTitle, issueURL string, updates []string) (string, error) {
	// Get logger from context if available
	logger, ok := ctx.Value(loggerContextKey{}).(*slog.Logger)
	if !ok {
		logger = slog.Default()
	}

	logger.Debug("AI summarizing multiple updates", "model", c.Model, "issue", issueURL, "count", len(updates))
	userPrompt := fmt.Sprintf("Issue: %s (%s)\nUpdates (newest first):", issueTitle, issueURL)

	for i, update := range updates {
		userPrompt += fmt.Sprintf("\n%d) %s", i+1, update)
	}

	return c.callAPI(ctx, userPrompt)
}

// callAPI makes the actual HTTP request to GitHub Models API with retry logic
func (c *GHModelsClient) callAPI(ctx context.Context, userPrompt string) (string, error) {
	// Get logger from context if available
	logger, ok := ctx.Value(loggerContextKey{}).(*slog.Logger)
	if !ok {
		logger = slog.Default()
	}

	request := chatCompletionRequest{
		Model:       c.Model,
		Temperature: temperature,
		Messages: []message{
			{Role: "system", Content: c.getSystemPrompt()},
			{Role: "user", Content: userPrompt},
		},
	}

	logger.Debug("Starting AI API request", "model", c.Model, "temperature", temperature, "maxRetries", maxRetries)

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Apply jittered exponential backoff
			delay := time.Duration(float64(baseDelay) * math.Pow(2, float64(attempt-1)))
			jitterMax := int64(float64(delay) * 0.1) // 10% jitter
			jitterBig, _ := rand.Int(rand.Reader, big.NewInt(jitterMax+1))
			jitter := time.Duration(jitterBig.Int64())
			logger.Debug("AI API retry backoff", "attempt", attempt, "delay", delay+jitter)
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(delay + jitter):
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

	url := c.BaseURL + "/inference/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("User-Agent", "weekly-report-cli/1.0")

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
	// Use custom system prompt for batch mode
	originalPrompt := c.SystemPrompt
	c.SystemPrompt = cfg.systemPrompt
	defer func() { c.SystemPrompt = originalPrompt }()

	// Call API
	response, err := c.callAPI(ctx, userPrompt)
	if err != nil {
		return "", fmt.Errorf("%s API call failed: %w", cfg.actionName, err)
	}

	return response, nil
}

// getContextLogger retrieves the logger from context, falling back to slog.Default.
func getContextLogger(ctx context.Context) *slog.Logger {
	logger, ok := ctx.Value(loggerContextKey{}).(*slog.Logger)
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

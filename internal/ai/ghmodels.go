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
func NewGHModelsClient(baseURL, model, token, systemPrompt string) *GHModelsClient {
	return &GHModelsClient{
		HTTP:         &http.Client{Timeout: 30 * time.Second},
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

Respond with ONLY a valid JSON object where:
- Keys are the item IDs (URLs) as strings
- Values are the summaries (3-5 sentences, present tense, third-person, markdown-ready)

Keep relevant markdown links intact. Do not add any prefatory text, explanation, or markdown code fences.

Example response format:
{
  "https://github.com/org/repo/issues/1": "Team completed OAuth integration...",
  "https://github.com/org/repo/issues/2": "Team fixed critical race condition..."
}`

	temperature    = 1
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
	ID      string   `json:"id"`
	Issue   string   `json:"issue"`
	Updates []string `json:"updates"`
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
			ID:      item.IssueURL,
			Issue:   item.IssueTitle,
			Updates: item.UpdateTexts,
		}
	}

	jsonBytes, err := json.Marshal(batchReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal batch request: %w", err)
	}

	return string(jsonBytes), nil
}

// parseBatchResponse attempts to parse the API response as JSON
// Returns a map of issueURL -> summary
func (c *GHModelsClient) parseBatchResponse(response string, items []BatchItem) (map[string]string, error) {
	// Try to parse as JSON first
	var jsonResponse map[string]string
	if err := json.Unmarshal([]byte(response), &jsonResponse); err == nil {
		// Successfully parsed JSON
		return jsonResponse, nil
	}

	// JSON parsing failed - try markdown fallback
	return c.parseMarkdownBatchResponse(response, items)
}

// parseMarkdownBatchResponse parses a markdown-formatted batch response
// Expected format:
// ## SUMMARY <URL>
// <summary text>
//
// ## SUMMARY <URL>
// <summary text>
func (c *GHModelsClient) parseMarkdownBatchResponse(response string, items []BatchItem) (map[string]string, error) {
	result := make(map[string]string)

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
				result[item.IssueURL] = summary
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

// SummarizeBatch generates summaries for multiple issues in a single request
// Implements chunking to avoid token limits
func (c *GHModelsClient) SummarizeBatch(ctx context.Context, items []BatchItem) (map[string]string, error) {
	// Get logger from context if available
	logger, ok := ctx.Value(loggerContextKey{}).(*slog.Logger)
	if !ok {
		logger = slog.Default()
	}

	if len(items) == 0 {
		return map[string]string{}, nil
	}

	// If we have more than maxBatchSize items, chunk them
	if len(items) > maxBatchSize {
		logger.Debug("Splitting batch into chunks", "totalItems", len(items), "chunkSize", maxBatchSize)
		return c.summarizeBatchChunked(ctx, items, logger)
	}

	logger.Debug("AI batch summarizing", "model", c.Model, "items", len(items))

	// Build the prompt
	userPrompt, err := c.buildBatchPrompt(items)
	if err != nil {
		return nil, fmt.Errorf("failed to build batch prompt: %w", err)
	}

	// Use custom system prompt for batch mode
	originalPrompt := c.SystemPrompt
	c.SystemPrompt = batchSystemPrompt
	defer func() { c.SystemPrompt = originalPrompt }()

	// Call API
	response, err := c.callAPI(ctx, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("batch API call failed: %w", err)
	}

	// Parse response
	summaries, err := c.parseBatchResponse(response, items)
	if err != nil {
		logger.Debug("Failed to parse batch response", "error", err)
		return nil, err
	}

	logger.Debug("Batch summarization succeeded", "summaries", len(summaries))
	return summaries, nil
}

// summarizeBatchChunked splits items into chunks and processes them sequentially
func (c *GHModelsClient) summarizeBatchChunked(ctx context.Context, items []BatchItem, logger *slog.Logger) (map[string]string, error) {
	result := make(map[string]string)

	for i := 0; i < len(items); i += maxBatchSize {
		end := i + maxBatchSize
		if end > len(items) {
			end = len(items)
		}

		chunk := items[i:end]
		logger.Debug("Processing batch chunk", "chunk", i/maxBatchSize+1, "items", len(chunk))

		chunkSummaries, err := c.SummarizeBatch(ctx, chunk)
		if err != nil {
			return nil, fmt.Errorf("chunk %d failed: %w", i/maxBatchSize+1, err)
		}

		// Merge results
		for url, summary := range chunkSummaries {
			result[url] = summary
		}
	}

	return result, nil
}

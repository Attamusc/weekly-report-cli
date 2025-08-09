package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"time"
)

// GHModelsClient implements Summarizer using GitHub Models API
type GHModelsClient struct {
	HTTP    *http.Client
	BaseURL string
	Model   string
	Token   string
}

// NewGHModelsClient creates a new GitHub Models API client
func NewGHModelsClient(baseURL, model, token string) *GHModelsClient {
	return &GHModelsClient{
		HTTP:    &http.Client{Timeout: 30 * time.Second},
		BaseURL: baseURL,
		Model:   model,
		Token:   token,
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
	systemPrompt = "Refine the content in the engineering status updates to be one paragraph of roughly 3-5 sentences, present tense, markdown-ready, no prefatory text. Keep relevant looking links intact in markdown format. Attempt to not lose any content of substance."
	temperature  = 0.2
	maxRetries   = 3
	baseDelay    = 1 * time.Second
)

// Summarize generates a summary for a single update using GitHub Models API
func (c *GHModelsClient) Summarize(ctx context.Context, issueTitle, issueURL, updateText string) (string, error) {
	// Get logger from context if available
	logger, ok := ctx.Value("logger").(*slog.Logger)
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
	logger, ok := ctx.Value("logger").(*slog.Logger)
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
	logger, ok := ctx.Value("logger").(*slog.Logger)
	if !ok {
		logger = slog.Default()
	}

	request := chatCompletionRequest{
		Model:       c.Model,
		Temperature: temperature,
		Messages: []message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	}

	logger.Debug("Starting AI API request", "model", c.Model, "temperature", temperature, "maxRetries", maxRetries)

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Apply jittered exponential backoff
			delay := time.Duration(float64(baseDelay) * math.Pow(2, float64(attempt-1)))
			jitter := time.Duration(rand.Float64() * float64(delay) * 0.1) // 10% jitter
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
	defer resp.Body.Close()

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

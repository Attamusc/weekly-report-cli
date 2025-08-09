package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	systemPrompt = "Summarize engineering status updates in ≤ 35 words, one sentence, present tense, markdown-ready, no prefatory text."
	temperature  = 0.2
	maxRetries   = 3
	baseDelay    = 1 * time.Second
)

// Summarize generates a summary for a single update using GitHub Models API
func (c *GHModelsClient) Summarize(ctx context.Context, issueTitle, issueURL, updateText string) (string, error) {
	userPrompt := fmt.Sprintf("Issue: %s (%s)\nUpdate:\n%s", issueTitle, issueURL, updateText)
	return c.callAPI(ctx, userPrompt)
}

// SummarizeMany generates a summary for multiple updates using GitHub Models API
func (c *GHModelsClient) SummarizeMany(ctx context.Context, issueTitle, issueURL string, updates []string) (string, error) {
	userPrompt := fmt.Sprintf("Issue: %s (%s)\nUpdates (newest first):", issueTitle, issueURL)
	
	for i, update := range updates {
		userPrompt += fmt.Sprintf("\n%d) %s", i+1, update)
	}
	
	return c.callAPI(ctx, userPrompt)
}

// callAPI makes the actual HTTP request to GitHub Models API with retry logic
func (c *GHModelsClient) callAPI(ctx context.Context, userPrompt string) (string, error) {
	request := chatCompletionRequest{
		Model:       c.Model,
		Temperature: temperature,
		Messages: []message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	}
	
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Apply jittered exponential backoff
			delay := time.Duration(float64(baseDelay) * math.Pow(2, float64(attempt-1)))
			jitter := time.Duration(rand.Float64() * float64(delay) * 0.1) // 10% jitter
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(delay + jitter):
			}
		}
		
		response, err := c.makeHTTPRequest(ctx, request)
		if err != nil {
			lastErr = err
			
			// Check if it's a rate limit error (429)
			if httpErr, ok := err.(*HTTPError); ok && httpErr.StatusCode == 429 {
				// Extract retry-after header if present
				if retryAfter := httpErr.Headers.Get("Retry-After"); retryAfter != "" {
					if seconds, parseErr := strconv.Atoi(retryAfter); parseErr == nil {
						select {
						case <-ctx.Done():
							return "", ctx.Err()
						case <-time.After(time.Duration(seconds) * time.Second):
						}
					}
				}
				continue // Retry on rate limit
			}
			
			// For other errors, return immediately
			return "", fmt.Errorf("GitHub Models API request failed: %w", err)
		}
		
		// Success - extract and return the response
		if len(response.Choices) == 0 {
			return "", fmt.Errorf("GitHub Models API returned empty response")
		}
		
		summary := response.Choices[0].Message.Content
		return summary, nil
	}
	
	return "", fmt.Errorf("GitHub Models API failed after %d retries: %w", maxRetries, lastErr)
}

// makeHTTPRequest performs the actual HTTP request
func (c *GHModelsClient) makeHTTPRequest(ctx context.Context, request chatCompletionRequest) (*chatCompletionResponse, error) {
	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	
	url := c.BaseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("User-Agent", "gh-epic-updates/1.0")
	
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
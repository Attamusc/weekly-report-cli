package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGHModelsClient_Summarize(t *testing.T) {
	tests := []struct {
		name           string
		issueTitle     string
		issueURL       string
		updateText     string
		responseBody   string
		statusCode     int
		expectedError  string
		expectedResult string
	}{
		{
			name:       "successful single update summarization",
			issueTitle: "Implement user authentication",
			issueURL:   "https://github.com/owner/repo/issues/123",
			updateText: "Completed the OAuth2 integration and added session management. All tests are passing.",
			responseBody: `{
				"choices": [
					{
						"message": {
							"role": "assistant",
							"content": "Completed OAuth2 integration and session management with passing tests."
						}
					}
				]
			}`,
			statusCode:     200,
			expectedResult: "Completed OAuth2 integration and session management with passing tests.",
		},
		{
			name:       "API returns empty choices",
			issueTitle: "Fix bug in payment processing",
			issueURL:   "https://github.com/owner/repo/issues/456",
			updateText: "Found the root cause and deployed a fix.",
			responseBody: `{
				"choices": []
			}`,
			statusCode:    200,
			expectedError: "GitHub Models API returned empty response",
		},
		{
			name:       "HTTP 500 error",
			issueTitle: "Optimize database queries",
			issueURL:   "https://github.com/owner/repo/issues/789",
			updateText: "Added indexes and reduced query time by 50%.",
			statusCode: 500,
			responseBody: `{
				"error": {
					"message": "Internal server error"
				}
			}`,
			expectedError: "GitHub Models API request failed",
		},
		{
			name:          "invalid JSON response",
			issueTitle:    "Update dependencies",
			issueURL:      "https://github.com/owner/repo/issues/321",
			updateText:    "Updated all packages to latest versions.",
			statusCode:    200,
			responseBody:  `invalid json`,
			expectedError: "failed to unmarshal response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request method and headers
				if r.Method != "POST" {
					t.Errorf("Expected POST request, got %s", r.Method)
				}

				if r.Header.Get("Authorization") != "Bearer test-token" {
					t.Errorf("Expected Authorization header with Bearer token")
				}

				if r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("Expected Content-Type: application/json")
				}

				if r.Header.Get("User-Agent") != "weekly-report-cli/1.0" {
					t.Errorf("Expected User-Agent: weekly-report-cli/1.0")
				}

				// Verify request body
				var request chatCompletionRequest
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Errorf("Failed to decode request body: %v", err)
				}

				// Verify request structure
				if request.Model != "gpt-4o-mini" {
					t.Errorf("Expected model gpt-4o-mini, got %s", request.Model)
				}

				if request.Temperature != 0.2 {
					t.Errorf("Expected temperature 0.2, got %f", request.Temperature)
				}

				if len(request.Messages) != 2 {
					t.Errorf("Expected 2 messages, got %d", len(request.Messages))
				}

				if request.Messages[0].Role != "system" {
					t.Errorf("Expected first message role 'system', got %s", request.Messages[0].Role)
				}

				if request.Messages[1].Role != "user" {
					t.Errorf("Expected second message role 'user', got %s", request.Messages[1].Role)
				}

				// Verify user prompt contains expected content
				userContent := request.Messages[1].Content
				if !strings.Contains(userContent, tt.issueTitle) {
					t.Errorf("User prompt should contain issue title")
				}
				if !strings.Contains(userContent, tt.issueURL) {
					t.Errorf("User prompt should contain issue URL")
				}
				if !strings.Contains(userContent, tt.updateText) {
					t.Errorf("User prompt should contain update text")
				}

				// Send response
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			// Create client
			client := NewGHModelsClient(server.URL, "gpt-4o-mini", "test-token", "")

			// Call method
			ctx := context.Background()
			result, err := client.Summarize(ctx, tt.issueTitle, tt.issueURL, tt.updateText)

			// Verify results
			if tt.expectedError != "" {
				if err == nil {
					t.Errorf("Expected error containing '%s', got nil", tt.expectedError)
				} else if !strings.Contains(err.Error(), tt.expectedError) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.expectedError, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got %v", err)
				}
				if result != tt.expectedResult {
					t.Errorf("Expected result '%s', got '%s'", tt.expectedResult, result)
				}
			}
		})
	}
}

func TestGHModelsClient_SummarizeMany(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		var request chatCompletionRequest
		json.NewDecoder(r.Body).Decode(&request)

		// Verify user prompt format for multiple updates
		userContent := request.Messages[1].Content
		expectedPatterns := []string{
			"Issue: Multiple updates test",
			"https://github.com/test/repo/issues/1",
			"Updates (newest first):",
			"1) First update text",
			"2) Second update text",
			"3) Third update text",
		}

		for _, pattern := range expectedPatterns {
			if !strings.Contains(userContent, pattern) {
				t.Errorf("User prompt should contain '%s', got: %s", pattern, userContent)
			}
		}

		// Send response
		response := `{
			"choices": [
				{
					"message": {
						"role": "assistant", 
						"content": "Completed three development phases with successful testing and deployment."
					}
				}
			]
		}`
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(response))
	}))
	defer server.Close()

	client := NewGHModelsClient(server.URL, "gpt-4o-mini", "test-token", "")

	updates := []string{
		"First update text",
		"Second update text",
		"Third update text",
	}

	ctx := context.Background()
	result, err := client.SummarizeMany(ctx, "Multiple updates test", "https://github.com/test/repo/issues/1", updates)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	expected := "Completed three development phases with successful testing and deployment."
	if result != expected {
		t.Errorf("Expected result '%s', got '%s'", expected, result)
	}
}

func TestGHModelsClient_RetryOnRateLimit(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		if callCount == 1 {
			// First call returns 429
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(429)
			w.Write([]byte(`{"error": {"message": "Rate limited"}}`))
			return
		}

		// Second call succeeds
		response := `{
			"choices": [
				{
					"message": {
						"role": "assistant",
						"content": "Success after retry."
					}
				}
			]
		}`
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(response))
	}))
	defer server.Close()

	client := NewGHModelsClient(server.URL, "gpt-4o-mini", "test-token", "")

	ctx := context.Background()
	result, err := client.Summarize(ctx, "Test", "https://github.com/test/repo/issues/1", "Update text")
	if err != nil {
		t.Errorf("Expected no error after retry, got %v", err)
	}

	if result != "Success after retry." {
		t.Errorf("Expected success result, got '%s'", result)
	}

	if callCount != 2 {
		t.Errorf("Expected 2 API calls (1 failure + 1 success), got %d", callCount)
	}
}

func TestGHModelsClient_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(200)
		w.Write([]byte(`{"choices": [{"message": {"content": "Test"}}]}`))
	}))
	defer server.Close()

	client := NewGHModelsClient(server.URL, "gpt-4o-mini", "test-token", "")

	// Create context that cancels immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.Summarize(ctx, "Test", "https://github.com/test/repo/issues/1", "Update text")

	if err == nil {
		t.Error("Expected context cancellation error")
	}

	if !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("Expected context canceled error, got %v", err)
	}
}

func TestGHModelsClient_CustomSystemPrompt(t *testing.T) {
	customPrompt := "This is a custom prompt for testing purposes."

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request contains custom prompt
		var request chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("Failed to decode request body: %v", err)
		}

		if len(request.Messages) != 2 {
			t.Errorf("Expected 2 messages, got %d", len(request.Messages))
		}

		if request.Messages[0].Role != "system" {
			t.Errorf("Expected first message role 'system', got %s", request.Messages[0].Role)
		}

		if request.Messages[0].Content != customPrompt {
			t.Errorf("Expected system prompt '%s', got '%s'", customPrompt, request.Messages[0].Content)
		}

		// Send response
		response := `{
			"choices": [
				{
					"message": {
						"role": "assistant", 
						"content": "Custom response."
					}
				}
			]
		}`
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(response))
	}))
	defer server.Close()

	// Create client with custom prompt
	client := NewGHModelsClient(server.URL, "gpt-4o-mini", "test-token", customPrompt)

	ctx := context.Background()
	result, err := client.Summarize(ctx, "Test Issue", "https://github.com/test/repo/issues/1", "Test update")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if result != "Custom response." {
		t.Errorf("Expected 'Custom response.', got '%s'", result)
	}
}

func TestGHModelsClient_DefaultSystemPrompt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request contains default prompt
		var request chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("Failed to decode request body: %v", err)
		}

		expectedDefaultPrompt := `Refine the content in the engineering status updates to be one
	paragraph of roughly 3-5 sentences, present tense, third-person, markdown-ready, 
	no prefatory text. Keep relevant looking links intact in markdown format. 
	Attempt to not lose context when summarizing.`

		if request.Messages[0].Content != expectedDefaultPrompt {
			t.Errorf("Expected default system prompt, got '%s'", request.Messages[0].Content)
		}

		// Send response
		response := `{
			"choices": [
				{
					"message": {
						"role": "assistant", 
						"content": "Default response."
					}
				}
			]
		}`
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(response))
	}))
	defer server.Close()

	// Create client with empty prompt (should use default)
	client := NewGHModelsClient(server.URL, "gpt-4o-mini", "test-token", "")

	ctx := context.Background()
	result, err := client.Summarize(ctx, "Test Issue", "https://github.com/test/repo/issues/1", "Test update")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if result != "Default response." {
		t.Errorf("Expected 'Default response.', got '%s'", result)
	}
}

func TestValidateWordCount(t *testing.T) {
	// Test helper to validate that responses are ≤35 words
	responses := []string{
		"Completed OAuth2 integration and session management with passing tests.",
		"Fixed database connection issues and improved query performance significantly.",
		"This is a very long response that exceeds the thirty-five word limit that we have set for our AI summarization system to ensure concise and readable status updates for engineering teams working on complex software development projects with multiple stakeholders and requirements.",
	}

	for i, response := range responses {
		words := strings.Fields(response)
		wordCount := len(words)

		if i < 2 {
			// First two should be ≤35 words
			if wordCount > 35 {
				t.Errorf("Response %d has %d words, should be ≤35: %s", i+1, wordCount, response)
			}
		} else {
			// Third one is intentionally over the limit for testing (should be >35 words)
			if wordCount <= 35 {
				t.Errorf("Test response %d has %d words, should exceed 35 words for validation test", i+1, wordCount)
			}
		}
	}
}

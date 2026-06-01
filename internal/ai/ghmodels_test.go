package ai

import (
	"context"
	"encoding/json"
	"io"
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

				if request.Temperature != 1 {
					t.Errorf("Expected temperature 1, got %f", request.Temperature)
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
			client := NewGHModelsClient(server.URL, "gpt-4o-mini", "test-token", "", 0)

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

	client := NewGHModelsClient(server.URL, "gpt-4o-mini", "test-token", "", 0)

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

	client := NewGHModelsClient(server.URL, "gpt-4o-mini", "test-token", "", 0)

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

	client := NewGHModelsClient(server.URL, "gpt-4o-mini", "test-token", "", 0)

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
	client := NewGHModelsClient(server.URL, "gpt-4o-mini", "test-token", customPrompt, 0)

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
	no prefatory text. Attempt to not lose context when summarizing.

	When the source material contains links (GitHub issue/PR references, URLs, etc.),
	weave them naturally into the summary as inline markdown links with descriptive
	anchor text that reflects the content the link relates to. Do NOT group links in
	parentheses or use raw GitHub shorthand references as the link text.

	Bad:  "...experiments reveal more viable endpoints (github/repo#523, github/repo#524)."
	Good: "...experiments reveal [more viable endpoints to migrate](url) and [new discovery results](url)."`

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
	client := NewGHModelsClient(server.URL, "gpt-4o-mini", "test-token", "", 0)

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

func TestGHModelsClient_GenerateHeader(t *testing.T) {
	transition := "At Risk→On Track"
	items := []HeaderItem{
		{StatusCaption: "On Track", Title: "Initiative A", Summary: "Making progress.", NewItem: false, StatusTransition: &transition},
		{StatusCaption: "Done", Title: "Initiative B", Summary: "Completed.", NewItem: true},
	}

	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"Good progress this week."}}]}`))
	}))
	defer server.Close()

	client := NewGHModelsClient(server.URL, "test-model", "test-token", "", 10*time.Second)
	result, err := client.GenerateHeader(context.Background(), items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Good progress this week." {
		t.Errorf("unexpected result: %q", result)
	}

	// Verify JSON payload contains header system prompt marker
	var req map[string]interface{}
	if err := json.Unmarshal(capturedBody, &req); err != nil {
		t.Fatalf("failed to parse request body: %v", err)
	}
	messages, _ := req["messages"].([]interface{})
	if len(messages) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(messages))
	}
	systemMsg, _ := messages[0].(map[string]interface{})
	if !strings.Contains(systemMsg["content"].(string), "weekly engineering status report") {
		t.Errorf("system prompt missing expected content, got %q", systemMsg["content"])
	}
	userMsg, _ := messages[1].(map[string]interface{})
	if !strings.Contains(userMsg["content"].(string), "Initiative A") {
		t.Errorf("user message missing item data, got %q", userMsg["content"])
	}
}

func TestGHModelsClient_GenerateHeader_Empty(t *testing.T) {
	client := NewGHModelsClient("http://unused", "model", "token", "", 10*time.Second)
	result, err := client.GenerateHeader(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestGHModelsClient_SummarizeHighlight_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "Fix login bug") {
			t.Errorf("request body missing expected title: %s", body)
		}
		resp := chatCompletionResponse{
			Choices: []choice{{Message: message{Role: "assistant", Content: `{"theme":"Bug Fixes","summary":"Fixed the login bug."}`}}},
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	}))
	defer srv.Close()

	client := NewGHModelsClient(srv.URL, "model", "token", "", 10*time.Second)
	item := HighlightItem{
		IssueURL:   "https://github.com/org/repo/issues/1",
		IssueTitle: "Fix login bug",
		IssueState: "closed",
		Labels:     []string{"bug"},
	}
	h, err := client.SummarizeHighlight(context.Background(), item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.Theme != "Bug Fixes" {
		t.Errorf("expected theme %q, got %q", "Bug Fixes", h.Theme)
	}
	if h.Summary != "Fixed the login bug." {
		t.Errorf("expected summary %q, got %q", "Fixed the login bug.", h.Summary)
	}
	if h.Title != item.IssueTitle {
		t.Errorf("expected title %q from input, got %q", item.IssueTitle, h.Title)
	}
	if h.URL != item.IssueURL {
		t.Errorf("expected URL %q from input, got %q", item.IssueURL, h.URL)
	}
}

func TestGHModelsClient_SummarizeHighlight_ArrayResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Model wraps the object in an array — the 5/5 failure mode from A/B run.
		resp := chatCompletionResponse{
			Choices: []choice{{Message: message{Role: "assistant", Content: `[{"theme":"Infrastructure","summary":"Refactors CI pipeline to reduce build times."}]`}}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewGHModelsClient(srv.URL, "model", "token", "", 10*time.Second)
	item := HighlightItem{
		IssueURL:   "https://github.com/org/repo/issues/5",
		IssueTitle: "Refactor CI",
		IssueState: "closed",
	}
	h, err := client.SummarizeHighlight(context.Background(), item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.Theme != "Infrastructure" {
		t.Errorf("expected theme %q, got %q", "Infrastructure", h.Theme)
	}
	if h.Summary != "Refactors CI pipeline to reduce build times." {
		t.Errorf("expected summary %q, got %q", "Refactors CI pipeline to reduce build times.", h.Summary)
	}
	if h.Title != item.IssueTitle {
		t.Errorf("expected title %q from input, got %q", item.IssueTitle, h.Title)
	}
	if h.URL != item.IssueURL {
		t.Errorf("expected URL %q from input, got %q", item.IssueURL, h.URL)
	}
}

func TestGHModelsClient_SummarizeHighlight_MalformedResponse(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "plain string", content: `"just a string"`},
		{name: "int array", content: `[1,2,3]`},
		{name: "multi-element array", content: `[{"theme":"A","summary":"X"},{"theme":"B","summary":"Y"}]`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				resp := chatCompletionResponse{
					Choices: []choice{{Message: message{Role: "assistant", Content: tc.content}}},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
			}))
			defer srv.Close()

			client := NewGHModelsClient(srv.URL, "model", "token", "", 10*time.Second)
			item := HighlightItem{
				IssueURL:   "https://github.com/org/repo/issues/9",
				IssueTitle: "Some issue",
			}
			_, err := client.SummarizeHighlight(context.Background(), item)
			if err == nil {
				t.Error("expected error for malformed response, got nil")
			}
		})
	}
}

func TestGHModelsClient_SummarizeHighlight_Truncation(t *testing.T) {
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		resp := chatCompletionResponse{
			Choices: []choice{{Message: message{Role: "assistant", Content: `{"theme":"General","summary":"ok"}`}}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewGHModelsClient(srv.URL, "model", "token", "", 10*time.Second)
	// Build UpdateTexts totalling well over 3000 chars.
	longText := strings.Repeat("x", 2000)
	item := HighlightItem{
		IssueURL:    "https://github.com/org/repo/issues/2",
		IssueTitle:  "Some issue",
		UpdateTexts: []string{longText, longText, longText},
	}
	_, err := client.SummarizeHighlight(context.Background(), item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The request body contains the full chat completion JSON. The user content
	// portion (UpdateTexts) must be bounded to ~maxSingleHighlightChars.
	// We assert the total body is well under 3*2000 = 6000 chars of raw texts.
	if len(capturedBody) >= 6000+500 {
		t.Errorf("expected request body to be truncated; got %d bytes", len(capturedBody))
	}
}

func TestGHModelsClient_MergeThemes_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := chatCompletionResponse{
			Choices: []choice{{Message: message{Role: "assistant", Content: `[{"url":"https://github.com/org/repo/issues/1","theme":"Infrastructure"},{"url":"https://github.com/org/repo/issues/2","theme":"Infrastructure"}]`}}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewGHModelsClient(srv.URL, "model", "token", "", 10*time.Second)
	in := []Highlight{
		{Theme: "Bug Fixes", Title: "Fix A", URL: "https://github.com/org/repo/issues/1", Summary: "summary A"},
		{Theme: "Performance", Title: "Perf B", URL: "https://github.com/org/repo/issues/2", Summary: "summary B"},
	}
	out, err := client.MergeThemes(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 highlights, got %d", len(out))
	}
	for i, h := range out {
		if h.Theme != "Infrastructure" {
			t.Errorf("[%d] expected theme %q, got %q", i, "Infrastructure", h.Theme)
		}
		// Title, URL, Summary must come from the input, not the response.
		if h.Title != in[i].Title {
			t.Errorf("[%d] title changed: want %q, got %q", i, in[i].Title, h.Title)
		}
		if h.URL != in[i].URL {
			t.Errorf("[%d] URL changed: want %q, got %q", i, in[i].URL, h.URL)
		}
		if h.Summary != in[i].Summary {
			t.Errorf("[%d] summary changed: want %q, got %q", i, in[i].Summary, h.Summary)
		}
	}
}

func TestGHModelsClient_MergeThemes_MalformedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := chatCompletionResponse{
			Choices: []choice{{Message: message{Role: "assistant", Content: "not json at all"}}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewGHModelsClient(srv.URL, "model", "token", "", 10*time.Second)
	in := []Highlight{
		{Theme: "Bug Fixes", Title: "Fix A", URL: "https://github.com/org/repo/issues/1", Summary: "summary A"},
	}
	out, err := client.MergeThemes(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error on malformed response: %v", err)
	}
	// Must return input unchanged.
	if len(out) != 1 || out[0].Theme != "Bug Fixes" {
		t.Errorf("expected input unchanged, got %+v", out)
	}
}

func TestNoopSummarizer_SummarizeHighlight(t *testing.T) {
	noop := NewNoopSummarizer()

	tests := []struct {
		name          string
		item          HighlightItem
		expectedTheme string
	}{
		{
			name: "with labels",
			item: HighlightItem{
				IssueURL:   "https://github.com/org/repo/issues/1",
				IssueTitle: "Fix login bug",
				Labels:     []string{"bug", "priority"},
			},
			expectedTheme: "bug",
		},
		{
			name: "no labels",
			item: HighlightItem{
				IssueURL:   "https://github.com/org/repo/issues/2",
				IssueTitle: "Some task",
				Labels:     nil,
			},
			expectedTheme: "General",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h, err := noop.SummarizeHighlight(context.Background(), tc.item)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if h.Theme != tc.expectedTheme {
				t.Errorf("expected theme %q, got %q", tc.expectedTheme, h.Theme)
			}
			if h.Title != tc.item.IssueTitle {
				t.Errorf("expected title %q, got %q", tc.item.IssueTitle, h.Title)
			}
			if h.URL != tc.item.IssueURL {
				t.Errorf("expected URL %q, got %q", tc.item.IssueURL, h.URL)
			}
			if h.Summary != tc.item.IssueTitle {
				t.Errorf("expected summary %q, got %q", tc.item.IssueTitle, h.Summary)
			}
		})
	}
}

func TestEndpointProfileFor(t *testing.T) {
	tests := []struct {
		name            string
		baseURL         string
		wantPath        string
		wantIntegration string
	}{
		{
			name:            "github models endpoint",
			baseURL:         "https://models.github.ai",
			wantPath:        "/inference/chat/completions",
			wantIntegration: "",
		},
		{
			name:            "copilot endpoint",
			baseURL:         "https://api.githubcopilot.com",
			wantPath:        "/chat/completions",
			wantIntegration: "vscode-chat",
		},
		{
			name:            "copilot endpoint with proxy subdomain",
			baseURL:         "https://proxy.githubcopilot.com",
			wantPath:        "/chat/completions",
			wantIntegration: "vscode-chat",
		},
		{
			name:            "unknown endpoint falls back to models path",
			baseURL:         "https://custom.example.com",
			wantPath:        "/inference/chat/completions",
			wantIntegration: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotPath, gotID := endpointProfileFor(tc.baseURL)
			if gotPath != tc.wantPath {
				t.Errorf("chatPath: got %q, want %q", gotPath, tc.wantPath)
			}
			if gotID != tc.wantIntegration {
				t.Errorf("integrationID: got %q, want %q", gotID, tc.wantIntegration)
			}
		})
	}
}

func TestGHModelsClient_CopilotEndpoint(t *testing.T) {
	var capturedPath string
	var capturedIntegrationID string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedIntegrationID = r.Header.Get("Copilot-Integration-Id")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"choices": [
				{
					"message": {
						"role": "assistant",
						"content": "Haiku summary of the update."
					}
				}
			]
		}`)
	}))
	defer server.Close()

	// Build a base URL that contains "githubcopilot.com" so that endpointProfileFor
	// selects the Copilot profile, while still pointing at the local httptest server.
	// We embed the magic substring as a path segment, which the server ignores.
	baseURL := server.URL + "/githubcopilot.com"

	client := NewGHModelsClient(baseURL, "claude-haiku-4.5", "test-token", "", 0)

	result, err := client.Summarize(
		context.Background(),
		"My Issue",
		"https://github.com/org/repo/issues/1",
		"Some update text.",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "Haiku summary of the update." {
		t.Errorf("unexpected result: %q", result)
	}
	if capturedPath != "/githubcopilot.com/chat/completions" {
		t.Errorf("expected Copilot chat path, got %q", capturedPath)
	}
	if capturedIntegrationID != "vscode-chat" {
		t.Errorf("expected Copilot-Integration-Id header to be %q, got %q", "vscode-chat", capturedIntegrationID)
	}
}

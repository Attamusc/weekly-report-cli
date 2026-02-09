package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGHModelsClient_SummarizeBatch(t *testing.T) {
	tests := []struct {
		name           string
		items          []BatchItem
		responseBody   string // Raw JSON response the mock server returns
		expectError    bool
		validateResult func(t *testing.T, result map[string]BatchResult)
	}{
		{
			name: "successful batch with nested JSON response",
			items: []BatchItem{
				{
					IssueURL:       "https://github.com/org/repo/issues/1",
					IssueTitle:     "Feature A",
					UpdateTexts:    []string{"Implemented feature A"},
					ReportedStatus: "On Track",
				},
				{
					IssueURL:       "https://github.com/org/repo/issues/2",
					IssueTitle:     "Bug B",
					UpdateTexts:    []string{"Fixed bug B"},
					ReportedStatus: "On Track",
				},
			},
			responseBody: `{
				"https://github.com/org/repo/issues/1": {
					"summary": "Team implemented feature A successfully",
					"sentiment": null
				},
				"https://github.com/org/repo/issues/2": {
					"summary": "Team fixed bug B in the system",
					"sentiment": {
						"status": "at_risk",
						"explanation": "Update mentions remaining test failures."
					}
				}
			}`,
			expectError: false,
			validateResult: func(t *testing.T, result map[string]BatchResult) {
				if len(result) != 2 {
					t.Errorf("Expected 2 results, got %d", len(result))
				}
				r1 := result["https://github.com/org/repo/issues/1"]
				if !strings.Contains(r1.Summary, "feature A") {
					t.Errorf("Summary for issue 1 doesn't mention feature A: %s", r1.Summary)
				}
				if r1.Sentiment != nil {
					t.Errorf("Expected nil sentiment for issue 1, got %+v", r1.Sentiment)
				}
				r2 := result["https://github.com/org/repo/issues/2"]
				if r2.Sentiment == nil {
					t.Fatalf("Expected non-nil sentiment for issue 2")
				}
				if r2.Sentiment.SuggestedStatus != "at_risk" {
					t.Errorf("Expected suggested status at_risk, got %s", r2.Sentiment.SuggestedStatus)
				}
			},
		},
		{
			name:         "empty batch",
			items:        []BatchItem{},
			responseBody: `{}`,
			expectError:  false,
			validateResult: func(t *testing.T, result map[string]BatchResult) {
				if len(result) != 0 {
					t.Errorf("Expected empty result for empty batch, got %d items", len(result))
				}
			},
		},
		{
			name: "single item batch",
			items: []BatchItem{
				{
					IssueURL:       "https://github.com/org/repo/issues/1",
					IssueTitle:     "Feature A",
					UpdateTexts:    []string{"Implemented feature A", "Added tests"},
					ReportedStatus: "On Track",
				},
			},
			responseBody: `{
				"https://github.com/org/repo/issues/1": {
					"summary": "Team implemented feature A and added tests",
					"sentiment": null
				}
			}`,
			expectError: false,
			validateResult: func(t *testing.T, result map[string]BatchResult) {
				if len(result) != 1 {
					t.Errorf("Expected 1 result, got %d", len(result))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request structure
				var req chatCompletionRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Fatalf("Failed to decode request: %v", err)
				}

				// Verify batch system prompt is used (if items > 0)
				if len(tt.items) > 0 && !strings.Contains(req.Messages[0].Content, "batch") {
					t.Errorf("Expected batch system prompt, got: %s", req.Messages[0].Content)
				}

				// Return mock response
				response := chatCompletionResponse{
					Choices: []choice{
						{
							Message: message{
								Role:    "assistant",
								Content: tt.responseBody,
							},
						},
					},
				}
				_ = json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()

			// Create client
			client := NewGHModelsClient(server.URL, "test-model", "test-token", "", 0)

			// Call SummarizeBatch
			result, err := client.SummarizeBatch(context.Background(), tt.items)

			// Check error expectation
			if tt.expectError && err == nil {
				t.Errorf("Expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// Validate result
			if tt.validateResult != nil {
				tt.validateResult(t, result)
			}
		})
	}
}

func TestGHModelsClient_buildBatchPrompt(t *testing.T) {
	client := NewGHModelsClient("http://test", "test-model", "test-token", "", 0)

	items := []BatchItem{
		{
			IssueURL:       "https://github.com/org/repo/issues/1",
			IssueTitle:     "Feature A",
			UpdateTexts:    []string{"Update 1", "Update 2"},
			ReportedStatus: "On Track",
		},
		{
			IssueURL:       "https://github.com/org/repo/issues/2",
			IssueTitle:     "Bug B",
			UpdateTexts:    []string{"Fixed the bug"},
			ReportedStatus: "At Risk",
		},
	}

	prompt, err := client.buildBatchPrompt(items)
	if err != nil {
		t.Fatalf("buildBatchPrompt failed: %v", err)
	}

	// Verify it's valid JSON
	var batchReq batchRequest
	if err := json.Unmarshal([]byte(prompt), &batchReq); err != nil {
		t.Fatalf("Prompt is not valid JSON: %v", err)
	}

	// Verify structure
	if len(batchReq.Items) != 2 {
		t.Errorf("Expected 2 items in batch request, got %d", len(batchReq.Items))
	}

	if batchReq.Items[0].ID != "https://github.com/org/repo/issues/1" {
		t.Errorf("Expected first item ID to be issue URL, got %s", batchReq.Items[0].ID)
	}

	if len(batchReq.Items[0].Updates) != 2 {
		t.Errorf("Expected first item to have 2 updates, got %d", len(batchReq.Items[0].Updates))
	}
}

func TestBuildBatchPrompt_IncludesReportedStatus(t *testing.T) {
	client := NewGHModelsClient("http://test", "test-model", "test-token", "", 0)

	items := []BatchItem{
		{
			IssueURL:       "https://github.com/org/repo/issues/1",
			IssueTitle:     "Feature A",
			UpdateTexts:    []string{"Update 1"},
			ReportedStatus: "On Track",
		},
	}

	prompt, err := client.buildBatchPrompt(items)
	if err != nil {
		t.Fatalf("buildBatchPrompt failed: %v", err)
	}

	// Verify reported_status appears in JSON
	if !strings.Contains(prompt, `"reported_status"`) {
		t.Errorf("Expected prompt to contain reported_status field, got: %s", prompt)
	}
	if !strings.Contains(prompt, "On Track") {
		t.Errorf("Expected prompt to contain 'On Track' value, got: %s", prompt)
	}

	// Verify it deserializes correctly
	var batchReq batchRequest
	if err := json.Unmarshal([]byte(prompt), &batchReq); err != nil {
		t.Fatalf("Prompt is not valid JSON: %v", err)
	}

	if batchReq.Items[0].ReportedStatus != "On Track" {
		t.Errorf("Expected reported_status 'On Track', got %q", batchReq.Items[0].ReportedStatus)
	}
}

func TestGHModelsClient_parseBatchResponse(t *testing.T) {
	client := NewGHModelsClient("http://test", "test-model", "test-token", "", 0)

	items := []BatchItem{
		{IssueURL: "https://github.com/org/repo/issues/1", IssueTitle: "Feature A"},
		{IssueURL: "https://github.com/org/repo/issues/2", IssueTitle: "Bug B"},
	}

	t.Run("valid JSON response with flat format", func(t *testing.T) {
		jsonResponse := `{
			"https://github.com/org/repo/issues/1": "Summary for feature A",
			"https://github.com/org/repo/issues/2": "Summary for bug B"
		}`

		result, err := client.parseBatchResponse(jsonResponse, items)
		if err != nil {
			t.Fatalf("parseBatchResponse failed: %v", err)
		}

		if len(result) != 2 {
			t.Errorf("Expected 2 results, got %d", len(result))
		}

		r1 := result["https://github.com/org/repo/issues/1"]
		if r1.Summary != "Summary for feature A" {
			t.Errorf("Unexpected summary for issue 1: %q", r1.Summary)
		}
		if r1.Sentiment != nil {
			t.Errorf("Expected nil sentiment for flat format, got %+v", r1.Sentiment)
		}
	})

	t.Run("invalid JSON triggers markdown fallback", func(t *testing.T) {
		// This should trigger markdown parsing fallback
		invalidJSON := "This is not JSON"

		result, err := client.parseBatchResponse(invalidJSON, items)
		// Should return error since markdown parsing will also fail
		if err == nil {
			t.Errorf("Expected error for invalid response format, got nil")
		}
		_ = result
	})
}

func TestParseBatchResponse_NestedFormat(t *testing.T) {
	client := NewGHModelsClient("http://test", "test-model", "test-token", "", 0)

	items := []BatchItem{
		{IssueURL: "https://github.com/org/repo/issues/1", IssueTitle: "Feature A"},
		{IssueURL: "https://github.com/org/repo/issues/2", IssueTitle: "Bug B"},
	}

	response := `{
		"https://github.com/org/repo/issues/1": {
			"summary": "Team completed feature A ahead of schedule.",
			"sentiment": null
		},
		"https://github.com/org/repo/issues/2": {
			"summary": "Bug fix is in progress but blocked on upstream dependency.",
			"sentiment": {
				"status": "off_track",
				"explanation": "Update describes a blocking dependency but status was reported as On Track."
			}
		}
	}`

	result, err := client.parseBatchResponse(response, items)
	if err != nil {
		t.Fatalf("parseBatchResponse failed: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(result))
	}

	// Issue 1: no sentiment
	r1 := result["https://github.com/org/repo/issues/1"]
	if r1.Summary != "Team completed feature A ahead of schedule." {
		t.Errorf("Unexpected summary: %q", r1.Summary)
	}
	if r1.Sentiment != nil {
		t.Errorf("Expected nil sentiment, got %+v", r1.Sentiment)
	}

	// Issue 2: with sentiment
	r2 := result["https://github.com/org/repo/issues/2"]
	if r2.Sentiment == nil {
		t.Fatalf("Expected non-nil sentiment for issue 2")
	}
	if r2.Sentiment.SuggestedStatus != "off_track" {
		t.Errorf("Expected suggested status off_track, got %q", r2.Sentiment.SuggestedStatus)
	}
	if !strings.Contains(r2.Sentiment.Explanation, "blocking dependency") {
		t.Errorf("Expected explanation to mention blocking dependency, got %q", r2.Sentiment.Explanation)
	}
}

func TestParseBatchResponse_FlatFormatFallback(t *testing.T) {
	client := NewGHModelsClient("http://test", "test-model", "test-token", "", 0)

	items := []BatchItem{
		{IssueURL: "https://github.com/org/repo/issues/1", IssueTitle: "Feature A"},
		{IssueURL: "https://github.com/org/repo/issues/2", IssueTitle: "Bug B"},
	}

	// Old flat format: {"url": "summary string"}
	response := `{
		"https://github.com/org/repo/issues/1": "Legacy summary for feature A",
		"https://github.com/org/repo/issues/2": "Legacy summary for bug B"
	}`

	result, err := client.parseBatchResponse(response, items)
	if err != nil {
		t.Fatalf("parseBatchResponse failed: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(result))
	}

	r1 := result["https://github.com/org/repo/issues/1"]
	if r1.Summary != "Legacy summary for feature A" {
		t.Errorf("Unexpected summary: %q", r1.Summary)
	}
	if r1.Sentiment != nil {
		t.Errorf("Expected nil sentiment for flat format fallback, got %+v", r1.Sentiment)
	}

	r2 := result["https://github.com/org/repo/issues/2"]
	if r2.Summary != "Legacy summary for bug B" {
		t.Errorf("Unexpected summary: %q", r2.Summary)
	}
	if r2.Sentiment != nil {
		t.Errorf("Expected nil sentiment for flat format fallback, got %+v", r2.Sentiment)
	}
}

func TestParseBatchResponse_MarkdownFallback(t *testing.T) {
	client := NewGHModelsClient("http://test", "test-model", "test-token", "", 0)

	items := []BatchItem{
		{IssueURL: "https://github.com/org/repo/issues/1", IssueTitle: "Feature A"},
		{IssueURL: "https://github.com/org/repo/issues/2", IssueTitle: "Bug B"},
	}

	// Markdown format that some models might return
	response := `## SUMMARY https://github.com/org/repo/issues/1
Markdown summary for feature A.

## SUMMARY https://github.com/org/repo/issues/2
Markdown summary for bug B.`

	result, err := client.parseBatchResponse(response, items)
	if err != nil {
		t.Fatalf("parseBatchResponse failed: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(result))
	}

	r1 := result["https://github.com/org/repo/issues/1"]
	if !strings.Contains(r1.Summary, "feature A") {
		t.Errorf("Expected summary to mention feature A, got %q", r1.Summary)
	}
	if r1.Sentiment != nil {
		t.Errorf("Expected nil sentiment for markdown fallback, got %+v", r1.Sentiment)
	}
}

func TestParseBatchResponse_MixedSentiment(t *testing.T) {
	client := NewGHModelsClient("http://test", "test-model", "test-token", "", 0)

	items := []BatchItem{
		{IssueURL: "https://github.com/org/repo/issues/1", IssueTitle: "Feature A"},
		{IssueURL: "https://github.com/org/repo/issues/2", IssueTitle: "Bug B"},
		{IssueURL: "https://github.com/org/repo/issues/3", IssueTitle: "Task C"},
	}

	// Some items have sentiment, others null
	response := `{
		"https://github.com/org/repo/issues/1": {
			"summary": "Feature A is on track.",
			"sentiment": null
		},
		"https://github.com/org/repo/issues/2": {
			"summary": "Bug B has unresolved blockers.",
			"sentiment": {
				"status": "at_risk",
				"explanation": "Mentions two unresolved blockers."
			}
		},
		"https://github.com/org/repo/issues/3": {
			"summary": "Task C not yet started.",
			"sentiment": null
		}
	}`

	result, err := client.parseBatchResponse(response, items)
	if err != nil {
		t.Fatalf("parseBatchResponse failed: %v", err)
	}

	if len(result) != 3 {
		t.Fatalf("Expected 3 results, got %d", len(result))
	}

	// Issue 1: no sentiment
	if result["https://github.com/org/repo/issues/1"].Sentiment != nil {
		t.Errorf("Expected nil sentiment for issue 1")
	}

	// Issue 2: has sentiment
	s2 := result["https://github.com/org/repo/issues/2"].Sentiment
	if s2 == nil {
		t.Fatalf("Expected non-nil sentiment for issue 2")
	}
	if s2.SuggestedStatus != "at_risk" {
		t.Errorf("Expected at_risk, got %q", s2.SuggestedStatus)
	}
	if s2.Explanation != "Mentions two unresolved blockers." {
		t.Errorf("Unexpected explanation: %q", s2.Explanation)
	}

	// Issue 3: no sentiment
	if result["https://github.com/org/repo/issues/3"].Sentiment != nil {
		t.Errorf("Expected nil sentiment for issue 3")
	}
}

func TestNoopSummarizer_SummarizeBatch(t *testing.T) {
	summarizer := NewNoopSummarizer()

	items := []BatchItem{
		{
			IssueURL:       "https://github.com/org/repo/issues/1",
			IssueTitle:     "Feature A",
			UpdateTexts:    []string{"Update 1", "Update 2"},
			ReportedStatus: "On Track",
		},
		{
			IssueURL:       "https://github.com/org/repo/issues/2",
			IssueTitle:     "Bug B",
			UpdateTexts:    []string{"Fixed the bug"},
			ReportedStatus: "At Risk",
		},
		{
			IssueURL:       "https://github.com/org/repo/issues/3",
			IssueTitle:     "Empty updates",
			UpdateTexts:    []string{},
			ReportedStatus: "Unknown",
		},
	}

	result, err := summarizer.SummarizeBatch(context.Background(), items)
	if err != nil {
		t.Fatalf("SummarizeBatch failed: %v", err)
	}

	if len(result) != 3 {
		t.Errorf("Expected 3 results, got %d", len(result))
	}

	r1 := result["https://github.com/org/repo/issues/1"]
	if r1.Summary != "Update 1 Update 2" {
		t.Errorf("Expected joined updates, got %q", r1.Summary)
	}
	if r1.Sentiment != nil {
		t.Errorf("Expected nil sentiment from NoopSummarizer, got %+v", r1.Sentiment)
	}

	r3 := result["https://github.com/org/repo/issues/3"]
	if r3.Summary != "" {
		t.Errorf("Expected empty string for empty updates, got %q", r3.Summary)
	}
	if r3.Sentiment != nil {
		t.Errorf("Expected nil sentiment from NoopSummarizer, got %+v", r3.Sentiment)
	}
}

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
		responseJSON   map[string]string
		expectError    bool
		validateResult func(t *testing.T, result map[string]string)
	}{
		{
			name: "successful batch with JSON response",
			items: []BatchItem{
				{
					IssueURL:    "https://github.com/org/repo/issues/1",
					IssueTitle:  "Feature A",
					UpdateTexts: []string{"Implemented feature A"},
				},
				{
					IssueURL:    "https://github.com/org/repo/issues/2",
					IssueTitle:  "Bug B",
					UpdateTexts: []string{"Fixed bug B"},
				},
			},
			responseJSON: map[string]string{
				"https://github.com/org/repo/issues/1": "Team implemented feature A successfully",
				"https://github.com/org/repo/issues/2": "Team fixed bug B in the system",
			},
			expectError: false,
			validateResult: func(t *testing.T, result map[string]string) {
				if len(result) != 2 {
					t.Errorf("Expected 2 summaries, got %d", len(result))
				}
				if !strings.Contains(result["https://github.com/org/repo/issues/1"], "feature A") {
					t.Errorf("Summary for issue 1 doesn't mention feature A")
				}
			},
		},
		{
			name:         "empty batch",
			items:        []BatchItem{},
			responseJSON: map[string]string{},
			expectError:  false,
			validateResult: func(t *testing.T, result map[string]string) {
				if len(result) != 0 {
					t.Errorf("Expected empty result for empty batch, got %d items", len(result))
				}
			},
		},
		{
			name: "single item batch",
			items: []BatchItem{
				{
					IssueURL:    "https://github.com/org/repo/issues/1",
					IssueTitle:  "Feature A",
					UpdateTexts: []string{"Implemented feature A", "Added tests"},
				},
			},
			responseJSON: map[string]string{
				"https://github.com/org/repo/issues/1": "Team implemented feature A and added tests",
			},
			expectError: false,
			validateResult: func(t *testing.T, result map[string]string) {
				if len(result) != 1 {
					t.Errorf("Expected 1 summary, got %d", len(result))
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
				responseJSON, _ := json.Marshal(tt.responseJSON)
				response := chatCompletionResponse{
					Choices: []choice{
						{
							Message: message{
								Role:    "assistant",
								Content: string(responseJSON),
							},
						},
					},
				}
				json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()

			// Create client
			client := NewGHModelsClient(server.URL, "test-model", "test-token", "")

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
	client := NewGHModelsClient("http://test", "test-model", "test-token", "")

	items := []BatchItem{
		{
			IssueURL:    "https://github.com/org/repo/issues/1",
			IssueTitle:  "Feature A",
			UpdateTexts: []string{"Update 1", "Update 2"},
		},
		{
			IssueURL:    "https://github.com/org/repo/issues/2",
			IssueTitle:  "Bug B",
			UpdateTexts: []string{"Fixed the bug"},
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

func TestGHModelsClient_parseBatchResponse(t *testing.T) {
	client := NewGHModelsClient("http://test", "test-model", "test-token", "")

	items := []BatchItem{
		{IssueURL: "https://github.com/org/repo/issues/1", IssueTitle: "Feature A"},
		{IssueURL: "https://github.com/org/repo/issues/2", IssueTitle: "Bug B"},
	}

	t.Run("valid JSON response", func(t *testing.T) {
		jsonResponse := `{
			"https://github.com/org/repo/issues/1": "Summary for feature A",
			"https://github.com/org/repo/issues/2": "Summary for bug B"
		}`

		result, err := client.parseBatchResponse(jsonResponse, items)
		if err != nil {
			t.Fatalf("parseBatchResponse failed: %v", err)
		}

		if len(result) != 2 {
			t.Errorf("Expected 2 summaries, got %d", len(result))
		}

		if result["https://github.com/org/repo/issues/1"] != "Summary for feature A" {
			t.Errorf("Unexpected summary for issue 1")
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

func TestNoopSummarizer_SummarizeBatch(t *testing.T) {
	summarizer := NewNoopSummarizer()

	items := []BatchItem{
		{
			IssueURL:    "https://github.com/org/repo/issues/1",
			IssueTitle:  "Feature A",
			UpdateTexts: []string{"Update 1", "Update 2"},
		},
		{
			IssueURL:    "https://github.com/org/repo/issues/2",
			IssueTitle:  "Bug B",
			UpdateTexts: []string{"Fixed the bug"},
		},
		{
			IssueURL:    "https://github.com/org/repo/issues/3",
			IssueTitle:  "Empty updates",
			UpdateTexts: []string{},
		},
	}

	result, err := summarizer.SummarizeBatch(context.Background(), items)
	if err != nil {
		t.Fatalf("SummarizeBatch failed: %v", err)
	}

	if len(result) != 3 {
		t.Errorf("Expected 3 results, got %d", len(result))
	}

	if result["https://github.com/org/repo/issues/1"] != "Update 1 Update 2" {
		t.Errorf("Expected joined updates, got %s", result["https://github.com/org/repo/issues/1"])
	}

	if result["https://github.com/org/repo/issues/3"] != "" {
		t.Errorf("Expected empty string for empty updates, got %s", result["https://github.com/org/repo/issues/3"])
	}
}

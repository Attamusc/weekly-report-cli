package projects

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClient_FetchProjectItems_OrgProject(t *testing.T) {
	// Create mock GraphQL server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request headers
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("expected Authorization header, got %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		// Parse request body
		var req graphQLRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		// Verify query contains "organization"
		if !strings.Contains(req.Query, "organization") {
			t.Errorf("expected query to contain 'organization'")
		}

		// Return mock response
		response := graphQLResponse{
			Data: &projectData{
				Organization: &projectV2Wrapper{
					ProjectV2: &projectV2{
						ID:    "PVT_123",
						Title: "Test Project",
						Fields: projectFields{
							Nodes: []projectField{
								{ID: "F1", Name: "Status"},
							},
						},
						Items: projectItems{
							Nodes: []projectItemNode{
								{
									ID:   "ITEM1",
									Type: "ISSUE",
									Content: &projectItemContent{
										ID:     "I1",
										Number: intPtr(123),
										URL:    "https://github.com/test/repo/issues/123",
										Repository: &contentRepository{
											Owner: repositoryOwner{Login: "test"},
											Name:  "repo",
										},
									},
									FieldValues: projectFieldValues{
										Nodes: []projectFieldValueNode{
											{
												Field: &projectFieldRef{Name: "Status"},
												Name:  stringPtr("In Progress"),
											},
										},
									},
								},
							},
							PageInfo: pageInfo{
								HasNextPage: false,
								EndCursor:   nil,
							},
						},
					},
				},
			},
		}

		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Create client
	client := NewClient("test-token")
	client.baseURL = server.URL

	// Create config
	ref, _ := ParseProjectURL("org:test-org/5")
	config := ProjectConfig{
		Ref:      ref,
		MaxItems: 100,
	}

	// Fetch items
	items, err := client.FetchProjectItems(context.Background(), config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify results
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	item := items[0]
	if item.ContentType != ContentTypeIssue {
		t.Errorf("expected ContentTypeIssue, got %v", item.ContentType)
	}
	if item.IssueRef == nil {
		t.Fatalf("expected IssueRef to be set")
	}
	if item.IssueRef.Number != 123 {
		t.Errorf("expected issue number 123, got %d", item.IssueRef.Number)
	}

	// Verify field value
	if val, ok := item.FieldValues["Status"]; ok {
		if val.Type != FieldTypeSingleSelect {
			t.Errorf("expected FieldTypeSingleSelect, got %v", val.Type)
		}
		if val.Text != "In Progress" {
			t.Errorf("expected 'In Progress', got %s", val.Text)
		}
	} else {
		t.Errorf("expected Status field to be present")
	}
}

func TestClient_FetchProjectItems_UserProject(t *testing.T) {
	// Create mock GraphQL server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse request
		var req graphQLRequest
		json.NewDecoder(r.Body).Decode(&req)

		// Verify query contains "user"
		if !strings.Contains(req.Query, "user") {
			t.Errorf("expected query to contain 'user'")
		}

		// Return mock response
		response := graphQLResponse{
			Data: &projectData{
				User: &projectV2Wrapper{
					ProjectV2: &projectV2{
						ID:    "PVT_456",
						Title: "User Project",
						Items: projectItems{
							Nodes: []projectItemNode{},
							PageInfo: pageInfo{
								HasNextPage: false,
							},
						},
					},
				},
			},
		}

		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient("test-token")
	client.baseURL = server.URL

	ref, _ := ParseProjectURL("user:johndoe/10")
	config := ProjectConfig{
		Ref:      ref,
		MaxItems: 100,
	}

	items, err := client.FetchProjectItems(context.Background(), config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(items))
	}
}

func TestClient_FetchProjectItems_Pagination(t *testing.T) {
	requestCount := 0
	endCursor1 := "cursor1"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req graphQLRequest
		json.NewDecoder(r.Body).Decode(&req)

		requestCount++
		var response graphQLResponse

		if requestCount == 1 {
			// First page
			if req.Variables["cursor"] != nil {
				t.Errorf("first request should not have cursor")
			}

			response = graphQLResponse{
				Data: &projectData{
					Organization: &projectV2Wrapper{
						ProjectV2: &projectV2{
							ID:    "PVT_123",
							Title: "Test Project",
							Items: projectItems{
								Nodes: []projectItemNode{
									{
										ID:   "ITEM1",
										Type: "ISSUE",
										Content: &projectItemContent{
											ID:     "I1",
											Number: intPtr(1),
											URL:    "https://github.com/test/repo/issues/1",
											Repository: &contentRepository{
												Owner: repositoryOwner{Login: "test"},
												Name:  "repo",
											},
										},
									},
								},
								PageInfo: pageInfo{
									HasNextPage: true,
									EndCursor:   &endCursor1,
								},
							},
						},
					},
				},
			}
		} else {
			// Second page
			if req.Variables["cursor"] != endCursor1 {
				t.Errorf("second request should have cursor=%s, got %v", endCursor1, req.Variables["cursor"])
			}

			response = graphQLResponse{
				Data: &projectData{
					Organization: &projectV2Wrapper{
						ProjectV2: &projectV2{
							ID:    "PVT_123",
							Title: "Test Project",
							Items: projectItems{
								Nodes: []projectItemNode{
									{
										ID:   "ITEM2",
										Type: "ISSUE",
										Content: &projectItemContent{
											ID:     "I2",
											Number: intPtr(2),
											URL:    "https://github.com/test/repo/issues/2",
											Repository: &contentRepository{
												Owner: repositoryOwner{Login: "test"},
												Name:  "repo",
											},
										},
									},
								},
								PageInfo: pageInfo{
									HasNextPage: false,
									EndCursor:   nil,
								},
							},
						},
					},
				},
			}
		}

		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient("test-token")
	client.baseURL = server.URL

	ref, _ := ParseProjectURL("org:test-org/5")
	config := ProjectConfig{
		Ref:      ref,
		MaxItems: 200, // Allow pagination
	}

	items, err := client.FetchProjectItems(context.Background(), config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should fetch both pages
	if len(items) != 2 {
		t.Fatalf("expected 2 items (from 2 pages), got %d", len(items))
	}

	if requestCount != 2 {
		t.Errorf("expected 2 requests, got %d", requestCount)
	}
}

func TestClient_FetchProjectItems_MaxItemsLimit(t *testing.T) {
	requestCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		endCursor := "cursor1"

		response := graphQLResponse{
			Data: &projectData{
				Organization: &projectV2Wrapper{
					ProjectV2: &projectV2{
						ID:    "PVT_123",
						Title: "Test Project",
						Items: projectItems{
							Nodes: []projectItemNode{
								{
									ID:   "ITEM1",
									Type: "ISSUE",
									Content: &projectItemContent{
										ID:     "I1",
										Number: intPtr(1),
										URL:    "https://github.com/test/repo/issues/1",
										Repository: &contentRepository{
											Owner: repositoryOwner{Login: "test"},
											Name:  "repo",
										},
									},
								},
							},
							PageInfo: pageInfo{
								HasNextPage: true,
								EndCursor:   &endCursor,
							},
						},
					},
				},
			},
		}

		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient("test-token")
	client.baseURL = server.URL

	ref, _ := ParseProjectURL("org:test-org/5")
	config := ProjectConfig{
		Ref:      ref,
		MaxItems: 1, // Limit to 1 item
	}

	items, err := client.FetchProjectItems(context.Background(), config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should only fetch 1 item despite hasNextPage=true
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	if requestCount != 1 {
		t.Errorf("expected 1 request, got %d", requestCount)
	}
}

func TestClient_FetchProjectItems_EmptyProject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := graphQLResponse{
			Data: &projectData{
				Organization: &projectV2Wrapper{
					ProjectV2: &projectV2{
						ID:    "PVT_123",
						Title: "Empty Project",
						Items: projectItems{
							Nodes: []projectItemNode{},
							PageInfo: pageInfo{
								HasNextPage: false,
							},
						},
					},
				},
			},
		}

		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient("test-token")
	client.baseURL = server.URL

	ref, _ := ParseProjectURL("org:test-org/5")
	config := ProjectConfig{
		Ref:      ref,
		MaxItems: 100,
	}

	items, err := client.FetchProjectItems(context.Background(), config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(items))
	}
}

func TestClient_FetchProjectItems_AuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message": "Bad credentials"}`))
	}))
	defer server.Close()

	client := NewClient("invalid-token")
	client.baseURL = server.URL

	ref, _ := ParseProjectURL("org:test-org/5")
	config := ProjectConfig{
		Ref:      ref,
		MaxItems: 100,
	}

	_, err := client.FetchProjectItems(context.Background(), config)
	if err == nil {
		t.Fatal("expected error for 401 response")
	}

	if !strings.Contains(err.Error(), "authentication failed") {
		t.Errorf("expected authentication error message, got: %v", err)
	}
}

func TestClient_FetchProjectItems_PermissionError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message": "Resource not accessible"}`))
	}))
	defer server.Close()

	client := NewClient("test-token")
	client.baseURL = server.URL

	ref, _ := ParseProjectURL("org:test-org/5")
	config := ProjectConfig{
		Ref:      ref,
		MaxItems: 100,
	}

	_, err := client.FetchProjectItems(context.Background(), config)
	if err == nil {
		t.Fatal("expected error for 403 response")
	}

	if !strings.Contains(err.Error(), "read:project") {
		t.Errorf("expected permission error message mentioning 'read:project', got: %v", err)
	}
}

func TestClient_FetchProjectItems_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message": "Not Found"}`))
	}))
	defer server.Close()

	client := NewClient("test-token")
	client.baseURL = server.URL

	ref, _ := ParseProjectURL("org:test-org/999")
	config := ProjectConfig{
		Ref:      ref,
		MaxItems: 100,
	}

	_, err := client.FetchProjectItems(context.Background(), config)
	if err == nil {
		t.Fatal("expected error for 404 response")
	}

	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not found error message, got: %v", err)
	}
}

func TestClient_FetchProjectItems_RateLimit(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		if requestCount <= 2 {
			// Return rate limit error for first 2 requests
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"message": "API rate limit exceeded"}`))
			return
		}

		// Success on 3rd attempt
		response := graphQLResponse{
			Data: &projectData{
				Organization: &projectV2Wrapper{
					ProjectV2: &projectV2{
						ID:    "PVT_123",
						Title: "Test Project",
						Items: projectItems{
							Nodes:    []projectItemNode{},
							PageInfo: pageInfo{HasNextPage: false},
						},
					},
				},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient("test-token")
	client.baseURL = server.URL

	ref, _ := ParseProjectURL("org:test-org/5")
	config := ProjectConfig{
		Ref:      ref,
		MaxItems: 100,
	}

	items, err := client.FetchProjectItems(context.Background(), config)
	if err != nil {
		t.Fatalf("unexpected error after retries: %v", err)
	}

	// Should succeed after retries
	if len(items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(items))
	}

	// Should have made 3 requests (2 failures + 1 success)
	if requestCount != 3 {
		t.Errorf("expected 3 requests (with retries), got %d", requestCount)
	}
}

func TestClient_FetchProjectItems_GraphQLErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := graphQLResponse{
			Errors: []graphQLError{
				{Message: "Field 'invalid' doesn't exist on type 'ProjectV2'"},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient("test-token")
	client.baseURL = server.URL

	ref, _ := ParseProjectURL("org:test-org/5")
	config := ProjectConfig{
		Ref:      ref,
		MaxItems: 100,
	}

	_, err := client.FetchProjectItems(context.Background(), config)
	if err == nil {
		t.Fatal("expected error for GraphQL errors")
	}

	if !strings.Contains(err.Error(), "doesn't exist") {
		t.Errorf("expected GraphQL error message, got: %v", err)
	}
}

func TestClient_FetchProjectItems_DifferentFieldTypes(t *testing.T) {
	dateStr := "2025-08-15"
	parsedDate, _ := time.Parse("2006-01-02", dateStr)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := graphQLResponse{
			Data: &projectData{
				Organization: &projectV2Wrapper{
					ProjectV2: &projectV2{
						ID:    "PVT_123",
						Title: "Test Project",
						Items: projectItems{
							Nodes: []projectItemNode{
								{
									ID:   "ITEM1",
									Type: "ISSUE",
									Content: &projectItemContent{
										ID:     "I1",
										Number: intPtr(1),
										URL:    "https://github.com/test/repo/issues/1",
										Repository: &contentRepository{
											Owner: repositoryOwner{Login: "test"},
											Name:  "repo",
										},
									},
									FieldValues: projectFieldValues{
										Nodes: []projectFieldValueNode{
											{
												Field: &projectFieldRef{Name: "Status"},
												Name:  stringPtr("Done"),
											},
											{
												Field: &projectFieldRef{Name: "TargetDate"},
												Date:  stringPtr(dateStr),
											},
											{
												Field:  &projectFieldRef{Name: "Priority"},
												Number: floatPtr(5.0),
											},
											{
												Field: &projectFieldRef{Name: "Notes"},
												Text:  stringPtr("Some notes"),
											},
										},
									},
								},
							},
							PageInfo: pageInfo{HasNextPage: false},
						},
					},
				},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient("test-token")
	client.baseURL = server.URL

	ref, _ := ParseProjectURL("org:test-org/5")
	config := ProjectConfig{
		Ref:      ref,
		MaxItems: 100,
	}

	items, err := client.FetchProjectItems(context.Background(), config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	item := items[0]

	// Check single-select field
	if val, ok := item.FieldValues["Status"]; ok {
		if val.Type != FieldTypeSingleSelect || val.Text != "Done" {
			t.Errorf("expected Status='Done', got Type=%v, Text=%s", val.Type, val.Text)
		}
	} else {
		t.Error("expected Status field")
	}

	// Check date field
	if val, ok := item.FieldValues["TargetDate"]; ok {
		if val.Type != FieldTypeDate {
			t.Errorf("expected FieldTypeDate, got %v", val.Type)
		}
		if val.Date == nil || !val.Date.Equal(parsedDate) {
			t.Errorf("expected date %v, got %v", parsedDate, val.Date)
		}
	} else {
		t.Error("expected TargetDate field")
	}

	// Check number field
	if val, ok := item.FieldValues["Priority"]; ok {
		if val.Type != FieldTypeNumber || val.Number != 5.0 {
			t.Errorf("expected Priority=5.0, got Type=%v, Number=%f", val.Type, val.Number)
		}
	} else {
		t.Error("expected Priority field")
	}

	// Check text field
	if val, ok := item.FieldValues["Notes"]; ok {
		if val.Type != FieldTypeText || val.Text != "Some notes" {
			t.Errorf("expected Notes='Some notes', got Type=%v, Text=%s", val.Type, val.Text)
		}
	} else {
		t.Error("expected Notes field")
	}
}

// Helper functions
func intPtr(i int) *int {
	return &i
}

func stringPtr(s string) *string {
	return &s
}

func floatPtr(f float64) *float64 {
	return &f
}

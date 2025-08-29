package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/Attamusc/weekly-report-cli/internal/input"
	"github.com/google/go-github/v66/github"
)

func TestFetchIssue_Open(t *testing.T) {
	// Create test server that simulates GitHub API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/issues/123" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		// Return mock open issue data
		issue := github.Issue{
			HTMLURL: github.String("https://github.com/owner/repo/issues/123"),
			Title:   github.String("Test Issue Title"),
			State:   github.String("open"),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(issue)
	}))
	defer server.Close()

	// Create GitHub client pointing to test server
	client := github.NewClient(server.Client())
	baseURL, _ := url.Parse(server.URL + "/")
	client.BaseURL = baseURL

	// Test issue reference
	ref := input.IssueRef{
		Owner:  "owner",
		Repo:   "repo",
		Number: 123,
		URL:    "https://github.com/owner/repo/issues/123",
	}

	// Fetch issue
	ctx := context.Background()
	issueData, err := FetchIssue(ctx, client, ref)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify results
	if issueData.URL != "https://github.com/owner/repo/issues/123" {
		t.Errorf("expected URL https://github.com/owner/repo/issues/123, got %s", issueData.URL)
	}
	if issueData.Title != "Test Issue Title" {
		t.Errorf("expected title 'Test Issue Title', got %s", issueData.Title)
	}
	if issueData.State != "open" {
		t.Errorf("expected state 'open', got %s", issueData.State)
	}
	if issueData.ClosedAt != nil {
		t.Errorf("expected ClosedAt to be nil for open issue, got %v", issueData.ClosedAt)
	}
	if issueData.CloseReason != "" {
		t.Errorf("expected CloseReason to be empty for open issue, got %s", issueData.CloseReason)
	}
}

func TestFetchIssue_Closed(t *testing.T) {
	closeTime := time.Date(2025, 8, 15, 12, 30, 0, 0, time.UTC)
	
	// Create test server that simulates GitHub API for closed issue
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/issues/456":
			// Return mock closed issue data
			issue := github.Issue{
				HTMLURL:  github.String("https://github.com/owner/repo/issues/456"),
				Title:    github.String("Closed Test Issue"),
				State:    github.String("closed"),
				ClosedAt: &github.Timestamp{Time: closeTime},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(issue)

		case "/repos/owner/repo/issues/456/events":
			// Mock close event
			events := []github.IssueEvent{
				{
					Event:     github.String("closed"),
					CreatedAt: &github.Timestamp{Time: closeTime},
					Actor: &github.User{
						Login: github.String("testuser"),
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(events)

		case "/repos/owner/repo/issues/456/comments":
			// Mock closing comment
			comments := []github.IssueComment{
				{
					Body:      github.String("Closing this issue as it's completed."),
					CreatedAt: &github.Timestamp{Time: closeTime.Add(-30 * time.Second)},
					User: &github.User{
						Login: github.String("testuser"),
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(comments)

		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Create GitHub client pointing to test server
	client := github.NewClient(server.Client())
	baseURL, _ := url.Parse(server.URL + "/")
	client.BaseURL = baseURL

	// Test issue reference
	ref := input.IssueRef{
		Owner:  "owner",
		Repo:   "repo",
		Number: 456,
		URL:    "https://github.com/owner/repo/issues/456",
	}

	// Fetch issue
	ctx := context.Background()
	issueData, err := FetchIssue(ctx, client, ref)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify results
	if issueData.URL != "https://github.com/owner/repo/issues/456" {
		t.Errorf("expected URL https://github.com/owner/repo/issues/456, got %s", issueData.URL)
	}
	if issueData.Title != "Closed Test Issue" {
		t.Errorf("expected title 'Closed Test Issue', got %s", issueData.Title)
	}
	if issueData.State != "closed" {
		t.Errorf("expected state 'closed', got %s", issueData.State)
	}
	if issueData.ClosedAt == nil {
		t.Error("expected ClosedAt to be set for closed issue")
	} else if !issueData.ClosedAt.Equal(closeTime) {
		t.Errorf("expected ClosedAt %v, got %v", closeTime, *issueData.ClosedAt)
	}
	if issueData.CloseReason != "Closing this issue as it's completed." {
		t.Errorf("expected CloseReason 'Closing this issue as it's completed.', got %s", issueData.CloseReason)
	}
}

func TestFetchCommentsSince(t *testing.T) {
	// Create test time window
	baseTime := time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC)
	sinceTime := baseTime.Add(24 * time.Hour) // 2025-08-02

	// Create test server with paginated comments
	var requestCount int
	var server *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		if r.URL.Path != "/repos/owner/repo/issues/123/comments" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		// Check for since parameter
		since := r.URL.Query().Get("since")
		if since == "" {
			t.Error("expected since parameter")
		}

		page := r.URL.Query().Get("page")
		var comments []github.IssueComment

		switch page {
		case "", "1":
			// First page - mix of comments before and after since time
			comments = []github.IssueComment{
				{
					Body:      github.String("Comment before window"),
					CreatedAt: &github.Timestamp{Time: baseTime}, // Before since time
					User:      &github.User{Login: github.String("user1")},
					HTMLURL:   github.String("https://github.com/owner/repo/issues/123#issuecomment-1"),
				},
				{
					Body:      github.String("Comment in window"),
					CreatedAt: &github.Timestamp{Time: sinceTime.Add(1 * time.Hour)}, // After since time
					User:      &github.User{Login: github.String("user2")},
					HTMLURL:   github.String("https://github.com/owner/repo/issues/123#issuecomment-2"),
				},
			}
			// Set pagination header for next page
			w.Header().Set("Link", `</repos/owner/repo/issues/123/comments?page=2>; rel="next"`)

		case "2":
			// Second page - more comments in window
			comments = []github.IssueComment{
				{
					Body:      github.String("Another comment in window"),
					CreatedAt: &github.Timestamp{Time: sinceTime.Add(2 * time.Hour)},
					User:      &github.User{Login: github.String("user3")},
					HTMLURL:   github.String("https://github.com/owner/repo/issues/123#issuecomment-3"),
				},
			}
			// No next page
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(comments)
	}))
	defer server.Close()

	// Create GitHub client pointing to test server
	client := github.NewClient(server.Client())
	baseURL, _ := url.Parse(server.URL + "/")
	client.BaseURL = baseURL

	// Test issue reference
	ref := input.IssueRef{
		Owner:  "owner",
		Repo:   "repo",
		Number: 123,
		URL:    "https://github.com/owner/repo/issues/123",
	}

	// Fetch comments
	ctx := context.Background()
	comments, err := FetchCommentsSince(ctx, client, ref, sinceTime)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify pagination occurred
	if requestCount != 2 {
		t.Errorf("expected 2 requests for pagination, got %d", requestCount)
	}

	// Should have filtered out the comment before since time
	if len(comments) != 2 {
		t.Fatalf("expected 2 comments after filtering, got %d", len(comments))
	}

	// Verify first comment
	if comments[0].Body != "Comment in window" {
		t.Errorf("expected first comment body 'Comment in window', got %s", comments[0].Body)
	}
	if comments[0].Author != "user2" {
		t.Errorf("expected first comment author 'user2', got %s", comments[0].Author)
	}

	// Verify second comment
	if comments[1].Body != "Another comment in window" {
		t.Errorf("expected second comment body 'Another comment in window', got %s", comments[1].Body)
	}
	if comments[1].Author != "user3" {
		t.Errorf("expected second comment author 'user3', got %s", comments[1].Author)
	}

	// Verify time filtering worked
	for i, comment := range comments {
		if comment.CreatedAt.Before(sinceTime) {
			t.Errorf("comment %d should be after since time, got %v", i, comment.CreatedAt)
		}
	}
}

func TestFetchIssue_NotFound(t *testing.T) {
	// Create test server that returns 404
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	// Create GitHub client pointing to test server
	client := github.NewClient(server.Client())
	baseURL, _ := url.Parse(server.URL + "/")
	client.BaseURL = baseURL

	ref := input.IssueRef{Owner: "owner", Repo: "repo", Number: 999}

	ctx := context.Background()
	_, err := FetchIssue(ctx, client, ref)
	if err == nil {
		t.Error("expected error for 404 response")
	}
}

func TestFetchCommentsSince_NoComments(t *testing.T) {
	// Create test server with empty comments
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]github.IssueComment{})
	}))
	defer server.Close()

	client := github.NewClient(server.Client())
	baseURL, _ := url.Parse(server.URL + "/")
	client.BaseURL = baseURL

	ref := input.IssueRef{Owner: "owner", Repo: "repo", Number: 123}
	sinceTime := time.Now().Add(-24 * time.Hour)

	ctx := context.Background()
	comments, err := FetchCommentsSince(ctx, client, ref, sinceTime)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(comments) != 0 {
		t.Errorf("expected 0 comments, got %d", len(comments))
	}
}

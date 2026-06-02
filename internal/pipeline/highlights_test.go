package pipeline

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Attamusc/weekly-report-cli/internal/github"
	"github.com/Attamusc/weekly-report-cli/internal/input"
)

// mockNarrativeFetcher implements IssueFetcher for highlights tests.
type mockNarrativeFetcher struct {
	issue    github.IssueData
	comments []github.Comment
	timeline []github.TimelineEvent
	prData   *github.PullRequestData
	issueErr error
}

func (m *mockNarrativeFetcher) FetchIssue(_ context.Context, _ input.IssueRef) (github.IssueData, error) {
	return m.issue, m.issueErr
}

func (m *mockNarrativeFetcher) FetchCommentsSince(_ context.Context, _ input.IssueRef, _ time.Time) ([]github.Comment, error) {
	return m.comments, nil
}

func (m *mockNarrativeFetcher) FetchTimelineSince(_ context.Context, _ input.IssueRef, _ time.Time) ([]github.TimelineEvent, error) {
	return m.timeline, nil
}

func (m *mockNarrativeFetcher) FetchPullRequest(_ context.Context, _ input.IssueRef) (*github.PullRequestData, error) {
	return m.prData, nil
}

func TestCollectNarrativeItem_FullyPopulated(t *testing.T) {
	now := time.Now()
	since := now.AddDate(0, 0, -7)
	closedAt := now.AddDate(0, 0, -1)
	mergedAt := now.AddDate(0, 0, -2)

	longBody := make([]byte, MaxBodyChars+100)
	for i := range longBody {
		longBody[i] = 'x'
	}

	longComment := make([]byte, MaxCommentChars+50)
	for i := range longComment {
		longComment[i] = 'y'
	}

	ref := input.IssueRef{
		Owner:       "owner",
		Repo:        "repo",
		Number:      1,
		URL:         "https://github.com/owner/repo/pull/1",
		IsPR:        true,
		AuthorLogin: "alice",
	}

	fetcher := &mockNarrativeFetcher{
		issue: github.IssueData{
			URL:       ref.URL,
			Title:     "Great PR",
			State:     "closed",
			Body:      string(longBody),
			Labels:    []string{"enhancement"},
			Assignees: []string{"bob"},
			CreatedAt: since.Add(-24 * time.Hour),
			ClosedAt:  &closedAt,
		},
		comments: []github.Comment{
			{Author: "bob", CreatedAt: since.Add(time.Hour), Body: string(longComment)},
			{Author: "carol", CreatedAt: since.Add(2 * time.Hour), Body: "looks good"},
		},
		timeline: []github.TimelineEvent{
			{Type: "review_requested", Actor: "bob", At: since.Add(30 * time.Minute)},
			{Type: "reviewed", Actor: "bob", At: since.Add(time.Hour), Detail: "approved"},
		},
		prData: &github.PullRequestData{MergedAt: &mergedAt},
	}

	item, err := CollectNarrativeItem(context.Background(), fetcher, ref, since)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Title, state, URL
	if item.Title != "Great PR" {
		t.Errorf("expected title 'Great PR', got %q", item.Title)
	}
	if item.State != "closed" {
		t.Errorf("expected state 'closed', got %q", item.State)
	}
	if item.URL != ref.URL {
		t.Errorf("expected URL %s, got %s", ref.URL, item.URL)
	}

	// Body truncated
	if len(item.Body) != MaxBodyChars {
		t.Errorf("expected body truncated to %d chars, got %d", MaxBodyChars, len(item.Body))
	}

	// IsPR and author
	if !item.IsPR {
		t.Error("expected IsPR=true")
	}
	if item.Author != "alice" {
		t.Errorf("expected author 'alice', got %q", item.Author)
	}

	// Labels and assignees
	if len(item.Labels) != 1 || item.Labels[0] != "enhancement" {
		t.Errorf("expected labels [enhancement], got %v", item.Labels)
	}
	if len(item.Assignees) != 1 || item.Assignees[0] != "bob" {
		t.Errorf("expected assignees [bob], got %v", item.Assignees)
	}

	// Comments: 2 comments, first one truncated
	if len(item.RecentComments) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(item.RecentComments))
	}
	if len(item.RecentComments[0].Body) != MaxCommentChars {
		t.Errorf("expected first comment truncated to %d chars, got %d", MaxCommentChars, len(item.RecentComments[0].Body))
	}
	if item.RecentComments[0].Author != "bob" {
		t.Errorf("expected comment author 'bob', got %q", item.RecentComments[0].Author)
	}
	if item.RecentComments[1].Body != "looks good" {
		t.Errorf("expected second comment body 'looks good', got %q", item.RecentComments[1].Body)
	}

	// Events: 2 events
	if len(item.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(item.Events))
	}
	if item.Events[0].Type != "review_requested" {
		t.Errorf("expected first event 'review_requested', got %q", item.Events[0].Type)
	}
	if item.Events[1].Type != "reviewed" {
		t.Errorf("expected second event 'reviewed', got %q", item.Events[1].Type)
	}
	if item.Events[1].Detail != "approved" {
		t.Errorf("expected event detail 'approved', got %q", item.Events[1].Detail)
	}

	// ClosedThisWeek and MergedThisWeek
	if !item.ClosedThisWeek {
		t.Error("expected ClosedThisWeek=true")
	}
	if item.MergedAt == nil {
		t.Fatal("expected MergedAt to be set")
	}
	if !item.MergedThisWeek {
		t.Error("expected MergedThisWeek=true (mergedAt is within since window)")
	}
}

func TestCollectNarrativeItem_TruncatesComments_KeepsMostRecent(t *testing.T) {
	since := time.Now().AddDate(0, 0, -7)

	comments := make([]github.Comment, MaxComments+3)
	for i := range comments {
		comments[i] = github.Comment{
			Author:    fmt.Sprintf("user%d", i),
			CreatedAt: since.Add(time.Duration(i) * time.Hour),
			Body:      fmt.Sprintf("comment %d", i),
		}
	}

	ref := input.IssueRef{Owner: "owner", Repo: "repo", Number: 1, URL: "https://github.com/owner/repo/issues/1"}
	fetcher := &mockNarrativeFetcher{
		issue: github.IssueData{
			URL:   ref.URL,
			Title: "Many Comments",
			State: "open",
		},
		comments: comments,
	}

	item, err := CollectNarrativeItem(context.Background(), fetcher, ref, since)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(item.RecentComments) != MaxComments {
		t.Errorf("expected %d comments (most recent), got %d", MaxComments, len(item.RecentComments))
	}

	// Should be the last MaxComments (most recent)
	lastAuthor := fmt.Sprintf("user%d", len(comments)-1)
	if item.RecentComments[MaxComments-1].Author != lastAuthor {
		t.Errorf("expected last comment from %s, got %s", lastAuthor, item.RecentComments[MaxComments-1].Author)
	}
}

func TestCollectNarrativeItem_IssueError(t *testing.T) {
	since := time.Now().AddDate(0, 0, -7)
	ref := input.IssueRef{Owner: "owner", Repo: "repo", Number: 1, URL: "https://github.com/owner/repo/issues/1"}

	fetcher := &mockNarrativeFetcher{
		issueErr: fmt.Errorf("API error"),
	}

	_, err := CollectNarrativeItem(context.Background(), fetcher, ref, since)
	if err == nil {
		t.Fatal("expected error when FetchIssue fails")
	}
}

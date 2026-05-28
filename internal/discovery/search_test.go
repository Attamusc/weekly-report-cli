package discovery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	githubapi "github.com/google/go-github/v66/github"
)

func TestBuildQueries(t *testing.T) {
	since := time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		users    []string
		orgs     []string
		wantLen  int
		wantSubs []string // substrings each query should contain
	}{
		{
			name:    "single user produces 2 queries",
			users:   []string{"alice"},
			wantLen: 2,
			wantSubs: []string{
				"involves:alice",
				"author:alice",
				"updated:>=2026-05-14",
			},
		},
		{
			name:    "5 users fit in one batch",
			users:   []string{"a", "b", "c", "d", "e"},
			wantLen: 2,
		},
		{
			name:    "6 users split into 2 batches",
			users:   []string{"a", "b", "c", "d", "e", "f"},
			wantLen: 4, // 2 queries per batch × 2 batches
		},
		{
			name:    "org filter added to queries",
			users:   []string{"alice"},
			orgs:    []string{"my-org"},
			wantLen: 2,
			wantSubs: []string{
				"org:my-org",
				"involves:alice",
			},
		},
		{
			name:    "multiple orgs",
			users:   []string{"alice"},
			orgs:    []string{"org1", "org2"},
			wantLen: 2,
			wantSubs: []string{
				"org:org1",
				"org:org2",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			queries := buildQueries(tc.users, tc.orgs, since)
			if len(queries) != tc.wantLen {
				t.Errorf("got %d queries, want %d", len(queries), tc.wantLen)
			}
			for _, sub := range tc.wantSubs {
				found := false
				for _, q := range queries {
					if strings.Contains(q, sub) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("no query contains %q", sub)
				}
			}
		})
	}
}

func TestIsBotUser(t *testing.T) {
	tests := []struct {
		login    string
		userType string
		want     bool
	}{
		{"dependabot[bot]", "Bot", true},
		{"renovate[bot]", "Bot", true},
		{"github-actions[bot]", "User", true}, // suffix match
		{"some-bot", "Bot", true},             // type match
		{"alice", "User", false},
		{"bob", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.login, func(t *testing.T) {
			got := isBotUser(tc.login, tc.userType)
			if got != tc.want {
				t.Errorf("isBotUser(%q, %q) = %v, want %v", tc.login, tc.userType, got, tc.want)
			}
		})
	}
}

func TestFilterBots(t *testing.T) {
	userLogin := "alice"
	userType := "User"
	botLogin := "dependabot[bot]"
	botType := "Bot"

	issues := []*githubapi.Issue{
		{User: &githubapi.User{Login: &userLogin, Type: &userType}},
		{User: &githubapi.User{Login: &botLogin, Type: &botType}},
	}

	filtered := filterBots(issues)
	if len(filtered) != 1 {
		t.Fatalf("got %d results, want 1", len(filtered))
	}
	if filtered[0].GetUser().GetLogin() != "alice" {
		t.Errorf("got user %q, want alice", filtered[0].GetUser().GetLogin())
	}
}

func TestDeduplicateToRefs(t *testing.T) {
	url1 := "https://github.com/org/repo/issues/1"
	url2 := "https://github.com/org/repo/pull/2"
	num1 := 1
	num2 := 2
	login := "alice"
	uType := "User"

	issues := []*githubapi.Issue{
		{HTMLURL: &url1, Number: &num1, User: &githubapi.User{Login: &login, Type: &uType}},
		{HTMLURL: &url1, Number: &num1, User: &githubapi.User{Login: &login, Type: &uType}}, // duplicate
		{HTMLURL: &url2, Number: &num2, User: &githubapi.User{Login: &login, Type: &uType}},
	}

	refs := deduplicateToRefs(issues)
	if len(refs) != 2 {
		t.Fatalf("got %d refs, want 2", len(refs))
	}
}

func TestSearch_Integration(t *testing.T) {
	total := 2
	searchResp := &githubapi.IssuesSearchResult{
		Total: &total,
		Issues: []*githubapi.Issue{
			{
				HTMLURL: strPtr("https://github.com/org/repo/issues/10"),
				Number:  intPtr(10),
				User: &githubapi.User{
					Login: strPtr("alice"),
					Type:  strPtr("User"),
				},
			},
			{
				HTMLURL: strPtr("https://github.com/org/repo/pull/20"),
				Number:  intPtr(20),
				User: &githubapi.User{
					Login: strPtr("dependabot[bot]"),
					Type:  strPtr("Bot"),
				},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/issues" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(searchResp)
	}))
	defer server.Close()

	serverURL, _ := url.Parse(server.URL + "/")
	client := githubapi.NewClient(nil)
	client.BaseURL = serverURL

	since := time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)
	refs, err := Search(context.Background(), client, []string{"alice"}, nil, since)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	// Bot should be filtered, leaving only issue #10
	if len(refs) != 1 {
		t.Fatalf("got %d refs, want 1", len(refs))
	}
	if refs[0].Number != 10 {
		t.Errorf("got issue #%d, want #10", refs[0].Number)
	}
	if refs[0].Owner != "org" || refs[0].Repo != "repo" {
		t.Errorf("got %s/%s, want org/repo", refs[0].Owner, refs[0].Repo)
	}
}

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }

func TestSearch_SignalsPopulated(t *testing.T) {
	closedAt := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 5, 21, 8, 0, 0, 0, time.UTC)

	searchResp := &githubapi.IssuesSearchResult{
		Total: intPtr(1),
		Issues: []*githubapi.Issue{
			{
				HTMLURL:  strPtr("https://github.com/org/repo/pull/42"),
				Number:   intPtr(42),
				Title:    strPtr("Fix the thing"),
				State:    strPtr("closed"),
				Comments: intPtr(7),
				User: &githubapi.User{
					Login: strPtr("alice"),
					Type:  strPtr("User"),
				},
				PullRequestLinks: &githubapi.PullRequestLinks{
					URL: strPtr("https://api.github.com/repos/org/repo/pulls/42"),
				},
				ClosedAt:  &githubapi.Timestamp{Time: closedAt},
				UpdatedAt: &githubapi.Timestamp{Time: updatedAt},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/issues" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(searchResp)
	}))
	defer server.Close()

	serverURL, _ := url.Parse(server.URL + "/")
	client := githubapi.NewClient(nil)
	client.BaseURL = serverURL

	since := time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)
	refs, err := Search(context.Background(), client, []string{"alice"}, nil, since)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("got %d refs, want 1", len(refs))
	}

	ref := refs[0]
	if ref.CommentCount != 7 {
		t.Errorf("CommentCount: got %d, want 7", ref.CommentCount)
	}
	if !ref.IsPR {
		t.Errorf("IsPR: got false, want true")
	}
	if ref.State != "closed" {
		t.Errorf("State: got %q, want \"closed\"", ref.State)
	}
	if ref.AuthorLogin != "alice" {
		t.Errorf("AuthorLogin: got %q, want \"alice\"", ref.AuthorLogin)
	}
	if ref.Title != "Fix the thing" {
		t.Errorf("Title: got %q, want \"Fix the thing\"", ref.Title)
	}
	if ref.ClosedAt == nil || !ref.ClosedAt.Equal(closedAt) {
		t.Errorf("ClosedAt: got %v, want %v", ref.ClosedAt, closedAt)
	}
	if !ref.UpdatedAt.Equal(updatedAt) {
		t.Errorf("UpdatedAt: got %v, want %v", ref.UpdatedAt, updatedAt)
	}
}

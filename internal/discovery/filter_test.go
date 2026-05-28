package discovery

import (
	"testing"

	githubapi "github.com/google/go-github/v66/github"
)

func TestFilterByTeamAuthorship(t *testing.T) {
	prLinks := &githubapi.PullRequestLinks{}

	tests := []struct {
		name      string
		issues    []*githubapi.Issue
		users     []string
		wantCount int
		wantNums  []int
	}{
		{
			name: "PR authored by team member is kept",
			issues: []*githubapi.Issue{
				{
					Number:           intPtr(1),
					HTMLURL:          strPtr("https://github.com/org/repo/pull/1"),
					PullRequestLinks: prLinks,
					User:             &githubapi.User{Login: strPtr("alice"), Type: strPtr("User")},
				},
			},
			users:     []string{"alice"},
			wantCount: 1,
			wantNums:  []int{1},
		},
		{
			name: "PR authored by non-team member is dropped",
			issues: []*githubapi.Issue{
				{
					Number:           intPtr(2),
					HTMLURL:          strPtr("https://github.com/org/repo/pull/2"),
					PullRequestLinks: prLinks,
					User:             &githubapi.User{Login: strPtr("outsider"), Type: strPtr("User")},
				},
			},
			users:     []string{"alice"},
			wantCount: 0,
		},
		{
			name: "Issue authored by non-team member with comments is kept",
			issues: []*githubapi.Issue{
				{
					Number:  intPtr(3),
					HTMLURL: strPtr("https://github.com/org/repo/issues/3"),
					// PullRequestLinks is nil → this is an issue
					User: &githubapi.User{Login: strPtr("outsider"), Type: strPtr("User")},
				},
			},
			users:     []string{"alice"},
			wantCount: 1,
			wantNums:  []int{3},
		},
		{
			name: "mixed: team PR kept, non-team PR dropped, issue kept",
			issues: []*githubapi.Issue{
				{
					Number:           intPtr(10),
					HTMLURL:          strPtr("https://github.com/org/repo/pull/10"),
					PullRequestLinks: prLinks,
					User:             &githubapi.User{Login: strPtr("alice"), Type: strPtr("User")},
				},
				{
					Number:           intPtr(11),
					HTMLURL:          strPtr("https://github.com/org/repo/pull/11"),
					PullRequestLinks: prLinks,
					User:             &githubapi.User{Login: strPtr("bob"), Type: strPtr("User")},
				},
				{
					Number:  intPtr(12),
					HTMLURL: strPtr("https://github.com/org/repo/issues/12"),
					User:    &githubapi.User{Login: strPtr("carol"), Type: strPtr("User")},
				},
			},
			users:     []string{"alice"},
			wantCount: 2,
			wantNums:  []int{10, 12},
		},
		{
			name: "user matching is case-insensitive",
			issues: []*githubapi.Issue{
				{
					Number:           intPtr(5),
					HTMLURL:          strPtr("https://github.com/org/repo/pull/5"),
					PullRequestLinks: prLinks,
					User:             &githubapi.User{Login: strPtr("Alice"), Type: strPtr("User")},
				},
			},
			users:     []string{"alice"},
			wantCount: 1,
			wantNums:  []int{5},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := filterByTeamAuthorship(tc.issues, tc.users)
			if len(got) != tc.wantCount {
				t.Fatalf("got %d items, want %d", len(got), tc.wantCount)
			}
			for i, wantNum := range tc.wantNums {
				if got[i].GetNumber() != wantNum {
					t.Errorf("item[%d] number = %d, want %d", i, got[i].GetNumber(), wantNum)
				}
			}
		})
	}
}

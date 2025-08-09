package github

import (
	"context"
	"fmt"
	"time"

	"github.com/Attamusc/weekly-report-cli/internal/input"
	"github.com/google/go-github/v66/github"
)

// IssueData represents GitHub issue metadata
type IssueData struct {
	URL   string
	Title string
}

// Comment represents a GitHub issue comment
type Comment struct {
	Body      string
	CreatedAt time.Time
	Author    string
	URL       string
}

// FetchIssue retrieves issue metadata from GitHub API
func FetchIssue(ctx context.Context, client *github.Client, ref input.IssueRef) (IssueData, error) {
	issue, _, err := client.Issues.Get(ctx, ref.Owner, ref.Repo, ref.Number)
	if err != nil {
		return IssueData{}, fmt.Errorf("failed to fetch issue %s: %w", ref.String(), err)
	}

	return IssueData{
		URL:   issue.GetHTMLURL(),
		Title: issue.GetTitle(),
	}, nil
}

// FetchCommentsSince retrieves issue comments created since the specified time
// Uses pagination to fetch all comments and filters by CreatedAt
func FetchCommentsSince(ctx context.Context, client *github.Client, ref input.IssueRef, since time.Time) ([]Comment, error) {
	var allComments []Comment
	
	// GitHub API pagination options
	opts := &github.IssueListCommentsOptions{
		Since: &since,
		ListOptions: github.ListOptions{
			Page:    1,
			PerPage: 100, // Maximum allowed per page
		},
	}

	for {
		// Fetch page of comments
		comments, resp, err := client.Issues.ListComments(ctx, ref.Owner, ref.Repo, ref.Number, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch comments for issue %s: %w", ref.String(), err)
		}

		// Convert GitHub comments to our Comment type
		for _, comment := range comments {
			// Double-check the since filter (GitHub API sometimes includes edge cases)
			commentTime := comment.GetCreatedAt().Time
			if commentTime.After(since) || commentTime.Equal(since) {
				allComments = append(allComments, Comment{
					Body:      comment.GetBody(),
					CreatedAt: comment.GetCreatedAt().Time,
					Author:    comment.GetUser().GetLogin(),
					URL:       comment.GetHTMLURL(),
				})
			}
		}

		// Check if there are more pages
		if resp.NextPage == 0 {
			break
		}

		// Move to next page
		opts.Page = resp.NextPage
	}

	return allComments, nil
}
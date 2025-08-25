package github

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
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
	// Get logger from context if available
	logger, ok := ctx.Value("logger").(*slog.Logger)
	if !ok {
		logger = slog.Default()
	}

	logger.Debug("Fetching issue metadata", "owner", ref.Owner, "repo", ref.Repo, "number", ref.Number)

	issue, _, err := client.Issues.Get(ctx, ref.Owner, ref.Repo, ref.Number)
	if err != nil {
		logger.Debug("GitHub API issue fetch failed", "issue", ref.String(), "error", err)

		// Check for specific error types and provide helpful error messages
		if enhancedErr := enhanceGitHubError(err, ref); enhancedErr != nil {
			return IssueData{}, enhancedErr
		}

		return IssueData{}, fmt.Errorf("failed to fetch issue %s: %w", ref.String(), err)
	}

	logger.Debug("Issue metadata fetched successfully", "issue", ref.String(), "title", issue.GetTitle())

	return IssueData{
		URL:   issue.GetHTMLURL(),
		Title: issue.GetTitle(),
	}, nil
}

// FetchCommentsSince retrieves issue comments created since the specified time
// Uses pagination to fetch all comments and filters by CreatedAt
func FetchCommentsSince(ctx context.Context, client *github.Client, ref input.IssueRef, since time.Time) ([]Comment, error) {
	// Get logger from context if available
	logger, ok := ctx.Value("logger").(*slog.Logger)
	if !ok {
		logger = slog.Default()
	}

	logger.Debug("Fetching comments", "issue", ref.String(), "since", since.Format("2006-01-02"))

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
		logger.Debug("Fetching comments page", "issue", ref.String(), "page", opts.Page)

		// Fetch page of comments
		comments, resp, err := client.Issues.ListComments(ctx, ref.Owner, ref.Repo, ref.Number, opts)
		if err != nil {
			logger.Debug("GitHub API comments fetch failed", "issue", ref.String(), "page", opts.Page, "error", err)

			// Check for specific error types and provide helpful error messages
			if enhancedErr := enhanceGitHubError(err, ref); enhancedErr != nil {
				return nil, enhancedErr
			}

			return nil, fmt.Errorf("failed to fetch comments for issue %s: %w", ref.String(), err)
		}

		logger.Debug("Comments page fetched", "issue", ref.String(), "page", opts.Page, "count", len(comments))

		// Convert GitHub comments to our Comment type
		pageComments := 0
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
				pageComments++
			}
		}

		logger.Debug("Comments filtered by date", "issue", ref.String(), "page", opts.Page, "filtered", pageComments)

		// Check if there are more pages
		if resp.NextPage == 0 {
			break
		}

		// Move to next page
		opts.Page = resp.NextPage
	}

	logger.Debug("Comments fetch completed", "issue", ref.String(), "total", len(allComments))
	return allComments, nil
}

// enhanceGitHubError checks for common GitHub API error conditions and provides helpful error messages
func enhanceGitHubError(err error, ref input.IssueRef) error {
	// Convert to GitHub ErrorResponse if possible
	if ghErr, ok := err.(*github.ErrorResponse); ok {
		switch ghErr.Response.StatusCode {
		case http.StatusUnauthorized:
			return fmt.Errorf("GitHub API authentication failed for %s. Please check your GITHUB_TOKEN is valid and has the required permissions", ref.String())

		case http.StatusForbidden:
			// Check if this might be an SSO authorization issue
			if strings.Contains(strings.ToLower(ghErr.Message), "sso") ||
				strings.Contains(strings.ToLower(ghErr.Message), "organization") {
				return fmt.Errorf("GitHub API access denied for %s. Your token may require SSO authorization for this organization. Visit: https://github.com/settings/tokens and authorize your token for SSO", ref.String())
			}

			// Generic 403 error
			return fmt.Errorf("GitHub API access denied for %s. Your token may not have sufficient permissions to access this repository", ref.String())

		case http.StatusNotFound:
			return fmt.Errorf("GitHub issue %s not found. This could mean the repository is private and your token lacks access, or the issue doesn't exist", ref.String())
		}
	}

	// Check for timeout errors
	if strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "deadline exceeded") {
		return fmt.Errorf("GitHub API request timed out for %s. Please check your network connection and try again", ref.String())
	}

	// Return nil to indicate no enhancement was applied
	return nil
}

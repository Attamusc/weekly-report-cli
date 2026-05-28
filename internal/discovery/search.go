package discovery

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	githubapi "github.com/google/go-github/v66/github"

	"github.com/Attamusc/weekly-report-cli/internal/input"
)

const (
	// maxUsersPerQuery limits how many users we OR together in a single search query
	// to stay within GitHub's query length limits.
	maxUsersPerQuery = 5

	// searchPageDelay is the pause between paginated search API calls to stay
	// under the 30 req/min secondary rate limit.
	searchPageDelay = 2 * time.Second

	// maxSearchPages caps the number of pages we fetch per query (100 results/page,
	// GitHub caps total at 1000 results per search query).
	maxSearchPages = 10
)

// Search discovers issues and PRs touched by the given users since the cutoff date.
// When orgs is non-empty, results are scoped to repositories in those organizations.
// Returns deduplicated, bot-filtered, authorship-filtered IssueRefs ready for pipeline hydration.
func Search(ctx context.Context, client *githubapi.Client, users []string, orgs []string, since time.Time) ([]input.IssueRef, error) {
	logger := getLogger(ctx)

	queries := buildQueries(users, orgs, since)

	var allIssues []*githubapi.Issue
	for _, q := range queries {
		logger.Debug("Executing search query", "query", q)
		results, err := executeSearch(ctx, client, q, logger)
		if err != nil {
			return nil, fmt.Errorf("search query failed: %w", err)
		}
		allIssues = append(allIssues, results...)
	}

	logger.Info("Search raw results", "total", len(allIssues))

	filtered := filterBots(allIssues)
	logger.Debug("After bot filtering", "remaining", len(filtered))

	filtered = filterByTeamAuthorship(filtered, users)
	logger.Debug("After authorship filter", "remaining", len(filtered))

	refs := deduplicateToRefs(filtered)
	logger.Info("Discovery complete", "total_found", len(allIssues), "after_filter", len(refs))

	return refs, nil
}

// buildQueries creates search query strings for issues and PRs, batching users
// into groups of maxUsersPerQuery.
func buildQueries(users []string, orgs []string, since time.Time) []string {
	dateStr := since.Format("2006-01-02")

	// Build org qualifier if specified
	var orgClause string
	if len(orgs) > 0 {
		var parts []string
		for _, o := range orgs {
			parts = append(parts, fmt.Sprintf("org:%s", o))
		}
		orgClause = strings.Join(parts, " ") + " "
	}

	var queries []string

	for i := 0; i < len(users); i += maxUsersPerQuery {
		end := i + maxUsersPerQuery
		if end > len(users) {
			end = len(users)
		}
		chunk := users[i:end]

		issueUserClauses := buildUserClauses("involves", chunk)
		prUserClauses := buildUserClauses("author", chunk)

		// Issues: any involvement (comments are usually substantive)
		queries = append(queries,
			fmt.Sprintf("%s%s is:issue updated:>=%s", orgClause, issueUserClauses, dateStr))

		// PRs: author only — reviewer/approver involvement is low-signal
		queries = append(queries,
			fmt.Sprintf("%s%s is:pr updated:>=%s", orgClause, prUserClauses, dateStr))
	}

	return queries
}

// buildUserClauses creates the OR-joined user qualifier string using the given qualifier.
// For issues use "involves" (covers author, commenter, assignee, mention).
// For PRs use "author" to avoid surfacing items where a user only reviewed.
func buildUserClauses(qualifier string, users []string) string {
	var parts []string
	for _, u := range users {
		parts = append(parts, fmt.Sprintf("%s:%s", qualifier, u))
	}
	return strings.Join(parts, " ")
}

// executeSearch runs a single search query with pagination, respecting rate limits.
func executeSearch(ctx context.Context, client *githubapi.Client, query string, logger *slog.Logger) ([]*githubapi.Issue, error) {
	var allResults []*githubapi.Issue

	opts := &githubapi.SearchOptions{
		Sort:  "updated",
		Order: "desc",
		ListOptions: githubapi.ListOptions{
			Page:    1,
			PerPage: 100,
		},
	}

	for page := 0; page < maxSearchPages; page++ {
		if page > 0 {
			select {
			case <-ctx.Done():
				return allResults, ctx.Err()
			case <-time.After(searchPageDelay):
			}
		}

		result, resp, err := client.Search.Issues(ctx, query, opts)
		if err != nil {
			return allResults, fmt.Errorf("search API error: %w", err)
		}

		allResults = append(allResults, result.Issues...)
		logger.Debug("Search page fetched",
			"page", opts.Page,
			"results_this_page", len(result.Issues),
			"total_so_far", len(allResults),
			"total_available", result.GetTotal())

		if result.GetTotal() > 1000 {
			logger.Warn("Search results exceed 1000 — some items may be missing",
				"total", result.GetTotal())
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return allResults, nil
}

// deduplicateToRefs converts search results to IssueRefs, deduplicating by URL.
func deduplicateToRefs(issues []*githubapi.Issue) []input.IssueRef {
	seen := make(map[string]bool)
	var refs []input.IssueRef

	for _, issue := range issues {
		htmlURL := issue.GetHTMLURL()
		if htmlURL == "" || seen[htmlURL] {
			continue
		}
		seen[htmlURL] = true

		ref, ok := parseSearchResult(issue)
		if !ok {
			continue
		}
		refs = append(refs, ref)
	}

	return refs
}

// parseSearchResult extracts an IssueRef from a GitHub search result issue.
// Returns false if the URL can't be parsed.
func parseSearchResult(issue *githubapi.Issue) (input.IssueRef, bool) {
	htmlURL := issue.GetHTMLURL()
	if htmlURL == "" {
		return input.IssueRef{}, false
	}

	// GitHub search results return issue/PR URLs like:
	//   https://github.com/{owner}/{repo}/issues/{number}
	//   https://github.com/{owner}/{repo}/pull/{number}
	parts := strings.Split(strings.TrimPrefix(htmlURL, "https://github.com/"), "/")
	if len(parts) < 4 {
		return input.IssueRef{}, false
	}

	ref := input.IssueRef{
		Owner:        parts[0],
		Repo:         parts[1],
		Number:       issue.GetNumber(),
		URL:          htmlURL,
		Title:        issue.GetTitle(),
		State:        issue.GetState(),
		IsPR:         issue.IsPullRequest(),
		AuthorLogin:  issue.GetUser().GetLogin(),
		CommentCount: issue.GetComments(),
		UpdatedAt:    issue.GetUpdatedAt().Time,
	}
	if issue.ClosedAt != nil {
		t := issue.ClosedAt.Time
		ref.ClosedAt = &t
	}
	return ref, true
}

// getLogger retrieves the logger from context, falling back to slog.Default.
func getLogger(ctx context.Context) *slog.Logger {
	logger, ok := ctx.Value(input.LoggerContextKey{}).(*slog.Logger)
	if !ok {
		return slog.Default()
	}
	return logger
}

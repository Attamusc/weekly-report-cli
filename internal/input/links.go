package input

import (
	"bufio"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// IssueRef represents a GitHub issue reference
type IssueRef struct {
	Owner  string
	Repo   string
	Number int
	URL    string
}

// String returns a string representation of the IssueRef
func (ref IssueRef) String() string {
	return fmt.Sprintf("%s/%s#%d", ref.Owner, ref.Repo, ref.Number)
}

// githubIssueRegex matches GitHub issue URLs
var githubIssueRegex = regexp.MustCompile(`^https://github\.com/([^/]+)/([^/]+)/issues/(\d+)`)

// ParseIssueLinks parses GitHub issue URLs from a reader
// Accepts URLs in the form: https://github.com/{owner}/{repo}/issues/{number}
// Allows query parameters and fragments. Deduplicates while maintaining stable order.
func ParseIssueLinks(r io.Reader) ([]IssueRef, error) {
	var refs []IssueRef
	seen := make(map[string]bool)
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse the URL to handle query parameters and fragments
		parsedURL, err := url.Parse(line)
		if err != nil {
			return nil, fmt.Errorf("invalid URL format: %s", line)
		}

		// Match against the GitHub issue pattern
		matches := githubIssueRegex.FindStringSubmatch(parsedURL.String())
		if matches == nil {
			return nil, fmt.Errorf("invalid GitHub issue URL format: %s", line)
		}

		owner := matches[1]
		repo := matches[2]
		numberStr := matches[3]

		number, err := strconv.Atoi(numberStr)
		if err != nil {
			return nil, fmt.Errorf("invalid issue number in URL: %s", line)
		}

		// Create canonical URL without query/fragment for deduplication
		canonicalURL := fmt.Sprintf("https://github.com/%s/%s/issues/%d", owner, repo, number)
		
		// Skip if we've already seen this issue
		if seen[canonicalURL] {
			continue
		}
		seen[canonicalURL] = true

		refs = append(refs, IssueRef{
			Owner:  owner,
			Repo:   repo,
			Number: number,
			URL:    canonicalURL,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading input: %w", err)
	}

	return refs, nil
}
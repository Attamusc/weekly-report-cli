package projects

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Project URL patterns for parsing
var (
	// Organization project full URL: https://github.com/orgs/{org}/projects/{number}
	orgFullURLPattern = regexp.MustCompile(`^https://github\.com/orgs/([^/]+)/projects/(\d+)`)

	// User project full URL: https://github.com/users/{username}/projects/{number}
	userFullURLPattern = regexp.MustCompile(`^https://github\.com/users/([^/]+)/projects/(\d+)`)

	// Organization project short form: org:{org}/{number}
	orgShortPattern = regexp.MustCompile(`^org:([^/]+)/(\d+)$`)

	// User project short form: user:{username}/{number}
	userShortPattern = regexp.MustCompile(`^user:([^/]+)/(\d+)$`)
)

// ParseProjectURL parses various project URL formats and returns a ProjectRef
// Supported formats:
//   - https://github.com/orgs/{org}/projects/{number}
//   - https://github.com/users/{username}/projects/{number}
//   - org:{org}/{number}
//   - user:{username}/{number}
//
// Returns an error if the format is invalid or the project number cannot be parsed.
func ParseProjectURL(raw string) (ProjectRef, error) {
	// Trim whitespace
	raw = strings.TrimSpace(raw)

	if raw == "" {
		return ProjectRef{}, fmt.Errorf("empty project URL")
	}

	// Try organization full URL
	if matches := orgFullURLPattern.FindStringSubmatch(raw); matches != nil {
		return parseMatches(ProjectTypeOrg, matches[1], matches[2])
	}

	// Try user full URL
	if matches := userFullURLPattern.FindStringSubmatch(raw); matches != nil {
		return parseMatches(ProjectTypeUser, matches[1], matches[2])
	}

	// Try organization short form
	if matches := orgShortPattern.FindStringSubmatch(raw); matches != nil {
		return parseMatches(ProjectTypeOrg, matches[1], matches[2])
	}

	// Try user short form
	if matches := userShortPattern.FindStringSubmatch(raw); matches != nil {
		return parseMatches(ProjectTypeUser, matches[1], matches[2])
	}

	// No pattern matched
	return ProjectRef{}, fmt.Errorf("invalid project URL format: %s\nExpected formats:\n  - https://github.com/orgs/{org}/projects/{number}\n  - https://github.com/users/{username}/projects/{number}\n  - org:{org}/{number}\n  - user:{username}/{number}", raw)
}

// parseMatches is a helper function that parses regex matches into a ProjectRef
func parseMatches(projectType ProjectType, owner, numberStr string) (ProjectRef, error) {
	// Parse project number
	number, err := strconv.Atoi(numberStr)
	if err != nil {
		return ProjectRef{}, fmt.Errorf("invalid project number: %s", numberStr)
	}

	if number <= 0 {
		return ProjectRef{}, fmt.Errorf("project number must be positive: %d", number)
	}

	// Validate owner name (no empty strings)
	if owner == "" {
		return ProjectRef{}, fmt.Errorf("owner name cannot be empty")
	}

	// Build canonical URL
	var canonicalURL string
	switch projectType {
	case ProjectTypeOrg:
		canonicalURL = fmt.Sprintf("https://github.com/orgs/%s/projects/%d", owner, number)
	case ProjectTypeUser:
		canonicalURL = fmt.Sprintf("https://github.com/users/%s/projects/%d", owner, number)
	default:
		return ProjectRef{}, fmt.Errorf("unknown project type: %v", projectType)
	}

	return ProjectRef{
		Type:   projectType,
		Owner:  owner,
		Number: number,
		URL:    canonicalURL,
	}, nil
}

package discovery

import (
	"strings"

	githubapi "github.com/google/go-github/v66/github"
)

// knownBotSuffixes identifies bot accounts by login suffix.
var knownBotSuffixes = []string{"[bot]"}

// filterBots removes issues authored by known bot accounts.
func filterBots(issues []*githubapi.Issue) []*githubapi.Issue {
	var filtered []*githubapi.Issue
	for _, issue := range issues {
		user := issue.GetUser()
		if user == nil {
			continue
		}
		if isBotUser(user.GetLogin(), user.GetType()) {
			continue
		}
		filtered = append(filtered, issue)
	}
	return filtered
}

// filterByTeamAuthorship keeps issues unconditionally but drops PRs not
// authored by a team member. This eliminates low-signal items such as PRs
// where a team member only reviewed or approved but did not write the code.
func filterByTeamAuthorship(issues []*githubapi.Issue, users []string) []*githubapi.Issue {
	teamSet := make(map[string]bool, len(users))
	for _, u := range users {
		teamSet[strings.ToLower(u)] = true
	}

	var filtered []*githubapi.Issue
	for _, issue := range issues {
		// Keep issues regardless of authorship — comments are usually substantive.
		if issue.PullRequestLinks == nil {
			filtered = append(filtered, issue)
			continue
		}
		// For PRs, only keep items authored by a team member.
		login := strings.ToLower(issue.GetUser().GetLogin())
		if teamSet[login] {
			filtered = append(filtered, issue)
		}
	}
	return filtered
}

// isBotUser returns true if the user appears to be a bot account.
func isBotUser(login string, userType string) bool {
	if userType == "Bot" {
		return true
	}
	for _, suffix := range knownBotSuffixes {
		if strings.HasSuffix(login, suffix) {
			return true
		}
	}
	return false
}

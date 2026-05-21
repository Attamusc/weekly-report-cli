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

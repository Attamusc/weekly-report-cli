package report

import (
	"sort"
	"time"

	"github.com/Attamusc/weekly-report-cli/internal/github"
)

// SelectReports extracts and filters reports from comments within a time window
// Returns ALL valid reports within the specified time window, sorted newest-first
func SelectReports(comments []github.Comment, since time.Time) []Report {
	var reports []Report

	// Extract reports from each comment
	for _, comment := range comments {
		// Skip comments outside the time window
		if comment.CreatedAt.Before(since) {
			continue
		}

		// Try to parse a report from this comment
		if report, ok := ParseReport(comment.Body, comment.CreatedAt, comment.URL); ok {
			reports = append(reports, report)
		}
	}

	// Sort reports newest-first by CreatedAt
	sort.Slice(reports, func(i, j int) bool {
		return reports[i].CreatedAt.After(reports[j].CreatedAt)
	})

	return reports
}

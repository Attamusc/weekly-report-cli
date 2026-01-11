package format

import (
	"fmt"
	"sort"
	"strings"
)

// DescribeRow represents a row for the describe command output
type DescribeRow struct {
	Title     string   // Issue title
	URL       string   // Issue URL
	Summary   string   // AI-generated or raw project/goals summary
	Labels    []string // Issue labels
	Assignees []string // Issue assignees (usernames)
}

// RenderDescribeTable generates a markdown table for describe output
// Columns: Initiative | Labels | Assignee | Summary
func RenderDescribeTable(rows []DescribeRow) string {
	if len(rows) == 0 {
		return ""
	}

	var builder strings.Builder

	// Write table header
	builder.WriteString("| Initiative | Labels | Assignee | Summary |\n")
	builder.WriteString("|------------|--------|----------|--------|\n")

	// Write each row
	for _, row := range rows {
		// Format initiative column with markdown link
		initiativeCol := fmt.Sprintf("[%s](%s)",
			escapeMarkdownTableCell(row.Title),
			row.URL)

		// Format labels column (comma-separated)
		labelsCol := escapeMarkdownTableCell(strings.Join(row.Labels, ", "))
		if labelsCol == "" {
			labelsCol = "-"
		}

		// Format assignees column (comma-separated with @ prefix)
		var assigneesList []string
		for _, a := range row.Assignees {
			assigneesList = append(assigneesList, "@"+a)
		}
		assigneesCol := escapeMarkdownTableCell(strings.Join(assigneesList, ", "))
		if assigneesCol == "" {
			assigneesCol = "-"
		}

		// Format summary column (collapse newlines and escape pipes)
		summaryCol := escapeMarkdownTableCell(collapseNewlines(row.Summary))
		if summaryCol == "" {
			summaryCol = "-"
		}

		// Write the row
		builder.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
			initiativeCol, labelsCol, assigneesCol, summaryCol))
	}

	return builder.String()
}

// RenderDescribeDetailed generates detailed markdown sections for each issue
// Each issue gets its own section with title, metadata, and full summary
func RenderDescribeDetailed(rows []DescribeRow) string {
	if len(rows) == 0 {
		return ""
	}

	var builder strings.Builder

	for i, row := range rows {
		// Section header with linked title
		builder.WriteString(fmt.Sprintf("## [%s](%s)\n\n", row.Title, row.URL))

		// Labels line
		if len(row.Labels) > 0 {
			builder.WriteString(fmt.Sprintf("**Labels:** %s  \n", strings.Join(row.Labels, ", ")))
		}

		// Assignees line
		if len(row.Assignees) > 0 {
			var assigneesList []string
			for _, a := range row.Assignees {
				assigneesList = append(assigneesList, "@"+a)
			}
			builder.WriteString(fmt.Sprintf("**Assignees:** %s\n", strings.Join(assigneesList, ", ")))
		}

		// Summary section
		builder.WriteString("\n### Summary\n\n")
		if row.Summary != "" {
			builder.WriteString(row.Summary)
		} else {
			builder.WriteString("_No description available._")
		}
		builder.WriteString("\n")

		// Add separator between entries (except after the last one)
		if i < len(rows)-1 {
			builder.WriteString("\n---\n\n")
		}
	}

	return builder.String()
}

// SortDescribeRowsByTitle sorts describe rows alphabetically by title
func SortDescribeRowsByTitle(rows []DescribeRow) {
	sort.Slice(rows, func(i, j int) bool {
		return strings.ToLower(rows[i].Title) < strings.ToLower(rows[j].Title)
	})
}

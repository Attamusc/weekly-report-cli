package format

import (
	"fmt"
	"strings"
	"time"

	"github.com/Attamusc/weekly-report-cli/internal/derive"
)

// Row represents a single row in the markdown table
type Row struct {
	StatusEmoji   string     // Status emoji (e.g., ":green_circle:")
	StatusCaption string     // Status caption (e.g., "On Track")
	EpicTitle     string     // Epic/issue title
	EpicURL       string     // Epic/issue URL
	TargetDate    *time.Time // Target date (nil renders as "TBD")
	UpdateMD      string     // Update summary/content (markdown-ready)
}

// NewRow creates a Row from components, handling status derivation and date parsing
func NewRow(status derive.Status, epicTitle, epicURL string, targetDate *time.Time, updateMD string) Row {
	return Row{
		StatusEmoji:   status.Emoji,
		StatusCaption: status.Caption,
		EpicTitle:     epicTitle,
		EpicURL:       epicURL,
		TargetDate:    targetDate,
		UpdateMD:      updateMD,
	}
}

// RenderTable generates a markdown table from a slice of rows
// Returns a properly formatted markdown table with headers and escaped content
func RenderTable(rows []Row) string {
	if len(rows) == 0 {
		return ""
	}

	var builder strings.Builder

	// Write table header
	builder.WriteString("| Status | Initiative/Epic | Target Date | Update |\n")
	builder.WriteString("|--------|-----------------|-------------|--------|\n")

	// Write each row
	for _, row := range rows {
		// Format status column
		statusCol := fmt.Sprintf("%s %s", row.StatusEmoji, row.StatusCaption)

		// Format epic column with markdown link
		epicCol := fmt.Sprintf("[%s](%s)",
			escapeMarkdownTableCell(row.EpicTitle),
			row.EpicURL)

		// Format target date column
		dateCol := derive.RenderTargetDate(row.TargetDate)

		// Format update column (collapse newlines and escape pipes)
		updateCol := escapeMarkdownTableCell(collapseNewlines(row.UpdateMD))

		// Write the row
		builder.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
			statusCol, epicCol, dateCol, updateCol))
	}

	return builder.String()
}

// escapeMarkdownTableCell escapes pipe characters and other problematic content for table cells
func escapeMarkdownTableCell(content string) string {
	// First escape existing backslashes to prevent unintended escaping
	content = strings.ReplaceAll(content, "\\", "\\\\")

	// Then replace pipe characters that would break table formatting
	content = strings.ReplaceAll(content, "|", "\\|")

	// Use collapseNewlines to properly handle line endings and spacing
	content = collapseNewlines(content)

	// Replace tabs
	content = strings.ReplaceAll(content, "\t", " ")

	return strings.TrimSpace(content)
}

// collapseNewlines replaces newlines with single spaces for table cell content
func collapseNewlines(content string) string {
	// Replace Windows line endings first to avoid double spaces
	content = strings.ReplaceAll(content, "\r\n", " ")
	// Then replace remaining Unix and Mac line endings
	content = strings.ReplaceAll(content, "\n", " ")
	content = strings.ReplaceAll(content, "\r", " ")

	// Collapse multiple spaces into single spaces
	for strings.Contains(content, "  ") {
		content = strings.ReplaceAll(content, "  ", " ")
	}

	return strings.TrimSpace(content)
}

// RenderTableWithTitle renders a table with an optional title/header
func RenderTableWithTitle(title string, rows []Row) string {
	table := RenderTable(rows)
	if table == "" {
		return ""
	}

	if title == "" {
		return table
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("# %s\n\n", title))
	builder.WriteString(table)
	return builder.String()
}


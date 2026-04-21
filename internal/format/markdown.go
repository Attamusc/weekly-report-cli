package format

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Attamusc/weekly-report-cli/internal/derive"
)

// Row represents a single row in the markdown table
type Row struct {
	StatusEmoji      string            // Status emoji (e.g., ":green_circle:")
	StatusCaption    string            // Status caption (e.g., "On Track")
	StatusTransition *string           // e.g., ":yellow_circle:→:green_circle:" — rendered instead of emoji when set
	NewItem          bool              // true if this item wasn't in the previous report
	EpicTitle        string            // Epic/issue title
	EpicURL          string            // Epic/issue URL
	TargetDate       *time.Time        // Target date (nil renders as "TBD")
	UpdateMD         string            // Update summary/content (markdown-ready)
	Assignees        []string          // For grouping by assignee
	Labels           []string          // For grouping by label
	ExtraColumns     map[string]string // For custom columns and field grouping
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

// RenderTable generates a markdown table from a slice of rows.
// extraColumns are optional column names inserted between "Initiative/Epic" and "Target Date".
// When nil or empty the output is identical to the original 4-column format.
// Extra column values are read from row.ExtraColumns[columnName]; missing map or key → empty cell.
func RenderTable(rows []Row, extraColumns []string) string {
	if len(rows) == 0 {
		return ""
	}

	var builder strings.Builder

	// Build header
	header := "| Status | Initiative/Epic |"
	sep := "|--------|-----------------|"
	for _, col := range extraColumns {
		header += fmt.Sprintf(" %s |", col)
		sep += fmt.Sprintf("%s|", strings.Repeat("-", len(col)+2))
	}
	header += " Target Date | Update |"
	sep += "-------------|--------|"
	builder.WriteString(header + "\n")
	builder.WriteString(sep + "\n")

	// Write each row
	for _, row := range rows {
		// Format status column
		var statusCol string
		if row.NewItem {
			statusCol = fmt.Sprintf("🆕 %s %s", row.StatusEmoji, row.StatusCaption)
		} else if row.StatusTransition != nil {
			statusCol = fmt.Sprintf("%s %s", *row.StatusTransition, row.StatusCaption)
		} else {
			statusCol = fmt.Sprintf("%s %s", row.StatusEmoji, row.StatusCaption)
		}

		// Format epic column with markdown link
		epicCol := fmt.Sprintf("[%s](%s)",
			escapeMarkdownTableCell(row.EpicTitle),
			row.EpicURL)

		// Format target date column
		dateCol := derive.RenderTargetDate(row.TargetDate)

		// Format update column (collapse newlines and escape pipes)
		updateCol := escapeMarkdownTableCell(collapseNewlines(row.UpdateMD))

		// Build extra column cells
		extraCells := ""
		for _, col := range extraColumns {
			val := ""
			if row.ExtraColumns != nil {
				val = escapeMarkdownTableCell(row.ExtraColumns[col])
			}
			extraCells += fmt.Sprintf(" %s |", val)
		}

		builder.WriteString(fmt.Sprintf("| %s | %s |%s %s | %s |\n",
			statusCol, epicCol, extraCells, dateCol, updateCol))
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
func RenderTableWithTitle(title string, rows []Row, extraColumns []string) string {
	table := RenderTable(rows, extraColumns)
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

// getSortPriority determines the sorting priority tier for a row
// Priority 1: Items with target dates (highest priority)
// Priority 2: Items with updates but no target date
// Priority 3: Items that need updates or haven't started (lowest priority)
func getSortPriority(row Row) int {
	// Priority 1: Has target date
	if row.TargetDate != nil {
		return 1
	}

	// Priority 3: Needs updates or not started (lowest priority among undated items)
	if row.StatusCaption == "Needs Update" || row.StatusCaption == "Not Started" || row.StatusCaption == "Shaping" {
		return 3
	}

	// Priority 2: Has updates but no date
	return 2
}

// SortRowsByTargetDate sorts a slice of rows by priority and target date
// Priority 1: Items with target dates (sorted chronologically, earliest first)
// Priority 2: Items with updates but no target date
// Priority 3: Items that need updates or haven't started
func SortRowsByTargetDate(rows []Row) {
	sort.Slice(rows, func(i, j int) bool {
		priorityI := getSortPriority(rows[i])
		priorityJ := getSortPriority(rows[j])

		// Different priorities - lower number = higher priority
		if priorityI != priorityJ {
			return priorityI < priorityJ
		}

		// Same priority - handle based on priority type
		if priorityI == 1 {
			// Both have dates - sort chronologically
			return rows[i].TargetDate.Before(*rows[j].TargetDate)
		}

		// Priority 2 or 3 with no dates - maintain stable order
		return false
	})
}

package diff

import (
	"regexp"
	"strings"
)

// PreviousRow represents a row parsed from a previous report's markdown table.
type PreviousRow struct {
	IssueURL      string // Extracted from markdown link [title](url)
	StatusEmoji   string // e.g., ":green_circle:"
	StatusCaption string // e.g., "On Track"
	TargetDate    string // Raw string: "2024-01-15" or "TBD"
}

var (
	mdLinkRe    = regexp.MustCompile(`\[.*?\]\((https?://[^)]+)\)`)
	emojiRe     = regexp.MustCompile(`^(:[a-z_]+:)\s*(.*)$`)
	separatorRe = regexp.MustCompile(`^\|[-| :]+\|$`)
)

// ParseReport parses a markdown table from a previous report into PreviousRow structs.
// Malformed rows are silently skipped. Returns nil if no valid rows are found.
func ParseReport(content string) []PreviousRow {
	lines := strings.Split(content, "\n")
	var rows []PreviousRow

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "|") || !strings.HasSuffix(line, "|") {
			continue
		}
		// Skip separator rows
		if separatorRe.MatchString(line) {
			continue
		}

		// Temporarily replace escaped pipes to avoid splitting on them
		escaped := strings.ReplaceAll(line, `\|`, "\x00")
		parts := strings.Split(escaped[1:len(escaped)-1], "|")
		// Restore escaped pipes in each part
		for i, p := range parts {
			parts[i] = strings.ReplaceAll(p, "\x00", "|")
		}

		if len(parts) < 4 {
			continue
		}

		statusCell := strings.TrimSpace(parts[0])
		issueCell := strings.TrimSpace(parts[1])
		targetCell := strings.TrimSpace(parts[2])

		// Must have a markdown link
		urlMatch := mdLinkRe.FindStringSubmatch(issueCell)
		if urlMatch == nil {
			continue
		}

		// Skip header row
		if strings.EqualFold(statusCell, "status") {
			continue
		}

		// Parse emoji and caption from status cell
		emojiMatch := emojiRe.FindStringSubmatch(statusCell)
		if emojiMatch == nil {
			continue
		}

		rows = append(rows, PreviousRow{
			IssueURL:      urlMatch[1],
			StatusEmoji:   emojiMatch[1],
			StatusCaption: strings.TrimSpace(emojiMatch[2]),
			TargetDate:    targetCell,
		})
	}

	if len(rows) == 0 {
		return nil
	}
	return rows
}

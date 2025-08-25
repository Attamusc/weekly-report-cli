package report

import (
	"regexp"
	"strings"
	"time"
)

// MarkerIsReport is the HTML comment marker that identifies a status report
const MarkerIsReport = `<!-- data key="isReport" value="true" -->`

// Report represents a structured status report extracted from a comment
type Report struct {
	TrendingRaw string    // Raw trending/status value
	TargetDate  string    // Raw target date value
	UpdateRaw   string    // Raw update text (may be multiline)
	CreatedAt   time.Time // When the comment was created
	SourceURL   string    // URL of the source comment
}

var (
	// Case-insensitive regex for the report marker
	reportMarkerRegex = regexp.MustCompile(`(?i)<!--\s*data\s+key\s*=\s*"isReport"\s+value\s*=\s*"true"\s*-->`)

	// Regex for extracting keyed data blocks
	// Matches: <!-- data key="<key>" start --> content <!-- data end -->
	// (?s) enables dotall mode so . matches newlines
	dataBlockRegex = regexp.MustCompile(`(?is)<!--\s*data\s+key\s*=\s*"([^"]+)"\s+start\s*-->(.*?)<!--\s*data\s+end\s*-->`)
)

// ParseReport extracts a structured report from comment body text
// Returns (Report, true) if the comment contains a valid report marker and at least one data key
// Returns (Report{}, false) if the comment is not a report or contains no valid data
func ParseReport(body string, createdAt time.Time, sourceURL string) (Report, bool) {
	// Check for report marker (case-insensitive)
	if !reportMarkerRegex.MatchString(body) {
		return Report{}, false
	}

	// Extract all data blocks
	matches := dataBlockRegex.FindAllStringSubmatch(body, -1)
	if len(matches) == 0 {
		return Report{}, false
	}

	// Initialize report with metadata
	report := Report{
		CreatedAt: createdAt,
		SourceURL: sourceURL,
	}

	hasValidData := false

	// Process each data block
	for _, match := range matches {
		if len(match) != 3 {
			continue
		}

		key := strings.TrimSpace(match[1])
		value := strings.TrimSpace(match[2])

		// Skip empty values
		if value == "" {
			continue
		}

		// Map keys to report fields
		switch strings.ToLower(key) {
		case "trending":
			report.TrendingRaw = value
			hasValidData = true
		case "target_date":
			report.TargetDate = value
			hasValidData = true
		case "update":
			report.UpdateRaw = value
			hasValidData = true
		}
	}

	// Return report only if we found at least one valid data key
	if !hasValidData {
		return Report{}, false
	}

	return report, true
}

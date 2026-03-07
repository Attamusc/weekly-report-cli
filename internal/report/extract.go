package report

import (
	"regexp"
	"strings"
	"time"

	"github.com/Attamusc/weekly-report-cli/internal/derive"
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

var (
	// Matches a markdown heading containing "trending" (any level h1-h6)
	semiTrendingHeadingRegex = regexp.MustCompile(`(?im)^#{1,6}\s+trending\s*$`)

	// Matches a markdown heading containing "update" (any level h1-h6)
	semiUpdateHeadingRegex = regexp.MustCompile(`(?im)^#{1,6}\s+update\s*$`)

	// Matches a markdown heading containing "target date" (any level h1-h6)
	semiTargetDateHeadingRegex = regexp.MustCompile(`(?im)^#{1,6}\s+target\s*date\s*$`)

	// Matches any known section heading (used to find section boundaries).
	// Only matches known headings to avoid truncating sub-headings (e.g.,
	// #### Completed) within update content.
	knownSectionHeadingRegex = regexp.MustCompile(`(?im)^#{1,6}\s+(trending|update|target\s*date|summary)\s*$`)
)

// extractSectionContent extracts the content between a heading match and the
// next known section heading (or end of string). Returns the trimmed content.
func extractSectionContent(body string, headingLoc []int) string {
	// Start after the heading
	start := headingLoc[1]
	rest := body[start:]

	// Find the next known section heading
	nextLoc := knownSectionHeadingRegex.FindStringIndex(rest)
	var section string
	if nextLoc != nil {
		section = rest[:nextLoc[0]]
	} else {
		section = rest
	}

	return strings.TrimSpace(section)
}

// firstNonEmptyLine returns the first non-empty line from the given text.
func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// ParseSemiStructured extracts a report from a comment that uses markdown
// headings (### Trending, ### Update, ### Target Date) but lacks HTML comment
// markers. Returns (Report, true) if a trending heading with a recognizable
// status pattern is found. Comments that contain the structured report marker
// are explicitly rejected to avoid double-counting.
//
// Note: this function calls derive.MapTrending() for status validation, creating
// a semantic dependency. Changes to statusMappings in derive will change what
// the semi-structured parser accepts. This is desirable (they should stay in sync).
func ParseSemiStructured(body string, createdAt time.Time, sourceURL string) (Report, bool) {
	// Reject if body contains structured report markers -- those belong to ParseReport()
	if reportMarkerRegex.MatchString(body) {
		return Report{}, false
	}

	// Find trending heading
	trendingLoc := semiTrendingHeadingRegex.FindStringIndex(body)
	if trendingLoc == nil {
		return Report{}, false
	}

	// Extract trending section content
	trendingContent := extractSectionContent(body, trendingLoc)
	trendingValue := firstNonEmptyLine(trendingContent)
	if trendingValue == "" {
		return Report{}, false
	}

	// Validate status via derive.MapTrending(). If it returns Unknown, reject
	// the parse to prevent false positives from comments with unrelated content
	// under a "Trending" heading.
	status := derive.MapTrending(trendingValue)
	if status == derive.Unknown {
		return Report{}, false
	}

	result := Report{
		TrendingRaw: trendingValue,
		CreatedAt:   createdAt,
		SourceURL:   sourceURL,
	}

	// Extract update section (optional)
	updateLoc := semiUpdateHeadingRegex.FindStringIndex(body)
	if updateLoc != nil {
		result.UpdateRaw = extractSectionContent(body, updateLoc)
	}

	// Extract target date section (optional)
	targetDateLoc := semiTargetDateHeadingRegex.FindStringIndex(body)
	if targetDateLoc != nil {
		targetDateContent := extractSectionContent(body, targetDateLoc)
		result.TargetDate = firstNonEmptyLine(targetDateContent)
	}

	return result, true
}

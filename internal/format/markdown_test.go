package format

import (
	"strings"
	"testing"
	"time"

	"github.com/Attamusc/weekly-report-cli/internal/derive"
)

func TestRenderTable(t *testing.T) {
	// Helper to create a UTC time
	utcTime := func(year int, month time.Month, day int) *time.Time {
		t := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
		return &t
	}

	tests := []struct {
		name     string
		rows     []Row
		expected string
	}{
		{
			name: "empty rows",
			rows: []Row{},
			expected: "",
		},
		{
			name: "single row",
			rows: []Row{
				{
					StatusEmoji:   ":green_circle:",
					StatusCaption: "On Track",
					EpicTitle:     "User Authentication",
					EpicURL:       "https://github.com/owner/repo/issues/123",
					TargetDate:    utcTime(2025, 8, 6),
					UpdateMD:      "Completed OAuth2 integration",
				},
			},
			expected: `| Status | Initiative/Epic | Target Date | Update |
|--------|-----------------|-------------|--------|
| :green_circle: On Track | [Epic: User Authentication](https://github.com/owner/repo/issues/123) | 2025-08-06 | Completed OAuth2 integration |
`,
		},
		{
			name: "multiple rows with different statuses",
			rows: []Row{
				{
					StatusEmoji:   ":green_circle:",
					StatusCaption: "On Track",
					EpicTitle:     "User Authentication",
					EpicURL:       "https://github.com/owner/repo/issues/123",
					TargetDate:    utcTime(2025, 8, 6),
					UpdateMD:      "OAuth2 integration complete",
				},
				{
					StatusEmoji:   ":red_circle:",
					StatusCaption: "Off Track",
					EpicTitle:     "Payment Processing",
					EpicURL:       "https://github.com/owner/repo/issues/456",
					TargetDate:    nil, // Should render as TBD
					UpdateMD:      "Blocked by API changes",
				},
			},
			expected: `| Status | Initiative/Epic | Target Date | Update |
|--------|-----------------|-------------|--------|
| :green_circle: On Track | [Epic: User Authentication](https://github.com/owner/repo/issues/123) | 2025-08-06 | OAuth2 integration complete |
| :red_circle: Off Track | [Epic: Payment Processing](https://github.com/owner/repo/issues/456) | TBD | Blocked by API changes |
`,
		},
		{
			name: "row with special characters requiring escaping",
			rows: []Row{
				{
					StatusEmoji:   ":yellow_circle:",
					StatusCaption: "At Risk",
					EpicTitle:     "Feature | With Pipes",
					EpicURL:       "https://github.com/owner/repo/issues/789",
					TargetDate:    utcTime(2025, 12, 25),
					UpdateMD:      "Update with | pipes and\nnewlines",
				},
			},
			expected: `| Status | Initiative/Epic | Target Date | Update |
|--------|-----------------|-------------|--------|
| :yellow_circle: At Risk | [Epic: Feature \| With Pipes](https://github.com/owner/repo/issues/789) | 2025-12-25 | Update with \| pipes and newlines |
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RenderTable(tt.rows)
			if result != tt.expected {
				t.Errorf("RenderTable() mismatch\nExpected:\n%s\nGot:\n%s", 
					tt.expected, result)
			}
		})
	}
}

func TestNewRow(t *testing.T) {
	targetDate := time.Date(2025, 8, 6, 0, 0, 0, 0, time.UTC)
	
	row := NewRow(
		derive.OnTrack,
		"Test Epic",
		"https://github.com/test/repo/issues/1",
		&targetDate,
		"Test update",
	)

	expected := Row{
		StatusEmoji:   ":green_circle:",
		StatusCaption: "On Track",
		EpicTitle:     "Test Epic",
		EpicURL:       "https://github.com/test/repo/issues/1",
		TargetDate:    &targetDate,
		UpdateMD:      "Test update",
	}

	if row != expected {
		t.Errorf("NewRow() = %+v, expected %+v", row, expected)
	}
}

func TestEscapeMarkdownTableCell(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no special characters",
			input:    "normal text",
			expected: "normal text",
		},
		{
			name:     "pipe characters",
			input:    "text | with | pipes",
			expected: "text \\| with \\| pipes",
		},
		{
			name:     "backslashes",
			input:    "text\\with\\backslashes",
			expected: "text\\\\with\\\\backslashes",
		},
		{
			name:     "newlines",
			input:    "text\nwith\nnewlines",
			expected: "text with newlines",
		},
		{
			name:     "carriage returns",
			input:    "text\rwith\rcarriage",
			expected: "text with carriage",
		},
		{
			name:     "tabs",
			input:    "text\twith\ttabs",
			expected: "text with tabs",
		},
		{
			name:     "mixed line endings",
			input:    "text\r\nwith\nmixed\rlines",
			expected: "text with mixed lines",
		},
		{
			name:     "leading and trailing whitespace",
			input:    "   text with spaces   ",
			expected: "text with spaces",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only whitespace",
			input:    "   \n\t\r   ",
			expected: "",
		},
		{
			name:     "complex combination",
			input:    "  Complex | text\\with\nnewlines\tand | pipes  ",
			expected: "Complex \\| text\\\\with newlines and \\| pipes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeMarkdownTableCell(tt.input)
			if result != tt.expected {
				t.Errorf("escapeMarkdownTableCell(%q) = %q, expected %q", 
					tt.input, result, tt.expected)
			}
		})
	}
}

func TestCollapseNewlines(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no newlines",
			input:    "normal text",
			expected: "normal text",
		},
		{
			name:     "unix newlines",
			input:    "text\nwith\nnewlines",
			expected: "text with newlines",
		},
		{
			name:     "windows newlines",
			input:    "text\r\nwith\r\nwindows",
			expected: "text with windows",
		},
		{
			name:     "mac newlines",
			input:    "text\rwith\rmac",
			expected: "text with mac",
		},
		{
			name:     "mixed newlines",
			input:    "text\nwith\r\nmixed\rlines",
			expected: "text with mixed lines",
		},
		{
			name:     "multiple spaces",
			input:    "text  with   multiple    spaces",
			expected: "text with multiple spaces",
		},
		{
			name:     "newlines and spaces",
			input:    "text\n  with  \nspaces  and\nnewlines",
			expected: "text with spaces and newlines",
		},
		{
			name:     "leading and trailing whitespace",
			input:    "  \ntext\n  ",
			expected: "text",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only whitespace and newlines",
			input:    "\n  \r\n  \n",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := collapseNewlines(tt.input)
			if result != tt.expected {
				t.Errorf("collapseNewlines(%q) = %q, expected %q", 
					tt.input, result, tt.expected)
			}
		})
	}
}

func TestRenderTableWithTitle(t *testing.T) {
	utcTime := func(year int, month time.Month, day int) *time.Time {
		t := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
		return &t
	}

	rows := []Row{
		{
			StatusEmoji:   ":green_circle:",
			StatusCaption: "On Track",
			EpicTitle:     "Test Epic",
			EpicURL:       "https://github.com/test/repo/issues/1",
			TargetDate:    utcTime(2025, 8, 6),
			UpdateMD:      "Test update",
		},
	}

	tests := []struct {
		name     string
		title    string
		rows     []Row
		expected string
	}{
		{
			name:     "with title",
			title:    "Weekly Status Report",
			rows:     rows,
			expected: `# Weekly Status Report

| Status | Initiative/Epic | Target Date | Update |
|--------|-----------------|-------------|--------|
| :green_circle: On Track | [Epic: Test Epic](https://github.com/test/repo/issues/1) | 2025-08-06 | Test update |
`,
		},
		{
			name:     "without title",
			title:    "",
			rows:     rows,
			expected: `| Status | Initiative/Epic | Target Date | Update |
|--------|-----------------|-------------|--------|
| :green_circle: On Track | [Epic: Test Epic](https://github.com/test/repo/issues/1) | 2025-08-06 | Test update |
`,
		},
		{
			name:     "empty rows with title",
			title:    "Empty Report",
			rows:     []Row{},
			expected: "",
		},
		{
			name:     "empty rows without title",
			title:    "",
			rows:     []Row{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RenderTableWithTitle(tt.title, tt.rows)
			if result != tt.expected {
				t.Errorf("RenderTableWithTitle() mismatch\nExpected:\n%s\nGot:\n%s", 
					tt.expected, result)
			}
		})
	}
}

// Test that the table format matches the expected golden output
func TestTableGoldenFormat(t *testing.T) {
	// Create test data that matches the documentation example
	utcTime := func(year int, month time.Month, day int) *time.Time {
		t := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
		return &t
	}

	rows := []Row{
		{
			StatusEmoji:   ":purple_circle:",
			StatusCaption: "Done",
			EpicTitle:     "User Authentication System",
			EpicURL:       "https://github.com/owner/repo/issues/123",
			TargetDate:    utcTime(2025, 8, 6),
			UpdateMD:      "Completed OAuth2 integration and session management with passing tests",
		},
		{
			StatusEmoji:   ":red_circle:",
			StatusCaption: "Off Track",
			EpicTitle:     "Payment Gateway Integration",
			EpicURL:       "https://github.com/owner/repo/issues/456",
			TargetDate:    nil,
			UpdateMD:      "Blocked by third-party API changes, need to redesign approach",
		},
	}

	expected := `| Status | Initiative/Epic | Target Date | Update |
|--------|-----------------|-------------|--------|
| :purple_circle: Done | [Epic: User Authentication System](https://github.com/owner/repo/issues/123) | 2025-08-06 | Completed OAuth2 integration and session management with passing tests |
| :red_circle: Off Track | [Epic: Payment Gateway Integration](https://github.com/owner/repo/issues/456) | TBD | Blocked by third-party API changes, need to redesign approach |
`

	result := RenderTable(rows)
	if result != expected {
		t.Errorf("Golden table format mismatch\nExpected:\n%s\nGot:\n%s", 
			expected, result)
	}

	// Verify table structure
	lines := strings.Split(strings.TrimSpace(result), "\n")
	if len(lines) != 4 { // header + separator + 2 data rows
		t.Errorf("Expected 4 lines in table output, got %d", len(lines))
	}

	// Verify header
	expectedHeader := "| Status | Initiative/Epic | Target Date | Update |"
	if lines[0] != expectedHeader {
		t.Errorf("Header mismatch\nExpected: %s\nGot: %s", expectedHeader, lines[0])
	}

	// Verify separator
	expectedSeparator := "|--------|-----------------|-------------|--------|"
	if lines[1] != expectedSeparator {
		t.Errorf("Separator mismatch\nExpected: %s\nGot: %s", expectedSeparator, lines[1])
	}

	// Verify each row has correct number of columns
	for i, line := range lines[2:] {
		columns := strings.Split(line, "|")
		if len(columns) != 6 { // empty + 4 columns + empty (due to leading/trailing |)
			t.Errorf("Row %d has %d columns, expected 6 (including empty): %s", 
				i, len(columns), line)
		}
	}
}
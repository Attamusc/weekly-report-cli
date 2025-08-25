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
			name:     "empty rows",
			rows:     []Row{},
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
| :green_circle: On Track | [User Authentication](https://github.com/owner/repo/issues/123) | 2025-08-06 | Completed OAuth2 integration |
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
| :green_circle: On Track | [User Authentication](https://github.com/owner/repo/issues/123) | 2025-08-06 | OAuth2 integration complete |
| :red_circle: Off Track | [Payment Processing](https://github.com/owner/repo/issues/456) | TBD | Blocked by API changes |
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
| :yellow_circle: At Risk | [Feature \| With Pipes](https://github.com/owner/repo/issues/789) | 2025-12-25 | Update with \| pipes and newlines |
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
			name:  "with title",
			title: "Weekly Status Report",
			rows:  rows,
			expected: `# Weekly Status Report

| Status | Initiative/Epic | Target Date | Update |
|--------|-----------------|-------------|--------|
| :green_circle: On Track | [Test Epic](https://github.com/test/repo/issues/1) | 2025-08-06 | Test update |
`,
		},
		{
			name:  "without title",
			title: "",
			rows:  rows,
			expected: `| Status | Initiative/Epic | Target Date | Update |
|--------|-----------------|-------------|--------|
| :green_circle: On Track | [Test Epic](https://github.com/test/repo/issues/1) | 2025-08-06 | Test update |
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
| :purple_circle: Done | [User Authentication System](https://github.com/owner/repo/issues/123) | 2025-08-06 | Completed OAuth2 integration and session management with passing tests |
| :red_circle: Off Track | [Payment Gateway Integration](https://github.com/owner/repo/issues/456) | TBD | Blocked by third-party API changes, need to redesign approach |
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

func TestSortRowsByTargetDate(t *testing.T) {
	utcTime := func(year int, month time.Month, day int) *time.Time {
		t := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
		return &t
	}

	tests := []struct {
		name     string
		input    []Row
		expected []string // Expected order of EpicTitle fields
	}{
		{
			name: "mixed dates and TBD",
			input: []Row{
				{EpicTitle: "Future Task", TargetDate: utcTime(2025, 12, 31), StatusCaption: "On Track"},
				{EpicTitle: "No Date Task", TargetDate: nil, StatusCaption: "At Risk"},
				{EpicTitle: "Soon Task", TargetDate: utcTime(2025, 8, 15), StatusCaption: "On Track"},
				{EpicTitle: "Another TBD", TargetDate: nil, StatusCaption: "Done"},
				{EpicTitle: "Earlier Task", TargetDate: utcTime(2025, 8, 1), StatusCaption: "On Track"},
			},
			expected: []string{
				"Earlier Task", // 2025-08-01 (Priority 1)
				"Soon Task",    // 2025-08-15 (Priority 1)
				"Future Task",  // 2025-12-31 (Priority 1)
				"No Date Task", // Priority 2 (has updates)
				"Another TBD",  // Priority 2 (has updates)
			},
		},
		{
			name: "all TBD",
			input: []Row{
				{EpicTitle: "Task A", TargetDate: nil, StatusCaption: "At Risk"},
				{EpicTitle: "Task B", TargetDate: nil, StatusCaption: "Done"},
			},
			expected: []string{"Task A", "Task B"}, // Stable order
		},
		{
			name: "all dated",
			input: []Row{
				{EpicTitle: "Late", TargetDate: utcTime(2025, 12, 1), StatusCaption: "On Track"},
				{EpicTitle: "Early", TargetDate: utcTime(2025, 6, 1), StatusCaption: "On Track"},
				{EpicTitle: "Middle", TargetDate: utcTime(2025, 9, 1), StatusCaption: "On Track"},
			},
			expected: []string{"Early", "Middle", "Late"},
		},
		{
			name:     "empty slice",
			input:    []Row{},
			expected: []string{},
		},
		{
			name:     "single item",
			input:    []Row{{EpicTitle: "Only Task", TargetDate: utcTime(2025, 8, 1), StatusCaption: "On Track"}},
			expected: []string{"Only Task"},
		},
		{
			name: "same dates",
			input: []Row{
				{EpicTitle: "Task B", TargetDate: utcTime(2025, 8, 1), StatusCaption: "On Track"},
				{EpicTitle: "Task A", TargetDate: utcTime(2025, 8, 1), StatusCaption: "On Track"},
			},
			expected: []string{"Task B", "Task A"}, // Stable order preserved
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy to avoid modifying the original
			rows := make([]Row, len(tt.input))
			copy(rows, tt.input)

			SortRowsByTargetDate(rows)

			if len(rows) != len(tt.expected) {
				t.Fatalf("Expected %d rows, got %d", len(tt.expected), len(rows))
			}

			for i, expectedTitle := range tt.expected {
				if rows[i].EpicTitle != expectedTitle {
					t.Errorf("Position %d: expected %q, got %q",
						i, expectedTitle, rows[i].EpicTitle)
				}
			}
		})
	}
}

func TestSortRowsByPriority(t *testing.T) {
	utcTime := func(year int, month time.Month, day int) *time.Time {
		t := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
		return &t
	}

	tests := []struct {
		name     string
		input    []Row
		expected []string // Expected order of EpicTitle fields
	}{
		{
			name: "priority ordering with mixed statuses",
			input: []Row{
				{EpicTitle: "Needs Update Task", TargetDate: nil, StatusCaption: "Needs Update"},
				{EpicTitle: "Future Dated", TargetDate: utcTime(2025, 12, 31), StatusCaption: "On Track"},
				{EpicTitle: "Not Started Task", TargetDate: nil, StatusCaption: "Not Started"},
				{EpicTitle: "Has Updates No Date", TargetDate: nil, StatusCaption: "At Risk"},
				{EpicTitle: "Early Dated", TargetDate: utcTime(2025, 8, 1), StatusCaption: "On Track"},
				{EpicTitle: "Done No Date", TargetDate: nil, StatusCaption: "Done"},
			},
			expected: []string{
				"Early Dated",         // Priority 1: Has date (2025-08-01)
				"Future Dated",        // Priority 1: Has date (2025-12-31)
				"Has Updates No Date", // Priority 2: Has updates, no date
				"Done No Date",        // Priority 2: Has updates, no date
				"Needs Update Task",   // Priority 3: Needs updates
				"Not Started Task",    // Priority 3: Not started
			},
		},
		{
			name: "all need updates or not started",
			input: []Row{
				{EpicTitle: "Task B", TargetDate: nil, StatusCaption: "Not Started"},
				{EpicTitle: "Task A", TargetDate: nil, StatusCaption: "Needs Update"},
				{EpicTitle: "Task C", TargetDate: nil, StatusCaption: "Not Started"},
			},
			expected: []string{"Task B", "Task A", "Task C"}, // Stable order within priority
		},
		{
			name: "all have updates but no dates",
			input: []Row{
				{EpicTitle: "Task Y", TargetDate: nil, StatusCaption: "At Risk"},
				{EpicTitle: "Task X", TargetDate: nil, StatusCaption: "Done"},
				{EpicTitle: "Task Z", TargetDate: nil, StatusCaption: "Off Track"},
			},
			expected: []string{"Task Y", "Task X", "Task Z"}, // Stable order within priority
		},
		{
			name: "mixed priorities ensure correct ordering",
			input: []Row{
				{EpicTitle: "Priority 3 A", TargetDate: nil, StatusCaption: "Needs Update"},
				{EpicTitle: "Priority 1", TargetDate: utcTime(2025, 6, 1), StatusCaption: "On Track"},
				{EpicTitle: "Priority 2", TargetDate: nil, StatusCaption: "At Risk"},
				{EpicTitle: "Priority 3 B", TargetDate: nil, StatusCaption: "Not Started"},
			},
			expected: []string{
				"Priority 1",   // Priority 1: Has date
				"Priority 2",   // Priority 2: Has updates
				"Priority 3 A", // Priority 3: Needs updates
				"Priority 3 B", // Priority 3: Not started
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy to avoid modifying the original
			rows := make([]Row, len(tt.input))
			copy(rows, tt.input)

			SortRowsByTargetDate(rows)

			if len(rows) != len(tt.expected) {
				t.Fatalf("Expected %d rows, got %d", len(tt.expected), len(rows))
			}

			for i, expectedTitle := range tt.expected {
				if rows[i].EpicTitle != expectedTitle {
					t.Errorf("Position %d: expected %q, got %q",
						i, expectedTitle, rows[i].EpicTitle)
				}
			}
		})
	}
}

func TestGetSortPriority(t *testing.T) {
	utcTime := func(year int, month time.Month, day int) *time.Time {
		t := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
		return &t
	}

	tests := []struct {
		name             string
		row              Row
		expectedPriority int
	}{
		{
			name: "has target date - priority 1",
			row: Row{
				TargetDate:    utcTime(2025, 8, 1),
				StatusCaption: "On Track",
			},
			expectedPriority: 1,
		},
		{
			name: "needs update, no date - priority 3",
			row: Row{
				TargetDate:    nil,
				StatusCaption: "Needs Update",
			},
			expectedPriority: 3,
		},
		{
			name: "not started, no date - priority 3",
			row: Row{
				TargetDate:    nil,
				StatusCaption: "Not Started",
			},
			expectedPriority: 3,
		},
		{
			name: "has updates, no date - priority 2",
			row: Row{
				TargetDate:    nil,
				StatusCaption: "At Risk",
			},
			expectedPriority: 2,
		},
		{
			name: "done, no date - priority 2",
			row: Row{
				TargetDate:    nil,
				StatusCaption: "Done",
			},
			expectedPriority: 2,
		},
		{
			name: "off track, no date - priority 2",
			row: Row{
				TargetDate:    nil,
				StatusCaption: "Off Track",
			},
			expectedPriority: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			priority := getSortPriority(tt.row)
			if priority != tt.expectedPriority {
				t.Errorf("getSortPriority() = %d, expected %d", priority, tt.expectedPriority)
			}
		})
	}
}

func TestSortAndRenderIntegration(t *testing.T) {
	// Test the complete flow: sort then render
	utcTime := func(year int, month time.Month, day int) *time.Time {
		t := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
		return &t
	}

	// Create rows in mixed order (like they might come from processing)
	rows := []Row{
		{
			StatusEmoji:   ":yellow_circle:",
			StatusCaption: "At Risk",
			EpicTitle:     "Future Feature",
			EpicURL:       "https://github.com/owner/repo/issues/789",
			TargetDate:    utcTime(2025, 12, 15),
			UpdateMD:      "Planning in progress",
		},
		{
			StatusEmoji:   ":red_circle:",
			StatusCaption: "Off Track",
			EpicTitle:     "No Target Task",
			EpicURL:       "https://github.com/owner/repo/issues/101",
			TargetDate:    nil,
			UpdateMD:      "Needs prioritization",
		},
		{
			StatusEmoji:   ":green_circle:",
			StatusCaption: "On Track",
			EpicTitle:     "Immediate Task",
			EpicURL:       "https://github.com/owner/repo/issues/456",
			TargetDate:    utcTime(2025, 8, 20),
			UpdateMD:      "In progress",
		},
	}

	// Sort the rows
	SortRowsByTargetDate(rows)

	// Render the table
	result := RenderTable(rows)

	// Verify the order in the rendered output
	lines := strings.Split(strings.TrimSpace(result), "\n")
	if len(lines) < 5 { // header + separator + 3 data rows
		t.Fatalf("Expected at least 5 lines, got %d", len(lines))
	}

	// Check that the rows are in the correct order after sorting
	dataLines := lines[2:] // Skip header and separator

	// First row should be "Immediate Task" (2025-08-20)
	if !strings.Contains(dataLines[0], "Immediate Task") {
		t.Errorf("First row should contain 'Immediate Task', got: %s", dataLines[0])
	}

	// Second row should be "Future Feature" (2025-12-15)
	if !strings.Contains(dataLines[1], "Future Feature") {
		t.Errorf("Second row should contain 'Future Feature', got: %s", dataLines[1])
	}

	// Third row should be "No Target Task" (TBD/nil)
	if !strings.Contains(dataLines[2], "No Target Task") {
		t.Errorf("Third row should contain 'No Target Task', got: %s", dataLines[2])
	}

	// Verify TBD appears in the last row
	if !strings.Contains(dataLines[2], "TBD") {
		t.Errorf("Last row should contain 'TBD', got: %s", dataLines[2])
	}
}

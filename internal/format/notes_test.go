package format

import (
	"strings"
	"testing"
)

func TestRenderNotes(t *testing.T) {
	tests := []struct {
		name     string
		notes    []Note
		expected string
	}{
		{
			name:  "empty notes",
			notes: []Note{},
			expected: "",
		},
		{
			name: "single multiple updates note",
			notes: []Note{
				{
					Kind:      NoteMultipleUpdates,
					IssueURL:  "https://github.com/owner/repo/issues/123",
					SinceDays: 7,
				},
			},
			expected: `## Notes

- https://github.com/owner/repo/issues/123: multiple structured updates in last 7 days
`,
		},
		{
			name: "single no updates note",
			notes: []Note{
				{
					Kind:      NoteNoUpdatesInWindow,
					IssueURL:  "https://github.com/owner/repo/issues/456",
					SinceDays: 14,
				},
			},
			expected: `## Notes

- https://github.com/owner/repo/issues/456: no update in last 14 days
`,
		},
		{
			name: "multiple notes of different types",
			notes: []Note{
				{
					Kind:      NoteMultipleUpdates,
					IssueURL:  "https://github.com/owner/repo/issues/123",
					SinceDays: 7,
				},
				{
					Kind:      NoteNoUpdatesInWindow,
					IssueURL:  "https://github.com/owner/repo/issues/456",
					SinceDays: 14,
				},
				{
					Kind:      NoteMultipleUpdates,
					IssueURL:  "https://github.com/owner/repo/issues/789",
					SinceDays: 3,
				},
			},
			expected: `## Notes

- https://github.com/owner/repo/issues/123: multiple structured updates in last 7 days
- https://github.com/owner/repo/issues/456: no update in last 14 days
- https://github.com/owner/repo/issues/789: multiple structured updates in last 3 days
`,
		},
		{
			name: "single day pluralization",
			notes: []Note{
				{
					Kind:      NoteNoUpdatesInWindow,
					IssueURL:  "https://github.com/owner/repo/issues/1",
					SinceDays: 1,
				},
				{
					Kind:      NoteMultipleUpdates,
					IssueURL:  "https://github.com/owner/repo/issues/2",
					SinceDays: 1,
				},
			},
			expected: `## Notes

- https://github.com/owner/repo/issues/1: no update in last 1 day
- https://github.com/owner/repo/issues/2: multiple structured updates in last 1 day
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RenderNotes(tt.notes)
			if result != tt.expected {
				t.Errorf("RenderNotes() mismatch\nExpected:\n%s\nGot:\n%s", 
					tt.expected, result)
			}
		})
	}
}

func TestRenderNoteBullet(t *testing.T) {
	tests := []struct {
		name     string
		note     Note
		expected string
	}{
		{
			name: "multiple updates note",
			note: Note{
				Kind:      NoteMultipleUpdates,
				IssueURL:  "https://github.com/owner/repo/issues/123",
				SinceDays: 7,
			},
			expected: "https://github.com/owner/repo/issues/123: multiple structured updates in last 7 days",
		},
		{
			name: "no updates note",
			note: Note{
				Kind:      NoteNoUpdatesInWindow,
				IssueURL:  "https://github.com/owner/repo/issues/456",
				SinceDays: 14,
			},
			expected: "https://github.com/owner/repo/issues/456: no update in last 14 days",
		},
		{
			name: "single day multiple updates",
			note: Note{
				Kind:      NoteMultipleUpdates,
				IssueURL:  "https://github.com/owner/repo/issues/1",
				SinceDays: 1,
			},
			expected: "https://github.com/owner/repo/issues/1: multiple structured updates in last 1 day",
		},
		{
			name: "single day no updates",
			note: Note{
				Kind:      NoteNoUpdatesInWindow,
				IssueURL:  "https://github.com/owner/repo/issues/2",
				SinceDays: 1,
			},
			expected: "https://github.com/owner/repo/issues/2: no update in last 1 day",
		},
		{
			name: "unknown note kind",
			note: Note{
				Kind:      NoteKind(999), // Invalid kind
				IssueURL:  "https://github.com/owner/repo/issues/123",
				SinceDays: 7,
			},
			expected: "", // Should return empty string for unknown kinds
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderNoteBullet(tt.note)
			if result != tt.expected {
				t.Errorf("renderNoteBullet(%+v) = %q, expected %q", 
					tt.note, result, tt.expected)
			}
		})
	}
}

func TestPluralizeDays(t *testing.T) {
	tests := []struct {
		name     string
		days     int
		expected string
	}{
		{
			name:     "zero days",
			days:     0,
			expected: "0 days",
		},
		{
			name:     "one day",
			days:     1,
			expected: "1 day",
		},
		{
			name:     "two days",
			days:     2,
			expected: "2 days",
		},
		{
			name:     "seven days",
			days:     7,
			expected: "7 days",
		},
		{
			name:     "fourteen days",
			days:     14,
			expected: "14 days",
		},
		{
			name:     "large number",
			days:     365,
			expected: "365 days",
		},
		{
			name:     "negative days (edge case)",
			days:     -1,
			expected: "-1 days",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pluralizeDays(tt.days)
			if result != tt.expected {
				t.Errorf("pluralizeDays(%d) = %q, expected %q", 
					tt.days, result, tt.expected)
			}
		})
	}
}

func TestHasNotesOfKind(t *testing.T) {
	notes := []Note{
		{Kind: NoteMultipleUpdates, IssueURL: "url1", SinceDays: 7},
		{Kind: NoteNoUpdatesInWindow, IssueURL: "url2", SinceDays: 14},
		{Kind: NoteMultipleUpdates, IssueURL: "url3", SinceDays: 3},
	}

	tests := []struct {
		name     string
		notes    []Note
		kind     NoteKind
		expected bool
	}{
		{
			name:     "has multiple updates",
			notes:    notes,
			kind:     NoteMultipleUpdates,
			expected: true,
		},
		{
			name:     "has no updates",
			notes:    notes,
			kind:     NoteNoUpdatesInWindow,
			expected: true,
		},
		{
			name:     "no matching kind",
			notes:    notes,
			kind:     NoteKind(999),
			expected: false,
		},
		{
			name:     "empty notes",
			notes:    []Note{},
			kind:     NoteMultipleUpdates,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasNotesOfKind(tt.notes, tt.kind)
			if result != tt.expected {
				t.Errorf("HasNotesOfKind() = %t, expected %t", result, tt.expected)
			}
		})
	}
}

func TestFilterNotesByKind(t *testing.T) {
	notes := []Note{
		{Kind: NoteMultipleUpdates, IssueURL: "url1", SinceDays: 7},
		{Kind: NoteNoUpdatesInWindow, IssueURL: "url2", SinceDays: 14},
		{Kind: NoteMultipleUpdates, IssueURL: "url3", SinceDays: 3},
		{Kind: NoteNoUpdatesInWindow, IssueURL: "url4", SinceDays: 21},
	}

	tests := []struct {
		name     string
		notes    []Note
		kind     NoteKind
		expected int // Expected count of filtered notes
	}{
		{
			name:     "filter multiple updates",
			notes:    notes,
			kind:     NoteMultipleUpdates,
			expected: 2,
		},
		{
			name:     "filter no updates",
			notes:    notes,
			kind:     NoteNoUpdatesInWindow,
			expected: 2,
		},
		{
			name:     "filter unknown kind",
			notes:    notes,
			kind:     NoteKind(999),
			expected: 0,
		},
		{
			name:     "empty notes",
			notes:    []Note{},
			kind:     NoteMultipleUpdates,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterNotesByKind(tt.notes, tt.kind)
			if len(result) != tt.expected {
				t.Errorf("FilterNotesByKind() returned %d notes, expected %d", 
					len(result), tt.expected)
			}
			
			// Verify all returned notes have the correct kind
			for _, note := range result {
				if note.Kind != tt.kind {
					t.Errorf("FilterNotesByKind() returned note with kind %v, expected %v", 
						note.Kind, tt.kind)
				}
			}
		})
	}
}

func TestCountNotesByKind(t *testing.T) {
	notes := []Note{
		{Kind: NoteMultipleUpdates, IssueURL: "url1", SinceDays: 7},
		{Kind: NoteNoUpdatesInWindow, IssueURL: "url2", SinceDays: 14},
		{Kind: NoteMultipleUpdates, IssueURL: "url3", SinceDays: 3},
		{Kind: NoteNoUpdatesInWindow, IssueURL: "url4", SinceDays: 21},
		{Kind: NoteMultipleUpdates, IssueURL: "url5", SinceDays: 1},
	}

	tests := []struct {
		name     string
		notes    []Note
		kind     NoteKind
		expected int
	}{
		{
			name:     "count multiple updates",
			notes:    notes,
			kind:     NoteMultipleUpdates,
			expected: 3,
		},
		{
			name:     "count no updates",
			notes:    notes,
			kind:     NoteNoUpdatesInWindow,
			expected: 2,
		},
		{
			name:     "count unknown kind",
			notes:    notes,
			kind:     NoteKind(999),
			expected: 0,
		},
		{
			name:     "empty notes",
			notes:    []Note{},
			kind:     NoteMultipleUpdates,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CountNotesByKind(tt.notes, tt.kind)
			if result != tt.expected {
				t.Errorf("CountNotesByKind() = %d, expected %d", result, tt.expected)
			}
		})
	}
}

func TestNotesStructureAndFormat(t *testing.T) {
	// Test that the notes output follows the expected structure from documentation
	notes := []Note{
		{
			Kind:      NoteMultipleUpdates,
			IssueURL:  "https://github.com/owner/repo/issues/123",
			SinceDays: 7,
		},
		{
			Kind:      NoteNoUpdatesInWindow,
			IssueURL:  "https://github.com/owner/repo/issues/456",
			SinceDays: 14,
		},
	}

	result := RenderNotes(notes)
	lines := strings.Split(strings.TrimSpace(result), "\n")

	// Should have header + empty line + 2 bullet points = 4 lines
	if len(lines) != 4 {
		t.Errorf("Expected 4 lines in notes output, got %d", len(lines))
	}

	// Check header
	if lines[0] != "## Notes" {
		t.Errorf("Expected '## Notes' header, got %q", lines[0])
	}

	// Check empty line after header
	if lines[1] != "" {
		t.Errorf("Expected empty line after header, got %q", lines[1])
	}

	// Check bullet points start with "- "
	for i, line := range lines[2:] {
		if !strings.HasPrefix(line, "- ") {
			t.Errorf("Line %d should start with '- ', got %q", i+2, line)
		}
	}

	// Check content format
	expectedPatterns := []string{
		"multiple structured updates in last 7 days",
		"no update in last 14 days",
	}

	for i, pattern := range expectedPatterns {
		if !strings.Contains(lines[i+2], pattern) {
			t.Errorf("Line %d should contain %q, got %q", 
				i+2, pattern, lines[i+2])
		}
	}
}
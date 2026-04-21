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
			name:     "empty notes",
			notes:    []Note{},
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
		{
			name: "unstructured fallback note",
			notes: []Note{
				{
					Kind:     NoteUnstructuredFallback,
					IssueURL: "https://github.com/owner/repo/issues/42",
				},
			},
			expected: "## Notes\n\n- https://github.com/owner/repo/issues/42: no structured update found \u2014 summary derived from most recent comment\n",
		},
		{
			name: "mixed notes including unstructured fallback",
			notes: []Note{
				{
					Kind:      NoteMultipleUpdates,
					IssueURL:  "https://github.com/owner/repo/issues/1",
					SinceDays: 7,
				},
				{
					Kind:     NoteUnstructuredFallback,
					IssueURL: "https://github.com/owner/repo/issues/2",
				},
				{
					Kind:      NoteNoUpdatesInWindow,
					IssueURL:  "https://github.com/owner/repo/issues/3",
					SinceDays: 14,
				},
			},
			expected: `## Notes

- https://github.com/owner/repo/issues/1: multiple structured updates in last 7 days
- https://github.com/owner/repo/issues/2: no structured update found` + " \u2014 " + `summary derived from most recent comment
- https://github.com/owner/repo/issues/3: no update in last 14 days
`,
		},
		{
			name: "sentiment mismatch note",
			notes: []Note{
				{
					Kind:            NoteSentimentMismatch,
					IssueURL:        "https://github.com/owner/repo/issues/99",
					ReportedStatus:  "On Track",
					SuggestedStatus: "At Risk",
					Explanation:     "Update mentions two unresolved blockers.",
				},
			},
			expected: "## Notes\n\n- https://github.com/owner/repo/issues/99: reported as On Track, but sentiment suggests At Risk \u2014 Update mentions two unresolved blockers.\n",
		},
		{
			name: "mixed notes including sentiment mismatch",
			notes: []Note{
				{
					Kind:      NoteMultipleUpdates,
					IssueURL:  "https://github.com/owner/repo/issues/1",
					SinceDays: 7,
				},
				{
					Kind:            NoteSentimentMismatch,
					IssueURL:        "https://github.com/owner/repo/issues/2",
					ReportedStatus:  "On Track",
					SuggestedStatus: "Off Track",
					Explanation:     "Blocked on upstream dependency.",
				},
			},
			expected: "## Notes\n\n- https://github.com/owner/repo/issues/1: multiple structured updates in last 7 days\n- https://github.com/owner/repo/issues/2: reported as On Track, but sentiment suggests Off Track \u2014 Blocked on upstream dependency.\n",
		},
		{
			name: "new issue shaping note",
			notes: []Note{
				{
					Kind:     NoteNewIssueShaping,
					IssueURL: "https://github.com/owner/repo/issues/77",
				},
			},
			expected: "## Notes\n\n- https://github.com/owner/repo/issues/77: new issue \u2014 still being shaped\n",
		},
		{
			name: "mixed notes including new issue shaping",
			notes: []Note{
				{
					Kind:      NoteNoUpdatesInWindow,
					IssueURL:  "https://github.com/owner/repo/issues/1",
					SinceDays: 7,
				},
				{
					Kind:     NoteNewIssueShaping,
					IssueURL: "https://github.com/owner/repo/issues/2",
				},
			},
			expected: "## Notes\n\n- https://github.com/owner/repo/issues/1: no update in last 7 days\n- https://github.com/owner/repo/issues/2: new issue \u2014 still being shaped\n",
		},
		{
			name: "semi-structured fallback note",
			notes: []Note{
				{
					Kind:     NoteSemiStructuredFallback,
					IssueURL: "https://github.com/owner/repo/issues/88",
				},
			},
			expected: "## Notes\n\n- https://github.com/owner/repo/issues/88: status derived from markdown-formatted comment (not structured report)\n",
		},
		{
			name: "label fallback note",
			notes: []Note{
				{
					Kind:     NoteLabelFallback,
					IssueURL: "https://github.com/owner/repo/issues/99",
				},
			},
			expected: "## Notes\n\n- https://github.com/owner/repo/issues/99: status derived from issue label\n",
		},
		{
			name: "mixed notes including all fallback types",
			notes: []Note{
				{
					Kind:     NoteSemiStructuredFallback,
					IssueURL: "https://github.com/owner/repo/issues/1",
				},
				{
					Kind:     NoteLabelFallback,
					IssueURL: "https://github.com/owner/repo/issues/2",
				},
				{
					Kind:     NoteUnstructuredFallback,
					IssueURL: "https://github.com/owner/repo/issues/3",
				},
			},
			expected: "## Notes\n\n- https://github.com/owner/repo/issues/1: status derived from markdown-formatted comment (not structured report)\n- https://github.com/owner/repo/issues/2: status derived from issue label\n- https://github.com/owner/repo/issues/3: no structured update found \u2014 summary derived from most recent comment\n",
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
			name: "unstructured fallback",
			note: Note{
				Kind:     NoteUnstructuredFallback,
				IssueURL: "https://github.com/owner/repo/issues/42",
			},
			expected: "https://github.com/owner/repo/issues/42: no structured update found \u2014 summary derived from most recent comment",
		},
		{
			name: "sentiment mismatch",
			note: Note{
				Kind:            NoteSentimentMismatch,
				IssueURL:        "https://github.com/owner/repo/issues/55",
				ReportedStatus:  "On Track",
				SuggestedStatus: "At Risk",
				Explanation:     "Content describes unresolved blockers.",
			},
			expected: "https://github.com/owner/repo/issues/55: reported as On Track, but sentiment suggests At Risk \u2014 Content describes unresolved blockers.",
		},
		{
			name: "new issue shaping",
			note: Note{
				Kind:     NoteNewIssueShaping,
				IssueURL: "https://github.com/owner/repo/issues/77",
			},
			expected: "https://github.com/owner/repo/issues/77: new issue \u2014 still being shaped",
		},
		{
			name: "semi-structured fallback",
			note: Note{
				Kind:     NoteSemiStructuredFallback,
				IssueURL: "https://github.com/owner/repo/issues/88",
			},
			expected: "https://github.com/owner/repo/issues/88: status derived from markdown-formatted comment (not structured report)",
		},
		{
			name: "label fallback",
			note: Note{
				Kind:     NoteLabelFallback,
				IssueURL: "https://github.com/owner/repo/issues/99",
			},
			expected: "https://github.com/owner/repo/issues/99: status derived from issue label",
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
		{Kind: NoteUnstructuredFallback, IssueURL: "url4"},
		{Kind: NoteSentimentMismatch, IssueURL: "url5", ReportedStatus: "On Track", SuggestedStatus: "At Risk", Explanation: "blockers"},
		{Kind: NoteNewIssueShaping, IssueURL: "url6"},
		{Kind: NoteSemiStructuredFallback, IssueURL: "url7"},
		{Kind: NoteLabelFallback, IssueURL: "url8"},
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
			name:     "has unstructured fallback",
			notes:    notes,
			kind:     NoteUnstructuredFallback,
			expected: true,
		},
		{
			name:     "has sentiment mismatch",
			notes:    notes,
			kind:     NoteSentimentMismatch,
			expected: true,
		},
		{
			name:     "has new issue shaping",
			notes:    notes,
			kind:     NoteNewIssueShaping,
			expected: true,
		},
		{
			name:     "has semi-structured fallback",
			notes:    notes,
			kind:     NoteSemiStructuredFallback,
			expected: true,
		},
		{
			name:     "has label fallback",
			notes:    notes,
			kind:     NoteLabelFallback,
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

func TestRenderNotesCollapsible_Empty(t *testing.T) {
	result := RenderNotesCollapsible([]Note{})
	if result != "" {
		t.Errorf("Expected empty string for empty notes, got %q", result)
	}
}

func TestRenderNotesCollapsible_Single(t *testing.T) {
	notes := []Note{
		{Kind: NoteNoUpdatesInWindow, IssueURL: "https://github.com/org/repo/issues/1", SinceDays: 7},
	}
	result := RenderNotesCollapsible(notes)

	if !strings.Contains(result, "<details>") {
		t.Error("Expected <details> tag")
	}
	if !strings.Contains(result, "<summary>📝 Notes (1)</summary>") {
		t.Errorf("Expected summary with count 1, got %q", result)
	}
	if !strings.Contains(result, "</details>") {
		t.Error("Expected </details> closing tag")
	}
}

func TestRenderNotesCollapsible_Multiple(t *testing.T) {
	notes := []Note{
		{Kind: NoteNoUpdatesInWindow, IssueURL: "https://github.com/org/repo/issues/1", SinceDays: 7},
		{Kind: NoteNewIssueShaping, IssueURL: "https://github.com/org/repo/issues/2"},
	}
	result := RenderNotesCollapsible(notes)

	if !strings.Contains(result, "<summary>📝 Notes (2)</summary>") {
		t.Errorf("Expected summary with count 2, got %q", result)
	}
	if !strings.Contains(result, "- https://github.com/org/repo/issues/1: no update in last 7 days") {
		t.Errorf("Expected first bullet, got %q", result)
	}
	if !strings.Contains(result, "- https://github.com/org/repo/issues/2: new issue — still being shaped") {
		t.Errorf("Expected second bullet, got %q", result)
	}
}

func TestRenderNoteBullet_DiffKinds(t *testing.T) {
	tests := []struct {
		name     string
		note     Note
		expected string
	}{
		{
			name: "NoteNewItem",
			note: Note{
				Kind:     NoteNewItem,
				IssueURL: "https://github.com/owner/repo/issues/1",
			},
			expected: "https://github.com/owner/repo/issues/1: new item (not in previous report)",
		},
		{
			name: "NoteRemovedItem",
			note: Note{
				Kind:           NoteRemovedItem,
				IssueURL:       "https://github.com/owner/repo/issues/2",
				ReportedStatus: "On Track",
			},
			expected: "https://github.com/owner/repo/issues/2: removed (was On Track in previous report)",
		},
		{
			name: "NoteStatusChanged",
			note: Note{
				Kind:            NoteStatusChanged,
				IssueURL:        "https://github.com/owner/repo/issues/3",
				ReportedStatus:  "At Risk",
				SuggestedStatus: "On Track",
			},
			expected: "https://github.com/owner/repo/issues/3: status changed from At Risk to On Track",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := renderNoteBullet(tc.note)
			if result != tc.expected {
				t.Errorf("Expected %q\nGot %q", tc.expected, result)
			}
		})
	}
}

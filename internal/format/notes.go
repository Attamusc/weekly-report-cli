package format

import (
	"fmt"
	"strings"
)

// NoteKind represents the type of note to be generated
type NoteKind int

const (
	// NoteMultipleUpdates indicates multiple structured updates were found for an issue
	NoteMultipleUpdates NoteKind = iota
	// NoteNoUpdatesInWindow indicates no updates were found within the time window
	NoteNoUpdatesInWindow
	// NoteUnstructuredFallback indicates the summary was derived from the most recent
	// comment rather than a structured report
	NoteUnstructuredFallback
	// NoteSentimentMismatch indicates the AI detected a mismatch between the
	// reported status and the content of the updates
	NoteSentimentMismatch
)

// Note represents a note entry about an issue's status reporting
type Note struct {
	Kind            NoteKind // Type of note
	IssueURL        string   // URL of the GitHub issue
	SinceDays       int      // Number of days in the search window
	ReportedStatus  string   // The original reported status caption (for sentiment mismatch)
	SuggestedStatus string   // AI-suggested status caption (for sentiment mismatch)
	Explanation     string   // AI explanation of the mismatch (for sentiment mismatch)
}

// RenderNotes generates a markdown notes section from a slice of notes
// Returns empty string if no notes are provided
// Format: "## Notes" header followed by bullet points
func RenderNotes(notes []Note) string {
	if len(notes) == 0 {
		return ""
	}

	var builder strings.Builder

	// Write section header
	builder.WriteString("## Notes\n\n")

	// Write each note as a bullet point
	for _, note := range notes {
		bullet := renderNoteBullet(note)
		if bullet != "" {
			builder.WriteString(fmt.Sprintf("- %s\n", bullet))
		}
	}

	return builder.String()
}

// renderNoteBullet generates the bullet point text for a single note
func renderNoteBullet(note Note) string {
	switch note.Kind {
	case NoteMultipleUpdates:
		// Handle pluralization for days
		dayText := pluralizeDays(note.SinceDays)
		return fmt.Sprintf("%s: multiple structured updates in last %s",
			note.IssueURL, dayText)

	case NoteNoUpdatesInWindow:
		// Handle pluralization for days
		dayText := pluralizeDays(note.SinceDays)
		return fmt.Sprintf("%s: no update in last %s",
			note.IssueURL, dayText)

	case NoteUnstructuredFallback:
		return fmt.Sprintf("%s: no structured update found — summary derived from most recent comment",
			note.IssueURL)

	case NoteSentimentMismatch:
		return fmt.Sprintf("%s: reported as %s, but sentiment suggests %s — %s",
			note.IssueURL, note.ReportedStatus, note.SuggestedStatus, note.Explanation)

	default:
		// Unknown note kind, return empty string
		return ""
	}
}

// pluralizeDays returns "N day" or "N days" with proper pluralization
func pluralizeDays(days int) string {
	if days == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", days)
}

// HasNotesOfKind checks if any notes of the specified kind exist
func HasNotesOfKind(notes []Note, kind NoteKind) bool {
	for _, note := range notes {
		if note.Kind == kind {
			return true
		}
	}
	return false
}

// FilterNotesByKind returns only notes of the specified kind
func FilterNotesByKind(notes []Note, kind NoteKind) []Note {
	var filtered []Note
	for _, note := range notes {
		if note.Kind == kind {
			filtered = append(filtered, note)
		}
	}
	return filtered
}

// CountNotesByKind returns the count of notes of the specified kind
func CountNotesByKind(notes []Note, kind NoteKind) int {
	count := 0
	for _, note := range notes {
		if note.Kind == kind {
			count++
		}
	}
	return count
}

package diff

import (
	"fmt"

	"github.com/Attamusc/weekly-report-cli/internal/format"
)

// Compare takes previous report rows and current format.Row slice, and returns
// annotated rows with status transitions plus additional notes for new/removed items.
func Compare(previous []PreviousRow, current []format.Row) ([]format.Row, []format.Note) {
	if len(previous) == 0 {
		return current, nil
	}

	prevByURL := make(map[string]PreviousRow, len(previous))
	for _, row := range previous {
		prevByURL[row.IssueURL] = row
	}

	var notes []format.Note
	currentURLs := make(map[string]bool, len(current))

	for i := range current {
		currentURLs[current[i].EpicURL] = true

		prev, existed := prevByURL[current[i].EpicURL]
		if !existed {
			current[i].NewItem = true
			notes = append(notes, format.Note{
				Kind:     format.NoteNewItem,
				IssueURL: current[i].EpicURL,
			})
			continue
		}

		if prev.StatusEmoji != current[i].StatusEmoji {
			transition := fmt.Sprintf("%s→%s", prev.StatusEmoji, current[i].StatusEmoji)
			current[i].StatusTransition = &transition
			notes = append(notes, format.Note{
				Kind:            format.NoteStatusChanged,
				IssueURL:        current[i].EpicURL,
				ReportedStatus:  prev.StatusCaption,
				SuggestedStatus: current[i].StatusCaption,
			})
		}
	}

	for _, prev := range previous {
		if !currentURLs[prev.IssueURL] {
			notes = append(notes, format.Note{
				Kind:           format.NoteRemovedItem,
				IssueURL:       prev.IssueURL,
				ReportedStatus: prev.StatusCaption,
			})
		}
	}

	return current, notes
}

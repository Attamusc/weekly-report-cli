package diff

import (
	"testing"

	"github.com/Attamusc/weekly-report-cli/internal/format"
)

func makeRow(url, emoji, caption string) format.Row {
	return format.Row{
		EpicURL:       url,
		StatusEmoji:   emoji,
		StatusCaption: caption,
		EpicTitle:     "Title",
	}
}

func makePrev(url, emoji, caption string) PreviousRow {
	return PreviousRow{
		IssueURL:      url,
		StatusEmoji:   emoji,
		StatusCaption: caption,
	}
}

func TestCompare_NoPrevious(t *testing.T) {
	current := []format.Row{makeRow("https://example.com/1", ":green_circle:", "On Track")}
	rows, notes := Compare(nil, current)
	if len(notes) != 0 {
		t.Errorf("expected 0 notes, got %d", len(notes))
	}
	if rows[0].NewItem {
		t.Error("expected NewItem=false when no previous")
	}
}

func TestCompare_AllSame(t *testing.T) {
	prev := []PreviousRow{makePrev("https://example.com/1", ":green_circle:", "On Track")}
	current := []format.Row{makeRow("https://example.com/1", ":green_circle:", "On Track")}
	rows, notes := Compare(prev, current)
	if len(notes) != 0 {
		t.Errorf("expected 0 notes, got %d", len(notes))
	}
	if rows[0].NewItem || rows[0].StatusTransition != nil {
		t.Error("expected no annotation for unchanged row")
	}
}

func TestCompare_StatusChanged(t *testing.T) {
	prev := []PreviousRow{makePrev("https://example.com/1", ":yellow_circle:", "At Risk")}
	current := []format.Row{makeRow("https://example.com/1", ":green_circle:", "On Track")}
	rows, notes := Compare(prev, current)
	if len(notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(notes))
	}
	if notes[0].Kind != format.NoteStatusChanged {
		t.Errorf("expected NoteStatusChanged, got %v", notes[0].Kind)
	}
	if notes[0].ReportedStatus != "At Risk" {
		t.Errorf("expected ReportedStatus=At Risk, got %s", notes[0].ReportedStatus)
	}
	if notes[0].SuggestedStatus != "On Track" {
		t.Errorf("expected SuggestedStatus=On Track, got %s", notes[0].SuggestedStatus)
	}
	if rows[0].StatusTransition == nil {
		t.Fatal("expected StatusTransition to be set")
	}
	if *rows[0].StatusTransition != ":yellow_circle:→:green_circle:" {
		t.Errorf("unexpected transition: %s", *rows[0].StatusTransition)
	}
}

func TestCompare_NewItem(t *testing.T) {
	prev := []PreviousRow{makePrev("https://example.com/1", ":green_circle:", "On Track")}
	current := []format.Row{
		makeRow("https://example.com/1", ":green_circle:", "On Track"),
		makeRow("https://example.com/2", ":blue_circle:", "Not Started"),
	}
	rows, notes := Compare(prev, current)
	if !rows[1].NewItem {
		t.Error("expected NewItem=true for new row")
	}
	found := false
	for _, n := range notes {
		if n.Kind == format.NoteNewItem && n.IssueURL == "https://example.com/2" {
			found = true
		}
	}
	if !found {
		t.Error("expected NoteNewItem for new URL")
	}
}

func TestCompare_RemovedItem(t *testing.T) {
	prev := []PreviousRow{
		makePrev("https://example.com/1", ":green_circle:", "On Track"),
		makePrev("https://example.com/2", ":red_circle:", "Off Track"),
	}
	current := []format.Row{makeRow("https://example.com/1", ":green_circle:", "On Track")}
	_, notes := Compare(prev, current)
	found := false
	for _, n := range notes {
		if n.Kind == format.NoteRemovedItem && n.IssueURL == "https://example.com/2" {
			found = true
			if n.ReportedStatus != "Off Track" {
				t.Errorf("expected ReportedStatus=Off Track, got %s", n.ReportedStatus)
			}
		}
	}
	if !found {
		t.Error("expected NoteRemovedItem for removed URL")
	}
}

func TestCompare_Mixed(t *testing.T) {
	prev := []PreviousRow{
		makePrev("https://example.com/1", ":green_circle:", "On Track"),
		makePrev("https://example.com/2", ":yellow_circle:", "At Risk"),
		makePrev("https://example.com/3", ":red_circle:", "Off Track"),
	}
	current := []format.Row{
		makeRow("https://example.com/1", ":green_circle:", "On Track"),   // unchanged
		makeRow("https://example.com/2", ":green_circle:", "On Track"),   // changed
		makeRow("https://example.com/4", ":blue_circle:", "Not Started"), // new
	}
	rows, notes := Compare(prev, current)

	// row[0] unchanged
	if rows[0].NewItem || rows[0].StatusTransition != nil {
		t.Error("row 0 should be unchanged")
	}
	// row[1] changed
	if rows[1].StatusTransition == nil {
		t.Error("row 1 should have StatusTransition")
	}
	// row[2] new
	if !rows[2].NewItem {
		t.Error("row 2 should be NewItem")
	}

	counts := map[format.NoteKind]int{}
	for _, n := range notes {
		counts[n.Kind]++
	}
	if counts[format.NoteStatusChanged] != 1 {
		t.Errorf("expected 1 NoteStatusChanged, got %d", counts[format.NoteStatusChanged])
	}
	if counts[format.NoteNewItem] != 1 {
		t.Errorf("expected 1 NoteNewItem, got %d", counts[format.NoteNewItem])
	}
	if counts[format.NoteRemovedItem] != 1 {
		t.Errorf("expected 1 NoteRemovedItem, got %d", counts[format.NoteRemovedItem])
	}
}

func TestCompare_EmptyCurrent(t *testing.T) {
	prev := []PreviousRow{
		makePrev("https://example.com/1", ":green_circle:", "On Track"),
		makePrev("https://example.com/2", ":yellow_circle:", "At Risk"),
	}
	_, notes := Compare(prev, nil)
	removed := 0
	for _, n := range notes {
		if n.Kind == format.NoteRemovedItem {
			removed++
		}
	}
	if removed != 2 {
		t.Errorf("expected 2 NoteRemovedItem, got %d", removed)
	}
}

# Milestone 2a: Historical Comparison & Collapsible Notes

**Date:** 2026-04-21
**Status:** Draft
**Directory:** /Users/attamusc/projects/github.com/Attamusc/weekly-report-cli

## Overview

Add week-over-week diff annotations to the generate command. When a previous report is provided via `--previous-report <path>`, the tool parses the markdown table, compares it against the current report, and annotates status transitions (e.g., "🟡→🟢"), new items ("🆕"), and removed items. Also adds collapsible `<details>` wrapping for the notes section.

## Approach

Parse previous report from markdown file → diff by issue URL → annotate current rows → render with transitions.

No external state storage. The markdown table IS the state. GitHub Actions pipe the previous Discussion comment body to a temp file.

## Design

### New Package: `internal/diff/`

**`parse.go`** — `ParseReport(content string) []PreviousRow`
- Split by newlines, skip header (first 2 lines)
- Split each row by unescaped `|`
- Extract status emoji+caption from col 1
- Extract issue URL from markdown link `[title](url)` in col 2
- Extract target date string from col 3
- Return slice of `PreviousRow{IssueURL, StatusEmoji, StatusCaption, TargetDate}`
- Malformed rows are silently skipped

**`compare.go`** — `Compare(previous []PreviousRow, current []format.Row) ([]format.Row, []format.Note)`
- Build URL→PreviousRow lookup map
- For each current row:
  - If URL exists in previous and status changed → set `StatusTransition` on row
  - If URL not in previous → add "🆕" prefix to status, add NewItem note
- For each previous row not in current → add RemovedItem note
- Return mutated rows and additional notes

### Modified Types

**`format.Row`** — add field:
```go
StatusTransition *string // e.g., "🟡→🟢" — rendered before status when set
NewItem          bool    // true if this item wasn't in the previous report
```

**`format.RenderTable`** — when `StatusTransition` is set, render as `"🟡→🟢 On Track"` instead of `":green_circle: On Track"`. When `NewItem` is true, prefix with "🆕".

**`format.Note`** — add `NoteKind` values:
- `NoteNewItem` — issue appeared in current but not previous
- `NoteRemovedItem` — issue was in previous but not current (includes URL + previous status)
- `NoteStatusChanged` — status transition occurred (informational)

**`format.RenderNotes`** — add `collapsible bool` parameter or new `RenderNotesCollapsible` function that wraps output in `<details><summary>📝 Notes (N)</summary>...</details>`.

### CLI Integration

**`cmd/generate.go`:**
- Add `--previous-report <path>` flag
- Add `--collapsible-notes` flag
- After Phase C (assemble), if previous report provided:
  - Read file
  - `diff.ParseReport(content)` → `previousRows`
  - `diff.Compare(previousRows, currentRows)` → mutated rows + diff notes
  - Append diff notes to existing notes
- Pass collapsible flag to render

### Pipeline

```
Phase A: Collect (existing, unchanged)
Phase B: Summarize (existing, unchanged)
Phase C: Assemble (existing, unchanged)
Phase D: Diff (NEW)
  - Read previous report file
  - ParseReport → []PreviousRow
  - Compare(previous, current) → annotated rows + diff notes
Phase E: Render (enhanced)
  - RenderTable handles StatusTransition/NewItem
  - RenderNotes wraps in <details> when collapsible
```

## Dependencies

- Modified: `internal/format/markdown.go` (Row struct, RenderTable)
- Modified: `internal/format/notes.go` (new NoteKinds, collapsible rendering)
- Modified: `cmd/generate.go` (new flags, Phase D integration)
- Created: `internal/diff/parse.go`
- Created: `internal/diff/compare.go`
- Created: `internal/diff/parse_test.go`
- Created: `internal/diff/compare_test.go`

## Risks & Open Questions

- Markdown table parsing is format-coupled — if we change the table format, the parser needs updating. Acceptable since we control both sides.
- Escaped pipes in titles (`\|`) need careful handling in the parser.
- The `<details>` HTML works in GitHub Discussions/Issues but not all markdown renderers. Fine for our use case.

# Fix "Unknown" Status for Semi-Structured Comments and Labels

## Problem

When an issue comment uses the visual markdown format (e.g. `### Trending` /
`🟢 on track` / `### Update`) but lacks the HTML comment markers
(`<!-- data key="isReport" ... -->`), the tool cannot parse it as a structured
report. If no other structured reports exist in the reporting window, the issue
falls into Case 1b (`cmd/generate.go:489-500`) and gets status **Unknown**, even
though:

1. The comment's `### Trending` heading clearly contains a parseable status like
   `🟢 on track`
2. The issue may carry a label like `"on track"` that maps directly to `OnTrack`

### Real-world example

Issue `github/graphql-platform#2458` has 7 comments. The first 6 use full HTML
markers. The 7th (most recent, March 5) uses the markdown-only format:

```markdown
### Trending

🟢 on track

### Update

**Completed:** ...
```

The issue also carries the label `on track`. Both signals are ignored and the
issue is reported as Unknown.

### Root cause

Two sources of recoverable signal are being ignored:

| Signal | Where it exists | Current handling |
|---|---|---|
| Semi-structured comment (headings + emoji, no HTML markers) | Most recent comment body | Treated as unstructured -> `Unknown` |
| Issue labels matching status patterns | `github.IssueData.Labels` | Completely ignored by `generate` |

## Design Decisions

- **Priority ordering**: Structured report (HTML markers) > Semi-structured
  comment (markdown headings) > Labels
- **Semi-structured detection**: Require a `### Trending` (or similar) heading
  followed by a recognized status pattern (validated via `MapTrending()`)
- **Label matching**: Use **word-boundary matching** (stricter than
  `MapTrending()`'s substring matching) because labels are freeform strings that
  serve many purposes — substring matching would cause false positives like
  `"greenfield"` -> OnTrack, `"deprecated"` -> OffTrack, `"featured"` -> OffTrack
- **Label fallback scope**: Apply anywhere status would otherwise be `Unknown`
  (Case 1b, Case 2 with unknown trending, etc.)
- **Note priority**: `NoteMultipleUpdates` (unconditional) >
  `NoteSemiStructuredFallback` > `NoteLabelFallback` > `NoteUnstructuredFallback`.
  New note assignments are guarded with `if result.Note == nil` to avoid
  overwriting higher-priority notes. Only `NoteMultipleUpdates` remains
  unconditional (existing behavior).

## Changes

### Step 1: Extract shared status pattern matching helper in `internal/derive/status.go`

Refactor `MapTrending()` to extract its core normalization + pattern matching
into a private helper. This lets both `MapTrending()` and the new
`MapLabelsToStatus()` share the same normalization logic (even though they use
different matching strategies).

```go
// matchStatusPattern normalizes a raw string and attempts to match it against
// known status patterns using substring matching. Returns (Status, true) on
// match, (Unknown, false) otherwise.
func matchStatusPattern(raw string) (Status, bool) {
    if raw == "" {
        return Unknown, false
    }

    normalized := circleEmojiRegex.ReplaceAllString(raw, "")
    normalized = strings.TrimSpace(strings.ToLower(normalized))

    if normalized == "" {
        return Unknown, false
    }

    for _, mapping := range statusMappings {
        for _, pattern := range mapping.patterns {
            if strings.Contains(normalized, pattern) {
                return mapping.status, true
            }
        }
    }

    return Unknown, false
}
```

Rewrite `MapTrending()` to delegate:

```go
func MapTrending(raw string) Status {
    status, _ := matchStatusPattern(raw)
    return status
}
```

Existing behavior is preserved -- `MapTrending()` returns exactly the same
values for all inputs.

**Files**: `internal/derive/status.go`
**Tests**: Existing `TestMapTrending` tests pass unchanged.

### Step 2: New `MapLabelsToStatus()` with word-boundary matching in `internal/derive/status.go`

Labels are freeform user-defined strings that serve many purposes, unlike
trending values which are purpose-specific status indicators. Substring matching
would cause false positives:

| Label | Substring match | Would match |
|-------|----------------|-------------|
| `"greenfield"` | `"green"` | OnTrack (wrong) |
| `"deprecated"` | `"red"` | OffTrack (wrong) |
| `"featured"` | `"red"` | OffTrack (wrong) |
| `"whitelisted"` | `"white"` | NotStarted (wrong) |

Use word-boundary matching instead: pad the normalized label with spaces and
check for ` pattern ` containment, or exact equality. Also skip emoji-only
patterns (labels don't contain status emojis).

```go
// isEmojiPattern returns true if the pattern is a single emoji character
// (used to skip emoji patterns when matching labels).
func isEmojiPattern(s string) bool {
    return s == "🟢" || s == "🟡" || s == "🔴" || s == "⚪" || s == "🟣"
}

// matchLabelPattern normalizes a label and attempts to match it against known
// status patterns using word-boundary matching (stricter than the substring
// matching used for trending values). Returns (Status, true) on match,
// (Unknown, false) otherwise.
func matchLabelPattern(label string) (Status, bool) {
    normalized := strings.TrimSpace(strings.ToLower(label))
    if normalized == "" {
        return Unknown, false
    }

    padded := " " + normalized + " "
    for _, mapping := range statusMappings {
        for _, pattern := range mapping.patterns {
            if isEmojiPattern(pattern) {
                continue
            }
            if normalized == pattern || strings.Contains(padded, " "+pattern+" ") {
                return mapping.status, true
            }
        }
    }

    return Unknown, false
}

// MapLabelsToStatus attempts to derive a status from issue labels using
// word-boundary matching. Returns the first matching status and true, or
// (Unknown, false) if no label matches a known status pattern.
func MapLabelsToStatus(labels []string) (Status, bool) {
    for _, label := range labels {
        if status, ok := matchLabelPattern(label); ok {
            return status, true
        }
    }
    return Unknown, false
}
```

**Files**: `internal/derive/status.go`
**Tests**: `internal/derive/status_test.go` -- new `TestMapLabelsToStatus` with:
- Single matching label (`"on track"` -> `OnTrack`)
- Case variations (`"At Risk"`, `"BLOCKED"`)
- Multiple labels, first match wins (`["epic", "at risk"]` -> `AtRisk`)
- No matching labels -> `(Unknown, false)`
- Empty labels slice -> `(Unknown, false)`
- **Rejects false positives**: `"greenfield"` does NOT match, `"deprecated"` does
  NOT match, `"featured"` does NOT match, `"whitelisted"` does NOT match
- Emoji-only patterns skipped: a label `"🟢"` does NOT match

Also add `TestMatchLabelPattern` for the helper:
- Exact matches: `"on track"` -> OnTrack, `"blocked"` -> OffTrack
- Word boundaries: `"is on track"` -> OnTrack (contains ` on track `)
- Rejects substrings: `"greenfield"` -> (Unknown, false)

### Step 3: New `ParseSemiStructured()` in `internal/report/extract.go`

Add a parser for markdown-headed comments without HTML markers.

```go
// ParseSemiStructured extracts a report from a comment that uses markdown
// headings (### Trending, ### Update, ### Target Date) but lacks HTML comment
// markers. Returns (Report, true) if a trending heading with a recognizable
// status pattern is found. Comments that contain the structured report marker
// are explicitly rejected to avoid double-counting.
func ParseSemiStructured(body string, createdAt time.Time, sourceURL string) (Report, bool)
```

**Detection logic:**

1. **Reject if structured**: If `reportMarkerRegex.MatchString(body)` is true,
   return `(Report{}, false)` -- these belong to `ParseReport()`.
2. **Find trending heading**: Use a regex like
   `(?im)^#{1,6}\s+trending\s*$` to locate a "trending" heading at any level
   (h1-h6). Then capture the content between that heading and the next known
   section heading (or end-of-string).
3. **Extract trending value**: Take the first non-empty line from the section
   content. This handles formats like `🟢 on track` on its own line.
4. **Validate status**: Call `derive.MapTrending()` on the extracted value. If
   it returns `Unknown`, reject the parse -- this prevents false positives from
   comments that happen to contain a "Trending" heading with unrelated content.
5. **Extract update section** (optional): Look for a `### Update` heading using
   the same approach. Capture content until the next known section heading or
   end-of-string. Store as `UpdateRaw`.
6. **Extract target date section** (optional): Look for a `### Target Date`
   heading. Capture the first non-empty line as `TargetDate`. Without this,
   semi-structured reports always get `TargetDate: ""` -> "TBD" in the output.
7. Return the populated `Report`.

**Regex patterns** (new module-level vars):

```go
// Matches a markdown heading containing "trending" (any level h1-h6)
semiTrendingHeadingRegex = regexp.MustCompile(`(?im)^#{1,6}\s+trending\s*$`)

// Matches a markdown heading containing "update" (any level h1-h6)
semiUpdateHeadingRegex = regexp.MustCompile(`(?im)^#{1,6}\s+update\s*$`)

// Matches a markdown heading containing "target date" (any level h1-h6)
semiTargetDateHeadingRegex = regexp.MustCompile(`(?im)^#{1,6}\s+target\s*date\s*$`)

// Matches any known section heading (used to find section boundaries).
// Only matches known headings to avoid truncating sub-headings (e.g.,
// #### Sub-heading) within update content.
knownSectionHeadingRegex = regexp.MustCompile(`(?im)^#{1,6}\s+(trending|update|target\s*date|summary)\s*$`)
```

**Important**: Use `knownSectionHeadingRegex` for section boundary detection
instead of a generic "any heading" regex. This prevents sub-headings within
update content (like `#### Completed items`) from truncating the update section.
The `summary` heading is included as a known boundary since the real-world
example uses `### Summary` as a separate section.

> **Note on circular import**: `ParseSemiStructured()` calls
> `derive.MapTrending()` for validation. The `report` package currently does not
> import `derive`. Verified: `derive` imports only `regexp` and `strings`, so
> adding this import creates no cycle. Note that this creates a semantic
> dependency: changes to `statusMappings` in `derive` will change what the
> semi-structured parser accepts. This is desirable (they should stay in sync)
> and should be documented with a comment in the code.

**Files**: `internal/report/extract.go`
**Tests**: `internal/report/extract_test.go` -- new `TestParseSemiStructured_*`:
- Valid: heading + emoji status (`### Trending\n\n🟢 on track`)
- Valid: heading + text status (`### Trending\n\non track`)
- Valid: with update section
- Valid: with target date section
- Valid: with all three sections (trending + update + target date)
- Valid: different heading levels (`# Trending`, `## Trending`)
- Valid: update section with sub-headings preserved (e.g., `#### Completed`)
- Invalid: no trending heading
- Invalid: trending heading with unrecognized text (maps to Unknown -> reject)
- Invalid: has HTML report markers (should be rejected)
- Invalid: empty body
- Edge: extra whitespace around heading and status text
- Edge: trending heading present but section content is empty
- **Known limitation test**: trending content `"green with envy"` matches
  OnTrack via `MapTrending()` substring matching -- document as inherited
  limitation, not a new bug

### Step 4: New `SelectSemiStructuredReports()` in `internal/report/select.go`

```go
// SelectSemiStructuredReports extracts reports from comments that use markdown
// heading format but lack HTML markers. Only considers comments within the time
// window. Returns reports sorted newest-first.
func SelectSemiStructuredReports(comments []github.Comment, since time.Time) []Report
```

Mirrors `SelectReports()` but calls `ParseSemiStructured()` instead. Comments
that have the `isReport` marker are naturally excluded by
`ParseSemiStructured()`'s guard clause.

**Files**: `internal/report/select.go`
**Tests**: `internal/report/select_test.go` -- new
`TestSelectSemiStructuredReports`:
- Mix of structured and semi-structured comments -- only semi-structured ones
  returned
- Time window filtering works
- Sorted newest-first
- Empty input

### Step 5: New `NoteKind` values in `internal/format/notes.go`

Add two new note kinds:

```go
// NoteSemiStructuredFallback indicates the status/update was derived from a
// comment using markdown headings rather than structured HTML markers.
NoteSemiStructuredFallback

// NoteLabelFallback indicates the status was derived from an issue label
// because no comment-based status signal was found.
NoteLabelFallback
```

Add rendering in `renderNoteBullet()`:

```go
case NoteSemiStructuredFallback:
    return fmt.Sprintf(
        "%s: status derived from markdown-formatted comment (not structured report)",
        note.IssueURL)

case NoteLabelFallback:
    return fmt.Sprintf("%s: status derived from issue label",
        note.IssueURL)
```

**Files**: `internal/format/notes.go`
**Tests**: `internal/format/notes_test.go` -- rendering tests for both new kinds

### Step 6: Add `Labels` to orchestration `IssueData` in `cmd/generate.go`

The orchestration-level `IssueData` struct (line 347) doesn't carry labels.

Add field:

```go
Labels []string // Issue labels (for status fallback)
```

Populate in `collectIssueData()` after `FetchIssue()`:

```go
result := IssueData{
    // ... existing fields ...
    Labels: issueData.Labels,
}
```

**Files**: `cmd/generate.go`

### Step 7: Update `collectIssueData()` in `cmd/generate.go`

This is the core orchestration change. Two modifications:

#### 7a: Semi-structured fallback in Case 1 (with note set immediately)

After `SelectReports()` returns empty (line 480), before entering the existing
Case 1 branches, try semi-structured parsing. **Set the semi-structured note
immediately** in the promotion block -- no boolean flag needed. This is simpler
and prevents note-loss scenarios where later logic sets a different note.

```go
// Case 1: No structured reports found
if len(reports) == 0 {
    // Try semi-structured comments before falling back
    semiReports := report.SelectSemiStructuredReports(comments, since)
    if len(semiReports) > 0 {
        // Promote to Case 2 -- we have usable reports
        reports = semiReports
        result.Reports = reports
        result.Note = &format.Note{
            Kind:     format.NoteSemiStructuredFallback,
            IssueURL: ref.URL,
        }
    }
}
```

If semi-structured reports are found, they replace the empty `reports` slice and
flow into the existing Case 2 logic naturally. The note is set early so that
only `NoteMultipleUpdates` (which is unconditional, line 567) can override it --
and that's correct behavior (knowing about multiple updates is more actionable
than knowing the format was semi-structured).

#### 7b: Label fallback after status determination (guarded note)

After the status is determined (end of Case 1b, or after `MapTrending()` in
Case 2), if it is still `Unknown`, try labels. **Guard the note assignment with
`if result.Note == nil`** to avoid overwriting higher-priority notes (e.g.,
`NoteUnstructuredFallback` in Case 1b).

```go
// Label fallback: if status is still Unknown, try to derive from labels.
// Note: only set NoteLabelFallback if no other note is already present,
// to avoid overwriting higher-priority notes.
if result.Status == derive.Unknown {
    if labelStatus, ok := derive.MapLabelsToStatus(result.Labels); ok {
        result.Status = labelStatus
        result.ReportedStatusCaption = labelStatus.Caption
        if result.Note == nil {
            result.Note = &format.Note{
                Kind:     format.NoteLabelFallback,
                IssueURL: ref.URL,
            }
        }
    }
}
```

This check goes right before each `return result, nil` statement where status
could be `Unknown`:
- End of Case 1b (line ~500, after unstructured fallback path)
- After status derivation in Case 2 when `MapTrending()` returns `Unknown`

**Note priority rationale**: In Case 1b, `NoteUnstructuredFallback` is already
set (the summary came from the most recent comment). The label only contributes
the status, not the summary. Preserving the unstructured note accurately tells
the user where the summary came from, while the status upgrade to a known value
is still applied.

**Files**: `cmd/generate.go`

### Step 8: Verify

- `make check` (fmt + vet + lint + test)
- Manual verification: the example issue should now resolve to `OnTrack` via the
  semi-structured comment path

## Updated Decision Tree

```
Issue fetched
|-- No structured reports in time window (Case 1)
|   |-- Try semi-structured comments                  <-- NEW
|   |   |-- Found -> set NoteSemiStructured, promote to Case 2
|   |   '-- Not found -> continue Case 1
|   |
|   |-- Issue is closed -> Done
|   |-- Has comments -> Unknown + AI summarize + NoteUnstructuredFallback
|   |   '-- Label fallback if still Unknown           <-- NEW
|   |       (upgrades status, preserves existing note)
|   '-- No comments
|       |-- Created within window -> Shaping
|       '-- Older issue -> NeedsUpdate
|
'-- Has reports (Case 2, including promoted semi-structured)
    |-- derive.MapTrending(newest_report.TrendingRaw)
    |   |-- Label fallback if Unknown (guarded note)  <-- NEW
    |   '-- ... existing pattern matching ...
    |
    |-- No update text in reports (Case 2a)
    |   |-- Issue is closed -> Done
    |   |-- Has comments -> keep status, AI summarize comment
    |   '-- No comments -> Shaping or NeedsUpdate
    |
    '-- Has update text (Case 2b)
        |-- Done or closed -> Done, no AI
        '-- Active -> AI summarize, keep derived status
            '-- If >=2 reports -> NoteMultipleUpdates (overrides any note)
```

## Note Priority (highest to lowest)

1. `NoteMultipleUpdates` -- unconditional (existing behavior)
2. `NoteSemiStructuredFallback` -- set on promotion, only overridden by #1
3. `NoteUnstructuredFallback` -- set in Case 1b, preserved by label fallback
4. `NoteLabelFallback` -- only set if `result.Note == nil`
5. `NoteNoUpdatesInWindow` / `NoteNewIssueShaping` -- set in no-comment paths

## File Change Summary

| File | Change |
|---|---|
| `internal/derive/status.go` | Extract `matchStatusPattern()` helper; add `matchLabelPattern()` with word-boundary matching; add `MapLabelsToStatus()` |
| `internal/derive/status_test.go` | Add `TestMapLabelsToStatus`, `TestMatchLabelPattern` |
| `internal/report/extract.go` | Add `ParseSemiStructured()` with known-section-heading regex; add target date extraction |
| `internal/report/extract_test.go` | Add `TestParseSemiStructured_*` cases including sub-heading preservation and known-limitation test |
| `internal/report/select.go` | Add `SelectSemiStructuredReports()` |
| `internal/report/select_test.go` | Add `TestSelectSemiStructuredReports` |
| `internal/format/notes.go` | Add `NoteSemiStructuredFallback`, `NoteLabelFallback` + rendering |
| `internal/format/notes_test.go` | Add rendering tests for new note kinds |
| `cmd/generate.go` | Add `Labels` to `IssueData`; semi-structured promotion with immediate note; label fallback with guarded note |

## Risk Assessment

- **False positives from semi-structured parsing**: Mitigated by requiring the
  trending heading content to map to a known status via `MapTrending()`. A
  comment with `### Trending topics` would not match the heading regex (requires
  "trending" as the full heading text). A heading like `### Trending` followed by
  prose like "green with envy" would match `OnTrack` -- this is an inherited
  limitation of `MapTrending()`'s substring matching and is documented with a
  test case.
- **False positives from label matching**: Mitigated by using word-boundary
  matching instead of substring matching. `"greenfield"` does NOT match `"green"`,
  `"deprecated"` does NOT match `"red"`, etc. Only labels that contain status
  patterns as whole words match (e.g., `"on track"`, `"blocked"`, `"done"`).
- **Note loss**: Mitigated by note priority design. Semi-structured note set
  early (only `NoteMultipleUpdates` can override). Label note guarded with
  `if result.Note == nil`. Existing notes preserved.
- **Sub-heading truncation**: Mitigated by using `knownSectionHeadingRegex`
  for section boundaries instead of matching any heading. Sub-headings like
  `#### Completed` within update content are preserved.
- **Import cycle**: `report` -> `derive` import. Verified: `derive` imports only
  `regexp` and `strings`, so no cycle. The semantic coupling (changes to
  `statusMappings` affect semi-structured parsing) is desirable and documented.
- **No breaking changes**: All existing behavior is preserved. The new paths only
  activate when the current code would produce `Unknown`.

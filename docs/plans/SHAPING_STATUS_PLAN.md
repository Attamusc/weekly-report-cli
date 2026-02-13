# Shaping Status Plan

## Overview

Add a new "Shaping" status for issues that were created within the reporting period and lack any update comments. These issues are new units of work that haven't had time to receive a status update yet. Instead of marking them as "Needs Update" (which implies a missed update), they are shown as "Shaping" — indicating the issue is new and still being defined.

## Key Behaviors

| Scenario | Behavior |
|----------|----------|
| Issue created within period, no comments | Status = Shaping, note = "new issue" |
| Issue created within period, has comments but no reports | Status = Unknown (existing behavior, not Shaping) |
| Issue created before period, no comments | Status = Needs Update (existing behavior) |
| Issue created within period, has structured reports | Status derived from report (existing behavior) |
| Issue created within period, closed | Status = Done (existing behavior) |

## Implementation Plan

### 1. Add `CreatedAt` to `IssueData` Struct

**File:** `internal/github/issues.go`

The issue creation date is available from the GitHub API via `issue.GetCreatedAt()` but is not currently extracted into the `IssueData` struct. We need it to determine whether an issue was created within the reporting period.

- Add `CreatedAt time.Time` field to the `IssueData` struct
- Populate it in `FetchIssue()`: `CreatedAt: issue.GetCreatedAt().Time`
- Update existing tests to verify `CreatedAt` is populated

### 2. Add `Shaping` Status

**File:** `internal/derive/status.go`

Add a new predefined status variable:

```go
Shaping = Status{Emoji: ":diamond_shape_with_a_dot_inside:", Caption: "Shaping"}
```

- Add `"shaping"` case to `Key()` method
- Add `"shaping"` case to `ParseStatusKey()`
- **No changes** to `MapTrending` or `statusMappings` — Shaping is never derived from trending text in structured reports; it is purely rule-based in `collectIssueData`

### 3. Add `NoteNewIssueShaping` Note Kind

**File:** `internal/format/notes.go`

Add a new note kind for the "new issue" annotation:

```go
NoteNewIssueShaping NoteKind = iota // after NoteSentimentMismatch
```

Rendered as:

```
- <url>: new issue — still being shaped
```

This replaces the `NoteNoUpdatesInWindow` note that would otherwise be generated for these issues.

### 4. Update Sort Priority for Shaping

**File:** `internal/format/markdown.go`

Update `getSortPriority()` to include "Shaping" in priority tier 3 (bottom of the table, alongside "Needs Update" and "Not Started"):

```go
if row.StatusCaption == "Needs Update" || row.StatusCaption == "Not Started" || row.StatusCaption == "Shaping" {
    return 3
}
```

### 5. Add Shaping Detection Logic

**File:** `cmd/generate.go`

This is the core behavioral change. The detection rule is:

> If the issue was **created within the period** (`issueData.CreatedAt` is after `since`) AND has **no update comments** (`len(comments) == 0`) AND is **not closed**, assign `derive.Shaping` instead of `derive.NeedsUpdate`.

#### a) Add `CreatedAt` to `cmd.IssueData`

Add a `CreatedAt time.Time` field to the pipeline's `IssueData` struct and populate it from `issueData.CreatedAt`.

#### b) Case 1: No reports, no comments (line ~473)

Currently this branch unconditionally assigns `NeedsUpdate`. Add a check for whether the issue was created within the period:

```go
} else if !issueData.CreatedAt.IsZero() && issueData.CreatedAt.After(since) {
    // Issue was created within the reporting period and has no comments — shaping
    result.Status = derive.Shaping
    result.ReportedStatusCaption = derive.Shaping.Caption
    result.ShouldSummarize = false
    result.FallbackSummary = "New issue — still being shaped"
    result.Note = &format.Note{
        Kind:     format.NoteNewIssueShaping,
        IssueURL: ref.URL,
    }
} else {
    // Existing NeedsUpdate logic
}
```

#### c) Case 2a: Reports exist but no update text, no comments (line ~524)

Same pattern — if the issue was created in the period and has no comments, use Shaping instead of NeedsUpdate.

#### d) Branches that should NOT trigger Shaping

- **Issue has comments but no structured reports** (Case 1 fallback) — the presence of comments means activity is happening, so the existing `Unknown` + `NoteUnstructuredFallback` behavior is correct.
- **Issue is closed** — always derives as `Done` regardless of creation date.
- **Issue has structured reports** — status comes from the report content via `MapTrending`.

### 6. AI Summarizer

**No changes.** Shaping issues have `ShouldSummarize = false`, so they are excluded from the AI batch call. Shaping is not added to the AI's status vocabulary or sentiment analysis.

## File Changes Summary

| File | Change Type | Description |
|------|-------------|-------------|
| `internal/github/issues.go` | Modify | Add `CreatedAt time.Time` field to `IssueData`; populate in `FetchIssue` |
| `internal/github/issues_test.go` | Modify | Verify `CreatedAt` is populated from API response |
| `internal/derive/status.go` | Modify | Add `Shaping` status variable; add key/parse cases |
| `internal/derive/status_test.go` | Modify | Test `Key()`, `ParseStatusKey()`, `String()` for Shaping |
| `internal/format/notes.go` | Modify | Add `NoteNewIssueShaping` kind and rendering |
| `internal/format/notes_test.go` | Modify | Test rendering of new note kind |
| `internal/format/markdown.go` | Modify | Add "Shaping" to priority 3 in `getSortPriority` |
| `internal/format/markdown_test.go` | Modify | Test sorting with Shaping rows |
| `cmd/generate.go` | Modify | Add `CreatedAt` to `IssueData`; add Shaping detection in Case 1 and Case 2a |

## Design Decisions

### Shaping vs. Needs Update

The distinction is important for report readers. "Needs Update" implies someone has been neglecting their status update — it signals a process failure. "Shaping" communicates that the issue is brand new and hasn't had time to receive an update yet — it signals intentional early-stage work. This avoids false "missing status" noise for freshly created issues.

### Rule-Based, Not AI

Shaping is determined purely by creation date and comment count. This keeps it deterministic, fast, and not dependent on AI availability. The AI sentiment analyzer is not taught about Shaping because the status is a factual statement (the issue is new) rather than a judgment about content.

### Priority 3 Sort Position

Shaping items sort at the bottom of the table alongside Needs Update and Not Started. These are all "low-information" rows — they don't have substantive updates to report. Keeping them at the bottom lets readers focus on active work first.

### Diamond Emoji

The `:diamond_shape_with_a_dot_inside:` emoji visually distinguishes Shaping from other statuses (which all use circles). This makes it easy to scan the report and identify new issues at a glance.

### Comment Presence Overrides Shaping

If an issue was created within the period but already has comments, it is not marked as Shaping. The presence of comments (even without structured reports) indicates activity beyond initial creation. In this case the existing `Unknown` + `NoteUnstructuredFallback` logic applies, which uses the comment content for AI summarization.

## Testing Strategy

### Unit Tests

1. **`internal/derive/status_test.go`**
   - `TestShaping_Key`: `Shaping.Key()` returns `"shaping"`
   - `TestShaping_String`: `Shaping.String()` returns `":diamond_shape_with_a_dot_inside: Shaping"`
   - `TestParseStatusKey_Shaping`: `ParseStatusKey("shaping")` returns `(Shaping, true)`

2. **`internal/format/notes_test.go`**
   - `TestRenderNotes_NewIssueShaping`: verify note renders as `"<url>: new issue — still being shaped"`

3. **`internal/format/markdown_test.go`**
   - `TestSortRowsByTargetDate_ShapingAtBottom`: Shaping rows sort at priority 3

4. **`internal/github/issues_test.go`**
   - Update `TestFetchIssue` cases to verify `CreatedAt` field is populated

5. **`cmd/generate_test.go`** (if applicable)
   - Issue created within period, no comments → Shaping status, NoteNewIssueShaping
   - Issue created within period, has comments but no reports → Unknown (not Shaping)
   - Issue created before period, no comments → NeedsUpdate (not Shaping)

### Table-Driven Test Structure

```go
func TestCollectIssueData_ShapingStatus(t *testing.T) {
    tests := []struct {
        name           string
        issueCreatedAt time.Time
        since          time.Time
        hasComments    bool
        wantStatus     derive.Status
        wantNoteKind   format.NoteKind
    }{
        {
            name:           "new issue no comments",
            issueCreatedAt: time.Now().Add(-2 * 24 * time.Hour), // 2 days ago
            since:          time.Now().Add(-7 * 24 * time.Hour), // 7 days ago
            hasComments:    false,
            wantStatus:     derive.Shaping,
            wantNoteKind:   format.NoteNewIssueShaping,
        },
        {
            name:           "new issue with comments",
            issueCreatedAt: time.Now().Add(-2 * 24 * time.Hour),
            since:          time.Now().Add(-7 * 24 * time.Hour),
            hasComments:    true,
            wantStatus:     derive.Unknown,
            wantNoteKind:   format.NoteUnstructuredFallback,
        },
        {
            name:           "old issue no comments",
            issueCreatedAt: time.Now().Add(-30 * 24 * time.Hour), // 30 days ago
            since:          time.Now().Add(-7 * 24 * time.Hour),
            hasComments:    false,
            wantStatus:     derive.NeedsUpdate,
            wantNoteKind:   format.NoteNoUpdatesInWindow,
        },
    }
    // ...
}
```

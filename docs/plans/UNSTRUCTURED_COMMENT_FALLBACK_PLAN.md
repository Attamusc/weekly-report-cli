# Unstructured Comment Fallback Plan

## Overview

When an issue has no structured reports (no `<!-- data key="isReport" value="true" -->` markers) but does have regular comments, the tool currently marks the issue as "Needs Update" and discards all comment content. This feature adds a fallback: use the most recent comment's full body as input for AI summarization, producing a useful update instead of silence.

This also covers Case 2a — when structured reports exist but their update text is empty. In that case, the structured report's status and target date are preserved, but the update text falls back to the most recent comment body.

## Key Differences from Current Behavior

| Scenario | Current Behavior | New Behavior |
|----------|-----------------|--------------|
| No reports, has comments | "Needs Update", no summary | AI-summarize most recent comment body, status = `Unknown` |
| No reports, no comments | "Needs Update" + `NoteNoUpdatesInWindow` | **No change** |
| Reports exist, empty update text, has comments | "Needs Update", no summary | Keep report status/target date, AI-summarize most recent comment body |
| Reports exist, empty update text, no comments | "Needs Update" + `NoteNoUpdatesInWindow` | **No change** |
| Reports with update text | AI-summarize update text | **No change** |

## Implementation Plan

### 1. Add `SelectMostRecentComment` to Report Package

**File:** `internal/report/select.go`

Add a function to select the most recent comment from the raw comment list, for use as fallback content when no structured reports are available.

```go
// SelectMostRecentComment returns the most recent comment body from the
// provided comments, or ("", false) if no comments exist. Comments are
// assumed to be returned chronologically by the GitHub API, so the last
// element is the most recent.
func SelectMostRecentComment(comments []github.Comment) (string, bool) {
    if len(comments) == 0 {
        return "", false
    }
    // Comments from the GitHub API are chronological (oldest first).
    // Return the most recent one.
    newest := comments[len(comments)-1]
    body := strings.TrimSpace(newest.Body)
    if body == "" {
        return "", false
    }
    return body, true
}
```

### 2. Add `NoteUnstructuredFallback` Note Kind

**File:** `internal/format/notes.go`

Extend the `NoteKind` enum and rendering to support flagging when a summary was derived from an unstructured comment rather than a structured report.

```go
const (
    NoteMultipleUpdates   NoteKind = iota // 0
    NoteNoUpdatesInWindow                 // 1
    NoteUnstructuredFallback              // 2 — NEW
)
```

Add rendering for the new note kind in `renderNoteBullet`:

```go
case NoteUnstructuredFallback:
    return fmt.Sprintf("- %s: no structured update found — summary derived from most recent comment", note.IssueURL)
```

### 3. Pass Raw Comments Through to `collectIssueData`

**File:** `cmd/generate.go`

Currently `collectIssueData` receives only the parsed `[]report.Report` from `SelectReports`. To support the fallback, the raw comments must also be available inside the decision tree.

Update the function signature to accept raw comments:

```go
func collectIssueData(
    ctx context.Context,
    client *githubapi.Client,
    ref input.IssueRef,
    since time.Time,
    sinceDays int,
) (IssueData, error)
```

The function already calls `githubapi.FetchCommentsSince` internally (around line 480) and passes the result to `report.SelectReports`. The raw comments slice is available as a local variable. No signature change is needed — the comments are already fetched inside `collectIssueData`. We just need to use them in the case logic below.

### 4. Modify Case 1 — No Reports Found

**File:** `cmd/generate.go` (around line 509)

Current behavior when `len(reports) == 0` and issue is open: status = `NeedsUpdate`, no summarization.

New behavior:

```go
// Case 1: No structured reports found
if len(reports) == 0 {
    if issueData.State == github.StateClosed {
        // Closed issues — unchanged
        data.Status = derive.Done
        data.TargetDate = issueData.ClosedAt
        data.ShouldSummarize = true
        data.FallbackSummary = issueData.CloseReason
        return data, nil
    }

    // Case 1 — open issue, no structured reports
    // Try falling back to the most recent comment body
    if commentBody, ok := report.SelectMostRecentComment(comments); ok {
        data.Status = derive.Unknown
        data.UpdateTexts = []string{commentBody}
        data.ShouldSummarize = true
        data.FallbackSummary = commentBody
        data.Note = &format.Note{
            Kind:     format.NoteUnstructuredFallback,
            IssueURL: ref.URL(),
        }
        return data, nil
    }

    // No comments at all — unchanged
    data.Status = derive.NeedsUpdate
    data.ShouldSummarize = false
    data.FallbackSummary = fmt.Sprintf("No update provided in last %d days", sinceDays)
    data.Note = &format.Note{
        Kind:      format.NoteNoUpdatesInWindow,
        IssueURL:  ref.URL(),
        SinceDays: sinceDays,
    }
    return data, nil
}
```

Key decisions:
- **Status = `Unknown`**: Since there is no structured trending indicator, we use `Unknown` (`:black_circle:`) rather than inferring status from comment content (that's Phase 2 — sentiment analysis).
- **`UpdateTexts = []string{commentBody}`**: The full comment body is passed as update text for AI summarization. The AI batch prompt already handles producing concise summaries from raw text.
- **`FallbackSummary = commentBody`**: If AI is disabled or fails, the raw comment body is used as-is.
- **Note added**: `NoteUnstructuredFallback` flags this row in the output notes section.

### 5. Modify Case 2a — Reports Exist but No Update Text

**File:** `cmd/generate.go` (around line 546)

Current behavior when reports exist but all `UpdateRaw` fields are empty and issue is open: status = `NeedsUpdate`, no summarization.

New behavior — preserve the structured report's status and target date, but fall back to the most recent comment for update text:

```go
// Case 2a: Reports exist but no update text
if len(updateTexts) == 0 {
    if issueData.State == github.StateClosed {
        // Closed issues — unchanged
        data.Status = derive.Done
        data.ShouldSummarize = true
        data.FallbackSummary = issueData.CloseReason
        return data, nil
    }

    // Try falling back to the most recent comment body
    if commentBody, ok := report.SelectMostRecentComment(comments); ok {
        // Keep status and target date from the structured report
        // (already set above from reports[0])
        data.UpdateTexts = []string{commentBody}
        data.ShouldSummarize = true
        data.FallbackSummary = commentBody
        data.Note = &format.Note{
            Kind:     format.NoteUnstructuredFallback,
            IssueURL: ref.URL(),
        }
        return data, nil
    }

    // No comments at all — unchanged
    data.Status = derive.NeedsUpdate
    data.ShouldSummarize = false
    data.FallbackSummary = fmt.Sprintf("No structured update found in last %d days", sinceDays)
    data.Note = &format.Note{
        Kind:      format.NoteNoUpdatesInWindow,
        IssueURL:  ref.URL(),
        SinceDays: sinceDays,
    }
    return data, nil
}
```

Key difference from Case 1: the status and target date come from the structured report (`reports[0]`), not from `derive.Unknown`. Only the update text is sourced from the comment fallback.

### 6. Ensure `batchSummarize` Handles Fallback Items

**File:** `cmd/generate.go` (Phase B, around line 302)

The existing `batchSummarize` function filters items where `ShouldSummarize == true && len(UpdateTexts) > 0`. Since the fallback sets both `ShouldSummarize = true` and `UpdateTexts = []string{commentBody}`, fallback items will automatically flow into the AI batch with no changes needed.

No code changes required in Phase B.

## File Changes Summary

| File | Change Type | Description |
|------|-------------|-------------|
| `internal/report/select.go` | Modify | Add `SelectMostRecentComment` function |
| `internal/format/notes.go` | Modify | Add `NoteUnstructuredFallback` kind and rendering |
| `cmd/generate.go` | Modify | Update Case 1 and Case 2a with comment fallback logic |

## CLI Usage Examples

No new flags or commands. The fallback is automatic:

```bash
# Issue with only regular comments (no structured reports) now produces output
weekly-report-cli generate --project "org:my-org/5"

# Notes section shows which issues used fallback
# Output includes:
# ## Notes
# - https://github.com/org/repo/issues/42: no structured update found — summary derived from most recent comment

# Disable AI (fallback still works, raw comment body used as-is)
DISABLE_SUMMARY=1 weekly-report-cli generate --project "org:my-org/5"
```

## Design Decisions

### Why `derive.Unknown` for Case 1?

When no structured report exists, there is no reported trending/status indicator. Rather than guessing (which could mislead), we use `Unknown` (`:black_circle:`) to clearly signal that status was not reported. Phase 2 (sentiment analysis) will later provide an AI-suggested status for these cases.

### Why the full comment body?

The AI summarizer already handles reducing verbose text to 3-5 sentences. Passing the full comment body maximizes the information available for summarization. Truncation would risk losing important context.

### Why only the most recent comment?

Using all comments would increase token usage and conflate old information with current state. The most recent comment is the best proxy for "current status" when no structured report exists.

### Why not filter out bot comments?

Bot comments (CI results, PR references, etc.) could produce noisy summaries. However, filtering requires heuristics that may incorrectly exclude real updates. We defer bot filtering to a future refinement — the AI summarizer handles noisy input reasonably well.

### Case 2a preserves report status

When a structured report exists but has empty update text, the reporter intentionally set a status and target date. We respect those values and only fall back for the missing update text.

## Testing Strategy

### Unit Tests

1. **`internal/report/select_test.go`**
   - `TestSelectMostRecentComment_WithComments`: returns most recent comment body
   - `TestSelectMostRecentComment_EmptyComments`: returns `("", false)`
   - `TestSelectMostRecentComment_EmptyBody`: most recent comment has empty body, returns `("", false)`
   - `TestSelectMostRecentComment_WhitespaceBody`: most recent comment is whitespace-only, returns `("", false)`

2. **`internal/format/notes_test.go`**
   - `TestRenderNotes_UnstructuredFallback`: verify note rendering text
   - `TestHasNotesOfKind_UnstructuredFallback`: utility function works with new kind

3. **`cmd/generate_test.go`** (integration-style with mocked GitHub API)
   - `TestCollectIssueData_NoReportsWithComments`: Case 1 fallback — returns `Unknown` status, `ShouldSummarize = true`, `NoteUnstructuredFallback`
   - `TestCollectIssueData_NoReportsNoComments`: Case 1 no fallback — unchanged `NeedsUpdate` behavior
   - `TestCollectIssueData_ReportsEmptyUpdateWithComments`: Case 2a fallback — preserves report status, uses comment body
   - `TestCollectIssueData_ReportsEmptyUpdateNoComments`: Case 2a no fallback — unchanged `NeedsUpdate` behavior
   - `TestCollectIssueData_ReportsWithUpdateText`: Case 2b — unchanged, no fallback triggered

### Table-Driven Test Structure

```go
func TestCollectIssueData_CommentFallback(t *testing.T) {
    tests := []struct {
        name            string
        issueState      string
        reports         []report.Report
        comments        []github.Comment
        wantStatus      derive.Status
        wantSummarize   bool
        wantNoteKind    *format.NoteKind
        wantUpdateTexts []string
    }{
        // ... test cases
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            // ...
        })
    }
}
```

## Future Considerations

1. **Bot comment filtering**: Add heuristics to skip comments from known bot accounts (e.g., `github-actions[bot]`, `dependabot[bot]`) or comments matching CI output patterns.
2. **Multiple comment fallback**: Use the last N comments (e.g., 3) instead of just the most recent, providing more context for AI summarization.
3. **Comment quality scoring**: Rank comments by length, author, and content to select the most informative one rather than the most recent.
4. **Interaction with Phase 2**: Sentiment analysis (Phase 2) will be especially valuable for fallback cases, where the status is `Unknown` and AI can suggest an appropriate status from the comment content.

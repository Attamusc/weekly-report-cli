# Milestone 2c: Executive Summary Header + Action Wrapper Polish

**Date:** 2026-04-21
**Status:** Draft
**Directory:** /Users/attamusc/projects/github.com/Attamusc/weekly-report-cli

## Overview

Two independent features:
1. **Executive summary header** — AI-generated "highlights this week" paragraph at the top of the report, opt-in via `--summary-header` flag. Falls back to stats-based header when AI is disabled.
2. **CI/CD Action wrapper polish** — extend the existing `action.yml` with missing inputs, fix stderr/stdout handling, add discussion posting support.

## Feature 1: Executive Summary Header

### Approach

Add `GenerateHeader` method to the `Summarizer` interface, following the existing pattern where every AI capability is a method on `Summarizer` with a `NoopSummarizer` fallback.

### Design

**New types in `internal/ai/summarizer.go`:**
```go
type HeaderItem struct {
    StatusCaption    string
    StatusTransition *string  // "At Risk→On Track" or nil
    NewItem          bool
    Title            string
    Summary          string
}
```

**New method on `Summarizer` interface:**
```go
GenerateHeader(ctx context.Context, items []HeaderItem) (string, error)
```

**`NoopSummarizer.GenerateHeader`:** Count statuses from items, produce stats string like "5 items: 3 on track, 1 at risk, 1 done. 2 new items, 1 status change."

**`GHModelsClient.GenerateHeader`:** Single API call with `headerSystemPrompt`. Input is JSON-serialized `HeaderItem` array. Output is 1-2 sentence paragraph.

**`cmd/generate.go` changes:**
1. New `--summary-header` bool flag
2. After Phase D (diff), convert `rows` → `[]ai.HeaderItem`, call `summarizer.GenerateHeader`
3. Pass header string to `renderGenerateOutput`
4. Render header as plain paragraph above the table

**`internal/config/config.go` changes:**
- Add `SummaryHeader bool` to `ConfigInput`

### Header System Prompt

```
You are summarizing a weekly engineering status report. You will receive a JSON array of items, each with:
- status: current status (e.g., "On Track", "At Risk", "Done")
- transition: status change from previous week (e.g., "At Risk→On Track") or null
- new: whether this is a new item this week
- title: the initiative/epic name
- summary: the update text

Produce a single paragraph (2-3 sentences) highlighting the most notable changes this week.
Focus on: status transitions, new blockers, completed items, and overall trajectory.
Do NOT list every item. Be concise and executive-level.

Respond with ONLY the paragraph text, no formatting, no prefatory text.
```

### Rendering

Header is rendered as a plain paragraph above the first table. When `--group-by` is used, the header still appears once at the top (not per-group).

```
Migration progress is strong with 5 items on track. Redis scaling is blocked pending capacity review. Payment Gateway moved from On Track to At Risk.

| Status | Initiative/Epic | Target Date | Update |
...
```

## Feature 2: Action Wrapper Polish

### Changes to `action.yml`

**New inputs:**
- `previous-report` — path to previous report file
- `group-by` — group rows by field
- `columns` — extra columns to show
- `collapsible-notes` — wrap notes in `<details>`
- `summary-header` — enable executive summary header
- `discussion-number` — GitHub Discussion number to post to
- `discussion-repo` — repo for discussion (default: current repo)

**New outputs:**
- `row-count` — number of report rows (0 when exit code 2)
- `exit-code` — raw exit code from CLI

**Bug fix:** Current step uses `2>&1` which mixes stderr into report. Fix: capture stdout to file, let stderr go to Actions log.

**Discussion posting:** Optional step that runs when `discussion-number` is provided. Uses `gh api` to post the report as a comment on the discussion.

## Dependencies

- Existing: `internal/ai/summarizer.go`, `internal/ai/ghmodels.go`, `cmd/generate.go`, `cmd/common.go`, `action.yml`
- No new external dependencies

## Risks

- AI header quality depends on prompt engineering — mitigated by stats fallback
- Discussion posting uses `gh api` which requires appropriate token permissions — documented in action inputs

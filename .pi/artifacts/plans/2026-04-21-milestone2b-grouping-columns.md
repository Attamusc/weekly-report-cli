# Milestone 2b: Team-level Grouping & Custom Columns

**Date:** 2026-04-21
**Status:** Draft
**Spec:** Provided inline in planner task (Milestone 2b features)
**Directory:** /Users/attamusc/projects/github.com/Attamusc/weekly-report-cli

## Overview

Add two complementary features to the generate command:
1. **`--group-by`** — Group report rows by assignee, label pattern, or project field for scannability
2. **`--columns`** — Surface project-board-specific fields (Sprint, Priority, Team) as extra table columns

Both features enhance the report output for teams using GitHub Project boards.

## Approach

### Data Threading Strategy

The core work is threading metadata that already exists in the system through to the rendering layer:

```
ProjectItem.FieldValues ──┐
ProjectItem.Assignees ────┤
                          ├──→ IssueRef (enriched) ──→ pipeline.IssueData ──→ format.Row
github.IssueData.Assignees┘
```

**`IssueRef`** gains two optional fields: `Assignees []string` and `FieldValues map[string]string`. These are populated by `projectClientAdapter` from `ProjectItem` data that's currently discarded. For non-project inputs (URL lists), these remain nil/empty.

**`pipeline.IssueData`** gains `Assignees []string` and `ExtraColumns map[string]string`. Populated in `CollectIssueData` by merging:
- Assignees from `github.IssueData` (always available via API)
- Assignees from `IssueRef` (project-sourced, may have additional data)
- Field values from `IssueRef.FieldValues` (project-sourced only)

**`format.Row`** gains `Assignees []string`, `Labels []string`, and `ExtraColumns map[string]string`. These support both grouping (needs assignees/labels for grouping key extraction) and custom columns (ExtraColumns rendered as additional table columns).

### Custom Columns Design

`--columns "Priority,Sprint"` adds extra columns to the markdown table.

- `RenderTable(rows []Row, extraColumns []string)` — extra column names determine which `ExtraColumns` map entries render
- Columns appear between "Initiative/Epic" and "Target Date"
- Header/separator rows are dynamically generated
- When `extraColumns` is empty (default), output is identical to today's 4-column table
- Values come from `Row.ExtraColumns[columnName]` — empty string if missing

### Grouping Design

`--group-by assignee` / `--group-by "label:team-*"` / `--group-by "field:Priority"`

New types and function in `format` package:

```go
type GroupMode int
const (
    GroupByAssignee GroupMode = iota
    GroupByLabel
    GroupByField
)

type GroupConfig struct {
    Mode    GroupMode
    Pattern string // label glob pattern or field name
}

type RowGroup struct {
    Title string
    Rows  []Row
}

func GroupRows(rows []Row, config GroupConfig) []RowGroup
```

**Grouping logic:**
- **Assignee:** Group by first assignee. Multi-assigned items appear under first assignee only. No assignee → "Unassigned" group.
- **Label:** Match labels against glob pattern (e.g., `team-*`). Use `filepath.Match` for glob. Group title is the matched label. No match → "Other" group.
- **Field:** Group by `ExtraColumns[fieldName]` value. Empty value → "Other" group.

**Sorting:** Each group's rows are sorted independently via `SortRowsByTargetDate`. Groups themselves are sorted alphabetically, with "Unassigned"/"Other" always last.

**Rendering:** Each group becomes a `RenderTableWithTitle(groupTitle, rows, extraColumns)` call. Groups are separated by a blank line.

### CLI Flag Design

```go
// In cmd/generate.go init()
generateCmd.Flags().StringVar(&groupBy, "group-by", "", "Group rows by: assignee, label:<glob>, field:<name>")
generateCmd.Flags().StringVar(&columns, "columns", "", "Comma-separated project field names to show as extra columns")
```

### Parse `--group-by` value

```go
func ParseGroupBy(raw string) (GroupConfig, error)
// "assignee"         → GroupConfig{Mode: GroupByAssignee}
// "label:team-*"     → GroupConfig{Mode: GroupByLabel, Pattern: "team-*"}
// "field:Priority"   → GroupConfig{Mode: GroupByField, Pattern: "Priority"}
```

## Dependencies

### Existing code modified
- `internal/input/links.go` — Add fields to `IssueRef`
- `internal/pipeline/types.go` — Add fields to `IssueData`
- `internal/pipeline/generate.go` — Thread metadata in `CollectIssueData` and `AssembleGenerateResults`
- `internal/format/markdown.go` — Add fields to `Row`, parameterize `RenderTable`, update `RenderTableWithTitle`
- `cmd/generate.go` — Add flags, grouping orchestration, updated rendering
- `cmd/common.go` — Update `projectClientAdapter` to preserve metadata

### New code
- `internal/format/group.go` — `GroupConfig`, `RowGroup`, `GroupRows`, `ParseGroupBy`
- Tests for all new/changed code

### No new libraries needed
- `filepath.Match` from stdlib for label glob matching
- `strings`, `sort` for grouping/sorting

## Risks & Open Questions

1. **Label glob matching edge cases** — `filepath.Match` handles `*`, `?`, `[...]` patterns. Should be sufficient for `team-*` style patterns. If users need regex, that's a future enhancement.
2. **Multi-assignee items** — Using first assignee for grouping is simple but loses info. Could duplicate rows across groups, but that inflates counts. First-assignee is the pragmatic choice.
3. **Diff compatibility** — Grouping happens after diff (Phase D), so status transitions still work correctly. The diff compares flat row lists regardless of grouping.
4. **`--columns` without `--project`** — If someone passes `--columns` without a project board, ExtraColumns will be empty. Columns will render with blank values. This is acceptable — not an error.

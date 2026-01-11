# Describe Command Implementation Plan

## Overview

A new CLI command (`describe`) that generates project/goal summaries by analyzing the issue body (description) rather than weekly status reports. It supports the same input sources as `generate` (project views, URL lists, mixed) and outputs in both table and detailed formats.

## Key Differences from `generate`

| Aspect | `generate` | `describe` |
|--------|------------|------------|
| **Data Source** | Comments (status reports via HTML markers) | Issue body (description) |
| **AI Focus** | Summarize recent progress/updates | Summarize project overview & goals |
| **Time Window** | Uses `--since-days` to filter recent comments | Not time-based (issue body is static) |
| **Output Content** | Status, Target Date, Update Summary | Title, Summary/Goals, Labels, Assignee |

## Implementation Plan

### 1. Extend GitHub Client to Fetch Issue Body and Metadata

**File:** `internal/github/issues.go`

Extend `IssueData` struct to include:

```go
type IssueData struct {
    URL         string
    Title       string
    State       string
    Body        string     // NEW: Issue body/description
    Labels      []string   // NEW: Issue labels
    Assignees   []string   // NEW: Issue assignees
    ClosedAt    *time.Time
    CloseReason string
}
```

Update `FetchIssue()` to populate these new fields from the GitHub API response (already available in `github.Issue`).

### 2. Create New AI Summarizer Method for Project Descriptions

**File:** `internal/ai/summarizer.go`

Add new interface methods for describing projects:

```go
type Summarizer interface {
    // Existing methods...
    
    // DescribeBatch generates project/goal summaries for issue descriptions
    // Returns a map of issueURL -> description summary
    DescribeBatch(ctx context.Context, items []DescribeBatchItem) (map[string]string, error)
}

type DescribeBatchItem struct {
    IssueURL   string
    IssueTitle string
    IssueBody  string
}
```

**File:** `internal/ai/ghmodels.go`

Implement `DescribeBatch()` with a specialized system prompt:

- Focus on extracting project overview, goals, and key objectives
- Different prompt than the update summarization prompt
- Support custom prompts via `--describe-prompt` flag

**Default System Prompt (suggested):**

```
You are a technical writer summarizing GitHub issues for project documentation.
For each issue, extract and summarize:
1. The main objective or goal of the project/feature
2. Key deliverables or scope items
3. Any important constraints or dependencies mentioned

Keep summaries concise (2-4 sentences), factual, and in third-person present tense.
Focus on WHAT the project is about, not progress or status updates.
```

### 3. Create New Format Types for Describe Output

**File:** `internal/format/describe.go` (new file)

```go
// DescribeRow represents a row for the describe command table format
type DescribeRow struct {
    Title     string
    URL       string
    Summary   string   // AI-generated project/goals summary
    Labels    []string
    Assignees []string
}

// RenderDescribeTable generates a markdown table for describe output
func RenderDescribeTable(rows []DescribeRow) string

// RenderDescribeDetailed generates detailed markdown sections for each issue
func RenderDescribeDetailed(rows []DescribeRow) string
```

#### Table Format Output

```markdown
| Initiative | Labels | Assignee | Summary |
|------------|--------|----------|---------|
| [Issue Title](url) | label1, label2 | @user1, @user2 | AI-generated summary |
```

#### Detailed Format Output

```markdown
## [Issue Title](url)

**Labels:** label1, label2  
**Assignees:** @user1, @user2

### Summary
AI-generated project overview and goals summary...

---
```

### 4. Create the `describe` Command

**File:** `cmd/describe.go` (new file)

- Reuse input resolution logic from `generate` (project views, URL lists, mixed mode)
- Support same project-related flags (`--project`, `--project-view`, etc.)
- Add new flags:
  - `--format` (`table` | `detailed`) - default: `table`
  - `--describe-prompt` - custom AI prompt for description generation
  - `--no-summary` - disable AI summarization (output raw body excerpt)
- **Not needed:** `--since-days` flag (not time-based)

#### Pipeline (3-phase, similar to generate)

**Phase A: Data Collection (Parallel)**
1. Resolve issue references (same as generate)
2. Fetch issue metadata including body, labels, assignees (extended FetchIssue)

**Phase B: Batch Summarization**
1. Collect all items that need AI summarization
2. Call `summarizer.DescribeBatch()` with issue bodies
3. Falls back to truncated body if AI fails

**Phase C: Result Assembly**
1. Match AI summaries to issues by URL
2. Create DescribeRow for each issue
3. Render based on `--format` flag (table or detailed)

### 5. Configuration Updates

**File:** `internal/config/config.go`

Add describe-specific configuration:

```go
type DescribeConfig struct {
    Format         string // "table" or "detailed"
    DescribePrompt string // Custom AI prompt
    DisableSummary bool   // Skip AI summarization
}
```

## File Changes Summary

| File | Change Type | Description |
|------|-------------|-------------|
| `internal/github/issues.go` | Modify | Add Body, Labels, Assignees to IssueData |
| `internal/ai/summarizer.go` | Modify | Add DescribeBatchItem type and DescribeBatch interface method |
| `internal/ai/ghmodels.go` | Modify | Implement DescribeBatch with project-focused prompt |
| `internal/format/describe.go` | New | DescribeRow type, table and detailed renderers |
| `cmd/describe.go` | New | New describe command with 3-phase pipeline |
| `internal/config/config.go` | Modify | Add DescribeConfig struct |

## CLI Usage Examples

```bash
# Basic usage from project board
weekly-report-cli describe --project "org:my-org/5"

# With project view filter
weekly-report-cli describe --project "org:my-org/5" --project-view "Current Sprint"

# Detailed format output
weekly-report-cli describe --project "org:my-org/5" --format detailed

# From URL list
cat issues.txt | weekly-report-cli describe

# Mixed mode (project + additional URLs)
weekly-report-cli describe --project "org:my-org/5" --input critical-issues.txt

# With custom AI prompt
weekly-report-cli describe --project "org:my-org/5" \
  --describe-prompt "Summarize the business value and technical scope of this project"

# Without AI (raw body excerpt)
weekly-report-cli describe --project "org:my-org/5" --no-summary
```

## Design Decisions

### Body Truncation (when AI is disabled)

- **Table format:** Truncate body to first 500 characters with ellipsis
- **Detailed format:** Show full body content

This ensures table readability while allowing full context in detailed view.

### Labels Display

- Comma-separated in a single column (e.g., `bug, enhancement, priority:high`)
- Show all labels without truncation

### Assignees Display

- All assignees shown, comma-separated with `@` prefix
- Example: `@user1, @user2, @user3`

## Testing Strategy

### Unit Tests

1. **GitHub Client Tests** (`internal/github/issues_test.go`)
   - Verify Body, Labels, Assignees extraction
   - Test with empty/missing fields

2. **AI Summarizer Tests** (`internal/ai/ghmodels_test.go`)
   - Mock server for DescribeBatch
   - Test JSON parsing and fallback handling
   - Test chunking for large batches

3. **Format Tests** (`internal/format/describe_test.go`)
   - Table rendering with various label/assignee combinations
   - Detailed format rendering
   - Edge cases (empty labels, no assignees, long titles)

4. **Command Tests** (`cmd/describe_test.go`)
   - Flag parsing
   - Input resolution modes
   - Format flag handling

### Integration Tests

- End-to-end test with mocked GitHub API
- Verify output format correctness
- Test project view + manual filter combination

## Future Considerations

1. **Caching:** Issue bodies rarely change; could cache descriptions to avoid re-fetching
2. **Custom Fields:** Support extracting custom project fields from GitHub Projects for additional metadata
3. **Export Formats:** JSON/CSV output for programmatic consumption
4. **Template Support:** User-defined output templates

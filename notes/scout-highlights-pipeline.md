# Scout: Highlights Pipeline Reconnaissance

## Current Control Flow

### Entry Point: `cmd/highlights.go`

**Flags:**
- `--users` (required): CSV of GitHub usernames
- `--orgs`: CSV of org names to scope results (optional)
- `--since-days`: Lookback window (default 7)
- `--concurrency`: Max parallel API requests (default 5)
- `--no-summary`: Disable AI curation — returns raw mechanical rollup
- `--summary-prompt`: Custom AI system prompt for highlights curation
- `--verbose`, `--quiet`

**Phase boundaries:**

1. **PHASE A: Discovery** (`discovery.Search`, line ~100)
   - Input: users, orgs, since date
   - Output: `[]input.IssueRef` (dedup'd, bot-filtered, authorship-filtered)
   - Preserves: owner, repo, number, URL (no comment count or search metadata)
   - Note: `IssueRef` has optional `Assignees` and `FieldValues` fields (from project board), not populated by discovery

2. **PHASE B: Collection** (`collectHighlightDataParallel`, line ~115)
   - Parallel goroutines with semaphore (concurrency control)
   - Per-ref: `pipeline.CollectHighlightData(ctx, fetcher, ref, since)`
   - Returns: `[]pipeline.HighlightData` (metadata + recent comment texts)
   - Progress logging to stderr if `!cfg.Quiet`
   - Errors are logged but don't stop pipeline (graceful degradation)

3. **PHASE C: AI Curation** (`summarizer.HighlightsBatch`, line ~135)
   - Input: `[]ai.HighlightItem` (converted from `HighlightData`)
   - AI filters noise, assigns themes, writes 1-line summaries
   - Falls back to `NoopSummarizer` if AI fails (logs warn)
   - Output: `[]ai.Highlight` (filtered, curated set with themes)

4. **PHASE D: Assembly** (`format.RenderHighlights`, line ~145)
   - Groups highlights by theme alphabetically
   - Renders markdown: `## Theme\n- [Title](URL) — Summary\n`
   - Output goes to stdout (no stderr)
   - Returns "" if no highlights (no error)

**`--no-summary` seam:**
- Line ~85: `if highlightsNoSummary || !cfg.Models.Enabled { summarizer = ai.NewNoopSummarizer() }`
- `NoopSummarizer.HighlightsBatch()` returns ALL items with label-based themes (no filtering)
- Fallback path is transparent — both AI and Noop implement same interface

---

## Key Types and Seams

### `ai.HighlightItem` (summarizer.go)
```go
type HighlightItem struct {
	IssueURL    string   // Issue/PR URL
	IssueTitle  string   // Title
	IssueState  string   // "open" or "closed"
	IsPR        bool     // True if pull request
	Labels      []string // Issue labels
	UpdateTexts []string // Recent comments (newest first? unordered?)
}
```

### `ai.Highlight` (summarizer.go)
```go
type Highlight struct {
	Theme   string // Category: "Bug Fixes", "Support & Reliability", etc.
	Title   string // Issue/PR title
	URL     string // Issue/PR URL
	Summary string // AI-written 1-line highlight
}
```

### `pipeline.HighlightData` (types.go)
```go
type HighlightData struct {
	IssueURL    string   // Issue URL
	IssueTitle  string   // Title
	IssueState  string   // "open" or "closed"
	Labels      []string // Issue labels
	UpdateTexts []string // Recent comment bodies (unfiltered text)
}
```

### `ai.Summarizer` interface (summarizer.go)

Relevant methods:
- `HighlightsBatch(ctx, []HighlightItem) ([]Highlight, error)` — **Only method used by highlights cmd**
  - Returns filtered/curated set with themes and summaries
  - AI filters noise; Noop returns all items
- `SummarizeBatch(ctx, []BatchItem) (map[string]BatchResult, error)` — Used by `generate` command only

### `format.RenderHighlights` (format/highlights.go)

- Input: `[]ai.Highlight`
- Output: Markdown string, grouped by theme (alphabetical), format: `## Theme\n- [Title](URL) — Summary\n`
- Returns "" if empty (no error)

---

## Existing Chunker Code in `internal/ai/ghmodels.go`

**Purpose:** Token budget management for batch API requests

### Size/chunking constants (top of file)
```go
maxBatchSize   = 25       // Item count cap per batch
maxBatchTokens = 8000     // Token budget per request (input only)
avgCharsPerToken = 3      // Conservative chars→tokens ratio
maxBatchChars = maxBatchTokens * avgCharsPerToken = 24,000 chars
```

### Functions ripped out when redesigning:

| Function | Purpose | Used by |
|----------|---------|---------|
| `HighlightsBatch()` (line ~625) | Entry point; decides single vs chunked | highlights cmd only |
| `singleHighlightsBatch()` (line ~650) | Single API call for one chunk; NO recursion | Internal to highlights chunker |
| `chunkedHighlightsBatch()` (line ~680) | Splits items, calls `singleHighlightsBatch()` on each | `HighlightsBatch()` via size/count checks |
| `packHighlightChunks()` (line ~700) | Packs items into chunks by size budget AND count cap | `chunkedHighlightsBatch()` |
| `estimateHighlightItemSize()` (line ~738) | Estimates JSON serialized size | `packHighlightChunks()`, truncation logic |
| `truncateHighlightItem()` (line ~755) | Truncates UpdateTexts to fit budget; preserves URL | `packHighlightChunks()` |
| `buildHighlightsPrompt()` (line ~800) | Builds JSON request body | `singleHighlightsBatch()` |
| `parseHighlightsResponse()` (line ~830) | Parses JSON array response → `[]Highlight` | `singleHighlightsBatch()` |

### Generic batch orchestrator (not highlights-specific):

| Function | Purpose |
|----------|---------|
| `runBatch[I, R any]()` (line ~560) | Generic chunker for `SummarizeBatch` and `DescribeBatch` only |
| `chunkedBatch[I, R any]()` (line ~595) | Generic chunk processor; calls recursively via function param |

**Note:** `runBatch` and `chunkedBatch` use **recursive function parameter** to avoid hardcoding which batch method to call. `HighlightsBatch()` implements its own chunking instead of using `runBatch` (intentionally separate pattern).

---

## Discovery Output & Metadata

### What `IssueRef` carries from discovery:

```go
type IssueRef struct {
	Owner       string
	Repo        string
	Number      int
	URL         string
	Assignees   []string          // NOT populated by discovery search
	FieldValues map[string]string // NOT populated by discovery search
}
```

- **Preserved from search result:** URL, owner, repo, number only
- **NOT preserved:** Comment count, search relevance, last update time, `--no-summary` signal
- **Assignees and FieldValues:** Only populated when input comes from GitHub Projects board, not from discovery search

### Discovery filtering pipeline (internal/discovery/search.go):

1. `executeSearch()` — Runs GitHub search API with pagination (respects rate limits)
2. `deduplicateToRefs()` — Dedup by URL, parse into `IssueRef`
3. `filterBots()` — Removes bot-authored items
4. `filterByTeamAuthorship()` — Removes items where NO team member was involved (author/commenter/assignee)

**Signal available but not currently used:**
- Search result includes `issue.GetCommentCount()`, `issue.GetHTMLURL()`, timestamps
- These are discarded during `deduplicateToRefs()` → `parseSearchResult()`
- Could preserve comment count or last-updated timestamp for "mechanical rollup" (e.g., sort by comment activity)

---

## Test Coverage Map

### Meaningful test files (by package):

| Package | Test files | Coverage notes |
|---------|-----------|-----------------|
| `internal/ai` | `batch_test.go`, `ghmodels_test.go` | ~15+ test cases covering `SummarizeBatch`, `HighlightsBatch`, response parsing (nested/flat/markdown formats), `NoopSummarizer` |
| `internal/format` | `highlights_test.go` | Tests `RenderHighlights()` grouping; fixtures use `[]ai.Highlight` |
| `internal/discovery` | `search_test.go`, `filter_test.go` | Tests query building, pagination, bot filtering, authorship filtering |
| `internal/pipeline` | `generate_test.go` | Tests `CollectIssueData()`, `BatchSummarize()`, fallback logic; NO highlights-specific tests |
| (no highlights tests in `cmd/`) | — | **Gap:** No CLI-level tests for `highlights` command |

### Test fixtures available (for reuse):

- `batch_test.go` has inline `[]BatchItem` and `[]ai.Highlight` fixtures (no files on disk)
- `format/highlights_test.go` has small test data for grouping
- Discovery tests use inline query/filter fixtures

---

## Generate Command's AI Pattern (for reference)

### Entry: `cmd/generate.go` → `internal/pipeline/generate.go`

**Batch summarization (map-reduce style):**

1. **Collection phase** (parallel, like highlights):
   - `collectIssueDataParallel()` fetches issue data + comments in parallel goroutines
   - Returns: `[]pipeline.IssueData` (includes reports, status, assignees, extra columns)

2. **Filtering phase** (sequential, line ~270 in generate.go):
   ```go
   var batchItems []ai.BatchItem
   for _, data := range allData {
       if data.ShouldSummarize && len(data.UpdateTexts) > 0 {
           batchItems = append(batchItems, ai.BatchItem{...})
       }
   }
   ```
   - Filters to only items that should be summarized
   - Creates `ai.BatchItem` (issue URL, title, updates, reported status)

3. **Batch AI call** (single orchestrated call):
   ```go
   summaries, err := summarizer.SummarizeBatch(ctx, batchItems)
   ```
   - `SummarizeBatch` chunks internally if needed (via `runBatch` + `chunkedBatch`)
   - Returns: `map[string]BatchResult` (URL → summary + optional sentiment)

4. **Result assembly** (sequential):
   ```go
   CreateResultFromData(data, summary)
   ```
   - Matches summaries back to original data by URL
   - Creates `format.Row` for final output

**Key insight:** Generate does **filtering** before batching (only items needing AI), then assembles result rows afterward. Highlights currently skips the filtering step — sends ALL items to AI.

---

## ⚠️ Observations

### Current highlights flow (problematic for redesign):

1. **No mechanical rollup** — AI curation is all-or-nothing
   - `--no-summary` uses label-based bucketing (crude fallback)
   - No intermediate "mechanical rollup" phase that filters/sorts before optional AI

2. **UpdateTexts order unclear** — Are comments ordered newest-first?
   - `CollectHighlightData()` iterates `fetcher.FetchCommentsSince()` results
   - Doesn't sort or guarantee order
   - AI `HighlightsBatch` system prompt says "newest first" but actual order may not be

3. **All items sent to AI** — No filtering before summarization
   - Unlike `generate` which filters `data.ShouldSummarize`
   - Highlights batches everything, relies on AI to filter
   - If AI fails, entire batch fails (no partial results)

4. **Test gap** — No integration tests for highlights pipeline
   - Only unit tests for format grouping and batch parsing
   - No end-to-end CLI tests

5. **Chunking is highlights-only** — Not shared with generate
   - `HighlightsBatch()` has custom chunker (`packHighlightChunks`, `singleHighlightsBatch`)
   - `SummarizeBatch()` uses generic `runBatch` + `chunkedBatch`
   - Different patterns for same problem (inconsistency)

6. **Metadata loss in discovery** — Search comment count discarded
   - Could use as signal for "mechanical importance" before AI curates
   - Currently thrown away during `parseSearchResult()`


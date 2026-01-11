# Sentiment Analysis Plan

## Overview

Add AI-powered sentiment analysis that detects mismatches between a reporter's stated status (e.g., "On Track") and the actual content of their updates (e.g., mentions blockers, delays, or risks). The analysis piggybacks on the existing AI batch call — the same API request returns both the summary and a sentiment assessment. When a mismatch is detected, a note is added with the AI-suggested status and an explanation.

This feature depends on Phase 1 (Unstructured Comment Fallback) being completed first, as it shares the same pipeline modifications and is especially valuable for fallback cases where the status is `Unknown`.

## Key Behaviors

| Scenario | Behavior |
|----------|----------|
| Status matches content | No note added |
| Status mismatches content | `NoteSentimentMismatch` note with suggested status + explanation |
| AI disabled (`DISABLE_SUMMARY`) | Sentiment analysis skipped entirely |
| `--no-sentiment` flag | Sentiment analysis skipped |
| AI returns old format (no sentiment) | Graceful fallback — summaries work, sentiments are nil |
| Fallback case (Phase 1, status = `Unknown`) | Sentiment suggests a status for the `Unknown` row |

## Implementation Plan

### 1. Add `SentimentResult` Type and Update `SummarizeBatch` Return Type

**File:** `internal/ai/summarizer.go`

The current `SummarizeBatch` returns `map[string]string` (URL -> summary). To carry sentiment data alongside summaries, introduce a new result type and a wrapper:

```go
// SentimentResult holds the AI's assessment of whether the reported status
// matches the content of the updates.
type SentimentResult struct {
    SuggestedStatus string // Canonical status key: "on_track", "at_risk", "off_track", etc.
    Explanation     string // Brief AI explanation of the mismatch
}

// BatchResult holds both the summary and optional sentiment for a single issue.
type BatchResult struct {
    Summary   string
    Sentiment *SentimentResult // nil when sentiment is disabled or unavailable
}
```

Update the `Summarizer` interface:

```go
type Summarizer interface {
    Summarize(ctx context.Context, issueTitle, issueURL, updateText string) (string, error)
    SummarizeMany(ctx context.Context, issueTitle, issueURL string, updates []string) (string, error)
    SummarizeBatch(ctx context.Context, items []BatchItem) (map[string]BatchResult, error) // CHANGED return type
    DescribeBatch(ctx context.Context, items []DescribeBatchItem) (map[string]string, error)
}
```

### 2. Add `ReportedStatus` to `BatchItem`

**File:** `internal/ai/summarizer.go`

The AI needs to know the reporter's claimed status to detect mismatches:

```go
type BatchItem struct {
    IssueURL       string   // Unique identifier for matching response
    IssueTitle     string   // Issue title for context
    UpdateTexts    []string // One or more updates (newest first)
    ReportedStatus string   // NEW: The reporter's claimed status (e.g., "On Track", "Unknown")
}
```

### 3. Update `NoopSummarizer`

**File:** `internal/ai/summarizer.go`

Update `SummarizeBatch` to return the new type:

```go
func (n *NoopSummarizer) SummarizeBatch(_ context.Context, items []BatchItem) (map[string]BatchResult, error) {
    results := make(map[string]BatchResult, len(items))
    for _, item := range items {
        summary := strings.TrimSpace(strings.Join(item.UpdateTexts, "\n\n---\n\n"))
        results[item.IssueURL] = BatchResult{Summary: summary, Sentiment: nil}
    }
    return results, nil
}
```

### 4. Update AI Batch Prompt and Response Format

**File:** `internal/ai/ghmodels.go`

#### Updated `batchSystemPrompt`

The prompt changes from requesting `map[string]string` to requesting a nested JSON structure:

```go
const batchSystemPrompt = `You are summarizing multiple engineering status updates in a single batch.

You will receive a JSON array of items, each with an "id" (GitHub issue URL), "issue" (title),
"updates" (array of update texts, newest first), and "reported_status" (the author's claimed status).

For each item, produce:
1. A summary: 3-5 sentences, present tense, third-person, markdown-ready, no prefatory text.
   Keep relevant links intact in markdown format. Do not lose context when summarizing.
2. A sentiment assessment: analyze whether the update content matches the reported status.
   - If the content describes blockers, delays, risks, or problems but the status is "On Track",
     suggest a more appropriate status.
   - If the content is positive but the status is "At Risk" or "Off Track", suggest a better status.
   - If the reported status is "Unknown", suggest an appropriate status based on the content.
   - If the status matches the content, set sentiment to null.

Respond ONLY with a valid JSON object. The keys are the issue URLs (from the "id" field).
Each value is an object with "summary" (string) and "sentiment" (object or null).

When sentiment is not null, it must have:
- "status": one of "on_track", "at_risk", "off_track", "not_started", "done"
- "explanation": one sentence explaining the mismatch

Example response:
{
  "https://github.com/org/repo/issues/1": {
    "summary": "The team completed the migration...",
    "sentiment": null
  },
  "https://github.com/org/repo/issues/2": {
    "summary": "Work on the API integration is ongoing...",
    "sentiment": {
      "status": "at_risk",
      "explanation": "Update mentions two unresolved blockers despite being reported as On Track."
    }
  }
}`
```

#### Updated `batchRequestItem`

```go
type batchRequestItem struct {
    ID             string   `json:"id"`
    Issue          string   `json:"issue"`
    Updates        []string `json:"updates"`
    ReportedStatus string   `json:"reported_status"`
}
```

#### Updated `buildBatchPrompt`

Pass `ReportedStatus` through to the request JSON:

```go
func (c *GHModelsClient) buildBatchPrompt(items []BatchItem) (string, error) {
    request := batchRequest{
        Items: make([]batchRequestItem, len(items)),
    }
    for i, item := range items {
        request.Items[i] = batchRequestItem{
            ID:             item.IssueURL,
            Issue:          item.IssueTitle,
            Updates:        item.UpdateTexts,
            ReportedStatus: item.ReportedStatus,
        }
    }
    jsonBytes, err := json.Marshal(request)
    if err != nil {
        return "", fmt.Errorf("marshal batch request: %w", err)
    }
    return string(jsonBytes), nil
}
```

#### Updated Response Parsing

Replace `parseBatchResponse` to handle the new nested format with fallback to the old flat format:

```go
// sentimentResponseItem represents the new nested AI response format.
type sentimentResponseItem struct {
    Summary   string           `json:"summary"`
    Sentiment *sentimentResult `json:"sentiment"`
}

type sentimentResult struct {
    Status      string `json:"status"`
    Explanation string `json:"explanation"`
}

func (c *GHModelsClient) parseBatchResponse(response string, items []BatchItem) (map[string]BatchResult, error) {
    // Try new nested format first
    var nested map[string]sentimentResponseItem
    if err := json.Unmarshal([]byte(response), &nested); err == nil {
        if len(nested) > 0 {
            // Verify at least one item has a "summary" field to distinguish
            // from the old flat format
            for _, v := range nested {
                if v.Summary != "" {
                    return convertNestedResponse(nested), nil
                }
                break
            }
        }
    }

    // Fall back to old flat format (map[string]string)
    var flat map[string]string
    if err := json.Unmarshal([]byte(response), &flat); err == nil && len(flat) > 0 {
        results := make(map[string]BatchResult, len(flat))
        for url, summary := range flat {
            results[url] = BatchResult{Summary: summary, Sentiment: nil}
        }
        return results, nil
    }

    // Fall back to markdown parsing
    return c.parseMarkdownBatchResponse(response, items)
}

func convertNestedResponse(nested map[string]sentimentResponseItem) map[string]BatchResult {
    results := make(map[string]BatchResult, len(nested))
    for url, item := range nested {
        var sentiment *SentimentResult
        if item.Sentiment != nil {
            sentiment = &SentimentResult{
                SuggestedStatus: item.Sentiment.Status,
                Explanation:     item.Sentiment.Explanation,
            }
        }
        results[url] = BatchResult{
            Summary:   item.Summary,
            Sentiment: sentiment,
        }
    }
    return results
}
```

Update `parseMarkdownBatchResponse` to return `map[string]BatchResult` (sentiments always nil in markdown fallback):

```go
func (c *GHModelsClient) parseMarkdownBatchResponse(response string, items []BatchItem) (map[string]BatchResult, error) {
    // Existing markdown parsing logic...
    // Wrap each summary in BatchResult{Summary: summary, Sentiment: nil}
}
```

### 5. Add Status Key Parsing

**File:** `internal/derive/status.go`

Add a `Key()` method to `Status` and a `ParseStatusKey()` function for converting between the canonical key format used in AI responses and the `Status` type:

```go
// Key returns the canonical snake_case key for the status.
func (s Status) Key() string {
    switch s {
    case OnTrack:
        return "on_track"
    case AtRisk:
        return "at_risk"
    case OffTrack:
        return "off_track"
    case NotStarted:
        return "not_started"
    case Done:
        return "done"
    case NeedsUpdate:
        return "needs_update"
    case Unknown:
        return "unknown"
    default:
        return "unknown"
    }
}

// ParseStatusKey converts a canonical snake_case status key to a Status value.
// Returns (Status, false) if the key is not recognized.
func ParseStatusKey(key string) (Status, bool) {
    switch strings.ToLower(strings.TrimSpace(key)) {
    case "on_track":
        return OnTrack, true
    case "at_risk":
        return AtRisk, true
    case "off_track":
        return OffTrack, true
    case "not_started":
        return NotStarted, true
    case "done":
        return Done, true
    case "needs_update":
        return NeedsUpdate, true
    case "unknown":
        return Unknown, true
    default:
        return Unknown, false
    }
}
```

### 6. Add `NoteSentimentMismatch` Note Kind

**File:** `internal/format/notes.go`

Extend the `NoteKind` enum and `Note` struct:

```go
const (
    NoteMultipleUpdates      NoteKind = iota // 0
    NoteNoUpdatesInWindow                    // 1
    NoteUnstructuredFallback                 // 2 (Phase 1)
    NoteSentimentMismatch                    // 3 — NEW
)

type Note struct {
    Kind            NoteKind // Type of note
    IssueURL        string   // URL of the GitHub issue
    SinceDays       int      // Number of days in the search window
    SuggestedStatus string   // NEW: AI-suggested status caption (e.g., "At Risk")
    Explanation     string   // NEW: AI explanation of the mismatch
}
```

Add rendering for the new note kind in `renderNoteBullet`:

```go
case NoteSentimentMismatch:
    return fmt.Sprintf("- %s: reported as %s, but sentiment suggests %s — %s",
        note.IssueURL, note.ReportedStatus, note.SuggestedStatus, note.Explanation)
```

Wait — the `Note` struct needs `ReportedStatus` too for the rendering:

```go
type Note struct {
    Kind            NoteKind
    IssueURL        string
    SinceDays       int
    ReportedStatus  string // NEW: The original reported status caption
    SuggestedStatus string // NEW: AI-suggested status caption
    Explanation     string // NEW: AI explanation of the mismatch
}
```

### 7. Add Sentiment Configuration

**File:** `internal/config/config.go`

Add a `Sentiment` field to the `Models` struct:

```go
Models struct {
    BaseURL      string
    Model        string
    Enabled      bool
    SystemPrompt string
    Sentiment    bool // NEW: true by default when AI enabled, false with --no-sentiment
}
```

**File:** `cmd/root.go` (or wherever flags are registered)

Add the `--no-sentiment` flag:

```go
generateCmd.Flags().BoolVar(&noSentiment, "no-sentiment", false, "Disable AI sentiment analysis")
```

**File:** `internal/config/config.go`

Update `FromEnvAndFlags` to accept and wire the new parameter:

```go
func FromEnvAndFlags(
    // ... existing params ...
    noSentiment bool,   // NEW
) (*Config, error) {
    // ...
    cfg.Models.Sentiment = cfg.Models.Enabled && !noSentiment
    // ...
}
```

### 8. Wire Sentiment into the Pipeline

**File:** `cmd/generate.go`

#### Phase A: Pass `ReportedStatus` into `IssueData`

Add a `ReportedStatusCaption` field to `IssueData`:

```go
type IssueData struct {
    // ... existing fields ...
    ReportedStatusCaption string // NEW: Original status caption for sentiment comparison
}
```

Set it in `collectIssueData` wherever status is derived:

```go
// In Case 2 (after MapTrending):
data.Status = derive.MapTrending(newestReport.TrendingRaw)
data.ReportedStatusCaption = data.Status.Caption  // NEW
```

For Case 1 fallback (Phase 1, status = `Unknown`):

```go
data.ReportedStatusCaption = derive.Unknown.Caption  // "Unknown"
```

#### Phase B: Build `BatchItem` with `ReportedStatus`

Update `batchSummarize` to populate the new field:

```go
func batchSummarize(ctx context.Context, summarizer ai.Summarizer, allData []IssueData, logger *slog.Logger) (map[string]ai.BatchResult, error) {
    var items []ai.BatchItem
    for _, d := range allData {
        if d.ShouldSummarize && len(d.UpdateTexts) > 0 {
            items = append(items, ai.BatchItem{
                IssueURL:       d.IssueURL,
                IssueTitle:     d.IssueTitle,
                UpdateTexts:    d.UpdateTexts,
                ReportedStatus: d.ReportedStatusCaption, // NEW
            })
        }
    }
    if len(items) == 0 {
        return nil, nil
    }
    return summarizer.SummarizeBatch(ctx, items)
}
```

Note: `batchSummarize` return type changes from `map[string]string` to `map[string]ai.BatchResult`.

#### Phase C: Extract Sentiment and Create Notes

Update result assembly to handle `BatchResult` and generate sentiment notes:

```go
// Phase C: Result Assembly
for _, data := range allData {
    var summary string
    var sentimentNote *format.Note

    if result, ok := summaries[data.IssueURL]; ok {
        summary = result.Summary

        // Check for sentiment mismatch (only if sentiment is enabled)
        if cfg.Models.Sentiment && result.Sentiment != nil {
            suggestedStatus, valid := derive.ParseStatusKey(result.Sentiment.SuggestedStatus)
            if valid && suggestedStatus != data.Status {
                sentimentNote = &format.Note{
                    Kind:            format.NoteSentimentMismatch,
                    IssueURL:        data.IssueURL,
                    ReportedStatus:  data.ReportedStatusCaption,
                    SuggestedStatus: suggestedStatus.Caption,
                    Explanation:     result.Sentiment.Explanation,
                }
            }
        }
    }
    if summary == "" {
        summary = data.FallbackSummary
    }

    result := createResultFromData(data, summary)
    // ... existing row/note collection ...

    // Add sentiment note if present
    if sentimentNote != nil {
        allNotes = append(allNotes, *sentimentNote)
    }
}
```

### 9. Handle Sentiment When AI is Disabled

When `cfg.Models.Sentiment` is false (either because `--no-sentiment` is passed or AI is entirely disabled), no changes are needed — the `NoopSummarizer` returns `nil` sentiments, and the Phase C check for `cfg.Models.Sentiment` short-circuits the note creation.

## File Changes Summary

| File | Change Type | Description |
|------|-------------|-------------|
| `internal/ai/summarizer.go` | Modify | Add `SentimentResult`, `BatchResult` types; add `ReportedStatus` to `BatchItem`; update `Summarizer` interface return type; update `NoopSummarizer` |
| `internal/ai/ghmodels.go` | Modify | Update `batchSystemPrompt`, `batchRequestItem`, `buildBatchPrompt`, `parseBatchResponse`, `parseMarkdownBatchResponse`; add nested response parsing with flat format fallback |
| `internal/derive/status.go` | Modify | Add `Key()` method and `ParseStatusKey()` function |
| `internal/format/notes.go` | Modify | Add `NoteSentimentMismatch` kind; add `ReportedStatus`, `SuggestedStatus`, `Explanation` fields to `Note` |
| `internal/config/config.go` | Modify | Add `Sentiment` field to `Models` struct |
| `cmd/root.go` | Modify | Add `--no-sentiment` flag |
| `cmd/generate.go` | Modify | Add `ReportedStatusCaption` to `IssueData`; update `batchSummarize` return type; add sentiment note creation in Phase C |

## CLI Usage Examples

```bash
# Sentiment analysis is on by default (when AI is enabled)
weekly-report-cli generate --project "org:my-org/5"
# Output notes section may include:
# ## Notes
# - https://github.com/org/repo/issues/42: reported as On Track, but sentiment suggests At Risk — Update mentions two unresolved blockers with no clear resolution timeline.

# Disable sentiment analysis
weekly-report-cli generate --project "org:my-org/5" --no-sentiment

# Sentiment is automatically disabled when AI is disabled
DISABLE_SUMMARY=1 weekly-report-cli generate --project "org:my-org/5"

# Combined with Phase 1 fallback — Unknown status gets a suggestion
# - https://github.com/org/repo/issues/99: reported as Unknown, but sentiment suggests On Track — Recent comment describes completed milestone and positive progress.
```

## Design Decisions

### Piggybacking on the Existing Batch Call

Rather than making a separate API call for sentiment, the same `SummarizeBatch` call returns both summary and sentiment. This:
- Avoids doubling API costs and latency
- Gives the AI full context (all updates) for both tasks
- Keeps the pipeline simple (one call, one response)

### Backward-Compatible Response Parsing

The biggest risk is the AI response format change. The parser tries formats in order:
1. **New nested JSON** (`{"url": {"summary": "...", "sentiment": {...}}}`)
2. **Old flat JSON** (`{"url": "summary text"}`) — sentiments are `nil`
3. **Markdown fallback** — sentiments are `nil`

This means a model that doesn't follow the new prompt still works — summaries are produced, sentiments are just absent.

### Sentiment as a Note, Not a Status Override

The AI-suggested status is informational, shown as a note. It does **not** override the reported status in the table row. This avoids:
- Surprising users with statuses they didn't report
- AI hallucinations directly affecting the report
- Needing a confidence threshold for status changes

Users can review the note and manually adjust their reports.

### On by Default

Sentiment analysis is on by default when AI is enabled because:
- It adds negligible cost (same API call)
- It provides value without user action
- `--no-sentiment` is available for users who find it noisy

### `ParseStatusKey` Uses Snake Case

The AI prompt specifies snake_case keys (`on_track`, `at_risk`) rather than display names ("On Track", "At Risk") because:
- Snake case is unambiguous in JSON
- No risk of case/spacing variations
- Clean separation between internal keys and display captions

## Testing Strategy

### Unit Tests

1. **`internal/ai/summarizer_test.go`**
   - `TestNoopSummarizer_SummarizeBatch_ReturnsBatchResult`: verify new return type
   - `TestBatchResult_NilSentiment`: noop returns nil sentiments

2. **`internal/ai/ghmodels_test.go`**
   - `TestParseBatchResponse_NestedFormat`: parse new JSON format with sentiment
   - `TestParseBatchResponse_NestedFormat_NullSentiment`: parse with `"sentiment": null`
   - `TestParseBatchResponse_FlatFormatFallback`: old format still works, sentiments nil
   - `TestParseBatchResponse_MarkdownFallback`: markdown format still works, sentiments nil
   - `TestParseBatchResponse_MixedSentiment`: some items have sentiment, others null
   - `TestBuildBatchPrompt_IncludesReportedStatus`: verify `reported_status` in request JSON
   - `TestSummarizeBatch_EndToEnd`: mock server returning nested format, verify full flow

3. **`internal/derive/status_test.go`**
   - `TestStatusKey`: all statuses produce correct snake_case keys
   - `TestParseStatusKey_ValidKeys`: all valid keys parse correctly
   - `TestParseStatusKey_InvalidKey`: unknown key returns `(Unknown, false)`
   - `TestParseStatusKey_CaseInsensitive`: `"ON_TRACK"` and `"on_track"` both work
   - `TestParseStatusKey_Whitespace`: `" at_risk "` trims and parses

4. **`internal/format/notes_test.go`**
   - `TestRenderNotes_SentimentMismatch`: verify note rendering with reported/suggested status and explanation
   - `TestRenderNotes_MultipleSentimentNotes`: multiple mismatches render correctly

5. **`cmd/generate_test.go`**
   - `TestPipelinePhaseC_SentimentMismatch`: mock BatchResult with sentiment, verify note created
   - `TestPipelinePhaseC_SentimentMatch`: sentiment suggests same status, no note
   - `TestPipelinePhaseC_SentimentDisabled`: `--no-sentiment` flag, no note even with mismatch
   - `TestPipelinePhaseC_NilSentiment`: BatchResult with nil sentiment, no note

### Table-Driven Test Structure

```go
func TestParseBatchResponse_Formats(t *testing.T) {
    tests := []struct {
        name             string
        response         string
        wantSummaries    map[string]string
        wantSentiments   map[string]*ai.SentimentResult
        wantErr          bool
    }{
        {
            name: "nested format with sentiment",
            response: `{
                "https://github.com/org/repo/issues/1": {
                    "summary": "Work continues...",
                    "sentiment": {"status": "at_risk", "explanation": "Mentions blockers"}
                }
            }`,
            wantSummaries:  map[string]string{"https://github.com/org/repo/issues/1": "Work continues..."},
            wantSentiments: map[string]*ai.SentimentResult{
                "https://github.com/org/repo/issues/1": {SuggestedStatus: "at_risk", Explanation: "Mentions blockers"},
            },
        },
        {
            name: "flat format fallback",
            response: `{"https://github.com/org/repo/issues/1": "Work continues..."}`,
            wantSummaries:  map[string]string{"https://github.com/org/repo/issues/1": "Work continues..."},
            wantSentiments: map[string]*ai.SentimentResult{"https://github.com/org/repo/issues/1": nil},
        },
        // ... more cases
    }
    // ...
}
```

## Future Considerations

1. **Confidence scoring**: Add a confidence field to `SentimentResult` so low-confidence mismatches can be suppressed.
2. **Historical trending**: Analyze the last N reports to detect status trajectory (improving, declining, stagnant) rather than single-point mismatch.
3. **Status override mode**: Optional flag to let AI-suggested status override the reported status in the table (for teams that want automated status correction).
4. **Custom sentiment rules**: Allow users to define organization-specific rules for what constitutes a mismatch (e.g., "any mention of 'blocked' with 'On Track' is always a mismatch").
5. **Sentiment in describe command**: Extend sentiment analysis to the `describe` command for assessing project health from issue bodies.

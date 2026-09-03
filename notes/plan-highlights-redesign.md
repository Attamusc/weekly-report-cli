# Highlights Redesign — Plan

Companion to: `notes/scout-highlights-pipeline.md`
Status: ready for execution.

## TL;DR

Replace the single fragile `HighlightsBatch` call with a deterministic mechanical rollup followed by a map-reduce AI layer over a pruned survivor set. Mechanical rollup is always computed and is the `--no-summary` output. AI default stays on, but each AI call now sees one item at a time so token pressure disappears. API volume drops because we only hydrate (fetch comment bodies for) items that actually go to the AI.

## Decisions

| # | Decision | Choice |
|---|----------|--------|
| Q1 | Mechanical rollup output shape | Grouped by author |
| Q2 | Score signals | Simple raw signals, normalized per-repo |
| Q3 | Mechanical→AI cut | `min(top_N=50, items where score ≥ median)` |
| Q4 | Default AI behavior | AI on by default (current preserved) |
| Q5 | AI layer shape | Map + reduce |
| Q6 | Summarizer interface | Split: `SummarizeHighlight` + `MergeThemes`; delete `HighlightsBatch` |
| Q7 | Chunker fate | Delete. Keep only a one-item `truncateHighlightItem` inside `SummarizeHighlight` |
| Q8 | Threshold value | `score ≥ median(score)` over the post-collection set |
| Q9 | Author attribution | Single primary: PR/issue author → closer → most recent commenter |
| Q10 | Mechanical columns | `#repo/num · title · state · score · labels` |
| Q11 | Score formula | `raw = comments + 5·closed_this_week + 2·is_pr`, then `score = raw / max(raw in same repo)` |
| Q12 | AI concurrency | New `--ai-concurrency` flag (default 5), separate from `--concurrency` |
| Q13 | Comment count source | Plumb `CommentCount` through `IssueRef` from search results |
| Q14 | Hydration scope | Only hydrate comment bodies for the AI survivor set |

### Commitment

User confirmed: keeping AI default-on means the redesign must actually fix fragility, not hide it. The map step (single item per request) is the structural fix — there is no path where the chunker has to do anything clever.

### Out of scope

- `generate` command — untouched.
- Discovery filters — already shipped, untouched.
- Larger model — explicitly rejected.

## New Pipeline

```
PHASE A  Discovery (unchanged shape)
         GitHub search → []IssueRef
         NEW: preserve CommentCount, State, IsPR, ClosedAt, AuthorLogin
              from each search result instead of discarding.

PHASE B  Mechanical rollup (NEW, no AI, no extra API calls)
         For each IssueRef:
            raw   = CommentCount + 5·closedThisWeek + 2·isPR
         For each repo, compute max(raw).
         score = raw / max(raw_in_repo)   // [0,1]
         Group by primary author (Q9 rule).
         → Rollup{ByAuthor map[string][]ScoredItem, AllSorted []ScoredItem}

PHASE C  Cut (NEW)
         median = median(score for all items)
         survivors = items where score ≥ median, capped at N=50, sorted desc.
         If --no-summary OR AI disabled: skip to PHASE F.

PHASE D  Hydration (NARROWED)
         Parallel collect comment bodies ONLY for survivors.
         Uses existing CollectHighlightData under --concurrency cap.
         Items below the cut never trigger API calls.

PHASE E  AI map-reduce (NEW shape)
         Map (parallel, --ai-concurrency cap):
           for each survivor: SummarizeHighlight(item) → Highlight{theme, summary}
           per-item input is one issue; cannot exceed token budget.
         Reduce (1 small call):
           MergeThemes([]Highlight) → []Highlight
           input is only {title, theme, summary} — no comment bodies.
           reduce is allowed to RENAME and MERGE Theme fields only.
           reduce must NOT touch Title, URL, or Summary.

PHASE F  Render
         If AI ran:    format.RenderHighlights(highlights)  // existing, grouped by theme
         If mechanical only: format.RenderRollup(rollup)    // NEW, grouped by author
```

## Types & Interface Changes

### `internal/input/links.go`

Add fields to `IssueRef`. They're populated from search and from the mechanical rollup pre-pass; safe defaults (zero values) when input comes from project board or URL list.

```go
type IssueRef struct {
    Owner       string
    Repo        string
    Number      int
    URL         string
    Assignees   []string
    FieldValues map[string]string

    // NEW — populated from discovery search; zero when input comes from other sources
    Title         string
    State         string    // "open" | "closed"
    IsPR          bool
    AuthorLogin   string
    CommentCount  int
    UpdatedAt     time.Time
    ClosedAt      *time.Time
}
```

### NEW package: `internal/rollup`

```go
type ScoredItem struct {
    Ref       input.IssueRef
    Raw       int
    Score     float64    // normalized [0,1]
    Author    string     // primary attribution
    Labels    []string   // optional, may be empty until hydration
}

type Rollup struct {
    ByAuthor   map[string][]ScoredItem   // sorted desc by score within each
    AllSorted  []ScoredItem              // sorted desc by score globally
    Median     float64
}

// Compute ranks refs into a Rollup. since is the start of the lookback window
// (used to compute closedThisWeek). Does not fetch anything.
func Compute(refs []input.IssueRef, since time.Time) Rollup

// Cut returns survivors = min(topN, items with score >= rollup.Median),
// preserving global score-desc order.
func (r Rollup) Cut(topN int) []ScoredItem
```

### `internal/ai/summarizer.go`

```go
type Summarizer interface {
    SummarizeBatch(ctx, []BatchItem) (map[string]BatchResult, error)  // unchanged
    DescribeBatch(...)                                                 // unchanged

    // NEW
    SummarizeHighlight(ctx context.Context, item HighlightItem) (Highlight, error)
    MergeThemes(ctx context.Context, in []Highlight) ([]Highlight, error)

    // REMOVED: HighlightsBatch
}
```

`Highlight` type unchanged.

### `internal/format`

Add `RenderRollup(r rollup.Rollup) string` — markdown, grouped by author (alphabetical), each section a table with `#repo/num · title · state · score · labels` columns.

`RenderHighlights` unchanged.

## Files Touched (rough map)

| File | Change |
|------|--------|
| `internal/input/links.go` | Add fields to `IssueRef` |
| `internal/discovery/search.go` | Preserve search metadata into `IssueRef` |
| `internal/rollup/rollup.go` (new) | Scoring, normalization, cut |
| `internal/rollup/rollup_test.go` (new) | Table-driven score/cut tests |
| `internal/ai/summarizer.go` | Interface change; `NoopSummarizer` updated |
| `internal/ai/ghmodels.go` | Add `SummarizeHighlight`, `MergeThemes`; **delete** `HighlightsBatch`, `singleHighlightsBatch`, `chunkedHighlightsBatch`, `packHighlightChunks`, `estimateHighlightItemSize`, `buildHighlightsPrompt`, `parseHighlightsResponse`. Keep `truncateHighlightItem` as a one-item safety cap inside `SummarizeHighlight`. |
| `internal/ai/ghmodels_test.go` | Replace highlights chunker tests with single-item + reduce tests |
| `internal/format/rollup.go` (new) | `RenderRollup` |
| `internal/format/rollup_test.go` (new) | Render tests |
| `cmd/highlights.go` | Rewire pipeline; add `--ai-top`, `--ai-concurrency` flags |

## Phased Rollout

Each phase is independently shippable.

1. **Plumb signals.** Expand `IssueRef`, populate from discovery, leave the rest of the pipeline alone. Mergeable on its own; no behavior change yet.
2. **Mechanical rollup + renderer.** New `rollup` package, new `format.RenderRollup`. Wire `--no-summary` to use the new renderer. Old AI path still runs unchanged in the default case.
3. **Narrow hydration.** Move `CollectHighlightData` to only run on `rollup.Cut` survivors. Old `HighlightsBatch` still consumes them. Token pressure already reduced because <=50 items.
4. **Interface split + map-reduce.** Replace `HighlightsBatch` with `SummarizeHighlight` + `MergeThemes`. Delete chunker code. This is the irreversible step.
5. **Polish.** Flag naming, docs, integration test for `cmd/highlights.go`.

Phases 1–3 can land as one PR or three; phase 4 is its own PR.

## Premortem

Most likely failure modes and the mitigations baked into the plan.

1. **Stale `comment_count` from search.** GitHub search lags reality by minutes. Acceptable: this is a ranking heuristic, not authoritative. Cut is `min(50, ≥median)` — small ranking errors don't drop deserving items off a cliff. No mitigation needed beyond accepting it.

2. **Per-repo normalization inflates singletons.** A repo with one item gets `score=1.0`, beating items from a busy repo. Mitigation: if a repo has `<3` items in the dataset, normalize against the global max instead of the per-repo max. Encoded as a constant in the rollup package, documented in the test file.

3. **Primary-author attribution requires data we may not have.** "Closer" and "most recent commenter" come from collection. For the rollup pass we only have search-author. Mitigation: attribution uses the search-author exclusively. (The downgraded rule still satisfies Q9=b: deterministic, single primary, easy to explain.) Documented as a known limitation.

4. **Map step request volume.** 50 small calls vs. today's 1–25 chunked calls. Net cost is similar or lower because each call is short, but rate-limit thrashing is a risk. Mitigations: separate `--ai-concurrency` knob (default 5), reuse existing `internal/retry` package, log per-call latency under verbose.

5. **Reduce step rewrites things it shouldn't.** Risk: AI rewrites `Title` or `Summary` during theme merging. Mitigation: reduce prompt is constrained to return `[{url, theme}]` only — we look up the original `Highlight` by URL and only overwrite the `Theme` field locally. The prompt cannot accidentally corrupt summaries because we never read them back.

6. **Visible cost increase on tiny runs.** A 10-item run now makes 10 AI calls instead of 1. Mitigation: short-circuit — if `len(survivors) ≤ smallBatchThreshold` (say 5), still run map (it's only 5 calls, fully parallel, ~1s wall clock). Not worth adding a special-case batched path; rejected.

7. **Test surface churn.** Deleting `HighlightsBatch` invalidates several `ai/` tests. Acceptable cost; new tests target a smaller, simpler surface.

8. **Backwards compat (per AGENTS.md Think Forward).** Visible UX changes the user accepted:
   - `--no-summary` output format changes (no longer label-buckets via Noop AI; now a real author-grouped rollup).
   - New flags: `--ai-top`, `--ai-concurrency`.
   - The themed AI output format is unchanged.

## Open Items Worker May Hit

- Whether `Highlight.Summary` should fall back to the issue title when AI fails on a single item (`SummarizeHighlight` returns error → use the item as-is with a label-derived theme). Plan says yes — partial results beat all-or-nothing failure. Encoded in todo for map orchestration.
- Output format of `RenderRollup` tables — markdown table vs. bullet list. Plan says markdown table (more scannable when each author has 10+ items).

## Observed after rollout

Smoke check date: 2026-06-01
Command: `weekly-report-cli highlights --users Attamusc --since-days 7 --ai-top 5 --ai-concurrency 5 --verbose`
Build: `bb104a8` (post-rewire + docs)

### Mechanical-only path (`--no-summary`)

- Discovered refs (post-filter): 33 (45 raw → 12 dropped by bot filter)
- Rollup: median 0.286 across 26 authors
- Hydration API calls: 0 ✅ (skipped entirely)
- AI calls: 0 ✅
- Wall clock: ~3s (search-dominated)
- Output: author-grouped markdown tables, format matches spec

### AI path (`--ai-top 5`)

- Discovered refs (post-filter): 33
- Survivors after cut: 5 ✅ (capped by `--ai-top`, not by median)
- Hydration API calls: 5 ✅ (matches survivors, not 33 — narrow hydration confirmed)
- AI map calls: 5 (1 fell back to local highlight, 4 returned content)
- AI reduce calls: 1 ✅
- Total AI calls: 6 (vs. theoretical 1 batched call pre-redesign — premortem #4 cost increase as expected)
- Wall clock: ~17s end-to-end (~8s map under retries, ~6s reduce)
- Rate-limit retries: 8 across 5 map calls. All succeeded eventually via existing retry plumbing.
- Output: themed markdown (Infrastructure / Security / Support & Reliability), grouped per spec

### Issues observed (worth follow-up todos, not blockers)

1. **Map prompt occasionally returns wrong JSON shape.** 1-of-5 map calls returned a JSON array when the parser expected a single object → `failed to parse highlight response: json: cannot unmarshal array into Go value of type ai.highlightSingleResponse`. Per-item fallback kicked in correctly (premortem partial-failure tolerance works). Fix: tighten `SummarizeHighlight` system prompt to explicitly require a single object, OR make the parser accept both shapes.
2. **Several "successful" map responses look degraded.** Some output lines show summary == title (e.g. `[Epic] Integrate Herb into GitHub.com — [Epic] Integrate Herb into GitHub.com`). Indicates either a second silent fallback path or low-quality content from the model. Worth a quality probe.
3. **Default `--ai-concurrency=5` saturates GitHub Models endpoint immediately.** 8 rate-limit hits across 5 calls. Not a correctness issue (retries handle it cleanly) but consider lowering default to 3 or adding token-bucket pacing if this is observed in production runs.
4. **Labels column always empty in mechanical rollup.** Discovery search results don't populate `Labels` on `IssueRef` — only AI-path hydration fetches labels. The renderer correctly shows blank. Two options: (a) add labels to search-result preservation in `internal/discovery/search.go`, or (b) document this as expected (labels appear only in the AI-themed output, not the mechanical rollup).

### Conclusion

The core thesis is confirmed: token pressure is structurally removed (one item per map call), hydration is narrowed (5 of 33), and partial failures degrade gracefully instead of taking down the whole pipeline. The new architecture is shippable. Issues 1–4 above are quality polish, not architectural problems.

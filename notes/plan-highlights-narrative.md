# Highlights Narrative Redesign — Plan

Companion to: `notes/plan-highlights-redesign.md` (the previous map-reduce redesign), `notes/research-copilot-integration.md` (model + endpoint findings).

Status: ready for execution. User has signed off on Tier 2 data + narrative product.

## Background

The previous redesign (map-reduce: per-item `SummarizeHighlight` + `MergeThemes`) shipped clean and structurally sound. But the product it produces — a list of per-item one-line paraphrases grouped into coarse themes — has no real value over searching GitHub manually. Direct user quote: *"these are extremely low quality and provide no real value over just searching manually myself."*

The structural cause: per-item map calls cannot do cross-item synthesis. The reduce step was deliberately neutered (premortem #5) to "rename themes only." The architecture optimized for robustness at the cost of depth.

We're partially backing out the map-reduce shape and replacing it with a single narrative-writing AI call over a richer input set.

## Decisions

| # | Decision | Choice |
|---|----------|--------|
| Q1 | Product shape | Narrative report + full mechanical rollup beneath (nothing hidden) |
| Q2 | AI call topology | Single batched narrative call (not per-item, not two-pass) |
| Q3 | Map step fate | Delete `SummarizeHighlight` + `MergeThemes` + `aiMapHighlights` + skip-empty filter |
| Q4 | Input data tier | Tier 2: attributed comments + issue body + ClosedAt + timeline events (1 extra API call per survivor) |
| Q5 | Hydration scope | Unchanged — only survivors after rollup cut get hydrated (now with timeline) |
| Q6 | Narrative input includes comment snippets? | Yes — attributed, ~300 chars each |
| Q7 | Output structure | Narrative section (themed prose + inline citations) + "## All activity" section using existing `RenderRollup` |
| Q8 | What happens when AI fails | Render rollup only (narrative omitted). Same failure mode as `--no-summary`. |
| Q9 | --no-summary fate | Unchanged — renders rollup only, no AI call |
| Q10 | Default `--ai-top` | Drop from 50 → unlimited within mechanical rollup, with practical bound via `--ai-top` (default 100 to capture full week of activity for narrative input; the narrative output is curated by the AI, not by us) |

### Commitment

User explicitly requested:
1. Narrative report shape, NOT digest.
2. Both major-initiative summary AND day-to-day work context.
3. Nothing hidden — the routine stuff still surfaces somewhere.
4. Title-echo paraphrases are unacceptable; better to skip than to paraphrase.

### Out of scope

- Discovery filters — unchanged.
- Project board / URL list input paths — unchanged.
- `generate` command — completely unchanged.
- Mechanical rollup score formula — unchanged.

## New Pipeline

```
PHASE A  Discovery (unchanged)
         GitHub search → []IssueRef (with title, state, isPR, author,
         labels, commentCount, updatedAt, closedAt populated by search)

PHASE B  Mechanical rollup (unchanged)
         rollup.Compute(refs, since) → Rollup{ByAuthor, AllSorted, Median}

PHASE C  Cut for AI input
         survivors := r.Cut(aiTop)   // default 100; bound for narrative input
         If --no-summary OR AI disabled: skip to Render with rollup only.

PHASE D  Tier-2 hydration (NEW richer collection)
         For each survivor in parallel (--concurrency bound):
           Fetch issue metadata (title, body, state, labels, assignees,
                                 closedAt, openedAt, openedBy)
           Fetch comments since lookback (with author + timestamp)
           Fetch timeline events since lookback (filtered to high-signal types)
         → []NarrativeItem  (one per survivor, fully populated)

PHASE E  Narrative AI call (NEW — replaces map-reduce entirely)
         Single call: summarizer.WriteNarrative(ctx, items, rollup) → Narrative
         Input: JSON array of NarrativeItem (compact, ~5-15K tokens for 100 items)
         Output: structured Narrative{Sections: [{Heading, Body (markdown with
                 inline [text](url) citations)}]}
         On error: log warn, return empty Narrative — render falls back gracefully.

PHASE F  Render
         If AI ran AND narrative non-empty:
           format.RenderNarrativeReport(narrative, rollup) → stdout
           // narrative themed sections + "## All activity" with RenderRollup
         Else (AI off / failed / --no-summary):
           format.RenderRollup(rollup) → stdout                  // unchanged
```

## Types & Interface Changes

### `internal/pipeline/types.go` — replace `HighlightData`

```go
// NarrativeItem is the rich per-survivor data shape consumed by the narrative
// AI call. Replaces HighlightData, which carried only unattributed comment
// bodies.
type NarrativeItem struct {
    URL            string
    Title          string
    Body           string         // truncated to ~500 chars
    State          string         // "open" | "closed"
    IsPR           bool
    Author         string         // issue/PR opener
    Assignees      []string
    Labels         []string
    OpenedAt       time.Time
    ClosedAt       *time.Time
    MergedAt       *time.Time     // PRs only
    ClosedThisWeek bool
    MergedThisWeek bool

    RecentComments []NarrativeComment   // since lookback, chronological
    Events         []NarrativeEvent     // since lookback, chronological, filtered
}

type NarrativeComment struct {
    Author    string
    CreatedAt time.Time
    Body      string    // truncated to ~300 chars
}

type NarrativeEvent struct {
    Type   string       // "closed" | "merged" | "review_requested" | "cross-referenced" | ...
    Actor  string       // username who performed the event
    At     time.Time
    Detail string       // event-specific text (e.g. "merged into main", "as duplicate of #123")
}
```

### Timeline event filter — high-signal only

Include:
- `closed` (with close-reason where available)
- `merged`
- `reopened`
- `review_requested`
- `review_dismissed`
- `pull_request_review` (approved / changes_requested / commented)
- `referenced` (cross-ref FROM another PR/issue)
- `cross-referenced` (mentioned IN another PR/issue)
- `assigned` (only if assignee != opener)
- `marked_as_draft`
- `ready_for_review`

Skip (noise):
- `labeled` / `unlabeled` (we already have current labels)
- `milestoned` / `demilestoned`
- `subscribed` / `unsubscribed` / `mentioned`
- `renamed`, `locked`, `unlocked`, `pinned`, `unpinned`

### `internal/ai/summarizer.go`

> **Architectural note** (added 2026-06-02 during Phase 2 execution): the shared narrative types live in a new leaf package `internal/narrative` (not `internal/pipeline`) to avoid an import cycle — `ai` needs the types, `pipeline` already imports `ai`, so the types had to live somewhere both could depend on. `pipeline.NarrativeItem` and `narrative.Item` are currently distinct types with a conversion adapter in `cmd/highlights.go`; Phase 4 cleanup should unify them by either (a) having `pipeline.CollectNarrativeItem` return `narrative.Item` directly, or (b) making `pipeline.NarrativeItem` a type alias for `narrative.Item`.

```go
type Summarizer interface {
    SummarizeBatch(...)          // unchanged
    DescribeBatch(...)           // unchanged

    // NEW
    WriteNarrative(ctx context.Context, items []pipeline.NarrativeItem, rollup rollup.Rollup) (Narrative, error)

    // REMOVED
    // SummarizeHighlight
    // MergeThemes
}

type Narrative struct {
    Sections []NarrativeSection
}

type NarrativeSection struct {
    Heading string    // e.g. "Azure migration progresses"
    Body    string    // markdown prose, may contain [text](url) inline citations
}
```

### `internal/format`

New: `RenderNarrativeReport(n ai.Narrative, r rollup.Rollup) string`. Outputs:

```markdown
# Weekly Highlights — <date range from rollup>

## Narrative

### <Section 1 heading>
<section 1 body>

### <Section 2 heading>
<section 2 body>

...

## All activity (<N> items)

<RenderRollup output verbatim>
```

`RenderHighlights` and `RenderRollup` themselves are **untouched**.

## Files Touched (rough map)

| File | Change |
|------|--------|
| `internal/narrative/types.go` (NEW, added in Phase 2) | Shared `Item`, `Comment`, `Event` types — leaf package, no internal deps. Phase 4 should reconcile with `pipeline.NarrativeItem`. |
| `internal/pipeline/types.go` | New `NarrativeItem`, `NarrativeComment`, `NarrativeEvent`. Delete or deprecate `HighlightData`. |
| `internal/pipeline/highlights.go` | Rewrite `CollectHighlightData` → `CollectNarrativeItem`. Use new fetcher methods. |
| `internal/github/issues.go` | Update `Comment` mapping to preserve author/timestamp through to caller. Add `FetchTimelineSince(ctx, ref, since) → []TimelineEvent` with the event-type filter. Add `FetchPullRequest(ctx, ref) → PRData` for merged_at on PRs. |
| `internal/github/issues_test.go` | Tests for new fetcher methods. |
| `internal/pipeline/generate.go` | Extend `IssueFetcher` interface with new methods. Update mocks where needed. |
| `internal/ai/summarizer.go` | Replace `SummarizeHighlight` + `MergeThemes` with `WriteNarrative`. Add `Narrative`, `NarrativeSection` types. Update `NoopSummarizer`. |
| `internal/ai/ghmodels.go` | Implement `WriteNarrative`. Delete `SummarizeHighlight`, `MergeThemes`, `parseSingleHighlightResponse`, `singleHighlightSystemPrompt`, `truncateHighlightItem`, `labelTheme` (if unused). |
| `internal/ai/ghmodels_test.go` | Delete old highlight tests. New: `WriteNarrative` happy-path, empty-input, AI-error fallback. |
| `internal/format/narrative.go` (new) | `RenderNarrativeReport`. |
| `internal/format/narrative_test.go` (new) | Tests. |
| `cmd/highlights.go` | Rewire phases D–F per the new pipeline. Delete `aiMapHighlights`, `fallbackHighlight` (no longer needed — narrative-or-rollup is the failure mode), `dataByURL` glue. Update flags: `--ai-top` default 100. `--ai-concurrency` becomes hydration concurrency only (no AI parallelism anymore). |
| `cmd/highlights_test.go` | Replace AI-path tests with new narrative path. |
| `README.md` | Update the highlights section: new product description, new sample output, new flag semantics. |

## Phased Rollout

Same pattern as the previous redesign. Each phase independently shippable; `make check` green at every commit.

1. **Tier-2 data layer.** Add fetcher methods (timeline, PR), new types, rewrite `CollectNarrativeItem`. cmd/highlights.go gets a TEMPORARY stub that adapts the new type back to the old AI call (so make check passes). One commit.
2. **AI layer replacement.** Delete old `SummarizeHighlight`/`MergeThemes`. Add `WriteNarrative`. cmd/highlights.go stub updated to wire the new AI method, but still uses old `RenderHighlights` (which will look weird but compiles). One commit.
3. **Renderer.** Add `RenderNarrativeReport`. cmd/highlights.go wired to use it. End-to-end works. One commit.
4. **Cleanup + docs.** Delete dead code (`aiMapHighlights`, `fallbackHighlight`, `labelTheme` if dead, etc.). Update README. One commit.
5. **Smoke check.** Orchestrator runs against real data, captures observations, appends to this file under "Observed after rollout".

## Premortem

### 1. The AI ignores or hallucinates citations
The prompt requires `[text](url)` inline citations to specific items. The model might invent URLs, miss items, or get URLs wrong.

**Mitigation**: post-process the narrative output. Extract all `[text](url)` references with a regex, verify each URL appears in the input `[]NarrativeItem`. Log warn for any unknown URL; do NOT strip it (the surrounding prose is still readable, and the rollup section below shows the real URL anyway). Don't try to "fix" the model — just observe.

### 2. Token budget blowout on busy weeks
100 NarrativeItems × ~1KB each = ~100KB ≈ ~33K tokens. Well within gpt-4o-mini's 128K, but if comment bodies aren't aggressively truncated we could spike.

**Mitigation**: hard truncate. Body 500 chars, comment 300 chars each, max 5 comments per item, max 10 events per item. These caps are constants in `internal/pipeline/highlights.go`. Document in the file.

### 3. Single AI call = all-or-nothing failure
This is the deliberate trade. We accept it because: (a) when narrative fails, the rollup still renders — user gets the raw signal. (b) retry plumbing already exists. (c) one call is structurally simpler than 50.

**Mitigation**: log AI call failures at WARN level with the underlying error. Render the rollup-only path. Document in README that narrative is best-effort.

### 4. Events from timeline include noise we forgot to filter
Some event types are deprecated or rare; some are useful only for PRs. The whitelist might miss things or include too much.

**Mitigation**: structured filter in `internal/github/issues.go`. Easy to add/remove event types as we learn. Add a verbose-only log of dropped event types so we can audit in the field.

### 5. Doubling hydration API cost
Each survivor now triggers issue-fetch + comments-fetch + timeline-fetch (+ PR-fetch for PRs). 3-4× the original 1 call. At `--ai-top 100`, that's 300-400 API calls.

**Mitigation**: parallel hydration already exists (`--concurrency` flag). GitHub API limit for authenticated users is 5,000/hour. 400 calls is 8% of an hour's budget — well within bounds. Watch for rate-limit logs.

### 6. Default `--ai-top` jumps from 50 to 100
We need MORE input to write a narrative that covers day-to-day work — the previous 50 was cutting context the narrative needs.

**Mitigation**: monitor smoke runs. If 100 produces too much noise, dial back to 75. The cap exists; it's a knob.

### 7. The narrative still echoes titles
The prompt explicitly forbids generic phrases and requires specificity. But if the input is thin (item has no body, no comments, no events), even a good model can't conjure detail.

**Mitigation**: items with effectively zero context (empty body, no recent comments, no events) should be excluded from the AI prompt input entirely — they appear in the rollup section only. This is the same principle as the (now-deleted) skip-AI-for-empty-context filter, just at a different stage. Implement in `cmd/highlights.go` before the WriteNarrative call.

### 8. We've now redesigned highlights twice in one session
Real concern: are we converging or thrashing? Honest answer: the first redesign solved structural fragility (real problem, real fix). The second redesign solves product utility (different real problem, different real fix). The data-collection and rollup work from both redesigns is reused entirely. We're not undoing work, we're swapping the AI layer.

## Open Items Worker May Hit

- Whether to derive `MergedAt` for PRs from the `merged` event in timeline OR from a separate `FetchPullRequest` call. Timeline is sufficient if we keep that event type; PR-object call is more authoritative. Plan recommends: use timeline for now, add PR-object call only if `merged_at` consistently missing.
- Whether the narrative prompt should receive the rollup data (scores, all-items summary) or just the survivor items. Plan recommends: prompt receives only survivor items, but the rollup is appended to output below the narrative. Keep the AI's job tightly scoped.
- Whether to truncate item Body before plumbing through types or only at JSON-serialization time for the AI prompt. Plan recommends: store full body in NarrativeItem (for future renderer use), truncate at AI prompt construction.

## Observed after rollout

*Phase 4 (cleanup + smoke check) run on 2026-06-02.*

### Run parameters
- `./weekly-report-cli highlights --users Attamusc --since-days 7 --verbose`
- Model: `gpt-4o-mini`
- Wall clock: **~24 seconds**

### Discovery + Rollup
- Discovered: **45 items** (33 after bot/authorship filter)
- Rollup: 33 items, median score 0.286, 26 authors
- Cut: **19 survivors** (ai_top=100; well under the cap for a solo user)

### Tier-2 Hydration
- Survivors: **19 items**
- API calls: ~4 × 19 = ~76 calls (issue metadata + comments + timeline + PR for PRs)
- No 429 rate-limit events observed
- Duration: approximately 12 seconds (concurrent at `--concurrency 5`)

### AI Narrative Call
- Items sent: **19**
- Wall clock: **~9.5 seconds** (gpt-4o-mini single call)
- Unknown citations: **0** (all inline links resolved to provided URLs)

### Narrative sections produced

```
### Progress on Azure Migration Initiative
The ongoing initiative to migrate significant traffic to Azure is central to our
operational strategy. This week, @meggiebot reported an update on the progress,
where we aim to achieve over 50% of monolith traffic routed through Azure by Q4
FY26...

### Enhancements to MySQL Management
Work in the MySQL management domain is progressing with several significant
initiatives. Notably, @jpl-coconut is spearheading the project to implement
MySQL load shedding via client proxy, an essential feature aimed at preventing
MySQL overload during peak usage...

### Security and Bug Bounty Reports
The team's focus on security remains vigilant, as demonstrated by the handling
of multiple bug bounty reports. In particular, the findings related to [symbol
and text injection](https://github.com/github/lifecycle/issues/1319) and
activity feed vulnerabilities are currently being triaged...
```

5 sections total: Azure migration, MySQL management, Security, API performance, Offsite planning.

### Honest Assessment vs Previous Production Output

**What's markedly better:**
1. **No title-echoes.** The previous production sample had pure paraphrases like "Addresses potential security vulnerability through symbol and text injection" that added zero value over reading the title. The new narrative actually synthesizes across items.
2. **Cross-item grouping.** Multiple Azure migration issues are grouped coherently with context that spans them, rather than one line per item.
3. **Inline citations with real links.** Citations resolve to actual issue URLs. Unknown citation count = 0.
4. **Narrative with actual detail.** "MySQL load shedding via client proxy, target completion end of June" is real actionable context, not title paraphrase.

**What could still improve:**
1. **Bot accounts treated as people.** "@meggiebot reported" — meggiebot is a GitHub bot, not a person. The narrative would read better if bot-authored comments were attributed to the issue/PR rather than the bot handle. → filed as follow-up TODO.
2. **Generic phrases still present.** Phrases like "This is a crucial part of moving towards a seamless integration" and "This continued emphasis on security is critical" are boilerplate that the prompt explicitly forbids. They're not as bad as title echoes, but there's room to tighten the prompt. → filed as follow-up TODO.
3. **Author attribution in narrative doesn't match rollup.** The narrative mentions @meggiebot as the actor, but the rollup correctly shows these items under @meggiebot. For a solo-user run (Attamusc), the narrative spends no time on the actual user's items — this is likely because Attamusc is only a bystander on most items, not the author. Fine for team-wide runs, slightly odd for personal runs.
4. **Some thin-context items still included.** A few items with no body and no comments appear in the narrative by title reference only. Pre-filtering thin items before the AI call (as planned in premortem #7) would improve output density.

**Overall verdict:** The redesign delivered on the primary goal. The output is now a readable narrative that covers the week's themes with cross-item synthesis and real citations. It is unambiguously more useful than the previous map-reduce output. Follow-up quality work filed as `highlights-narrative-followup` todos.

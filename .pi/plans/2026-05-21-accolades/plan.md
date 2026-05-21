# Highlights Command

**Date:** 2026-05-21
**Status:** Draft
**Directory:** /Users/attamusc/projects/github.com/Attamusc/weekly-report-cli

## Intent
Add a `highlights` command that surfaces notable smaller work items (bug fixes, support resolutions, repair work, meaningful discussions) that don't appear on the main project board. This complements the existing `generate` command — `generate` catches the big tent-pole items, `highlights` catches everything else worth mentioning.

## User Story
As an engineering manager, I want to see a curated summary of notable smaller work from the past week, so that I can recognize and communicate progress that doesn't show up on the big project board.

## Behavior

### Happy Path
1. User runs `weekly-report-cli highlights --users "user1,user2,user3" --since-days 7`
2. CLI uses GitHub Search API to discover issues and PRs touched by those users in the time window
3. Results are deduplicated and mechanically pre-filtered (bots, trivial actions removed)
4. Full issue/PR data is fetched in parallel (reusing existing bounded worker pool pattern)
5. AI curates the notable items, assigns themes, and writes short highlight summaries
6. Output is rendered as themed markdown sections with bullet points to stdout
7. Progress and diagnostics go to stderr

### Edge Cases & Error Handling
- All users have zero activity: print "No notable activity found" to stderr, exit 0
- Search returns 1000+ results: log warning to stderr ("results may be incomplete"), proceed with what we have
- Rate limit hit: existing retry transport handles 403/429 with backoff; add small sleep between search API pages
- AI returns zero highlights: print "No notable highlights found" to stderr, exit 0
- Private repos: included automatically if token has access, no special handling

## Scope

### In Scope
- New `highlights` Cobra subcommand
- Search-based discovery of user activity (issues, PRs, reviews, comments)
- Mechanical pre-filtering (bot accounts, trivial activity)
- New AI prompt for curation and theme-grouping
- New markdown output format (themed sections with bullet points)
- `--users` flag (comma-separated GitHub usernames)
- `--since-days` flag (default 7)
- `--summary-prompt` flag for custom AI prompt override
- NoopSummarizer fallback (list all items without AI curation)
- Smoke tests for discovery, formatting, and command wiring

### Out of Scope
- Per-person grouping in output
- Dedup against project board items
- Persistent state or running tallies
- AI pre-filter to reduce hydration calls (noted as follow-up)
- Org-wide discovery without user list (future: GitHub org members API)

## Effort & Quality
- **Level:** MVP
- **Tests:** Smoke (key functions — search query building, dedup, bot filtering, format rendering)
- **Docs:** Inline + CLI help text

## Constraints
- GitHub Search API: 30 requests/min rate limit, 1000 results per query
- GitHub REST API: 5000 requests/hr rate limit
- Must follow existing codebase conventions (import groups, error wrapping, slog logging, stderr for diagnostics)
- No circular imports — new `discovery` package must not import `input` or `projects`

## Ideal State Criteria

### Core Functionality
- [ ] ISC-1: `weekly-report-cli highlights` command exists and accepts `--users` flag with comma-separated GitHub usernames
- [ ] ISC-2: Command accepts `--since-days` flag defaulting to 7
- [ ] ISC-3: Command discovers repos each user has activity in within the time window via Search API
- [ ] ISC-4: Command collects closed issues, merged PRs, PR reviews, and comments for those users
- [ ] ISC-5: Bot accounts and trivial activity are filtered out before AI processing
- [ ] ISC-6: AI groups activity into themes (e.g., Bug Fixes, Support, Infrastructure) and selects notable items
- [ ] ISC-7: Output is markdown with per-theme sections containing bullet-point highlights
- [ ] ISC-8: Output goes to stdout; progress/diagnostics go to stderr

### Edge Cases
- [ ] ISC-9: Command handles users with zero activity gracefully (no crash, informative message)
- [ ] ISC-10: Command respects GitHub API rate limits and reports when limits are hit

### Anti-Criteria
- [ ] ISC-A-1: No per-person grouping in the output
- [ ] ISC-A-2: No dedup against project board items
- [ ] ISC-A-3: No persistent state or running tallies between invocations

## Approach
Use GitHub Search API to discover the universe of items, then hydrate with REST API for full context, then AI curates and themes the highlights. This reuses the existing parallel fetch pipeline pattern and keeps API calls well within rate limits.

### Key Decisions
- **Search API for discovery** — fewest API calls, cross-repo by default, scales to 40 users
- **Hydrate all discovered items** — send full context to AI rather than pre-filtering (follow-up: AI pre-filter when volume becomes a pain point)
- **New `discovery` package** — keeps search logic separate from existing `input` package (different concern)
- **New `HighlightsBatch` method on Summarizer** — extends existing interface rather than creating a parallel AI system
- **No project board dedup** — rely on AI prompt to naturally focus on smaller/notable work
- **Themed output** — AI assigns themes, formatter groups by theme with bullet points

### Architecture

```
cmd/highlights.go          — Cobra command, flags, 4-phase orchestration
internal/discovery/         — New package: search API queries, dedup, bot filtering
  search.go                — Search(ctx, client, users, since) → []IssueRef
  filter.go                — Bot detection, trivial activity filtering
internal/pipeline/          — Reuse CollectIssueData for parallel hydration
internal/ai/summarizer.go   — Add HighlightsBatch to Summarizer interface
internal/ai/ghmodels.go     — Implement HighlightsBatch with highlights system prompt
internal/ai/summarizer.go   — NoopSummarizer fallback for HighlightsBatch
internal/format/highlights.go — RenderHighlights: themed markdown sections
internal/config/config.go    — Minor: users list, highlights defaults
```

### Data Flow

```
cmd/highlights.go
  │
  ├─ Parse --users "user1,user2,..." and --since-days 7
  │
  ├─ PHASE A: discovery.Search(ctx, client, users, since)
  │    ├─ Issue queries: "author:{users} OR commenter:{users} is:issue updated:>=date"
  │    ├─ PR queries: "author:{users} OR reviewed-by:{users} is:pr updated:>=date"
  │    ├─ Paginate, collect all results
  │    ├─ Deduplicate by URL
  │    ├─ Filter out bots
  │    └─ Return []IssueRef (owner, repo, number)
  │
  ├─ PHASE B: pipeline.CollectIssueData(ctx, client, refs, concurrency, since)
  │    └─ Existing parallel fetch — issue details + comments since date
  │
  ├─ PHASE C: summarizer.HighlightsBatch(ctx, items)
  │    ├─ Chunk items (reuse existing batch size logic)
  │    ├─ System prompt: curate notable items, assign themes, write 1-line summaries
  │    └─ Returns []Highlight{Theme, Title, URL, Summary}
  │
  └─ PHASE D: format.RenderHighlights(highlights)
       ├─ Group by theme
       ├─ Sort themes alphabetically
       └─ Render markdown to stdout
```

## Dependencies
- No new external libraries — reuses go-github, cobra, oauth2
- GitHub Search API (authenticated, 30 req/min)
- GitHub REST API (existing client)
- GitHub Models API (existing AI client)

## Risks & Open Questions
- **Search API OR-query batching** — assumption that multiple users can be OR'd in one query. If not, fall back to one query per user (~80 queries, still under rate limits). Accepted — easy fallback.
- **AI prompt quality** — highlights may be generic initially. Mitigated by making prompt tweakable via `--summary-prompt` flag. Will need iteration.
- **Search rate limit (30/min)** — manageable with batching and sleep between pages. Worst case run takes 2-3 minutes. Accepted.
- **1000 result cap per search query** — for 40 users in a week, likely under cap. Log warning if hit. Accepted for MVP.

## Follow-Up Ideas
- AI pre-filter step: before hydration, run a cheap AI triage pass on search result titles/labels to pick the top N items worth hydrating. Reduces REST calls when volume is high.
- Org-wide discovery: use GitHub org members API to auto-populate user list instead of manual `--users` flag.
- Combined output: flag to append highlights section to `generate` output in a single run.

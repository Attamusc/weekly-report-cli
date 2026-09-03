# Context for: Accolades Feature (User Activity Recognition)

## Feature Intent
Accept a list of GitHub user handles, collect their activity across a week (issues touched, PRs, comments, reviews, discussions), and generate "accolades" — recognizing smaller contributions like support work, bug fixes, repairs.

## Relevant Files

### Pipeline Orchestration
- `cmd/generate.go` — Main command orchestration; defines **3-phase execution**: PHASE A (parallel collection), PHASE B (batch summarization), PHASE C (result assembly)
- `internal/pipeline/generate.go` — Core functions: `CollectIssueData()`, `BatchSummarize()`, `AssembleGenerateResults()`, fallback logic
- `internal/pipeline/types.go` — Data structures: `IssueData`, `IssueResult`, `DescribeIssueData`

### GitHub API Client (REST)
- `internal/github/client.go` — OAuth2 client wrapper with **retry logic** (3 retries, exponential backoff 1s base)
- `internal/github/issues.go` — Functions: `FetchIssue()`, `FetchCommentsSince()`, `FetchIssueBody()`
- Retry strategy: handles 5xx, 403 (rate limit), respects `Retry-After` and `X-RateLimit-Reset` headers

### GitHub Projects Client (GraphQL)
- `internal/projects/client.go` — GraphQL client for Projects V2; **pagination with cursors**, max 25 items per page
- `internal/projects/query.go` — GraphQL templates: `projectItemsQueryTemplate`, `projectViewsQueryTemplate`; server-side filtering with query strings
- `internal/projects/types.go` — Data types: `ProjectRef`, `ProjectItem`, `ProjectView`, `FieldFilter`, `FieldType` (text, single-select, date, number)

### AI Summarization
- `internal/ai/summarizer.go` — **Interface**: `Summarizer` with methods `Summarize()`, `SummarizeMany()`, `SummarizeBatch()`, `DescribeBatch()`, `GenerateHeader()`
- `internal/ai/summarizer.go` — **NoopSummarizer**: fallback that returns raw text (used when AI disabled)
- `internal/ai/ghmodels.go` — GitHub Models API client (OpenAI-compatible, uses `gpt-5-mini`, temperature=1)
  - **Batch processing**: `SummarizeBatch()` splits items into chunks (max 25 items/chunk, ~8000 token limit)
  - System prompts for: summary, sentiment analysis, project description, executive header
  - Retry logic: 3 attempts with jittered backoff; respects `Retry-After`

### Input Handling
- `internal/input/resolver.go` — **Entry point**: `ResolveIssueRefs()`; detects input mode (URLList, Project, Mixed)
- `internal/input/links.go` — Parses GitHub issue URLs; validates format, deduplicates
- Current input modes: URL list (stdin/file) or project board (GraphQL); **no user-based queries yet**

### Configuration
- `internal/config/config.go` — Config from env + CLI flags; keys: `GITHUB_TOKEN` (required), `GITHUB_MODELS_BASE_URL`, `GITHUB_MODELS_MODEL`, `DISABLE_SUMMARY`, `AI_TIMEOUT`
- `cmd/common.go` — Shared command setup: logger init, client creation, dependency injection via `commandDeps` struct

### CLI Commands (Cobra)
- `cmd/root.go` — Root command; global flags: `--verbose`, `--quiet`
- `cmd/generate.go` — Generate command with flags: `--since-days` (7), `--concurrency` (4), `--input`, `--project`, `--project-field`, `--project-field-values`, `--project-view`, `--no-notes`, `--no-sentiment`, etc.
- `cmd/describe.go` — Describe command (existing; summarizes issue descriptions)

### Output Formatting
- `internal/format/markdown.go` — `Row` struct + `RenderTable()`; columns: Status | Initiative/Epic | Target Date | Update (+ optional extra columns)
- `internal/format/notes.go` — Note rendering for metadata (multiple updates, no updates, new items, status changes, sentiment mismatches)
- `internal/format/group.go` — Grouping by assignee, label, or field value

### Status & Date Derivation
- `internal/derive/status.go` — Maps trending keywords/emojis to canonical statuses (On Track, At Risk, Off Track, Done, etc.)
- `internal/derive/date.go` — Parses target dates (YYYY-MM-DD, RFC3339); renders as "TBD" if nil

### Report Extraction (Structured Comments)
- `internal/report/extract.go` — Parses keyed blocks from issue comments (markers: `<!-- data key="isReport" ... -->`)
- `internal/report/select.go` — Selects valid reports from comments within time window; returns newest-first

## Project Structure

```
cmd/
  root.go           — Root command, flags, logger setup
  generate.go       — Main pipeline orchestration (PHASE A/B/C)
  describe.go       — Describe/summarize issue bodies
  common.go         — Shared setup (config, clients, adapters)

internal/
  ai/
    summarizer.go   — Summarizer interface + NoopSummarizer
    ghmodels.go     — GitHub Models API client, batch chunking
  config/
    config.go       — Config from env + flags
  derive/
    status.go       — Map keywords → canonical statuses
    date.go         — Parse/render target dates
  diff/
    compare.go      — Compare previous report for status changes
  format/
    markdown.go     — Render markdown table with rows
    notes.go        — Render notes section
    group.go        — Group rows by assignee/label/field
  github/
    client.go       — OAuth2 + retry logic (3x exponential backoff)
    issues.go       — Fetch issues + comments since window
  input/
    resolver.go     — ResolveIssueRefs() — detects mode (URLList/Project/Mixed)
    links.go        — Parse GitHub URLs from stdin/file
  pipeline/
    generate.go     — CollectIssueData(), BatchSummarize(), AssembleGenerateResults()
    types.go        — IssueData, IssueResult, etc.
  projects/
    client.go       — GraphQL client for Projects V2
    query.go        — GraphQL query templates
    types.go        — ProjectRef, ProjectItem, FieldFilter, etc.
    parser.go       — Parse project URLs
    filter.go       — Convert field filters to query strings
    view_filter.go  — Parse view filters
  report/
    extract.go      — Parse keyed blocks from comments
    select.go       — Select reports within time window
  retry/
    backoff.go      — Calculate exponential backoff delays

main.go             — Entry point
```

## Conventions

### Naming
- **Packages**: lowercase, single-word (`ai`, `derive`, `format`, `github`, `input`, `pipeline`, `projects`, `report`)
- **Types**: PascalCase with full words (`IssueRef`, `ProjectItem`, `BatchItem`, `SentimentResult`)
- **Unexported functions**: camelCase (`collectIssueData`, `deduplicateRefs`)
- **Constants**: PascalCase if exported (`StateClosed`, `MarkerIsReport`), camelCase if unexported (`maxRetries`, `baseBackoffMs`)
- **Acronyms**: uppercase in type names (`URL`, `HTML`, `ID`, `API`), but `htmlURL` in field access
- **Enums**: iota-based int types with prefix naming (`ProjectTypeOrg`, `ContentTypeIssue`)

### Error Handling
- Wrap errors with `fmt.Errorf("context: %w", err)` to preserve chain
- Use `errors.New()` for simple sentinel errors
- Return `(value, error)` pairs; never panic in library code
- Discard intentionally unused errors with `_ =` (e.g., `_ = resp.Body.Close()`)

### Code Organization
- Three import groups separated by blank lines: stdlib, external, internal
- Imports ordered by `goimports`
- No circular imports

### Testing
- Standard library only (no testify)
- Table-driven tests with `t.Run(tc.name, ...)`
- Assertions: `if got != want { t.Errorf(...) }`
- HTTP mocking: `net/http/httptest.NewServer` with custom handlers

### Logging
- Use `log/slog` structured logging
- Retrieve logger from context: `logger, ok := ctx.Value(input.LoggerContextKey{}).(*slog.Logger)`
- Fall back to `slog.Default()` if not in context
- Progress/diagnostic to stderr; stdout reserved for report output
- Levels: Debug (internal details), Info (progress), Warn (recoverable issues)

### Concurrency
- Bounded worker pools with channel semaphores: `make(chan struct{}, concurrency)`
- `sync.WaitGroup` for goroutine coordination
- `sync/atomic` for progress counters
- Close channels from dedicated goroutine after `wg.Wait()`

## Key Findings

### 3-Phase Pipeline (Current)
1. **PHASE A (Parallel Collection)**: For each issue URL, fetch metadata + comments since `now - sinceDays` using bounded worker pool (default 4 workers)
2. **PHASE B (Batch Summarization)**: Collect all issues needing summaries, batch into groups (max 25 items, ~8k tokens), send single AI request per batch
3. **PHASE C (Result Assembly)**: Merge AI results with collected data, apply fallback summaries, create final rows/notes

### Input Resolution Pipeline
- **Current entry point**: `input.ResolveIssueRefs()` — detects input mode (URLList or Project)
- **URLList mode**: Reads URLs from stdin or file, parses issue refs (owner/repo/number)
- **Project mode**: Uses GraphQL to fetch project items, filters by field values or view, deduplicates
- **No user-based mode yet** — all queries are issue/project-centric

### GitHub REST API Usage (Current)
- `Issues.Get()` — fetch single issue metadata
- `Issues.ListComments()` — paginated comment fetch with `since` filter (1000 per page)
- Retry strategy: 3 attempts, exponential backoff 1s base, respects rate limit headers

### GitHub GraphQL API Usage (Projects)
- Single query fetches up to 25 project items per page, with cursor pagination
- Server-side filtering using GitHub's query syntax (e.g., `"field:value is:issue"`)
- Supports view-based filtering (view filter stored as JSON in project view)

### AI Summarization
- **Batch size**: Max 25 items per batch to avoid token limits (~8k token "safe zone")
- **Chunking**: Automatic splitting of large requests into sequential chunks
- **System prompts**: Configured per action (summary, sentiment, describe, header)
- **Sentiment analysis**: Optional; compares reported status to content, suggests corrections
- **Fallback**: NoopSummarizer concatenates raw text when AI disabled or fails

### Current Support for User-Based Queries
- **None**: The system assumes issues/PRs as primary data source
- Input resolver only handles issue URLs or project boards
- No GitHub REST API calls for user activity (`/users/{login}/events`, `/repos/{owner}/{repo}/issues?involves={login}`)
- No GraphQL queries for user contributions across repos

### Rate Limiting Strategy
- **REST API**: Respects `X-RateLimit-Remaining`, `X-RateLimit-Reset`, `Retry-After` headers
- **Retry logic**: 3 attempts, exponential backoff with jitter, up to 60s default retry delay
- **GraphQL**: Also uses same retry logic via HTTP client wrapper
- **AI API**: Separate 3-attempt retry with jittered backoff; respects `Retry-After`
- **No request coalescing or batching** across commands — each run is independent

### Missing Pieces for Accolades
1. **User activity collection**: No API calls to fetch user event streams or contributions
2. **Issue involvement queries**: GitHub supports `involves:` filter but not used in current code
3. **PR/discussion/review handling**: Current system focuses on issues; PRs included in projects but not queried directly
4. **Activity aggregation**: No logic to group activity by user across multiple repos
5. **Accolade generation**: No AI prompt template for generating recognition text vs. status summaries

## Integration Risks

### Risk 1: Input Resolution Refactoring
**Location**: `input.ResolveIssueRefs()` and `input.ResolverConfig`  
**Issue**: Current system expects issue references (URLs or project items). For accolades, input should be user handles. The mode detection will need a new `InputModeUser` variant, and the resolver will delegate to a new user activity collector.  
**Risk**: If not designed as an adapter like `ProjectClientAdapter`, this creates tight coupling. Recommend: Create `UserActivityClient` interface (parallel to `ProjectClient`), implement in `cmd/common.go` adapter.

### Risk 2: GitHub API Request Explosion
**Location**: GitHub REST client, rate limiting  
**Issue**: Current pipeline fetches N issues × 1-2 API calls per issue (issue metadata, comments). For accolades, querying user activity across a whole org could mean:
- 1 call per user to list events
- Additional calls to fetch comment details, PR metadata, etc.
- Multiple repos, multiple activity types
- Could easily hit GitHub REST API limits (5000/hour for OAuth)  
**Risk**: Without careful batching or GraphQL migration, user activity collection could throttle the entire pipeline.  
**Mitigation**: (a) Use GraphQL for user contributions if possible, (b) Paginate events carefully, (c) Cache/deduplicate activity data, (d) Add rate limit awareness to user activity collector.

### Risk 3: Pipeline Phases Don't Align with User Activity
**Location**: `cmd/generate.go` phases, `pipeline.IssueData` struct  
**Issue**: Current PHASE A assumes each issue is fetched independently. For users, you'd likely:
- PHASE A: Fetch all activity for each user (events, PRs, comments, reviews)
- PHASE B: Categorize/summarize activity (support work, bug fixes, etc.)
- PHASE C: Generate accolades paragraph
The `IssueData` struct assumes an issue as the unit; accolades would have a user as the unit.  
**Risk**: Reusing the same `IssueData` struct for users is misleading. Recommend: Create new `UserActivityData` struct (parallel structure), potentially reuse phases concept but with different semantics.

### Risk 4: AI Prompt Expectations
**Location**: `internal/ai/ghmodels.go`, batch processing, system prompts  
**Issue**: Current batch system prompt expects "update texts" and "reported status" — the format for accolades (recognizing smaller contributions) is very different. The prompt expects 3-5 sentences about progress; accolades need recognition language ("great debugging", "solid support work", etc.).  
**Risk**: Reusing `BatchSummarizer.SummarizeBatch()` with a completely different system prompt might work but violates semantic cohesion.  
**Mitigation**: Either (a) extend `BatchItem` to allow action/type field indicating intent (summary vs. accolade), or (b) create separate `AccoladeItem` struct and `AccoladeSummarizer` interface method.

### Risk 5: Fallback Summarizer Assumptions
**Location**: `ai.NoopSummarizer`  
**Issue**: When AI is disabled, `NoopSummarizer.SummarizeBatch()` just concatenates raw update texts. For accolades, there may be no "raw text" to concatenate — just a list of activity events (PR merged, issue closed, review given, etc.). The fallback logic needs to handle this gracefully.  
**Risk**: If the pipeline assumes NoopSummarizer will always produce *something*, accolades without AI will produce empty or nonsensical output.  
**Mitigation**: When designing accolade data structures, ensure NoopSummarizer can produce sensible fallback accolades (e.g., "Contributed to X issues, opened Y PRs, reviewed Z PRs").

### Risk 6: Command-Level Input Parsing
**Location**: `cmd/generate.go`, flag parsing  
**Issue**: Current flags focus on project board and URL list input. For accolades, you'd need `--users "alice,bob,charlie"` or `--user-file users.txt`. This is a new input paradigm that the command needs to understand and dispatch to.  
**Risk**: If the accolades command (or a new flag on generate) doesn't cleanly separate user input handling from issue input handling, the command logic becomes tangled.  
**Mitigation**: Either (a) create a separate `accolades` command (similar to `describe`), or (b) add user input flags and have `setupCommand()` handle both issue and user resolution modes.

### Risk 7: Output Format Incompatibility
**Location**: `format.Row`, `format.RenderTable()`  
**Issue**: Current output is a markdown table with columns: Status | Initiative/Epic | Target Date | Update. Accolades are likely a narrative paragraph or bulleted list of recognitions, not a table. The format package assumes tabular output.  
**Risk**: Either accolades are shoe-horned into the table format (awkward), or a new format output is created (duplication).  
**Mitigation**: Design accolade output as a separate renderer; reuse `format.Note` concept or create `format.AccoladeSection` type.

## Gotchas

1. **Empty User Handle List**: What if a user provides no user handles, or all handles are invalid? Current pipeline returns `config.ErrNoRows` for no issues; need equivalent for no valid users.

2. **Private Repositories**: Current GitHub client respects private repo access (via OAuth). User event queries also need to respect this. An event in a private repo the user has no access to should be filtered.

3. **Duplicate Activity**: If a user is involved in an issue (commenting, assigning, reviewing), that activity might be represented multiple times in events. Need deduplication logic.

4. **Activity Window Boundary**: Issues have a "created" and "updated" timestamp. For accolades, the activity window might span multiple repo event types (created issue, opened PR, merged PR, left review) — need consistent `since` filtering across all activity types.

5. **GitHub API Event Types**: GitHub events API returns 30+ event types (`PushEvent`, `IssuesEvent`, `PullRequestEvent`, `PullRequestReviewEvent`, `IssueCommentEvent`, etc.). Each has different payload structure. Current code doesn't parse events.

6. **GraphQL vs REST for User Activity**: GraphQL doesn't have a direct "user activity stream" query like REST does. Might need REST `/users/{login}/events` (public events only) or `/repos/{owner}/{repo}/issues?involves={user}` (private events), or iterate repos and check each one.

7. **Rate Limit Complexity**: Mixing user event queries with existing issue queries in the same pipeline could cause unexpected rate limit behavior. Events API has its own rate limit bucket.

8. **Accolade Bias**: AI-generated accolades might be biased toward quantitative metrics (more PRs = more praise). Need thoughtful prompt design to recognize diverse contributions.

## Architectural Patterns to Reuse

1. **Adapter Pattern**: Use `ProjectClientAdapter` as a model for adapting a user activity collector to the pipeline's resolver interface.
2. **Chunked Batch Processing**: Reuse `chunkedBatch[I, R]()` logic from AI client for splitting large user lists into manageable groups.
3. **Fallback Strategy**: Follow the `ShouldSummarize` + `FallbackSummary` pattern for when AI is unavailable.
4. **Structured Logging**: Inject logger via context key (`input.LoggerContextKey{}`); use `slog.Debug`/`Info`/`Warn` consistently.
5. **Retry + Backoff**: Reuse `retry.CalculateBackoff()` for user activity API calls.
6. **Worker Pool**: Use the bounded semaphore pattern for parallel user activity fetching.

## Next Steps for Implementation Planning

1. **Define UserActivity Data Model**: What constitutes a "user activity"? (issue/PR/review/comment with timestamp, type, repo, description)
2. **Design User Activity Collection**: REST API (`/users/{login}/events`), paginate, filter by `since`, handle event type variations
3. **Accolade Generation Prompt**: What should AI generate? Recognition paragraph? Bullet list? Multiple accolades per user?
4. **Output Format**: Markdown table (per user) or narrative section? Single accolade per user or multiple?
5. **Rate Limit Capacity**: Estimate API calls for N users, M repos, 1-week window; validate against quotas
6. **Extend Input Resolver**: Add `InputModeUser` mode detection, user handle parsing/deduplication
7. **Command Structure**: New `accolades` subcommand or extend `generate --accolades`?


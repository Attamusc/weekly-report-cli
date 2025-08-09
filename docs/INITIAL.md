TASK CHECKLIST

Phase 1 — CLI, config, link parsing
• ☐ go.mod add deps (cobra, go-github, oauth2)
• ☐ main.go root bootstrap
• ☐ cmd/root.go flags/env, require GITHUB_TOKEN
• ☐ cmd/generate.go pipeline + worker pool
• ☐ internal/config/config.go resolve env+flags
• ☐ internal/input/links.go parse GitHub issue URLs
• ☐ Tests: URL parser

Phase 2 — GitHub fetch + report extraction/selection
• ☐ internal/github/client.go PAT auth + retry
• ☐ internal/github/issues.go issue + comments since window
• ☐ internal/report/extract.go parse keyed blocks + isReport
• ☐ internal/report/select.go select 0/1/≥2 reports (newest-first)
• ☐ Tests: extract/select + http fakes

Phase 3 — Summarization (GitHub Models)
• ☐ internal/ai/summarizer.go interface + noop
• ☐ internal/ai/ghmodels.go GitHub Models client
• ☐ Tests: single/multi prompts

Phase 4 — Derivation + rendering (TBD + Notes)
• ☐ internal/derive/status.go map trending → emoji/caption
• ☐ internal/derive/date.go parse + render TBD
• ☐ internal/format/markdown.go table render
• ☐ internal/format/notes.go notes render
• ☐ Wire in cmd/generate.go
• ☐ Tests: status/date mapping, table golden, notes formatting, e2e

⸻

PHASE 1 — CLI, config, link parsing

Affected files + summary
• go.mod, go.sum: add github.com/spf13/cobra, github.com/google/go-github/v66/github, golang.org/x/oauth2.
• main.go: cobra root execute.
• cmd/root.go: define gh-epic-updates with flags:
• --since-days (int, default 7)
• --input (path, default stdin)
• --concurrency (int, default 4)
• --no-notes (bool, default false)
Env:
• required GITHUB_TOKEN (private repos + GitHub Models)
• optional GITHUB_MODELS_BASE_URL (default <https://models.github.ai/v1>)
• optional GITHUB_MODELS_MODEL (default gpt-4o-mini)
• optional DISABLE_SUMMARY (if set, disable AI)
• cmd/generate.go: read links → bounded worker pool → per-issue pipeline → accumulate rows + notes → write to stdout.
• internal/config/config.go:

type Config struct {
GitHubToken string
SinceDays int
Concurrency int
Notes bool
Models struct{ BaseURL, Model string; Enabled bool }
}
// FromEnvAndFlags(); error if GitHubToken == ""

    • internal/input/links.go:

type IssueRef struct{ Owner, Repo string; Number int; URL string }
func ParseIssueLinks(r io.Reader) ([]IssueRef, error)
// Accept: <https://github.com/{owner}/{repo}/issues/{n}> (allow ?#)
// Dedupe, stable order.

Unit tests
• internal/input/links_test.go: valid forms, fragments/queries, duplicate removal, invalid paths rejected.

⸻

PHASE 2 — GitHub fetch + report extraction/selection

Affected files + summary
• internal/github/client.go:

func New(ctx context.Context, token string) \*github.Client
// oauth2 transport, UA "gh-epic-updates/1.0"
// retry/backoff on 5xx and 403 w/ RateLimit-Reset / Retry-After

    • internal/github/issues.go:

type IssueData struct{ URL, Title string }
type Comment struct{ Body string; CreatedAt time.Time; Author, URL string }

func FetchIssue(ctx context.Context, c *github.Client, ref IssueRef) (IssueData, error)
func FetchCommentsSince(ctx context.Context, c*github.Client, ref IssueRef, since time.Time) ([]Comment, error)
// paginate, filter CreatedAt >= since

    • internal/report/extract.go:

const MarkerIsReport = `<!-- data key="isReport" value="true" -->`

type Report struct{
TrendingRaw string // key "trending" (status source)
TargetDate string // key "target_date"
UpdateRaw string // key "update" (may be multiline)
CreatedAt time.Time
SourceURL string
}

// ParseReport returns (Report, ok) if MarkerIsReport present (case-insensitive),
// and at least one of the three keys extracted.
// Keys parsed using:
// <!-- data key="<k>" start --> ... <!-- data end -->
// Non-greedy, Unicode, trimmed.
func ParseReport(body string, createdAt time.Time, url string) (Report, bool)

    • internal/report/select.go:

// Return ALL valid reports within window, newest-first.
func SelectReports(cs []Comment, since time.Time) []Report

Unit tests
• internal/github/issues_test.go: httptest.Server for issue + paginated comments; verify since filtering.
• internal/report/extract_test.go: exact provided sample parses (trending = “🟣 done”, target_date = “2025-08-06”, multiline update), isReport gate required.
• internal/report/select_test.go: 0/1/≥2 in-window cases; newest-first order.

⸻

PHASE 3 — Summarization (GitHub Models)

Affected files + summary
• internal/ai/summarizer.go:

type Summarizer interface{
Summarize(ctx context.Context, issueTitle, issueURL, updateText string) (string, error)
SummarizeMany(ctx context.Context, issueTitle, issueURL string, updates []string) (string, error)
}
type NoopSummarizer struct{}
func (NoopSummarizer) Summarize(_context.Context,_, _, u string)(string,error){ return strings.TrimSpace(u), nil }
func (NoopSummarizer) SummarizeMany(_ context.Context, _,_ string, us []string)(string,error){
return strings.TrimSpace(strings.Join(us, " ")), nil
}

    • internal/ai/ghmodels.go:

type GHModelsClient struct{
HTTP *http.Client
BaseURL string // default <https://models.github.ai/v1>
Model string // default gpt-4o-mini
Token string // use GITHUB_TOKEN PAT
}
func (c*GHModelsClient) Summarize(ctx context.Context, issueTitle, issueURL, update string) (string, error)
func (c \*GHModelsClient) SummarizeMany(ctx context.Context, issueTitle, issueURL string, updates []string) (string, error)
// POST /chat/completions with:
// - System: "Summarize engineering status updates in ≤ 35 words, one sentence, present tense, markdown-ready, no prefatory text."
// - User (single): "Issue: <title> (<url>)\nUpdate:\n<raw>"
// - User (many): "Issue: <title> (<url>)\nUpdates (newest first):\n1) <u1>\n2) <u2>\n..."
// temperature 0.2; Authorization: Bearer <GITHUB_TOKEN>
// Handle 429 with jittered backoff.

    • cmd/generate.go: choose summarizer = GHModelsClient unless DISABLE_SUMMARY set.

Unit tests
• internal/ai/ghmodels_test.go: fake server validates single/multi payloads; returns canned completion; assert single sentence and ≤ 35 words.
• cmd/generate_integration_test.go: fake GitHub + Models; aggregated summary when multiple reports.

⸻

PHASE 4 — Derivation + rendering (TBD + Notes)

Affected files + summary
• internal/derive/status.go:

type Status struct{ Emoji, Caption string }

// Map free-form/emoji from "trending" to canonical output "Status".
// on track/green/🟢 → :green_circle: On Track
// at risk/yellow/🟡 → :yellow_circle: At Risk
// off track/blocked/red/🔴 → :red_circle: Off Track
// not started/white/⚪ → :white_circle: Not Started
// done/complete/purple/🟣 → :purple_circle: Done
func MapTrending(raw string) Status
// strip leading circle emoji if present; case/whitespace tolerant.

    • internal/derive/date.go:

func ParseTargetDate(raw string) *time.Time { // try YYYY-MM-DD, then RFC3339; else nil }
func RenderTargetDate(t*time.Time) string {
if t==nil { return "TBD" }
return t.UTC().Format("2006-01-02")
}

    • internal/format/markdown.go:

type Row struct{
StatusEmoji, StatusCaption string
EpicTitle, EpicURL string
TargetDate \*time.Time
UpdateMD string
}
func RenderTable(rows []Row) string
// Header: | Status | Initiative/Epic | Target Date | Update |
// Row: "EMOJI Caption" | [Epic: TITLE](URL) | YYYY-MM-DD or "TBD" | summary
// Escape pipes in TITLE/Update; collapse newlines to single spaces.

    • internal/format/notes.go:

type NoteKind int
const (
NoteMultipleUpdates NoteKind = iota
NoteNoUpdatesInWindow
)
type Note struct{ Kind NoteKind; IssueURL string; SinceDays int }

func RenderNotes(notes []Note) string
// "## Notes" then bullets:
// - <url>: multiple structured updates in last N days
// - <url>: no update in last N days

    • cmd/generate.go (final wiring):
    • For each issue:
    • fetch IssueData, comments since now - sinceDays
    • reports := SelectReports(...)
    • if len(reports) == 0 → append NoteNoUpdatesInWindow; omit table row
    • else:
    • collect non-empty update texts (newest-first)
    • summary = SummarizeMany if >1 else Summarize
    • if len(reports) >= 2 → append NoteMultipleUpdates
    • status = MapTrending(reports[0].TrendingRaw) (newest)
    • date   = ParseTargetDate(reports[0].TargetDate) (newest; may be nil → renders TBD)
    • append Row
    • Print RenderTable(rows) to stdout.
    • If any notes and Notes enabled → append RenderNotes(notes).

Unit tests
• internal/derive/status_test.go: "🟣 done" → :purple_circle: Done; "⚪ not started" → :white_circle: Not Started; "🔴 blocked" → red; casing/emoji variants.
• internal/derive/date_test.go: parse YYYY-MM-DD and RFC3339; nil renders TBD.
• internal/format/markdown_test.go: golden table with correct header, link [Epic: TITLE](URL), emoji+caption, date or TBD, escaped pipes.
• internal/format/notes_test.go: bullets for multiple/no updates; correct N substitution.
• cmd/generate_integration_test.go:
• 2 reports → single table row with aggregated summary; Notes mention multiple updates.
• 0 reports → no row; Notes mention no update.
• Missing target_date → row present with TBD.

⸻

COMMAND INTERFACE

cat links.txt | gh-epic-updates generate --since-days 7

    • Input: newline-separated GitHub issue URLs (stdin or --input).
    • Auth: required GITHUB_TOKEN (private repo + GitHub Models access).
    • Output: markdown table to stdout; optional ## Notes section (not part of the table).
    • Exit: 0 success; 2 no rows produced; >2 fatal.

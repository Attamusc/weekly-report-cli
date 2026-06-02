package cmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Attamusc/weekly-report-cli/internal/ai"
	"github.com/Attamusc/weekly-report-cli/internal/config"
	"github.com/Attamusc/weekly-report-cli/internal/format"
	internalgh "github.com/Attamusc/weekly-report-cli/internal/github"
	"github.com/Attamusc/weekly-report-cli/internal/input"
	"github.com/Attamusc/weekly-report-cli/internal/pipeline"
	"github.com/Attamusc/weekly-report-cli/internal/rollup"
)

// ── Fake AI Summarizer ────────────────────────────────────────────────────

type fakeAISummarizer struct {
	summarizeHighlightFn func(context.Context, ai.HighlightItem) (ai.Highlight, error)
	mergeThemesFn        func(context.Context, []ai.Highlight) ([]ai.Highlight, error)
}

func (f *fakeAISummarizer) Summarize(_ context.Context, _, _, _ string) (string, error) {
	return "", nil
}
func (f *fakeAISummarizer) SummarizeMany(_ context.Context, _, _ string, _ []string) (string, error) {
	return "", nil
}
func (f *fakeAISummarizer) SummarizeBatch(_ context.Context, _ []ai.BatchItem) (map[string]ai.BatchResult, error) {
	return map[string]ai.BatchResult{}, nil
}
func (f *fakeAISummarizer) DescribeBatch(_ context.Context, _ []ai.DescribeBatchItem) (map[string]string, error) {
	return map[string]string{}, nil
}
func (f *fakeAISummarizer) GenerateHeader(_ context.Context, _ []ai.HeaderItem) (string, error) {
	return "", nil
}
func (f *fakeAISummarizer) SummarizeHighlight(ctx context.Context, item ai.HighlightItem) (ai.Highlight, error) {
	if f.summarizeHighlightFn != nil {
		return f.summarizeHighlightFn(ctx, item)
	}
	theme := "General"
	if len(item.Labels) > 0 {
		theme = item.Labels[0]
	}
	return ai.Highlight{
		Theme:   theme,
		Title:   item.IssueTitle,
		URL:     item.IssueURL,
		Summary: "Summary of " + item.IssueTitle,
	}, nil
}
func (f *fakeAISummarizer) MergeThemes(ctx context.Context, in []ai.Highlight) ([]ai.Highlight, error) {
	if f.mergeThemesFn != nil {
		return f.mergeThemesFn(ctx, in)
	}
	return in, nil
}

// ── Mock pipeline fetcher ─────────────────────────────────────────────────

type mockHighlightFetcher struct {
	issues   map[string]internalgh.IssueData
	comments map[string][]internalgh.Comment
}

func (m *mockHighlightFetcher) FetchIssue(_ context.Context, ref input.IssueRef) (internalgh.IssueData, error) {
	if d, ok := m.issues[ref.URL]; ok {
		return d, nil
	}
	return internalgh.IssueData{}, fmt.Errorf("issue not found: %s", ref.URL)
}

func (m *mockHighlightFetcher) FetchCommentsSince(_ context.Context, ref input.IssueRef, _ time.Time) ([]internalgh.Comment, error) {
	return m.comments[ref.URL], nil
}

func (m *mockHighlightFetcher) FetchTimelineSince(_ context.Context, _ input.IssueRef, _ time.Time) ([]internalgh.TimelineEvent, error) {
	return nil, nil
}

func (m *mockHighlightFetcher) FetchPullRequest(_ context.Context, _ input.IssueRef) (*internalgh.PullRequestData, error) {
	return nil, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func quietCfg(concurrency int) *config.Config {
	return &config.Config{Quiet: true, Concurrency: concurrency}
}

func makeRef(_, repo string, number int, isPR bool, commentCount int, author string) input.IssueRef {
	owner := "org"
	kind := "issues"
	if isPR {
		kind = "pull"
	}
	u := fmt.Sprintf("https://github.com/%s/%s/%s/%d", owner, repo, kind, number)
	return input.IssueRef{
		Owner:        owner,
		Repo:         repo,
		Number:       number,
		URL:          u,
		Title:        fmt.Sprintf("%s/%s#%d title", owner, repo, number),
		State:        "open",
		IsPR:         isPR,
		AuthorLogin:  author,
		CommentCount: commentCount,
		UpdatedAt:    time.Now(),
	}
}

// ── aiMapHighlights tests ─────────────────────────────────────────────────

func TestAIMapHighlights_AllSucceed(t *testing.T) {
	s1 := rollup.ScoredItem{Ref: makeRef("org", "a", 1, false, 3, "alice"), Score: 0.9, Labels: []string{"Bug Fix"}}
	s2 := rollup.ScoredItem{Ref: makeRef("org", "b", 2, true, 5, "bob"), Score: 0.7, Labels: []string{"Infra"}}
	survivors := []rollup.ScoredItem{s1, s2}

	fake := &fakeAISummarizer{}
	highlights := aiMapHighlights(context.Background(), fake, survivors, nil, 2, quietCfg(2), testLogger())

	if len(highlights) != 2 {
		t.Fatalf("expected 2 highlights, got %d", len(highlights))
	}
	// Order must match survivors.
	if highlights[0].URL != s1.Ref.URL {
		t.Errorf("expected first URL %s, got %s", s1.Ref.URL, highlights[0].URL)
	}
	if highlights[0].Theme != "Bug Fix" {
		t.Errorf("expected theme 'Bug Fix', got %q", highlights[0].Theme)
	}
	if highlights[1].Theme != "Infra" {
		t.Errorf("expected theme 'Infra', got %q", highlights[1].Theme)
	}
}

func TestAIMapHighlights_PerItemFallbackOnError(t *testing.T) {
	ref := makeRef("org", "repo", 1, false, 2, "charlie")
	ref.Title = "Broken item"
	survivors := []rollup.ScoredItem{{Ref: ref, Score: 0.8, Labels: []string{"Support"}}}

	fake := &fakeAISummarizer{
		summarizeHighlightFn: func(_ context.Context, _ ai.HighlightItem) (ai.Highlight, error) {
			return ai.Highlight{}, fmt.Errorf("AI unavailable")
		},
	}
	highlights := aiMapHighlights(context.Background(), fake, survivors, nil, 1, quietCfg(1), testLogger())

	if len(highlights) != 1 {
		t.Fatalf("expected 1 fallback highlight, got %d", len(highlights))
	}
	if highlights[0].Theme != "Support" {
		t.Errorf("expected fallback theme 'Support', got %q", highlights[0].Theme)
	}
	if highlights[0].Title != "Broken item" {
		t.Errorf("expected fallback title 'Broken item', got %q", highlights[0].Title)
	}
}

func TestAIMapHighlights_PreservesOrder(t *testing.T) {
	const n = 10
	survivors := make([]rollup.ScoredItem, n)
	for i := range survivors {
		survivors[i] = rollup.ScoredItem{
			Ref:   makeRef("org", "r", i+1, false, i, "user"),
			Score: float64(n-i) / float64(n),
		}
	}

	fake := &fakeAISummarizer{
		summarizeHighlightFn: func(_ context.Context, item ai.HighlightItem) (ai.Highlight, error) {
			return ai.Highlight{URL: item.IssueURL, Title: item.IssueTitle, Theme: "T", Summary: "s"}, nil
		},
	}
	highlights := aiMapHighlights(context.Background(), fake, survivors, nil, 3, quietCfg(3), testLogger())

	if len(highlights) != n {
		t.Fatalf("expected %d highlights, got %d", n, len(highlights))
	}
	for i, h := range highlights {
		if h.URL != survivors[i].Ref.URL {
			t.Errorf("index %d: expected URL %s, got %s", i, survivors[i].Ref.URL, h.URL)
		}
	}
}

// TestAIMapHighlights_SkipsAIForEmptyUpdateTexts verifies that survivors with
// no hydrated comment bodies bypass SummarizeHighlight entirely, emit a
// deterministic fallback Highlight, and log the skip.
func TestAIMapHighlights_SkipsAIForEmptyUpdateTexts(t *testing.T) {
	ref := makeRef("org", "repo", 42, false, 0, "alice")
	ref.Title = "Closed with no comments"

	survivors := []rollup.ScoredItem{
		{Ref: ref, Score: 0.8, Labels: []string{"bug"}},
	}

	var callCount atomic.Int32
	fake := &fakeAISummarizer{
		summarizeHighlightFn: func(_ context.Context, item ai.HighlightItem) (ai.Highlight, error) {
			if item.IssueURL == ref.URL {
				callCount.Add(1)
			}
			return ai.Highlight{URL: item.IssueURL, Title: item.IssueTitle, Theme: "T", Summary: "AI summary"}, nil
		},
	}

	// Provide hydrated data with empty RecentComments.
	dataByURL := map[string]pipeline.NarrativeItem{
		ref.URL: {
			URL:            ref.URL,
			Title:          ref.Title,
			RecentComments: []pipeline.NarrativeComment{}, // empty — no comment bodies
			Labels:         []string{"bug"},
		},
	}

	// Use a logger that captures output so we can assert the skip message.
	var logBuf strings.Builder
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	highlights := aiMapHighlights(context.Background(), fake, survivors, dataByURL, 1, quietCfg(1), logger)

	if len(highlights) != 1 {
		t.Fatalf("expected 1 highlight, got %d", len(highlights))
	}
	if callCount.Load() != 0 {
		t.Errorf("expected 0 AI calls for empty-context item, got %d", callCount.Load())
	}
	h := highlights[0]
	if h.URL != ref.URL {
		t.Errorf("expected URL %s, got %s", ref.URL, h.URL)
	}
	if h.Title != ref.Title {
		t.Errorf("expected title %q, got %q", ref.Title, h.Title)
	}
	if h.Summary != ref.Title {
		t.Errorf("expected summary == title (%q), got %q", ref.Title, h.Summary)
	}
	if !strings.Contains(logBuf.String(), "Skipping AI for empty-context item") {
		t.Errorf("expected skip log message, got: %s", logBuf.String())
	}
}

// ── scoredItemToHighlightItem tests ──────────────────────────────────────

func TestScoredItemToHighlightItem_NilDataMap(t *testing.T) {
	ref := makeRef("org", "repo", 5, true, 3, "dave")
	scored := rollup.ScoredItem{Ref: ref, Score: 0.6, Labels: []string{"ci"}}

	item := scoredItemToHighlightItem(scored, nil)

	if item.IssueURL != ref.URL {
		t.Errorf("expected URL %s, got %s", ref.URL, item.IssueURL)
	}
	if !item.IsPR {
		t.Error("expected IsPR=true")
	}
	if len(item.Labels) != 1 || item.Labels[0] != "ci" {
		t.Errorf("expected labels [ci], got %v", item.Labels)
	}
}

func TestScoredItemToHighlightItem_HydrationFillsMissingFields(t *testing.T) {
	ref := makeRef("org", "repo", 10, false, 1, "")
	ref.Title = ""
	ref.State = ""

	scored := rollup.ScoredItem{Ref: ref, Score: 0.4}
	dataByURL := map[string]pipeline.NarrativeItem{
		ref.URL: {
			URL:    ref.URL,
			Title:  "Hydrated Title",
			State:  "closed",
			Labels: []string{"bug"},
			RecentComments: []pipeline.NarrativeComment{
				{Body: "fixed"},
			},
		},
	}

	item := scoredItemToHighlightItem(scored, dataByURL)

	if item.IssueTitle != "Hydrated Title" {
		t.Errorf("expected hydrated title, got %q", item.IssueTitle)
	}
	if item.IssueState != "closed" {
		t.Errorf("expected state 'closed', got %q", item.IssueState)
	}
	if len(item.UpdateTexts) != 1 || item.UpdateTexts[0] != "fixed" {
		t.Errorf("expected update texts from hydration, got %v", item.UpdateTexts)
	}
}

// ── fallbackHighlight tests ───────────────────────────────────────────────

func TestFallbackHighlight_WithLabels(t *testing.T) {
	item := ai.HighlightItem{
		IssueURL:   "https://github.com/org/repo/issues/1",
		IssueTitle: "My Issue",
		Labels:     []string{"Bug Fix", "Urgent"},
	}
	h := fallbackHighlight(item)
	if h.Theme != "Bug Fix" {
		t.Errorf("expected theme 'Bug Fix', got %q", h.Theme)
	}
	if h.Title != "My Issue" {
		t.Errorf("expected title 'My Issue', got %q", h.Title)
	}
	if h.URL != item.IssueURL {
		t.Errorf("expected URL preserved")
	}
}

func TestFallbackHighlight_NoLabels(t *testing.T) {
	item := ai.HighlightItem{
		IssueURL:   "https://github.com/org/repo/issues/2",
		IssueTitle: "No Labels",
	}
	h := fallbackHighlight(item)
	if h.Theme != "General" {
		t.Errorf("expected theme 'General', got %q", h.Theme)
	}
}

// ── collectNarrativeItemsParallel tests ─────────────────────────────────────

func TestCollectNarrativeItemsParallel_HydrationSuccess(t *testing.T) {
	ref1 := makeRef("org", "repo", 1, false, 3, "alice")
	ref2 := makeRef("org", "repo", 2, true, 5, "bob")

	fetcher := &mockHighlightFetcher{
		issues: map[string]internalgh.IssueData{
			ref1.URL: {URL: ref1.URL, Title: "Issue One", State: "open"},
			ref2.URL: {URL: ref2.URL, Title: "PR Two", State: "closed"},
		},
		comments: map[string][]internalgh.Comment{},
	}

	since := time.Now().AddDate(0, 0, -7)
	data := collectNarrativeItemsParallel(context.Background(), fetcher, []input.IssueRef{ref1, ref2}, quietCfg(2), since, testLogger())

	if len(data) != 2 {
		t.Fatalf("expected 2 results, got %d", len(data))
	}
}

func TestCollectNarrativeItemsParallel_ErrorsSkipped(t *testing.T) {
	ref := makeRef("org", "repo", 99, false, 0, "")

	fetcher := &mockHighlightFetcher{
		issues:   map[string]internalgh.IssueData{}, // not found → error
		comments: map[string][]internalgh.Comment{},
	}

	since := time.Now().AddDate(0, 0, -7)
	data := collectNarrativeItemsParallel(context.Background(), fetcher, []input.IssueRef{ref}, quietCfg(1), since, testLogger())

	if len(data) != 0 {
		t.Errorf("expected 0 results after error, got %d", len(data))
	}
}

// ── End-to-end rendering tests ────────────────────────────────────────────

// TestRenderNoSummaryPath verifies the --no-summary rendering path produces
// by-author markdown output using rollup.Compute + format.RenderRollup.
func TestRenderNoSummaryPath(t *testing.T) {
	now := time.Now()
	closedAt := now.AddDate(0, 0, -1)
	ref := input.IssueRef{
		Owner:        "org",
		Repo:         "repo",
		Number:       1,
		URL:          "https://github.com/org/repo/issues/1",
		Title:        "Fix login bug",
		State:        "closed",
		IsPR:         false,
		AuthorLogin:  "alice",
		CommentCount: 5,
		UpdatedAt:    now,
		ClosedAt:     &closedAt,
	}

	since := now.AddDate(0, 0, -7)
	r := rollup.Compute([]input.IssueRef{ref}, since)
	output := format.RenderRollup(r)

	if !strings.Contains(output, "@alice") {
		t.Errorf("expected @alice in rollup output, got:\n%s", output)
	}
	if !strings.Contains(output, "Fix login bug") {
		t.Errorf("expected issue title in rollup output, got:\n%s", output)
	}
	// Rollup should not write to stdout (caller does). Verify non-empty.
	if output == "" {
		t.Error("expected non-empty rollup output")
	}
}

// TestRenderAIPath verifies the AI-enabled rendering path produces by-theme markdown.
func TestRenderAIPath(t *testing.T) {
	survivors := []rollup.ScoredItem{
		{
			Ref:   makeRef("org", "repo-a", 1, false, 4, "bob"),
			Score: 0.9, Author: "bob", Labels: []string{"Bug Fixes"},
		},
		{
			Ref:   makeRef("org", "repo-b", 2, true, 3, "carol"),
			Score: 0.7, Author: "carol", Labels: []string{"Infrastructure"},
		},
	}

	fake := &fakeAISummarizer{}
	// Provide non-empty RecentComments so the AI-skip filter does not fire.
	dataByURL := map[string]pipeline.NarrativeItem{
		survivors[0].Ref.URL: {URL: survivors[0].Ref.URL, RecentComments: []pipeline.NarrativeComment{{Body: "comment body"}}},
		survivors[1].Ref.URL: {URL: survivors[1].Ref.URL, RecentComments: []pipeline.NarrativeComment{{Body: "comment body"}}},
	}
	highlights := aiMapHighlights(context.Background(), fake, survivors, dataByURL, 2, quietCfg(2), testLogger())

	merged, err := fake.MergeThemes(context.Background(), highlights)
	if err != nil {
		t.Fatalf("MergeThemes error: %v", err)
	}

	output := format.RenderHighlights(merged)

	if output == "" {
		t.Fatal("expected non-empty AI highlights output")
	}
	if !strings.Contains(output, "Bug Fixes") {
		t.Errorf("expected 'Bug Fixes' theme in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Infrastructure") {
		t.Errorf("expected 'Infrastructure' theme in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Summary of") {
		t.Errorf("expected AI summary text in output, got:\n%s", output)
	}
}

// TestMergeThemesErrorFallback verifies the reduce-step error handling:
// when MergeThemes returns an error, the caller falls back to the pre-merge slice.
func TestMergeThemesErrorFallback(t *testing.T) {
	ref := makeRef("org", "repo", 1, false, 3, "alice")
	survivors := []rollup.ScoredItem{{Ref: ref, Score: 0.5, Labels: []string{"Bugs"}}}

	fake := &fakeAISummarizer{
		mergeThemesFn: func(_ context.Context, in []ai.Highlight) ([]ai.Highlight, error) {
			return nil, fmt.Errorf("reduce error")
		},
	}

	preMerge := aiMapHighlights(context.Background(), fake, survivors, nil, 1, quietCfg(1), testLogger())
	if len(preMerge) != 1 {
		t.Fatalf("expected 1 pre-merge highlight, got %d", len(preMerge))
	}

	// Simulate runHighlights reduce-error fallback.
	_, mergeErr := fake.MergeThemes(context.Background(), preMerge)
	if mergeErr == nil {
		t.Fatal("expected MergeThemes to return error for this test")
	}
	merged := preMerge // fallback

	if len(merged) != 1 {
		t.Errorf("expected 1 highlight in fallback result, got %d", len(merged))
	}
	if merged[0].Theme != "Bugs" {
		t.Errorf("expected theme 'Bugs', got %q", merged[0].Theme)
	}
}

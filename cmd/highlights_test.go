package cmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Attamusc/weekly-report-cli/internal/ai"
	"github.com/Attamusc/weekly-report-cli/internal/config"
	"github.com/Attamusc/weekly-report-cli/internal/format"
	internalgh "github.com/Attamusc/weekly-report-cli/internal/github"
	"github.com/Attamusc/weekly-report-cli/internal/input"
	"github.com/Attamusc/weekly-report-cli/internal/narrative"
	"github.com/Attamusc/weekly-report-cli/internal/pipeline"
	"github.com/Attamusc/weekly-report-cli/internal/rollup"
)

// ── Fake AI Summarizer ───────────────────────────────────────────────────────────

type fakeAISummarizer struct {
	writeNarrativeFn func(context.Context, []narrative.Item, rollup.Rollup) (ai.Narrative, error)
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
func (f *fakeAISummarizer) WriteNarrative(ctx context.Context, items []narrative.Item, r rollup.Rollup) (ai.Narrative, error) {
	if f.writeNarrativeFn != nil {
		return f.writeNarrativeFn(ctx, items, r)
	}
	return ai.Narrative{}, nil
}

// ── Mock pipeline fetcher ───────────────────────────────────────────────────────

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

// ── Helpers ────────────────────────────────────────────────────────────────

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

// ── WriteNarrative integration tests ──────────────────────────────────────────

// TestWriteNarrativePath verifies the AI path passes narrativeItems to WriteNarrative
// and the sections flow into RenderHighlights via the placeholder conversion.
func TestWriteNarrativePath(t *testing.T) {
	expectedNarr := ai.Narrative{
		Sections: []ai.NarrativeSection{
			{Heading: "Bug Fixes", Body: "Fixed the login bug."},
			{Heading: "Infrastructure", Body: "Improved CI pipeline."},
		},
	}

	fake := &fakeAISummarizer{
		writeNarrativeFn: func(_ context.Context, items []narrative.Item, _ rollup.Rollup) (ai.Narrative, error) {
			if len(items) == 0 {
				t.Error("WriteNarrative called with empty items")
			}
			return expectedNarr, nil
		},
	}

	// Build narrative items.
	narItems := []pipeline.NarrativeItem{
		{
			URL:   "https://github.com/org/repo/issues/1",
			Title: "Fix login bug",
			Body:  "login broken",
		},
	}

	// Convert and call.
	narrItems := pipelineToNarrativeItems(narItems)
	narr, err := fake.WriteNarrative(context.Background(), narrItems, rollup.Rollup{})
	if err != nil {
		t.Fatalf("WriteNarrative error: %v", err)
	}

	// Build placeholder highlights and render.
	var highlights []ai.Highlight
	for _, sec := range narr.Sections {
		highlights = append(highlights, ai.Highlight{Theme: sec.Heading, Summary: sec.Body})
	}

	output := format.RenderHighlights(highlights)
	if output == "" {
		t.Fatal("expected non-empty output")
	}
	if !strings.Contains(output, "Bug Fixes") {
		t.Errorf("expected 'Bug Fixes' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Infrastructure") {
		t.Errorf("expected 'Infrastructure' in output, got:\n%s", output)
	}
}

// TestWriteNarrativeErrorFallback verifies that when WriteNarrative returns an error,
// the caller handles it gracefully (empty narrative, render-only path).
func TestWriteNarrativeErrorFallback(t *testing.T) {
	fake := &fakeAISummarizer{
		writeNarrativeFn: func(_ context.Context, _ []narrative.Item, _ rollup.Rollup) (ai.Narrative, error) {
			return ai.Narrative{}, fmt.Errorf("AI unavailable")
		},
	}

	narItems := pipelineToNarrativeItems([]pipeline.NarrativeItem{
		{URL: "https://github.com/org/repo/issues/1", Title: "thing", Body: "body"},
	})

	narr, err := fake.WriteNarrative(context.Background(), narItems, rollup.Rollup{})
	if err == nil {
		t.Fatal("expected error")
	}

	// Simulate the runHighlights error-handling path.
	if err != nil {
		narr = ai.Narrative{}
	}

	var highlights []ai.Highlight
	for _, sec := range narr.Sections {
		highlights = append(highlights, ai.Highlight{Theme: sec.Heading, Summary: sec.Body})
	}
	// With empty narrative, highlights is nil — RenderHighlights returns "".
	if len(highlights) != 0 {
		t.Errorf("expected empty highlights after error fallback, got %d", len(highlights))
	}
}

// ── collectNarrativeItemsParallel tests ───────────────────────────────────────────

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

// ── End-to-end rendering tests ────────────────────────────────────────────────

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
	if output == "" {
		t.Error("expected non-empty rollup output")
	}
}

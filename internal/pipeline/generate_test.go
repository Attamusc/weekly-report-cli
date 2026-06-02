package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/Attamusc/weekly-report-cli/internal/ai"
	"github.com/Attamusc/weekly-report-cli/internal/derive"
	"github.com/Attamusc/weekly-report-cli/internal/format"
	"github.com/Attamusc/weekly-report-cli/internal/github"
	"github.com/Attamusc/weekly-report-cli/internal/input"
)

// mockFetcher implements IssueFetcher for tests.
type mockFetcher struct {
	issue    github.IssueData
	comments []github.Comment
	err      error
}

func (m *mockFetcher) FetchIssue(_ context.Context, _ input.IssueRef) (github.IssueData, error) {
	return m.issue, m.err
}

func (m *mockFetcher) FetchCommentsSince(_ context.Context, _ input.IssueRef, _ time.Time) ([]github.Comment, error) {
	return m.comments, m.err
}

// makeRef creates a test IssueRef.
func makeRef(url string) input.IssueRef {
	return input.IssueRef{URL: url, Owner: "owner", Repo: "repo", Number: 1}
}

// makeReport creates a structured report comment body.
func makeReport(trending, update string) string {
	return fmt.Sprintf(`<!-- data key="isReport" value="true" -->
<!-- data key="trending" start -->%s<!-- data end -->
<!-- data key="update" start -->%s<!-- data end -->`, trending, update)
}

var (
	now       = time.Now()
	since     = now.AddDate(0, 0, -7)
	sinceDays = 7
)

func TestCollectIssueData_ClosedNoReports(t *testing.T) {
	closedAt := now.AddDate(0, 0, -1)
	fetcher := &mockFetcher{
		issue: github.IssueData{
			Title:    "Closed Issue",
			State:    github.StateClosed,
			ClosedAt: &closedAt,
		},
	}
	data, err := CollectIssueData(context.Background(), fetcher, makeRef("https://github.com/o/r/issues/1"), since, sinceDays)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.Status != derive.Done {
		t.Errorf("expected Done, got %v", data.Status)
	}
	if data.ShouldSummarize {
		t.Error("expected ShouldSummarize=false for closed issue")
	}
	if data.FallbackSummary != SummaryCompleted {
		t.Errorf("expected %q, got %q", SummaryCompleted, data.FallbackSummary)
	}
}

func TestCollectIssueData_NewIssueShaping(t *testing.T) {
	fetcher := &mockFetcher{
		issue: github.IssueData{
			Title:     "Brand New Issue",
			State:     github.StateOpen,
			CreatedAt: now.AddDate(0, 0, -2), // created within the window
		},
	}
	data, err := CollectIssueData(context.Background(), fetcher, makeRef("https://github.com/o/r/issues/2"), since, sinceDays)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.Status != derive.Shaping {
		t.Errorf("expected Shaping, got %v", data.Status)
	}
	if data.Note == nil || data.Note.Kind != format.NoteNewIssueShaping {
		t.Error("expected NoteNewIssueShaping note")
	}
}

func TestCollectIssueData_OldIssueNeedsUpdate(t *testing.T) {
	fetcher := &mockFetcher{
		issue: github.IssueData{
			Title:     "Old Issue",
			State:     github.StateOpen,
			CreatedAt: now.AddDate(0, 0, -30),
		},
	}
	data, err := CollectIssueData(context.Background(), fetcher, makeRef("https://github.com/o/r/issues/3"), since, sinceDays)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.Status != derive.NeedsUpdate {
		t.Errorf("expected NeedsUpdate, got %v", data.Status)
	}
	if data.Note == nil || data.Note.Kind != format.NoteNoUpdatesInWindow {
		t.Error("expected NoteNoUpdatesInWindow note")
	}
}

func TestCollectIssueData_ReportsWithUpdate_ActiveIssue(t *testing.T) {
	commentTime := now.AddDate(0, 0, -1)
	fetcher := &mockFetcher{
		issue: github.IssueData{
			Title: "Active Issue",
			State: github.StateOpen,
		},
		comments: []github.Comment{
			{Body: makeReport("🟢 on track", "Made progress this week"), CreatedAt: commentTime},
		},
	}
	data, err := CollectIssueData(context.Background(), fetcher, makeRef("https://github.com/o/r/issues/4"), since, sinceDays)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !data.ShouldSummarize {
		t.Error("expected ShouldSummarize=true for active issue with updates")
	}
	if len(data.UpdateTexts) == 0 {
		t.Error("expected update texts to be populated")
	}
}

func TestCollectIssueData_ReportsWithUpdate_DoneIssue(t *testing.T) {
	commentTime := now.AddDate(0, 0, -1)
	fetcher := &mockFetcher{
		issue: github.IssueData{
			Title: "Done Issue",
			State: github.StateOpen,
		},
		comments: []github.Comment{
			{Body: makeReport("🟣 done", "Completed everything"), CreatedAt: commentTime},
		},
	}
	data, err := CollectIssueData(context.Background(), fetcher, makeRef("https://github.com/o/r/issues/5"), since, sinceDays)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.ShouldSummarize {
		t.Error("expected ShouldSummarize=false for done status")
	}
	if data.Status != derive.Done {
		t.Errorf("expected Done, got %v", data.Status)
	}
}

func TestCollectIssueData_SemiStructuredFallback(t *testing.T) {
	commentTime := now.AddDate(0, 0, -1)
	fetcher := &mockFetcher{
		issue: github.IssueData{
			Title:     "Semi Structured Issue",
			State:     github.StateOpen,
			CreatedAt: now.AddDate(0, 0, -30),
		},
		comments: []github.Comment{
			{Body: "## Update\nDid some work this week", CreatedAt: commentTime},
		},
	}
	data, err := CollectIssueData(context.Background(), fetcher, makeRef("https://github.com/o/r/issues/6"), since, sinceDays)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Semi-structured comments with no structured update text fall back to unstructured
	if data.Note == nil {
		t.Fatal("expected a note to be set")
	}
	if data.Note.Kind != format.NoteSemiStructuredFallback && data.Note.Kind != format.NoteUnstructuredFallback {
		t.Errorf("expected semi-structured or unstructured fallback note, got %v", data.Note.Kind)
	}
}

func TestCollectIssueData_LabelFallback(t *testing.T) {
	fetcher := &mockFetcher{
		issue: github.IssueData{
			Title:     "Labeled Issue",
			State:     github.StateOpen,
			CreatedAt: now.AddDate(0, 0, -30),
			Labels:    []string{"blocked"},
		},
		comments: []github.Comment{
			{Body: "Just a plain comment, no structure", CreatedAt: now.AddDate(0, 0, -1)},
		},
	}
	data, err := CollectIssueData(context.Background(), fetcher, makeRef("https://github.com/o/r/issues/7"), since, sinceDays)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Label "blocked" should map to a non-Unknown status
	if data.Status == derive.Unknown {
		t.Error("expected label fallback to map blocked label to a status")
	}
}

func TestCollectIssueData_FetchError(t *testing.T) {
	fetcher := &mockFetcher{err: fmt.Errorf("network error")}
	_, err := CollectIssueData(context.Background(), fetcher, makeRef("https://github.com/o/r/issues/8"), since, sinceDays)
	if err == nil {
		t.Error("expected error from failed fetch")
	}
}

func TestCollectIssueData_MultipleReports(t *testing.T) {
	t1 := now.AddDate(0, 0, -1)
	t2 := now.AddDate(0, 0, -3)
	fetcher := &mockFetcher{
		issue: github.IssueData{
			Title: "Multi-report Issue",
			State: github.StateOpen,
		},
		comments: []github.Comment{
			{Body: makeReport("🟢 on track", "Latest update"), CreatedAt: t1},
			{Body: makeReport("🟢 on track", "Earlier update"), CreatedAt: t2},
		},
	}
	data, err := CollectIssueData(context.Background(), fetcher, makeRef("https://github.com/o/r/issues/9"), since, sinceDays)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.Note == nil || data.Note.Kind != format.NoteMultipleUpdates {
		t.Errorf("expected NoteMultipleUpdates note, got %v", data.Note)
	}
	if len(data.UpdateTexts) < 2 {
		t.Errorf("expected 2 update texts, got %d", len(data.UpdateTexts))
	}
}

func TestAssembleGenerateResults_WithBatchResults(t *testing.T) {
	logger := slog.Default()
	allData := []IssueData{
		{
			IssueURL:        "https://github.com/o/r/issues/1",
			IssueTitle:      "Test Issue",
			Status:          derive.OnTrack,
			FallbackSummary: "fallback",
		},
	}
	batchResults := map[string]ai.BatchResult{
		"https://github.com/o/r/issues/1": {Summary: "AI summary"},
	}
	rows, notes := AssembleGenerateResults(allData, batchResults, false, logger)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].UpdateMD != "AI summary" {
		t.Errorf("expected AI summary in row, got %q", rows[0].UpdateMD)
	}
	if len(notes) != 0 {
		t.Errorf("expected 0 notes, got %d", len(notes))
	}
}

func TestAssembleGenerateResults_FallbackWhenNoAI(t *testing.T) {
	logger := slog.Default()
	allData := []IssueData{
		{
			IssueURL:        "https://github.com/o/r/issues/2",
			IssueTitle:      "Test Issue 2",
			Status:          derive.NeedsUpdate,
			FallbackSummary: "no AI summary",
		},
	}
	rows, _ := AssembleGenerateResults(allData, map[string]ai.BatchResult{}, false, logger)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].UpdateMD != "no AI summary" {
		t.Errorf("expected fallback summary, got %q", rows[0].UpdateMD)
	}
}

func TestAssembleGenerateResults_WithNote(t *testing.T) {
	logger := slog.Default()
	allData := []IssueData{
		{
			IssueURL:        "https://github.com/o/r/issues/3",
			IssueTitle:      "Noted Issue",
			Status:          derive.NeedsUpdate,
			FallbackSummary: "update needed",
			Note:            &format.Note{Kind: format.NoteNoUpdatesInWindow, IssueURL: "https://github.com/o/r/issues/3"},
		},
	}
	_, notes := AssembleGenerateResults(allData, map[string]ai.BatchResult{}, false, logger)
	if len(notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(notes))
	}
	if notes[0].Kind != format.NoteNoUpdatesInWindow {
		t.Errorf("expected NoteNoUpdatesInWindow, got %v", notes[0].Kind)
	}
}

func TestCreateResultFromData_ThreadsMetadata(t *testing.T) {
	data := IssueData{
		IssueURL:        "https://github.com/o/r/issues/10",
		IssueTitle:      "Test Issue",
		Status:          derive.OnTrack,
		FallbackSummary: "some update",
		Assignees:       []string{"alice", "bob"},
		Labels:          []string{"bug", "priority"},
		ExtraColumns:    map[string]string{"Sprint": "Sprint 1", "Status": "In Progress"},
	}

	result := CreateResultFromData(data, "AI summary")

	if result.Row == nil {
		t.Fatal("expected non-nil row")
	}
	row := result.Row

	if len(row.Assignees) != 2 || row.Assignees[0] != "alice" || row.Assignees[1] != "bob" {
		t.Errorf("unexpected assignees: %v", row.Assignees)
	}
	if len(row.Labels) != 2 || row.Labels[0] != "bug" || row.Labels[1] != "priority" {
		t.Errorf("unexpected labels: %v", row.Labels)
	}
	if row.ExtraColumns["Sprint"] != "Sprint 1" || row.ExtraColumns["Status"] != "In Progress" {
		t.Errorf("unexpected extra columns: %v", row.ExtraColumns)
	}
	if row.UpdateMD != "AI summary" {
		t.Errorf("unexpected update: %q", row.UpdateMD)
	}
}

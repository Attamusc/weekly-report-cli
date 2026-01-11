package report

import (
	"testing"
	"time"

	"github.com/Attamusc/weekly-report-cli/internal/github"
)

func TestSelectReports_MultipleReports(t *testing.T) {
	// Create test time window
	baseTime := time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC)
	sinceTime := baseTime.Add(24 * time.Hour) // 2025-08-02

	comments := []github.Comment{
		{
			Body: `<!-- data key="isReport" value="true" -->
<!-- data key="trending" start -->🟢 on track<!-- data end -->
<!-- data key="update" start -->First update<!-- data end -->`,
			CreatedAt: sinceTime.Add(1 * time.Hour), // 2025-08-02 01:00
			Author:    "user1",
			URL:       "comment-url-1",
		},
		{
			Body: `<!-- data key="isReport" value="true" -->
<!-- data key="trending" start -->🟣 done<!-- data end -->
<!-- data key="update" start -->Final update<!-- data end -->`,
			CreatedAt: sinceTime.Add(3 * time.Hour), // 2025-08-02 03:00 (newer)
			Author:    "user2",
			URL:       "comment-url-2",
		},
		{
			Body: `<!-- data key="isReport" value="true" -->
<!-- data key="trending" start -->🟡 at risk<!-- data end -->
<!-- data key="update" start -->Middle update<!-- data end -->`,
			CreatedAt: sinceTime.Add(2 * time.Hour), // 2025-08-02 02:00
			Author:    "user3",
			URL:       "comment-url-3",
		},
	}

	reports := SelectReports(comments, sinceTime)

	// Should return all 3 reports
	if len(reports) != 3 {
		t.Fatalf("expected 3 reports, got %d", len(reports))
	}

	// Should be sorted newest-first
	expectedOrder := []string{"🟣 done", "🟡 at risk", "🟢 on track"}
	for i, report := range reports {
		if report.TrendingRaw != expectedOrder[i] {
			t.Errorf("report %d: expected trending '%s', got '%s'", i, expectedOrder[i], report.TrendingRaw)
		}
	}

	// Verify timestamps are in descending order
	for i := 1; i < len(reports); i++ {
		if reports[i].CreatedAt.After(reports[i-1].CreatedAt) {
			t.Errorf("reports not sorted newest-first: report %d (%v) is newer than report %d (%v)",
				i, reports[i].CreatedAt, i-1, reports[i-1].CreatedAt)
		}
	}
}

func TestSelectReports_TimeWindowFiltering(t *testing.T) {
	baseTime := time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC)
	sinceTime := baseTime.Add(24 * time.Hour) // 2025-08-02

	comments := []github.Comment{
		{
			Body: `<!-- data key="isReport" value="true" -->
<!-- data key="trending" start -->old<!-- data end -->`,
			CreatedAt: baseTime, // Before since time
			URL:       "old-comment",
		},
		{
			Body: `<!-- data key="isReport" value="true" -->
<!-- data key="trending" start -->exactly at since<!-- data end -->`,
			CreatedAt: sinceTime, // Exactly at since time
			URL:       "exact-comment",
		},
		{
			Body: `<!-- data key="isReport" value="true" -->
<!-- data key="trending" start -->after since<!-- data end -->`,
			CreatedAt: sinceTime.Add(1 * time.Hour), // After since time
			URL:       "new-comment",
		},
	}

	reports := SelectReports(comments, sinceTime)

	// Should include comments at or after since time
	if len(reports) != 2 {
		t.Fatalf("expected 2 reports, got %d", len(reports))
	}

	// Check that old comment was filtered out
	for _, report := range reports {
		if report.TrendingRaw == "old" {
			t.Error("old comment should have been filtered out")
		}
	}

	// Check that both valid comments are included
	trendingValues := make(map[string]bool)
	for _, report := range reports {
		trendingValues[report.TrendingRaw] = true
	}

	if !trendingValues["exactly at since"] {
		t.Error("comment exactly at since time should be included")
	}
	if !trendingValues["after since"] {
		t.Error("comment after since time should be included")
	}
}

func TestSelectReports_NoReports(t *testing.T) {
	sinceTime := time.Now()

	// Test with no comments
	reports := SelectReports([]github.Comment{}, sinceTime)
	if len(reports) != 0 {
		t.Errorf("expected 0 reports for empty input, got %d", len(reports))
	}

	// Test with comments but no valid reports
	comments := []github.Comment{
		{
			Body:      "Just a regular comment without report markers",
			CreatedAt: sinceTime.Add(1 * time.Hour),
			URL:       "regular-comment",
		},
		{
			Body: `<!-- data key="isReport" value="false" -->
<!-- data key="trending" start -->not a report<!-- data end -->`,
			CreatedAt: sinceTime.Add(2 * time.Hour),
			URL:       "false-marker-comment",
		},
	}

	reports = SelectReports(comments, sinceTime)
	if len(reports) != 0 {
		t.Errorf("expected 0 reports for comments without valid reports, got %d", len(reports))
	}
}

func TestSelectReports_OneReport(t *testing.T) {
	sinceTime := time.Now().Add(-1 * time.Hour)

	comments := []github.Comment{
		{
			Body: `<!-- data key="isReport" value="true" -->
<!-- data key="trending" start -->🟢 on track<!-- data end -->
<!-- data key="target_date" start -->2025-08-15<!-- data end -->
<!-- data key="update" start -->Making steady progress<!-- data end -->`,
			CreatedAt: sinceTime.Add(30 * time.Minute),
			Author:    "developer",
			URL:       "single-report-url",
		},
	}

	reports := SelectReports(comments, sinceTime)

	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}

	report := reports[0]
	if report.TrendingRaw != "🟢 on track" {
		t.Errorf("expected trending '🟢 on track', got '%s'", report.TrendingRaw)
	}
	if report.TargetDate != "2025-08-15" {
		t.Errorf("expected target_date '2025-08-15', got '%s'", report.TargetDate)
	}
	if report.UpdateRaw != "Making steady progress" {
		t.Errorf("expected update 'Making steady progress', got '%s'", report.UpdateRaw)
	}
	if report.SourceURL != "single-report-url" {
		t.Errorf("expected SourceURL 'single-report-url', got '%s'", report.SourceURL)
	}
}

func TestSelectReports_MixedValidAndInvalid(t *testing.T) {
	sinceTime := time.Now().Add(-1 * time.Hour)

	comments := []github.Comment{
		{
			Body:      "Regular comment",
			CreatedAt: sinceTime.Add(10 * time.Minute),
			URL:       "regular",
		},
		{
			Body: `<!-- data key="isReport" value="true" -->
<!-- data key="trending" start -->valid report<!-- data end -->`,
			CreatedAt: sinceTime.Add(20 * time.Minute),
			URL:       "valid-report-1",
		},
		{
			Body: `<!-- data key="isReport" value="true" -->
<!-- no valid data blocks -->`,
			CreatedAt: sinceTime.Add(30 * time.Minute),
			URL:       "invalid-report",
		},
		{
			Body: `<!-- data key="isReport" value="true" -->
<!-- data key="trending" start -->another valid<!-- data end -->`,
			CreatedAt: sinceTime.Add(40 * time.Minute),
			URL:       "valid-report-2",
		},
	}

	reports := SelectReports(comments, sinceTime)

	// Should only extract the 2 valid reports
	if len(reports) != 2 {
		t.Fatalf("expected 2 valid reports, got %d", len(reports))
	}

	// Check newest-first ordering
	if reports[0].TrendingRaw != "another valid" {
		t.Errorf("expected first report to be 'another valid', got '%s'", reports[0].TrendingRaw)
	}
	if reports[1].TrendingRaw != "valid report" {
		t.Errorf("expected second report to be 'valid report', got '%s'", reports[1].TrendingRaw)
	}
}

func TestSelectMostRecentComment(t *testing.T) {
	tests := []struct {
		name     string
		comments []github.Comment
		wantBody string
		wantOK   bool
	}{
		{
			name:     "empty comments",
			comments: []github.Comment{},
			wantBody: "",
			wantOK:   false,
		},
		{
			name: "single comment",
			comments: []github.Comment{
				{Body: "only comment", CreatedAt: time.Now(), URL: "url-1"},
			},
			wantBody: "only comment",
			wantOK:   true,
		},
		{
			name: "multiple comments returns most recent",
			comments: []github.Comment{
				{Body: "oldest comment", CreatedAt: time.Now().Add(-2 * time.Hour), URL: "url-1"},
				{Body: "middle comment", CreatedAt: time.Now().Add(-1 * time.Hour), URL: "url-2"},
				{Body: "newest comment", CreatedAt: time.Now(), URL: "url-3"},
			},
			wantBody: "newest comment",
			wantOK:   true,
		},
		{
			name: "most recent comment has empty body",
			comments: []github.Comment{
				{Body: "has content", CreatedAt: time.Now().Add(-1 * time.Hour), URL: "url-1"},
				{Body: "", CreatedAt: time.Now(), URL: "url-2"},
			},
			wantBody: "",
			wantOK:   false,
		},
		{
			name: "most recent comment is whitespace only",
			comments: []github.Comment{
				{Body: "has content", CreatedAt: time.Now().Add(-1 * time.Hour), URL: "url-1"},
				{Body: "   \n\t  ", CreatedAt: time.Now(), URL: "url-2"},
			},
			wantBody: "",
			wantOK:   false,
		},
		{
			name: "trims whitespace from body",
			comments: []github.Comment{
				{Body: "  trimmed content  \n", CreatedAt: time.Now(), URL: "url-1"},
			},
			wantBody: "trimmed content",
			wantOK:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, ok := SelectMostRecentComment(tc.comments)
			if ok != tc.wantOK {
				t.Errorf("SelectMostRecentComment() ok = %t, want %t", ok, tc.wantOK)
			}
			if body != tc.wantBody {
				t.Errorf("SelectMostRecentComment() body = %q, want %q", body, tc.wantBody)
			}
		})
	}
}

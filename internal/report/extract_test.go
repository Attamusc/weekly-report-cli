package report

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestParseReport_ValidReport(t *testing.T) {
	// Test with the exact sample from the specification
	body := `Some discussion text here.

<!-- data key="isReport" value="true" -->
<!-- data key="trending" start -->🟣 done<!-- data end -->
<!-- data key="target_date" start -->2025-08-06<!-- data end -->
<!-- data key="update" start -->Completed feature implementation
Added comprehensive tests
Ready for deployment<!-- data end -->

More discussion after the report.`

	createdAt := time.Date(2025, 8, 6, 10, 30, 0, 0, time.UTC)
	sourceURL := "https://github.com/owner/repo/issues/123#issuecomment-456"

	report, ok := ParseReport(body, createdAt, sourceURL)

	if !ok {
		t.Fatal("expected successful report parsing")
	}

	// Verify extracted data matches spec
	if report.TrendingRaw != "🟣 done" {
		t.Errorf("expected trending '🟣 done', got '%s'", report.TrendingRaw)
	}

	if report.TargetDate != "2025-08-06" {
		t.Errorf("expected target_date '2025-08-06', got '%s'", report.TargetDate)
	}

	expectedUpdate := "Completed feature implementation\nAdded comprehensive tests\nReady for deployment"
	if report.UpdateRaw != expectedUpdate {
		t.Errorf("expected multiline update:\n%s\ngot:\n%s", expectedUpdate, report.UpdateRaw)
	}

	if report.CreatedAt != createdAt {
		t.Errorf("expected CreatedAt %v, got %v", createdAt, report.CreatedAt)
	}

	if report.SourceURL != sourceURL {
		t.Errorf("expected SourceURL %s, got %s", sourceURL, report.SourceURL)
	}
}

func TestParseReport_CaseInsensitiveMarker(t *testing.T) {
	// Test case-insensitive report marker
	testCases := []string{
		`<!-- data key="isReport" value="true" -->`,
		`<!-- DATA KEY="isReport" VALUE="true" -->`,
		`<!-- Data Key="IsReport" Value="True" -->`,
		`<!--data key="isreport" value="true"-->`,
		`<!--   DATA   KEY  =  "ISREPORT"   VALUE  =  "TRUE"   -->`,
	}

	baseBody := `%s
<!-- data key="trending" start -->green<!-- data end -->`

	for i, marker := range testCases {
		body := fmt.Sprintf(baseBody, marker)
		_, ok := ParseReport(body, time.Now(), "test-url")

		if !ok {
			t.Errorf("test case %d failed: marker should be case-insensitive: %s", i, marker)
		}
	}
}

func TestParseReport_PartialData(t *testing.T) {
	testCases := []struct {
		name string
		body string
		want bool
	}{
		{
			name: "only trending",
			body: `<!-- data key="isReport" value="true" -->
<!-- data key="trending" start -->yellow<!-- data end -->`,
			want: true,
		},
		{
			name: "only target_date",
			body: `<!-- data key="isReport" value="true" -->
<!-- data key="target_date" start -->2025-12-01<!-- data end -->`,
			want: true,
		},
		{
			name: "only update",
			body: `<!-- data key="isReport" value="true" -->
<!-- data key="update" start -->Working on feature X<!-- data end -->`,
			want: true,
		},
		{
			name: "trending and update",
			body: `<!-- data key="isReport" value="true" -->
<!-- data key="trending" start -->red<!-- data end -->
<!-- data key="update" start -->Blocked by dependency<!-- data end -->`,
			want: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := ParseReport(tc.body, time.Now(), "test-url")
			if ok != tc.want {
				t.Errorf("expected %t, got %t", tc.want, ok)
			}
		})
	}
}

func TestParseReport_InvalidCases(t *testing.T) {
	testCases := []struct {
		name string
		body string
	}{
		{
			name: "no report marker",
			body: `<!-- data key="trending" start -->green<!-- data end -->`,
		},
		{
			name: "wrong marker value",
			body: `<!-- data key="isReport" value="false" -->
<!-- data key="trending" start -->green<!-- data end -->`,
		},
		{
			name: "marker but no data blocks",
			body: `<!-- data key="isReport" value="true" -->
Some text without data blocks.`,
		},
		{
			name: "marker but empty data blocks",
			body: `<!-- data key="isReport" value="true" -->
<!-- data key="trending" start --><!-- data end -->
<!-- data key="update" start -->   <!-- data end -->`,
		},
		{
			name: "malformed data blocks",
			body: `<!-- data key="isReport" value="true" -->
<!-- data key="trending" start -->green
<!-- missing end tag -->`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := ParseReport(tc.body, time.Now(), "test-url")
			if ok {
				t.Error("expected parsing to fail for invalid case")
			}
		})
	}
}

func TestParseReport_WhitespaceAndUnicode(t *testing.T) {
	body := `<!-- data key="isReport" value="true" -->
<!--   data   key  =  "trending"   start   -->  🟢 on track  <!--   data   end   -->
<!-- data key="target_date" start -->
  2025-09-15  
<!-- data end -->
<!-- data key="update" start -->
Progress update with émojis and ünicode 🚀
Multiple lines with    extra spaces    
<!-- data end -->`

	report, ok := ParseReport(body, time.Now(), "test")

	if !ok {
		t.Fatal("expected successful parsing with whitespace and unicode")
	}

	if report.TrendingRaw != "🟢 on track" {
		t.Errorf("expected trending '🟢 on track', got '%s'", report.TrendingRaw)
	}

	if report.TargetDate != "2025-09-15" {
		t.Errorf("expected target_date '2025-09-15', got '%s'", report.TargetDate)
	}

	expectedUpdate := "Progress update with émojis and ünicode 🚀\nMultiple lines with    extra spaces"
	if report.UpdateRaw != expectedUpdate {
		t.Errorf("expected update with unicode:\n%s\ngot:\n%s", expectedUpdate, report.UpdateRaw)
	}
}

// ========== ParseSemiStructured Tests ==========

func TestParseSemiStructured_EmojiStatus(t *testing.T) {
	body := "### Trending\n\n🟢 on track\n"
	createdAt := time.Date(2025, 3, 5, 14, 0, 0, 0, time.UTC)
	sourceURL := "https://github.com/owner/repo/issues/1#issuecomment-100"

	report, ok := ParseSemiStructured(body, createdAt, sourceURL)
	if !ok {
		t.Fatal("expected successful semi-structured parsing")
	}
	if report.TrendingRaw != "🟢 on track" {
		t.Errorf("expected trending '🟢 on track', got %q", report.TrendingRaw)
	}
	if report.CreatedAt != createdAt {
		t.Errorf("expected CreatedAt %v, got %v", createdAt, report.CreatedAt)
	}
	if report.SourceURL != sourceURL {
		t.Errorf("expected SourceURL %q, got %q", sourceURL, report.SourceURL)
	}
}

func TestParseSemiStructured_TextStatus(t *testing.T) {
	body := "### Trending\n\non track\n"

	report, ok := ParseSemiStructured(body, time.Now(), "url")
	if !ok {
		t.Fatal("expected successful semi-structured parsing")
	}
	if report.TrendingRaw != "on track" {
		t.Errorf("expected trending 'on track', got %q", report.TrendingRaw)
	}
}

func TestParseSemiStructured_WithUpdate(t *testing.T) {
	body := `### Trending

🟢 on track

### Update

**Completed:** implemented the feature
Added tests for all edge cases
`

	report, ok := ParseSemiStructured(body, time.Now(), "url")
	if !ok {
		t.Fatal("expected successful semi-structured parsing")
	}
	if report.TrendingRaw != "🟢 on track" {
		t.Errorf("expected trending '🟢 on track', got %q", report.TrendingRaw)
	}
	if report.UpdateRaw == "" {
		t.Fatal("expected non-empty UpdateRaw")
	}
	if !strings.Contains(report.UpdateRaw, "Completed") {
		t.Errorf("expected update to contain 'Completed', got %q", report.UpdateRaw)
	}
}

func TestParseSemiStructured_WithTargetDate(t *testing.T) {
	body := `### Trending

🟡 at risk

### Target Date

2025-08-15
`

	report, ok := ParseSemiStructured(body, time.Now(), "url")
	if !ok {
		t.Fatal("expected successful semi-structured parsing")
	}
	if report.TrendingRaw != "🟡 at risk" {
		t.Errorf("expected trending '🟡 at risk', got %q", report.TrendingRaw)
	}
	if report.TargetDate != "2025-08-15" {
		t.Errorf("expected target date '2025-08-15', got %q", report.TargetDate)
	}
}

func TestParseSemiStructured_AllSections(t *testing.T) {
	body := `### Trending

🔴 blocked

### Update

Blocked on upstream dependency.
Waiting for team to resolve issue.

### Target Date

2025-09-01
`

	report, ok := ParseSemiStructured(body, time.Now(), "url")
	if !ok {
		t.Fatal("expected successful semi-structured parsing")
	}
	if report.TrendingRaw != "🔴 blocked" {
		t.Errorf("expected trending '🔴 blocked', got %q", report.TrendingRaw)
	}
	if !strings.Contains(report.UpdateRaw, "upstream dependency") {
		t.Errorf("expected update to contain 'upstream dependency', got %q", report.UpdateRaw)
	}
	if report.TargetDate != "2025-09-01" {
		t.Errorf("expected target date '2025-09-01', got %q", report.TargetDate)
	}
}

func TestParseSemiStructured_DifferentHeadingLevels(t *testing.T) {
	testCases := []struct {
		name string
		body string
	}{
		{
			name: "h1 heading",
			body: "# Trending\n\n🟢 on track\n",
		},
		{
			name: "h2 heading",
			body: "## Trending\n\n🟢 on track\n",
		},
		{
			name: "h4 heading",
			body: "#### Trending\n\n🟢 on track\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := ParseSemiStructured(tc.body, time.Now(), "url")
			if !ok {
				t.Errorf("expected successful parsing for %s", tc.name)
			}
		})
	}
}

func TestParseSemiStructured_SubHeadingsPreserved(t *testing.T) {
	body := `### Trending

🟢 on track

### Update

#### Completed
- Feature A done
- Feature B done

#### In Progress
- Feature C started
`

	report, ok := ParseSemiStructured(body, time.Now(), "url")
	if !ok {
		t.Fatal("expected successful semi-structured parsing")
	}
	// Sub-headings (#### Completed, #### In Progress) should be preserved
	// because knownSectionHeadingRegex only matches known headings
	if !strings.Contains(report.UpdateRaw, "#### Completed") {
		t.Errorf("expected update to preserve '#### Completed' sub-heading, got %q", report.UpdateRaw)
	}
	if !strings.Contains(report.UpdateRaw, "#### In Progress") {
		t.Errorf("expected update to preserve '#### In Progress' sub-heading, got %q", report.UpdateRaw)
	}
}

func TestParseSemiStructured_NoTrendingHeading(t *testing.T) {
	body := `### Update

Some update text here.
`

	_, ok := ParseSemiStructured(body, time.Now(), "url")
	if ok {
		t.Error("expected parsing to fail when no trending heading present")
	}
}

func TestParseSemiStructured_UnrecognizedTrending(t *testing.T) {
	body := `### Trending

just some random text about project management
`

	_, ok := ParseSemiStructured(body, time.Now(), "url")
	if ok {
		t.Error("expected parsing to fail when trending text is unrecognized")
	}
}

func TestParseSemiStructured_HasHTMLMarkers(t *testing.T) {
	body := `<!-- data key="isReport" value="true" -->
### Trending

🟢 on track
`

	_, ok := ParseSemiStructured(body, time.Now(), "url")
	if ok {
		t.Error("expected parsing to fail when HTML report markers are present")
	}
}

func TestParseSemiStructured_EmptyBody(t *testing.T) {
	_, ok := ParseSemiStructured("", time.Now(), "url")
	if ok {
		t.Error("expected parsing to fail for empty body")
	}
}

func TestParseSemiStructured_WhitespaceAroundHeading(t *testing.T) {
	// Extra whitespace around status text should be handled
	body := "###   Trending  \n\n  🟢 on track  \n"

	report, ok := ParseSemiStructured(body, time.Now(), "url")
	if !ok {
		t.Fatal("expected successful parsing with whitespace around heading")
	}
	if report.TrendingRaw != "🟢 on track" {
		t.Errorf("expected trimmed trending '🟢 on track', got %q", report.TrendingRaw)
	}
}

func TestParseSemiStructured_EmptyTrendingContent(t *testing.T) {
	body := "### Trending\n\n### Update\n\nSome update\n"

	_, ok := ParseSemiStructured(body, time.Now(), "url")
	if ok {
		t.Error("expected parsing to fail when trending section is empty")
	}
}

func TestParseSemiStructured_KnownLimitation_SubstringMatch(t *testing.T) {
	// Known limitation: MapTrending() uses substring matching, so "green with envy"
	// matches OnTrack via the "green" pattern. This is inherited from MapTrending()
	// and is not a new bug introduced by ParseSemiStructured().
	body := "### Trending\n\ngreen with envy\n"

	_, ok := ParseSemiStructured(body, time.Now(), "url")
	if !ok {
		t.Log("Known limitation: 'green with envy' matches OnTrack via substring match in MapTrending()")
		t.Fatal("expected this known limitation to cause a match (test documents inherited behavior)")
	}
}

func TestParseSemiStructured_RealWorldExample(t *testing.T) {
	// Simulates the real-world example from github/graphql-platform#2458
	body := `### Trending

🟢 on track

### Update

**Completed:** Finished implementing the GraphQL schema changes.
Updated documentation for the new endpoints.

### Summary

Overall the project is progressing well with no blockers.
`

	report, ok := ParseSemiStructured(body, time.Now(), "https://github.com/org/repo/issues/2458#issuecomment-999")
	if !ok {
		t.Fatal("expected successful parsing of real-world example")
	}
	if report.TrendingRaw != "🟢 on track" {
		t.Errorf("expected trending '🟢 on track', got %q", report.TrendingRaw)
	}
	if !strings.Contains(report.UpdateRaw, "GraphQL schema changes") {
		t.Errorf("expected update to contain 'GraphQL schema changes', got %q", report.UpdateRaw)
	}
	// TargetDate should be empty since no target date section
	if report.TargetDate != "" {
		t.Errorf("expected empty target date, got %q", report.TargetDate)
	}
}

func TestParseReport_MultipleDataBlocks(t *testing.T) {
	// Test with multiple data blocks - should extract all matching keys
	body := `<!-- data key="isReport" value="true" -->
<!-- data key="trending" start -->final status<!-- data end -->
<!-- data key="target_date" start -->2025-08-15<!-- data end -->

Some text in between.

<!-- data key="update" start -->Latest progress update<!-- data end -->`

	report, ok := ParseReport(body, time.Now(), "test")

	if !ok {
		t.Fatal("expected successful parsing")
	}

	// Should extract data from all matching blocks
	if report.TrendingRaw != "final status" {
		t.Errorf("expected trending 'final status', got '%s'", report.TrendingRaw)
	}
	if report.TargetDate != "2025-08-15" {
		t.Errorf("expected target_date '2025-08-15', got '%s'", report.TargetDate)
	}
	if report.UpdateRaw != "Latest progress update" {
		t.Errorf("expected update 'Latest progress update', got '%s'", report.UpdateRaw)
	}
}

package report

import (
	"fmt"
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


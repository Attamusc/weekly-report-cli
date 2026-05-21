package format

import (
	"strings"
	"testing"

	"github.com/Attamusc/weekly-report-cli/internal/ai"
)

func TestRenderHighlights(t *testing.T) {
	tests := []struct {
		name       string
		highlights []ai.Highlight
		want       string // empty means check for empty output
		wantSubs   []string
	}{
		{
			name:       "empty highlights returns empty string",
			highlights: nil,
			want:       "",
		},
		{
			name: "single theme",
			highlights: []ai.Highlight{
				{Theme: "Bug Fixes", Title: "Fix nil pointer", URL: "https://github.com/org/repo/pull/1", Summary: "Fixes crash on login."},
			},
			wantSubs: []string{
				"## Bug Fixes",
				"[Fix nil pointer](https://github.com/org/repo/pull/1)",
				"Fixes crash on login.",
			},
		},
		{
			name: "multiple themes sorted alphabetically",
			highlights: []ai.Highlight{
				{Theme: "Support & Reliability", Title: "Add retry", URL: "https://github.com/org/repo/pull/2", Summary: "Adds retries."},
				{Theme: "Bug Fixes", Title: "Fix crash", URL: "https://github.com/org/repo/pull/1", Summary: "Fixes crash."},
				{Theme: "Infrastructure", Title: "Upgrade CI", URL: "https://github.com/org/repo/pull/3", Summary: "Faster builds."},
			},
			wantSubs: []string{
				"## Bug Fixes",
				"## Infrastructure",
				"## Support & Reliability",
			},
		},
		{
			name: "title with pipe character is escaped",
			highlights: []ai.Highlight{
				{Theme: "Bug Fixes", Title: "Fix A | B issue", URL: "https://example.com/1", Summary: "Fixed."},
			},
			wantSubs: []string{
				"Fix A \\| B issue",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := RenderHighlights(tc.highlights)

			if tc.want != "" || (tc.highlights == nil && got != "") {
				if got != tc.want {
					t.Errorf("got %q, want %q", got, tc.want)
				}
				return
			}

			for _, sub := range tc.wantSubs {
				if !strings.Contains(got, sub) {
					t.Errorf("output missing %q\ngot:\n%s", sub, got)
				}
			}
		})
	}
}

func TestRenderHighlights_ThemeOrder(t *testing.T) {
	highlights := []ai.Highlight{
		{Theme: "Zebra", Title: "Z", URL: "https://example.com/z", Summary: "Z."},
		{Theme: "Alpha", Title: "A", URL: "https://example.com/a", Summary: "A."},
	}

	got := RenderHighlights(highlights)
	alphaIdx := strings.Index(got, "## Alpha")
	zebraIdx := strings.Index(got, "## Zebra")

	if alphaIdx == -1 || zebraIdx == -1 {
		t.Fatalf("missing theme headers in output:\n%s", got)
	}
	if alphaIdx >= zebraIdx {
		t.Errorf("expected Alpha before Zebra, got Alpha at %d, Zebra at %d", alphaIdx, zebraIdx)
	}
}

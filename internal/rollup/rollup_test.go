package rollup

import (
	"math"
	"testing"
	"time"

	"github.com/Attamusc/weekly-report-cli/internal/input"
)

var (
	t0    = time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC)
	since = time.Date(2026, 5, 26, 0, 0, 0, 0, time.UTC)
	t1    = time.Date(2026, 5, 27, 0, 0, 0, 0, time.UTC) // after since
	t2    = time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC) // after since, newer
)

func ref(repo string, num int, comments int, isPR bool, author string, updatedAt time.Time) input.IssueRef {
	owner := "o"
	url := "https://github.com/" + owner + "/" + repo + "/issues/" + itoa(num)
	return input.IssueRef{
		Owner:        owner,
		Repo:         repo,
		Number:       num,
		URL:          url,
		IsPR:         isPR,
		AuthorLogin:  author,
		CommentCount: comments,
		ClosedAt:     nil,
		UpdatedAt:    updatedAt,
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

func TestCompute(t *testing.T) {
	tests := []struct {
		name          string
		refs          []input.IssueRef
		wantLen       int
		wantFirstURL  string
		wantMedian    float64
		wantByAuthors []string // expected keys (sorted alpha not checked here)
	}{
		{
			name:          "empty input",
			refs:          nil,
			wantLen:       0,
			wantMedian:    0,
			wantByAuthors: []string{},
		},
		{
			name: "all-zero scores",
			refs: []input.IssueRef{
				ref("r", 1, 0, false, "alice", t0),
				ref("r", 2, 0, false, "bob", t0),
			},
			wantLen:    2,
			wantMedian: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := Compute(tc.refs, since)
			if len(r.AllSorted) != tc.wantLen {
				t.Errorf("AllSorted len = %d, want %d", len(r.AllSorted), tc.wantLen)
			}
			if r.Median != tc.wantMedian {
				t.Errorf("Median = %v, want %v", r.Median, tc.wantMedian)
			}
			if tc.wantFirstURL != "" && len(r.AllSorted) > 0 && r.AllSorted[0].Ref.URL != tc.wantFirstURL {
				t.Errorf("AllSorted[0].URL = %s, want %s", r.AllSorted[0].Ref.URL, tc.wantFirstURL)
			}
		})
	}
}

func TestSingletonRepoMitigation(t *testing.T) {
	// busy-repo has 3 items (meets threshold); lone-repo has 1 (below threshold).
	// lone-repo's item has raw=2 (is_pr), busy-repo items have raw=1 each.
	// Without mitigation, lone-repo item would score 1.0 (2/2).
	// With mitigation, it scores against global max=2: 2/2=1.0 still,
	// but busy-repo max=1 and there are 3 items so per-repo: 1/1=1.0 also.
	// Use a case where mitigation clearly helps: busy-repo max=3, lone=2.
	refs := []input.IssueRef{
		ref("busy", 1, 3, false, "alice", t0), // raw=3
		ref("busy", 2, 2, false, "alice", t0), // raw=2
		ref("busy", 3, 1, false, "alice", t0), // raw=1
		ref("lone", 1, 0, true, "bob", t0),    // raw=2 (isPR), lone repo (1 item)
	}
	r := Compute(refs, since)
	// busy-repo: count=3 >= threshold, max=3; scores: 1.0, 0.667, 0.333
	// lone-repo: count=1 < threshold, normalizes by global max=3; score=2/3≈0.667
	// So lone item should NOT score 1.0.
	var loneScore float64
	for _, it := range r.AllSorted {
		if it.Ref.Repo == "lone" {
			loneScore = it.Score
		}
	}
	if loneScore >= 1.0 {
		t.Errorf("lone-repo item score = %v, want < 1.0 (singleton mitigation failed)", loneScore)
	}
	expected := 2.0 / 3.0
	if math.Abs(loneScore-expected) > 1e-9 {
		t.Errorf("lone-repo item score = %v, want %v", loneScore, expected)
	}
}

func TestTieBreaking(t *testing.T) {
	// Two items in the same repo with same raw score.
	// One has later UpdatedAt → should sort first.
	refs := []input.IssueRef{
		ref("r", 1, 2, false, "alice", t1), // older
		ref("r", 2, 2, false, "alice", t2), // newer
	}
	r := Compute(refs, since)
	if len(r.AllSorted) != 2 {
		t.Fatalf("expected 2 items")
	}
	if r.AllSorted[0].Ref.Number != 2 {
		t.Errorf("expected newer item (2) first, got %d", r.AllSorted[0].Ref.Number)
	}
}

func TestTieBreakingURL(t *testing.T) {
	// Same repo, same raw, same updatedAt → sort by URL asc.
	refs := []input.IssueRef{
		ref("r", 2, 2, false, "alice", t1),
		ref("r", 1, 2, false, "alice", t1),
	}
	r := Compute(refs, since)
	if r.AllSorted[0].Ref.URL > r.AllSorted[1].Ref.URL {
		t.Errorf("expected URL-ascending tie-break, got %s before %s",
			r.AllSorted[0].Ref.URL, r.AllSorted[1].Ref.URL)
	}
}

func TestCut(t *testing.T) {
	refs := []input.IssueRef{
		ref("r", 1, 5, false, "a", t0), // raw=5, score=1.0
		ref("r", 2, 3, false, "b", t0), // raw=3, score=0.6
		ref("r", 3, 1, false, "c", t0), // raw=1, score=0.2
		ref("r", 4, 4, false, "d", t0), // raw=4, score=0.8
	}
	r := Compute(refs, since)
	// scores: 1.0, 0.8, 0.6, 0.2 → median of 4 = (0.6+0.8)/2 = 0.7
	wantMedian := 0.7
	if r.Median != wantMedian {
		t.Errorf("Median = %v, want %v", r.Median, wantMedian)
	}

	// topN < items where score >= median
	cut2 := r.Cut(2)
	if len(cut2) != 2 {
		t.Errorf("Cut(2) len = %d, want 2", len(cut2))
	}

	// topN > items where score >= median (2 items: 1.0 and 0.8)
	cut10 := r.Cut(10)
	if len(cut10) != 2 {
		t.Errorf("Cut(10) len = %d, want 2 (only items >= median)", len(cut10))
	}

	// verify non-nil on empty
	r2 := Compute(nil, since)
	empty := r2.Cut(5)
	if empty == nil {
		t.Error("Cut on empty rollup returned nil, want empty slice")
	}
}

func TestAllZeroCutEmpty(t *testing.T) {
	refs := []input.IssueRef{
		ref("r", 1, 0, false, "a", t0),
		ref("r", 2, 0, false, "b", t0),
	}
	r := Compute(refs, since)
	cut := r.Cut(10)
	// all scores 0, median 0; items with score >= 0 = all items, but check docstring:
	// "all-zero → cut returns empty". Re-reading: "if all raw scores are 0 → cut returns empty"
	// The median would be 0, score >= 0 would include everything. But the AC says cut returns empty.
	// We need a special case: if globalMax == 0, Cut returns empty.
	if len(cut) != 0 {
		t.Errorf("Cut with all-zero scores returned %d items, want 0", len(cut))
	}
	_ = r
}

func TestMedianOdd(t *testing.T) {
	refs := []input.IssueRef{
		ref("r", 1, 5, false, "a", t0), // raw=5, score=1.0
		ref("r", 2, 2, false, "b", t0), // raw=2, score=0.4
		ref("r", 3, 3, false, "c", t0), // raw=3, score=0.6
	}
	r := Compute(refs, since)
	// scores sorted: 0.4, 0.6, 1.0 → median = 0.6
	if r.Median != 0.6 {
		t.Errorf("Median = %v, want 0.6", r.Median)
	}
}

func TestUnknownAuthor(t *testing.T) {
	refs := []input.IssueRef{
		ref("r", 1, 1, false, "", t0), // no author
	}
	r := Compute(refs, since)
	if _, ok := r.ByAuthor["unknown"]; !ok {
		t.Error("expected 'unknown' bucket for empty author")
	}
	if _, ok := r.ByAuthor[""]; ok {
		t.Error("expected no empty-string key in ByAuthor")
	}
}

package format

import (
	"strings"
	"testing"
	"time"

	"github.com/Attamusc/weekly-report-cli/internal/input"
	"github.com/Attamusc/weekly-report-cli/internal/narrative"
	"github.com/Attamusc/weekly-report-cli/internal/rollup"
)

func makeItem(repo string, num int, url, title, state, author string, score float64, labels []string) rollup.ScoredItem {
	return rollup.ScoredItem{
		Ref: input.IssueRef{
			Repo:      repo,
			Number:    num,
			URL:       url,
			Title:     title,
			State:     state,
			UpdatedAt: time.Time{},
		},
		Score:  score,
		Author: author,
		Labels: labels,
	}
}

func TestRenderRollup(t *testing.T) {
	tests := []struct {
		name         string
		rollup       rollup.Rollup
		wantEmpty    bool
		wantContains []string
		wantOrder    []string // substrings that must appear in this order
	}{
		{
			name: "empty rollup returns empty string",
			rollup: rollup.Rollup{
				ByAuthor:  map[string][]rollup.ScoredItem{},
				AllSorted: []rollup.ScoredItem{},
			},
			wantEmpty: true,
		},
		{
			name: "single author with one item",
			rollup: func() rollup.Rollup {
				item := makeItem("myrepo", 42, "https://github.com/org/myrepo/issues/42", "Fix bug", "open", "alice", 0.87, []string{"bug", "p1"})
				return rollup.Rollup{
					ByAuthor:  map[string][]rollup.ScoredItem{"alice": {item}},
					AllSorted: []rollup.ScoredItem{item},
				}
			}(),
			wantContains: []string{
				"## @alice",
				"| Item | Title | State | Score | Activity | Labels |",
				"|------|-------|-------|-------|----------|--------|",
				"[myrepo#42](https://github.com/org/myrepo/issues/42)",
				"Fix bug",
				"open",
				"0.87",
				"bug, p1",
			},
		},
		{
			name: "no labels renders empty cell",
			rollup: func() rollup.Rollup {
				item := makeItem("repo", 1, "https://github.com/org/repo/issues/1", "Title", "closed", "bob", 0.50, nil)
				return rollup.Rollup{
					ByAuthor:  map[string][]rollup.ScoredItem{"bob": {item}},
					AllSorted: []rollup.ScoredItem{item},
				}
			}(),
			wantContains: []string{"0.50", "|  |"},
		},
		{
			name: "score formatting precision",
			rollup: func() rollup.Rollup {
				item := makeItem("repo", 7, "https://github.com/org/repo/issues/7", "T", "open", "carol", 0.333333, nil)
				return rollup.Rollup{
					ByAuthor:  map[string][]rollup.ScoredItem{"carol": {item}},
					AllSorted: []rollup.ScoredItem{item},
				}
			}(),
			wantContains: []string{"0.33"},
		},
		{
			name: "unknown bucket appears last",
			rollup: func() rollup.Rollup {
				a := makeItem("r", 1, "u1", "T1", "open", "alice", 0.9, nil)
				u := makeItem("r", 2, "u2", "T2", "open", "unknown", 0.8, nil)
				z := makeItem("r", 3, "u3", "T3", "open", "zara", 0.7, nil)
				return rollup.Rollup{
					ByAuthor: map[string][]rollup.ScoredItem{
						"alice":   {a},
						"unknown": {u},
						"zara":    {z},
					},
					AllSorted: []rollup.ScoredItem{a, u, z},
				}
			}(),
			wantOrder: []string{"## @alice", "## @zara", "## @unknown"},
		},
		{
			name: "multiple authors sorted alphabetically",
			rollup: func() rollup.Rollup {
				c := makeItem("r", 1, "u1", "T1", "open", "carol", 0.9, nil)
				a := makeItem("r", 2, "u2", "T2", "open", "alice", 0.8, nil)
				b := makeItem("r", 3, "u3", "T3", "open", "bob", 0.7, nil)
				return rollup.Rollup{
					ByAuthor: map[string][]rollup.ScoredItem{
						"carol": {c},
						"alice": {a},
						"bob":   {b},
					},
					AllSorted: []rollup.ScoredItem{c, a, b},
				}
			}(),
			wantOrder: []string{"## @alice", "## @bob", "## @carol"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := RenderRollup(tc.rollup, nil, nil, time.Time{})

			if tc.wantEmpty {
				if got != "" {
					t.Errorf("expected empty string, got %q", got)
				}
				return
			}

			for _, want := range tc.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q\nfull output:\n%s", want, got)
				}
			}

			// Verify ordering of substrings.
			if len(tc.wantOrder) > 0 {
				pos := 0
				for _, want := range tc.wantOrder {
					idx := strings.Index(got[pos:], want)
					if idx == -1 {
						t.Errorf("ordering: %q not found after position %d\nfull output:\n%s", want, pos, got)
						break
					}
					pos += idx + len(want)
				}
			}
		})
	}
}

// ── Attribution tests ──────────────────────────────────────────────────────────

// makeHydrated builds a minimal hydrated map keyed by URL.
func makeHydrated(items ...narrative.Item) map[string]narrative.Item {
	m := make(map[string]narrative.Item, len(items))
	for _, it := range items {
		m[it.URL] = it
	}
	return m
}

func makeRollupItem(url string, num int, author string) rollup.ScoredItem {
	return rollup.ScoredItem{
		Ref: input.IssueRef{
			Repo:   "repo",
			Number: num,
			URL:    url,
			Title:  "repo title",
			State:  "open",
		},
		Score:  0.50,
		Author: author,
		Labels: nil,
	}
}

func singleItemRollup(item rollup.ScoredItem) rollup.Rollup {
	return rollup.Rollup{
		ByAuthor:  map[string][]rollup.ScoredItem{item.Author: {item}},
		AllSorted: []rollup.ScoredItem{item},
	}
}

func TestRenderRollup_AttributionFromRecentComment(t *testing.T) {
	const url = "https://github.com/org/repo/issues/1"
	item := makeRollupItem(url, 1, "jaeden")
	r := singleItemRollup(item)

	now := time.Now()
	hi := narrative.Item{
		URL:      url,
		Author:   "jaeden",
		OpenedAt: now.AddDate(0, 0, -30),
		RecentComments: []narrative.Comment{
			{Author: "external-person", CreatedAt: now.Add(-3 * 24 * time.Hour)},
			{Author: "gina", CreatedAt: now.Add(-2 * 24 * time.Hour)},
		},
	}
	hydrated := makeHydrated(hi)
	users := []string{"gina"}
	since := now.AddDate(0, 0, -7)

	got := RenderRollup(r, hydrated, users, since)

	if !strings.Contains(got, "## @gina") {
		t.Errorf("expected attribution to @gina; got:\n%s", got)
	}
	if !strings.Contains(got, "commented") {
		t.Errorf("expected 'commented' in activity; got:\n%s", got)
	}
	if !strings.Contains(got, "by @gina") {
		t.Errorf("expected 'by @gina' in activity; got:\n%s", got)
	}
	if strings.Contains(got, "## @jaeden") {
		t.Errorf("should not attribute to opener @jaeden; got:\n%s", got)
	}
}

func TestRenderRollup_AttributionFromClosure(t *testing.T) {
	const url = "https://github.com/org/repo/issues/2"
	item := makeRollupItem(url, 2, "jaeden")
	r := singleItemRollup(item)

	now := time.Now()
	closedAt := now.Add(-2 * 24 * time.Hour)
	hi := narrative.Item{
		URL:            url,
		Author:         "jaeden",
		OpenedAt:       now.AddDate(0, 0, -30),
		ClosedThisWeek: true,
		ClosedAt:       &closedAt,
		Events: []narrative.Event{
			{Type: "closed", Actor: "bob", At: closedAt},
		},
	}
	hydrated := makeHydrated(hi)
	users := []string{"bob"}
	since := now.AddDate(0, 0, -7)

	got := RenderRollup(r, hydrated, users, since)

	if !strings.Contains(got, "## @bob") {
		t.Errorf("expected attribution to @bob; got:\n%s", got)
	}
	if !strings.Contains(got, "closed") {
		t.Errorf("expected 'closed' in activity; got:\n%s", got)
	}
	if !strings.Contains(got, "by @bob") {
		t.Errorf("expected 'by @bob' in activity; got:\n%s", got)
	}
}

func TestRenderRollup_AttributionFromMerge(t *testing.T) {
	const url = "https://github.com/org/repo/pull/3"
	item := makeRollupItem(url, 3, "jaeden")
	r := singleItemRollup(item)

	now := time.Now()
	mergedAt := now.Add(-1 * 24 * time.Hour)
	hi := narrative.Item{
		URL:            url,
		Author:         "jaeden",
		IsPR:           true,
		OpenedAt:       now.AddDate(0, 0, -10),
		MergedThisWeek: true,
		MergedAt:       &mergedAt,
		Events: []narrative.Event{
			{Type: "merged", Actor: "alice", At: mergedAt},
		},
	}
	hydrated := makeHydrated(hi)
	users := []string{"alice"}
	since := now.AddDate(0, 0, -7)

	got := RenderRollup(r, hydrated, users, since)

	if !strings.Contains(got, "## @alice") {
		t.Errorf("expected attribution to @alice; got:\n%s", got)
	}
	if !strings.Contains(got, "merged") {
		t.Errorf("expected 'merged' in activity; got:\n%s", got)
	}
	if !strings.Contains(got, "by @alice") {
		t.Errorf("expected 'by @alice' in activity; got:\n%s", got)
	}
}

func TestRenderRollup_FallbackToOriginalAuthor(t *testing.T) {
	const url = "https://github.com/org/repo/issues/4"
	item := makeRollupItem(url, 4, "carol")
	r := singleItemRollup(item)

	now := time.Now()
	hi := narrative.Item{
		URL:            url,
		Author:         "carol",
		OpenedAt:       now.AddDate(0, 0, -14),
		RecentComments: []narrative.Comment{}, // no comments
	}
	hydrated := makeHydrated(hi)
	users := []string{"carol"}
	since := now.AddDate(0, 0, -7)

	got := RenderRollup(r, hydrated, users, since)

	if !strings.Contains(got, "## @carol") {
		t.Errorf("expected fallback attribution to @carol; got:\n%s", got)
	}
	if !strings.Contains(got, "opened") {
		t.Errorf("expected 'opened' in activity for fallback; got:\n%s", got)
	}
	if !strings.Contains(got, "by @carol") {
		t.Errorf("expected 'by @carol' in activity; got:\n%s", got)
	}
}

func TestRenderRollup_IgnoresExternalActivity(t *testing.T) {
	const url = "https://github.com/org/repo/issues/5"
	item := makeRollupItem(url, 5, "alice")
	r := singleItemRollup(item)

	now := time.Now()
	hi := narrative.Item{
		URL:      url,
		Author:   "alice",
		OpenedAt: now.AddDate(0, 0, -5),
		RecentComments: []narrative.Comment{
			{Author: "external-bot", CreatedAt: now.Add(-1 * time.Hour)},
			{Author: "random-outsider", CreatedAt: now.Add(-2 * time.Hour)},
		},
	}
	hydrated := makeHydrated(hi)
	users := []string{"alice"}
	since := now.AddDate(0, 0, -7)

	got := RenderRollup(r, hydrated, users, since)

	// External commenters are not in users list, so attribution falls back to author.
	if !strings.Contains(got, "## @alice") {
		t.Errorf("expected attribution to @alice (author), not external commenter; got:\n%s", got)
	}
	if strings.Contains(got, "## @external-bot") || strings.Contains(got, "## @random-outsider") {
		t.Errorf("should not attribute to external users; got:\n%s", got)
	}
	if !strings.Contains(got, "opened") {
		t.Errorf("expected 'opened' in activity for fallback; got:\n%s", got)
	}
}

func TestRenderRollup_ItemNotHydrated(t *testing.T) {
	const url = "https://github.com/org/repo/issues/6"
	item := makeRollupItem(url, 6, "dave")
	r := singleItemRollup(item)

	// Empty hydrated map — item not present.
	hydrated := map[string]narrative.Item{}
	users := []string{"dave"}
	since := time.Now().AddDate(0, 0, -7)

	got := RenderRollup(r, hydrated, users, since)

	if !strings.Contains(got, "## @dave") {
		t.Errorf("expected fallback to ref.AuthorLogin @dave; got:\n%s", got)
	}
	if !strings.Contains(got, "| — |") {
		t.Errorf("expected activity '—' for non-hydrated item; got:\n%s", got)
	}
}

func TestRenderRollup_BotFilteredImplicitly(t *testing.T) {
	const url = "https://github.com/org/repo/issues/7"
	item := makeRollupItem(url, 7, "alice")
	r := singleItemRollup(item)

	now := time.Now()
	hi := narrative.Item{
		URL:      url,
		Author:   "alice",
		OpenedAt: now.AddDate(0, 0, -3),
		RecentComments: []narrative.Comment{
			// Bot commenters — not in users list.
			{Author: "dependabot[bot]", CreatedAt: now.Add(-1 * time.Hour)},
			{Author: "github-actions[bot]", CreatedAt: now.Add(-2 * time.Hour)},
		},
	}
	hydrated := makeHydrated(hi)
	users := []string{"alice"} // bots are not here
	since := now.AddDate(0, 0, -7)

	got := RenderRollup(r, hydrated, users, since)

	// Bots not in users list → ignored; fallback to author.
	if !strings.Contains(got, "## @alice") {
		t.Errorf("expected attribution to @alice; got:\n%s", got)
	}
	if strings.Contains(got, "dependabot") || strings.Contains(got, "github-actions") {
		t.Errorf("bot names should not appear in attribution; got:\n%s", got)
	}
	if !strings.Contains(got, "opened") {
		t.Errorf("expected 'opened' in activity (fallback to author); got:\n%s", got)
	}
}

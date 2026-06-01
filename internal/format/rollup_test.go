package format

import (
	"strings"
	"testing"
	"time"

	"github.com/Attamusc/weekly-report-cli/internal/input"
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
				"| Item | Title | State | Score | Labels |",
				"|------|-------|-------|-------|--------|",
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
			got := RenderRollup(tc.rollup)

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

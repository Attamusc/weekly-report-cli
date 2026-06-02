// Package rollup implements deterministic mechanical scoring and ranking of
// GitHub issues and PRs over a lookback window. It does not make any API calls.
package rollup

import (
	"math"
	"sort"
	"time"

	"github.com/Attamusc/weekly-report-cli/internal/input"
)

// singletonRepoThreshold is the minimum number of items a repo must contribute
// to the dataset before per-repo normalization applies. Repos below this
// threshold normalize against the global max instead.
//
// Motivation (premortem item 2): a repo with only one item would always receive
// score=1.0 under strict per-repo normalization, beating busy repos with many
// competitive items. Normalizing against the global max keeps such singletons
// proportionally ranked.
const singletonRepoThreshold = 3

// ScoredItem is an IssueRef augmented with its computed score.
type ScoredItem struct {
	Ref    input.IssueRef
	Raw    int
	Score  float64 // normalized [0,1]
	Author string  // primary attribution (see note below)
	Labels []string
}

// Rollup is the result of Compute.
type Rollup struct {
	ByAuthor  map[string][]ScoredItem // sorted desc by score within each slice
	AllSorted []ScoredItem            // sorted desc by score globally
	Median    float64
}

// Compute ranks refs into a Rollup. since marks the start of the lookback
// window and is used to determine whether an item was closed this week.
//
// Author attribution uses Ref.AuthorLogin exclusively (the search-time author).
// Future hydration could refine this with closer or last-commenter data once
// comment bodies are fetched for the survivor set.
func Compute(refs []input.IssueRef, since time.Time) Rollup {
	if len(refs) == 0 {
		return Rollup{ByAuthor: map[string][]ScoredItem{}, AllSorted: []ScoredItem{}}
	}

	// Compute raw scores.
	items := make([]ScoredItem, len(refs))
	for i, ref := range refs {
		closedThisWeek := ref.ClosedAt != nil && ref.ClosedAt.After(since)
		raw := ref.CommentCount
		if closedThisWeek {
			raw += 5
		}
		if ref.IsPR {
			raw += 2
		}
		author := ref.AuthorLogin
		if author == "" {
			author = "unknown"
		}
		items[i] = ScoredItem{
			Ref:    ref,
			Raw:    raw,
			Author: author,
			Labels: ref.Labels,
		}
	}

	// Global max for singleton mitigation.
	globalMax := 0
	for _, it := range items {
		if it.Raw > globalMax {
			globalMax = it.Raw
		}
	}

	// Per-repo max and count.
	repoKey := func(it ScoredItem) string { return it.Ref.Owner + "/" + it.Ref.Repo }
	repoMax := map[string]int{}
	repoCount := map[string]int{}
	for _, it := range items {
		k := repoKey(it)
		repoCount[k]++
		if it.Raw > repoMax[k] {
			repoMax[k] = it.Raw
		}
	}

	// Normalize.
	if globalMax == 0 {
		// All-zero degenerate case: scores stay 0.
	} else {
		for i, it := range items {
			k := repoKey(it)
			effectiveMax := repoMax[k]
			if repoCount[k] < singletonRepoThreshold {
				effectiveMax = globalMax
			}
			if effectiveMax > 0 {
				items[i].Score = float64(it.Raw) / float64(effectiveMax)
			}
		}
	}

	sortItems(items)

	median := computeMedian(items)

	// Group by author.
	byAuthor := map[string][]ScoredItem{}
	for _, it := range items {
		byAuthor[it.Author] = append(byAuthor[it.Author], it)
	}
	// Items within each author bucket are already in score-desc order because
	// items is sorted globally and we append in that order.

	return Rollup{
		ByAuthor:  byAuthor,
		AllSorted: items,
		Median:    median,
	}
}

// Cut returns survivors: items with score >= rollup.Median, capped at topN,
// preserving AllSorted order. Returns an empty (non-nil) slice if none survive.
// If all scores are zero (degenerate input), returns empty.
func (r Rollup) Cut(topN int) []ScoredItem {
	result := []ScoredItem{}
	// Degenerate case: if median is 0 and all scores are 0, nothing is meaningful.
	if r.Median == 0 && (len(r.AllSorted) == 0 || r.AllSorted[0].Score == 0) {
		return result
	}
	for _, it := range r.AllSorted {
		if len(result) >= topN {
			break
		}
		if it.Score >= r.Median {
			result = append(result, it)
		}
	}
	return result
}

// sortItems sorts in place: Score desc, UpdatedAt desc, URL asc for stability.
func sortItems(items []ScoredItem) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if !a.Ref.UpdatedAt.Equal(b.Ref.UpdatedAt) {
			return a.Ref.UpdatedAt.After(b.Ref.UpdatedAt)
		}
		return a.Ref.URL < b.Ref.URL
	})
}

// computeMedian returns the median score over all items (stdlib only).
// For even N, it averages the two middle values.
func computeMedian(items []ScoredItem) float64 {
	n := len(items)
	if n == 0 {
		return 0
	}
	scores := make([]float64, n)
	for i, it := range items {
		scores[i] = it.Score
	}
	sort.Float64s(scores)
	if n%2 == 1 {
		return scores[n/2]
	}
	return math.Round((scores[n/2-1]+scores[n/2])/2*1e9) / 1e9
}

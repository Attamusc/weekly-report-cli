package format

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Attamusc/weekly-report-cli/internal/narrative"
	"github.com/Attamusc/weekly-report-cli/internal/rollup"
)

const authorUnknown = "unknown"

// RenderRollup generates by-author markdown tables from a mechanical rollup.
// hydrated maps item URL → narrative.Item for recency-based attribution; pass
// nil when hydration data is unavailable (e.g. --no-summary path).
// users is the input --users list; only their activity drives attribution.
// since is the start of the lookback window used to compute relative times.
//
// Authors are sorted alphabetically; the "unknown" bucket appears last.
// Returns "" if the rollup contains no items.
func RenderRollup(r rollup.Rollup, hydrated map[string]narrative.Item, users []string, since time.Time) string {
	if len(r.AllSorted) == 0 {
		return ""
	}

	now := time.Now()

	// Build a lowercase user set for O(1) membership checks.
	userSet := make(map[string]bool, len(users))
	for _, u := range users {
		userSet[strings.ToLower(u)] = true
	}

	// Compute attribution and activity for every item, then regroup.
	type displayItem struct {
		scored   rollup.ScoredItem
		activity string
	}
	byAuthor := map[string][]displayItem{}

	for _, item := range r.AllSorted {
		attr, act := itemAttribution(item, hydrated, userSet, now)
		byAuthor[attr] = append(byAuthor[attr], displayItem{scored: item, activity: act})
	}

	// Sort authors: alphabetical, "unknown" last.
	authors := make([]string, 0, len(byAuthor))
	for author := range byAuthor {
		authors = append(authors, author)
	}
	sort.Slice(authors, func(i, j int) bool {
		a, b := authors[i], authors[j]
		if a == authorUnknown {
			return false
		}
		if b == authorUnknown {
			return true
		}
		return a < b
	})

	var builder strings.Builder
	for i, author := range authors {
		if i > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(fmt.Sprintf("## @%s\n\n", author))
		builder.WriteString("| Item | Title | State | Score | Activity | Labels |\n")
		builder.WriteString("|------|-------|-------|-------|----------|--------|\n")
		for _, di := range byAuthor[author] {
			ref := di.scored.Ref
			itemLink := fmt.Sprintf("[%s#%d](%s)", ref.Repo, ref.Number, ref.URL)
			labels := strings.Join(di.scored.Labels, ", ")
			builder.WriteString(fmt.Sprintf("| %s | %s | %s | %.2f | %s | %s |\n",
				itemLink,
				escapeMarkdownTableCell(ref.Title),
				ref.State,
				di.scored.Score,
				di.activity,
				labels,
			))
		}
	}

	return builder.String()
}

// itemAttribution computes the display attribution (author bucket) and activity
// description for a single rollup item. Uses only events/comments by users in
// userSet. Falls back gracefully when hydration data is unavailable.
func itemAttribution(
	item rollup.ScoredItem,
	hydrated map[string]narrative.Item,
	userSet map[string]bool,
	now time.Time,
) (attribution, activity string) {
	hi, ok := hydrated[item.Ref.URL]
	if !ok {
		// Not hydrated: keep mechanical rollup attribution, no activity signal.
		attr := item.Author
		if attr == "" {
			attr = authorUnknown
		}
		return attr, "—"
	}

	// Priority 1: closed this week by an input-user.
	if hi.ClosedThisWeek {
		for _, ev := range hi.Events {
			if ev.Type == "closed" && userSet[strings.ToLower(ev.Actor)] {
				return ev.Actor, fmt.Sprintf("closed %s by @%s", relTime(ev.At, now), ev.Actor)
			}
		}
	}

	// Priority 2: merged this week by an input-user (PRs only).
	if hi.MergedThisWeek && hi.IsPR {
		for _, ev := range hi.Events {
			if ev.Type == "merged" && userSet[strings.ToLower(ev.Actor)] {
				return ev.Actor, fmt.Sprintf("merged %s by @%s", relTime(ev.At, now), ev.Actor)
			}
		}
	}

	// Priority 3: most recent comment by an input-user (comments are chronological).
	for i := len(hi.RecentComments) - 1; i >= 0; i-- {
		c := hi.RecentComments[i]
		if userSet[strings.ToLower(c.Author)] {
			return c.Author, fmt.Sprintf("commented %s by @%s", relTime(c.CreatedAt, now), c.Author)
		}
	}

	// Priority 4a: original author is an input-user.
	if userSet[strings.ToLower(hi.Author)] {
		return hi.Author, fmt.Sprintf("opened %s by @%s", relTime(hi.OpenedAt, now), hi.Author)
	}

	// Priority 4b: first input-user assignee.
	for _, a := range hi.Assignees {
		if userSet[strings.ToLower(a)] {
			return a, fmt.Sprintf("opened %s by @%s", relTime(hi.OpenedAt, now), a)
		}
	}

	// Last resort: use the mechanical rollup author — this shouldn't occur in
	// practice because discovery requires input-user involvement, but degrade
	// gracefully if it does.
	attr := item.Author
	if attr == "" {
		attr = authorUnknown
	}
	return attr, "—"
}

// relTime returns a short human-readable string for how long ago t was,
// relative to now.
func relTime(t, now time.Time) string {
	d := now.Sub(t)
	if d < 0 {
		d = 0
	}
	hours := int(d.Hours())
	if hours < 1 {
		return "<1h ago"
	}
	if hours < 24 {
		return fmt.Sprintf("%dh ago", hours)
	}
	days := hours / 24
	if days < 7 {
		return fmt.Sprintf("%dd ago", days)
	}
	weeks := days / 7
	return fmt.Sprintf("%dw ago", weeks)
}

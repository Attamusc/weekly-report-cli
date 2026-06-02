package format

import (
	"strings"
	"testing"
	"time"

	"github.com/Attamusc/weekly-report-cli/internal/ai"
	"github.com/Attamusc/weekly-report-cli/internal/input"
	"github.com/Attamusc/weekly-report-cli/internal/rollup"
)

// makeRollup builds a minimal Rollup from a slice of (author, title, repo) triples.
func makeRollup(items []struct{ author, title, repo string }) rollup.Rollup {
	var all []rollup.ScoredItem
	byAuthor := map[string][]rollup.ScoredItem{}
	for i, it := range items {
		si := rollup.ScoredItem{
			Ref: input.IssueRef{
				Owner:  "org",
				Repo:   it.repo,
				Number: i + 1,
				URL:    "https://github.com/org/" + it.repo + "/issues/" + string(rune('1'+i)),
				Title:  it.title,
				State:  "open",
			},
			Author: it.author,
			Score:  float64(len(items)-i) / float64(len(items)),
		}
		all = append(all, si)
		byAuthor[it.author] = append(byAuthor[it.author], si)
	}
	return rollup.Rollup{AllSorted: all, ByAuthor: byAuthor}
}

func threeSection() ai.Narrative {
	return ai.Narrative{
		Sections: []ai.NarrativeSection{
			{Heading: "Infrastructure work", Body: "We migrated the database to a new host with zero downtime."},
			{Heading: "Bug fixes", Body: "Several critical bugs were resolved this week, improving stability."},
			{Heading: "Feature progress", Body: "The new dashboard is 80% complete; remaining work is polish."},
		},
	}
}

func threeItemRollup() rollup.Rollup {
	return makeRollup([]struct{ author, title, repo string }{
		{"alice", "Migrate DB", "backend"},
		{"bob", "Fix crash on login", "backend"},
		{"alice", "Dashboard polish", "frontend"},
	})
}

func TestRenderNarrativeReport_FullReport(t *testing.T) {
	n := threeSection()
	r := threeItemRollup()
	out := RenderNarrativeReport(n, r, 7)

	checks := []string{
		"# Weekly Highlights — Last 7 days (3 items)",
		"## Narrative",
		"### Infrastructure work",
		"We migrated the database",
		"### Bug fixes",
		"Several critical bugs",
		"### Feature progress",
		"new dashboard is 80%",
		"## All activity (3 items)",
		"| Item | Title | State | Score | Labels |",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestRenderNarrativeReport_EmptyNarrative(t *testing.T) {
	r := threeItemRollup()
	out := RenderNarrativeReport(ai.Narrative{}, r, 7)

	if strings.Contains(out, "## Narrative") {
		t.Errorf("expected no ## Narrative heading when narrative is empty; got:\n%s", out)
	}
	if !strings.Contains(out, "## All activity") {
		t.Errorf("expected ## All activity heading; got:\n%s", out)
	}
	if !strings.Contains(out, "# Weekly Highlights") {
		t.Errorf("expected title; got:\n%s", out)
	}
}

func TestRenderNarrativeReport_EmptyRollup(t *testing.T) {
	out := RenderNarrativeReport(ai.Narrative{}, rollup.Rollup{}, 7)
	if out != "" {
		t.Errorf("expected empty string when both narrative and rollup are empty; got %q", out)
	}
}

func TestRenderNarrativeReport_NarrativeOnly(t *testing.T) {
	n := threeSection()
	out := RenderNarrativeReport(n, rollup.Rollup{}, 7)

	if !strings.Contains(out, "## Narrative") {
		t.Errorf("expected ## Narrative heading; got:\n%s", out)
	}
	if strings.Contains(out, "## All activity") {
		t.Errorf("expected no ## All activity when rollup is empty; got:\n%s", out)
	}
}

// TestRenderNarrativeReport_LargeFixture is a visual sanity check.
// Run with go test -v to see the output.
func TestRenderNarrativeReport_LargeFixture(t *testing.T) {
	sections := make([]ai.NarrativeSection, 4)
	sections[0] = ai.NarrativeSection{
		Heading: "Azure migration progresses",
		Body: "The team completed the first two phases of the Azure migration, moving the primary " +
			"compute workloads to the new region. Service reliability held above 99.9% throughout " +
			"the migration window. Remaining work focuses on data-tier cutover scheduled for next sprint. " +
			"Key contributors include [#42](https://github.com/org/infra/issues/42) and " +
			"[#51](https://github.com/org/infra/issues/51).",
	}
	sections[1] = ai.NarrativeSection{
		Heading: "Reliability improvements",
		Body: "Three high-priority production incidents were resolved this week. Root-cause analysis " +
			"identified a misconfigured cache TTL that caused cascading failures under load. A new " +
			"circuit-breaker was introduced in [#88](https://github.com/org/backend/issues/88), " +
			"reducing error rates by 60%.",
	}
	sections[2] = ai.NarrativeSection{
		Heading: "Dashboard feature nearing completion",
		Body: "Frontend work on the analytics dashboard reached 85% completion. The team landed " +
			"filter controls, date-range pickers, and export-to-CSV in this iteration. Outstanding " +
			"items are accessibility polish and final QA sign-off, expected by end of next week.",
	}
	sections[3] = ai.NarrativeSection{
		Heading: "Onboarding improvements",
		Body: "Several friction points in the new-user onboarding flow were addressed. Time-to-first-value " +
			"dropped from 12 minutes to 4 minutes in A/B testing. Documentation updates are " +
			"included in [#104](https://github.com/org/docs/issues/104).",
	}
	n := ai.Narrative{Sections: sections}

	// Build a ~30-item rollup across three authors.
	rawItems := make([]struct{ author, title, repo string }, 0, 30)
	authors := []string{"alice", "bob", "carol"}
	repos := []string{"backend", "frontend", "infra"}
	titles := []string{
		"Migrate compute to Azure", "Fix cache TTL bug", "Add circuit breaker",
		"Dashboard filters", "Dashboard date picker", "Export CSV",
		"Onboarding flow v2", "Reduce time-to-value", "Update docs",
		"Load test new region", "Fix flaky test", "Add monitoring alert",
	}
	for i := 0; i < 30; i++ {
		rawItems = append(rawItems, struct{ author, title, repo string }{
			author: authors[i%len(authors)],
			title:  titles[i%len(titles)],
			repo:   repos[i%len(repos)],
		})
	}
	r := makeRollup(rawItems)
	// Stamp realistic UpdatedAt so Compute wouldn't filter (we already have ScoredItems).
	for k := range r.ByAuthor {
		for j := range r.ByAuthor[k] {
			r.ByAuthor[k][j].Ref.UpdatedAt = time.Now()
		}
	}

	out := RenderNarrativeReport(n, r, 14)
	t.Log("\n" + out)

	if !strings.Contains(out, "# Weekly Highlights — Last 14 days (30 items)") {
		t.Errorf("title line incorrect; got first 120 chars: %q", out[:min(120, len(out))])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

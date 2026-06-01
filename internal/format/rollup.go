package format

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Attamusc/weekly-report-cli/internal/rollup"
)

// RenderRollup generates by-author markdown tables from a mechanical rollup.
// Authors are sorted alphabetically; the "unknown" bucket appears last.
// Returns "" if the rollup contains no items.
func RenderRollup(r rollup.Rollup) string {
	if len(r.AllSorted) == 0 {
		return ""
	}

	// Collect and sort authors: alphabetical, "unknown" last.
	authors := make([]string, 0, len(r.ByAuthor))
	for author := range r.ByAuthor {
		authors = append(authors, author)
	}
	sort.Slice(authors, func(i, j int) bool {
		a, b := authors[i], authors[j]
		if a == "unknown" {
			return false
		}
		if b == "unknown" {
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
		builder.WriteString("| Item | Title | State | Score | Labels |\n")
		builder.WriteString("|------|-------|-------|-------|--------|\n")
		for _, item := range r.ByAuthor[author] {
			ref := item.Ref
			itemLink := fmt.Sprintf("[%s#%d](%s)", ref.Repo, ref.Number, ref.URL)
			labels := strings.Join(item.Labels, ", ")
			builder.WriteString(fmt.Sprintf("| %s | %s | %s | %.2f | %s |\n",
				itemLink,
				escapeMarkdownTableCell(ref.Title),
				ref.State,
				item.Score,
				labels,
			))
		}
	}

	return builder.String()
}

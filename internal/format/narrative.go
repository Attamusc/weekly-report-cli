package format

import (
	"fmt"
	"strings"

	"github.com/Attamusc/weekly-report-cli/internal/ai"
	"github.com/Attamusc/weekly-report-cli/internal/rollup"
)

// RenderNarrativeReport formats the AI narrative output plus the full
// mechanical rollup beneath. When narrative is empty (AI failed or returned
// nothing), the function still renders the title + rollup section so users
// always get the raw signal.
//
// Returns "" only when both narrative and rollup are empty.
func RenderNarrativeReport(n ai.Narrative, r rollup.Rollup, sinceDays int) string {
	itemCount := len(r.AllSorted)
	hasNarrative := len(n.Sections) > 0
	hasRollup := itemCount > 0

	if !hasNarrative && !hasRollup {
		return ""
	}

	var b strings.Builder

	// Title line.
	b.WriteString(fmt.Sprintf("# Weekly Highlights — Last %d days (%d items)\n", sinceDays, itemCount))

	// Narrative block.
	if hasNarrative {
		b.WriteString("\n## Narrative\n")
		for _, sec := range n.Sections {
			b.WriteString("\n### ")
			b.WriteString(strings.TrimSpace(sec.Heading))
			b.WriteString("\n")
			b.WriteString(strings.TrimSpace(sec.Body))
			b.WriteString("\n")
		}
	}

	// Rollup block.
	if hasRollup {
		b.WriteString(fmt.Sprintf("\n## All activity (%d items)\n\n", itemCount))
		b.WriteString(RenderRollup(r))
	}

	return b.String()
}

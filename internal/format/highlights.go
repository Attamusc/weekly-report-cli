package format

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Attamusc/weekly-report-cli/internal/ai"
)

// RenderHighlights generates themed markdown sections from AI-curated highlights.
func RenderHighlights(highlights []ai.Highlight) string {
	if len(highlights) == 0 {
		return ""
	}

	// Group by theme
	grouped := make(map[string][]ai.Highlight)
	for _, h := range highlights {
		grouped[h.Theme] = append(grouped[h.Theme], h)
	}

	// Sort themes alphabetically
	themes := make([]string, 0, len(grouped))
	for theme := range grouped {
		themes = append(themes, theme)
	}
	sort.Strings(themes)

	var builder strings.Builder
	for i, theme := range themes {
		if i > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(fmt.Sprintf("## %s\n\n", theme))
		for _, h := range grouped[theme] {
			builder.WriteString(fmt.Sprintf("- [%s](%s) — %s\n",
				escapeMarkdownTableCell(h.Title), h.URL, h.Summary))
		}
	}

	return builder.String()
}

package derive

import (
	"regexp"
	"strings"
)

// Status represents a canonical status with emoji and caption
type Status struct {
	Emoji   string
	Caption string
}

// Predefined status mappings
var (
	OnTrack    = Status{Emoji: ":green_circle:", Caption: "On Track"}
	AtRisk     = Status{Emoji: ":yellow_circle:", Caption: "At Risk"}
	OffTrack   = Status{Emoji: ":red_circle:", Caption: "Off Track"}
	NotStarted = Status{Emoji: ":white_circle:", Caption: "Not Started"}
	Done       = Status{Emoji: ":purple_circle:", Caption: "Done"}
	Unknown    = Status{Emoji: ":black_circle:", Caption: "Unknown"}
)

// Status mapping patterns (case-insensitive)
var statusMappings = []struct {
	patterns []string
	status   Status
}{
	// Green/On Track patterns
	{
		patterns: []string{"on track", "green", "🟢"},
		status:   OnTrack,
	},
	// Yellow/At Risk patterns  
	{
		patterns: []string{"at risk", "yellow", "🟡"},
		status:   AtRisk,
	},
	// Red/Off Track/Blocked patterns
	{
		patterns: []string{"off track", "blocked", "red", "🔴"},
		status:   OffTrack,
	},
	// White/Not Started patterns
	{
		patterns: []string{"not started", "white", "⚪"},
		status:   NotStarted,
	},
	// Purple/Done/Complete patterns
	{
		patterns: []string{"done", "complete", "completed", "purple", "🟣"},
		status:   Done,
	},
}

// Circle emoji regex to strip leading emoji
var circleEmojiRegex = regexp.MustCompile(`^[🟢🟡🔴⚪🟣]\s*`)

// MapTrending maps a free-form trending status string to canonical Status
// Handles case-insensitive matching, strips leading circle emojis, and normalizes whitespace
func MapTrending(raw string) Status {
	if raw == "" {
		return Unknown
	}

	// Normalize the input: strip leading circle emoji, trim whitespace, lowercase
	normalized := circleEmojiRegex.ReplaceAllString(raw, "")
	normalized = strings.TrimSpace(strings.ToLower(normalized))

	if normalized == "" {
		return Unknown
	}

	// Try to match against known patterns
	for _, mapping := range statusMappings {
		for _, pattern := range mapping.patterns {
			if strings.Contains(normalized, pattern) {
				return mapping.status
			}
		}
	}

	// If no pattern matches, return unknown
	return Unknown
}

// String returns a formatted status string for display
func (s Status) String() string {
	return s.Emoji + " " + s.Caption
}
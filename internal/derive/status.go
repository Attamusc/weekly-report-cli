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
	OnTrack     = Status{Emoji: ":green_circle:", Caption: "On Track"}
	AtRisk      = Status{Emoji: ":yellow_circle:", Caption: "At Risk"}
	OffTrack    = Status{Emoji: ":red_circle:", Caption: "Off Track"}
	NotStarted  = Status{Emoji: ":white_circle:", Caption: "Not Started"}
	NeedsUpdate = Status{Emoji: ":white_circle:", Caption: "Needs Update"}
	Shaping     = Status{Emoji: ":diamond_shape_with_a_dot_inside:", Caption: "Shaping"}
	Done        = Status{Emoji: ":purple_circle:", Caption: "Done"}
	Unknown     = Status{Emoji: ":black_circle:", Caption: "Unknown"}
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

// statusKeyUnknown is the canonical snake_case key for the Unknown status.
const statusKeyUnknown = "unknown"

// Circle emoji regex to strip leading emoji
var circleEmojiRegex = regexp.MustCompile(`^[🟢🟡🔴⚪🟣]\s*`)

// matchStatusPattern normalizes a raw string and attempts to match it against
// known status patterns using substring matching. Returns (Status, true) on
// match, (Unknown, false) otherwise.
func matchStatusPattern(raw string) (Status, bool) {
	if raw == "" {
		return Unknown, false
	}

	normalized := circleEmojiRegex.ReplaceAllString(raw, "")
	normalized = strings.TrimSpace(strings.ToLower(normalized))

	if normalized == "" {
		return Unknown, false
	}

	for _, mapping := range statusMappings {
		for _, pattern := range mapping.patterns {
			if strings.Contains(normalized, pattern) {
				return mapping.status, true
			}
		}
	}

	return Unknown, false
}

// MapTrending maps a free-form trending status string to canonical Status.
// Handles case-insensitive matching, strips leading circle emojis, and normalizes whitespace.
func MapTrending(raw string) Status {
	status, _ := matchStatusPattern(raw)
	return status
}

// isEmojiPattern returns true if the pattern is a single emoji character
// (used to skip emoji patterns when matching labels).
func isEmojiPattern(s string) bool {
	return s == "🟢" || s == "🟡" || s == "🔴" || s == "⚪" || s == "🟣"
}

// matchLabelPattern normalizes a label and attempts to match it against known
// status patterns using word-boundary matching (stricter than the substring
// matching used for trending values). Returns (Status, true) on match,
// (Unknown, false) otherwise.
func matchLabelPattern(label string) (Status, bool) {
	normalized := strings.TrimSpace(strings.ToLower(label))
	if normalized == "" {
		return Unknown, false
	}

	padded := " " + normalized + " "
	for _, mapping := range statusMappings {
		for _, pattern := range mapping.patterns {
			if isEmojiPattern(pattern) {
				continue
			}
			if normalized == pattern || strings.Contains(padded, " "+pattern+" ") {
				return mapping.status, true
			}
		}
	}

	return Unknown, false
}

// MapLabelsToStatus attempts to derive a status from issue labels using
// word-boundary matching. Returns the first matching status and true, or
// (Unknown, false) if no label matches a known status pattern.
func MapLabelsToStatus(labels []string) (Status, bool) {
	for _, label := range labels {
		if status, ok := matchLabelPattern(label); ok {
			return status, true
		}
	}
	return Unknown, false
}

// String returns a formatted status string for display
func (s Status) String() string {
	return s.Emoji + " " + s.Caption
}

// Key returns the canonical snake_case key for the status.
func (s Status) Key() string {
	switch s {
	case OnTrack:
		return "on_track"
	case AtRisk:
		return "at_risk"
	case OffTrack:
		return "off_track"
	case NotStarted:
		return "not_started"
	case Done:
		return "done"
	case NeedsUpdate:
		return "needs_update"
	case Shaping:
		return "shaping"
	case Unknown:
		return statusKeyUnknown
	default:
		return statusKeyUnknown
	}
}

// ParseStatusKey converts a canonical snake_case status key to a Status value.
// Returns (Status, false) if the key is not recognized.
func ParseStatusKey(key string) (Status, bool) {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "on_track":
		return OnTrack, true
	case "at_risk":
		return AtRisk, true
	case "off_track":
		return OffTrack, true
	case "not_started":
		return NotStarted, true
	case "done":
		return Done, true
	case "needs_update":
		return NeedsUpdate, true
	case "shaping":
		return Shaping, true
	case statusKeyUnknown:
		return Unknown, true
	default:
		return Unknown, false
	}
}

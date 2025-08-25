package derive

import (
	"testing"
)

func TestMapTrending(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected Status
	}{
		// On Track variations
		{
			name:     "green emoji",
			input:    "🟢 on track",
			expected: OnTrack,
		},
		{
			name:     "green text",
			input:    "green",
			expected: OnTrack,
		},
		{
			name:     "on track text",
			input:    "on track",
			expected: OnTrack,
		},
		{
			name:     "on track uppercase",
			input:    "ON TRACK",
			expected: OnTrack,
		},
		{
			name:     "green emoji only",
			input:    "🟢",
			expected: Unknown, // Just emoji without text maps to unknown
		},

		// At Risk variations
		{
			name:     "yellow emoji",
			input:    "🟡 at risk",
			expected: AtRisk,
		},
		{
			name:     "yellow text",
			input:    "yellow",
			expected: AtRisk,
		},
		{
			name:     "at risk text",
			input:    "at risk",
			expected: AtRisk,
		},
		{
			name:     "at risk mixed case",
			input:    "At Risk",
			expected: AtRisk,
		},

		// Off Track variations
		{
			name:     "red emoji",
			input:    "🔴 off track",
			expected: OffTrack,
		},
		{
			name:     "red text",
			input:    "red",
			expected: OffTrack,
		},
		{
			name:     "off track text",
			input:    "off track",
			expected: OffTrack,
		},
		{
			name:     "blocked text",
			input:    "blocked",
			expected: OffTrack,
		},
		{
			name:     "blocked uppercase",
			input:    "BLOCKED",
			expected: OffTrack,
		},

		// Not Started variations
		{
			name:     "white emoji",
			input:    "⚪ not started",
			expected: NotStarted,
		},
		{
			name:     "white text",
			input:    "white",
			expected: NotStarted,
		},
		{
			name:     "not started text",
			input:    "not started",
			expected: NotStarted,
		},
		{
			name:     "not started mixed case",
			input:    "Not Started",
			expected: NotStarted,
		},

		// Done variations
		{
			name:     "purple emoji",
			input:    "🟣 done",
			expected: Done,
		},
		{
			name:     "purple text",
			input:    "purple",
			expected: Done,
		},
		{
			name:     "done text",
			input:    "done",
			expected: Done,
		},
		{
			name:     "complete text",
			input:    "complete",
			expected: Done,
		},
		{
			name:     "completed text",
			input:    "completed",
			expected: Done,
		},
		{
			name:     "done uppercase",
			input:    "DONE",
			expected: Done,
		},

		// Edge cases
		{
			name:     "empty string",
			input:    "",
			expected: Unknown,
		},
		{
			name:     "whitespace only",
			input:    "   ",
			expected: Unknown,
		},
		{
			name:     "emoji only with spaces",
			input:    "🟢   ",
			expected: Unknown,
		},
		{
			name:     "unknown status",
			input:    "something random",
			expected: Unknown,
		},
		{
			name:     "multiple emojis",
			input:    "🟢🟡 mixed signals",
			expected: AtRisk, // Should match "at risk" in "mixed signals" first
		},
		{
			name:     "contains multiple statuses",
			input:    "was blocked but now on track",
			expected: OnTrack, // Should match "on track" which comes later in string but first in pattern order
		},

		// Whitespace handling
		{
			name:     "extra whitespace",
			input:    "  🟣  done  ",
			expected: Done,
		},
		{
			name:     "tab and newlines",
			input:    "\t🟢\non track\n",
			expected: OnTrack,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapTrending(tt.input)
			if result != tt.expected {
				t.Errorf("MapTrending(%q) = %+v, expected %+v",
					tt.input, result, tt.expected)
			}
		})
	}
}

func TestStatusString(t *testing.T) {
	tests := []struct {
		name     string
		status   Status
		expected string
	}{
		{
			name:     "on track",
			status:   OnTrack,
			expected: ":green_circle: On Track",
		},
		{
			name:     "at risk",
			status:   AtRisk,
			expected: ":yellow_circle: At Risk",
		},
		{
			name:     "off track",
			status:   OffTrack,
			expected: ":red_circle: Off Track",
		},
		{
			name:     "not started",
			status:   NotStarted,
			expected: ":white_circle: Not Started",
		},
		{
			name:     "needs update",
			status:   NeedsUpdate,
			expected: ":white_circle: Needs Update",
		},
		{
			name:     "done",
			status:   Done,
			expected: ":purple_circle: Done",
		},
		{
			name:     "unknown",
			status:   Unknown,
			expected: ":black_circle: Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.status.String()
			if result != tt.expected {
				t.Errorf("Status.String() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestCircleEmojiRegex(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "green circle with text",
			input:    "🟢 on track",
			expected: "on track",
		},
		{
			name:     "yellow circle with text",
			input:    "🟡 at risk",
			expected: "at risk",
		},
		{
			name:     "red circle with text",
			input:    "🔴 blocked",
			expected: "blocked",
		},
		{
			name:     "white circle with text",
			input:    "⚪ not started",
			expected: "not started",
		},
		{
			name:     "purple circle with text",
			input:    "🟣 done",
			expected: "done",
		},
		{
			name:     "no circle emoji",
			input:    "just text",
			expected: "just text",
		},
		{
			name:     "circle emoji only",
			input:    "🟢",
			expected: "",
		},
		{
			name:     "circle emoji with extra spaces",
			input:    "🟢   text",
			expected: "text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := circleEmojiRegex.ReplaceAllString(tt.input, "")
			if result != tt.expected {
				t.Errorf("circleEmojiRegex.ReplaceAllString(%q) = %q, expected %q",
					tt.input, result, tt.expected)
			}
		})
	}
}

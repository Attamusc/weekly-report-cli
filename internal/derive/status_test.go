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

func TestStatusKey(t *testing.T) {
	tests := []struct {
		name     string
		status   Status
		expected string
	}{
		{name: "on track", status: OnTrack, expected: "on_track"},
		{name: "at risk", status: AtRisk, expected: "at_risk"},
		{name: "off track", status: OffTrack, expected: "off_track"},
		{name: "not started", status: NotStarted, expected: "not_started"},
		{name: "done", status: Done, expected: "done"},
		{name: "needs update", status: NeedsUpdate, expected: "needs_update"},
		{name: "unknown", status: Unknown, expected: "unknown"},
		{name: "unrecognized status", status: Status{Emoji: ":star:", Caption: "Custom"}, expected: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.status.Key()
			if result != tt.expected {
				t.Errorf("Status.Key() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestParseStatusKey_ValidKeys(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		expected Status
	}{
		{name: "on_track", key: "on_track", expected: OnTrack},
		{name: "at_risk", key: "at_risk", expected: AtRisk},
		{name: "off_track", key: "off_track", expected: OffTrack},
		{name: "not_started", key: "not_started", expected: NotStarted},
		{name: "done", key: "done", expected: Done},
		{name: "needs_update", key: "needs_update", expected: NeedsUpdate},
		{name: "unknown", key: "unknown", expected: Unknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ok := ParseStatusKey(tt.key)
			if !ok {
				t.Fatalf("ParseStatusKey(%q) returned ok=false, expected ok=true", tt.key)
			}
			if result != tt.expected {
				t.Errorf("ParseStatusKey(%q) = %+v, expected %+v", tt.key, result, tt.expected)
			}
		})
	}
}

func TestParseStatusKey_InvalidKey(t *testing.T) {
	result, ok := ParseStatusKey("nonexistent")
	if ok {
		t.Errorf("ParseStatusKey(\"nonexistent\") returned ok=true, expected ok=false")
	}
	if result != Unknown {
		t.Errorf("ParseStatusKey(\"nonexistent\") = %+v, expected Unknown", result)
	}
}

func TestParseStatusKey_CaseInsensitive(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		expected Status
	}{
		{name: "uppercase ON_TRACK", key: "ON_TRACK", expected: OnTrack},
		{name: "mixed case At_Risk", key: "At_Risk", expected: AtRisk},
		{name: "uppercase DONE", key: "DONE", expected: Done},
		{name: "mixed Off_Track", key: "Off_Track", expected: OffTrack},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ok := ParseStatusKey(tt.key)
			if !ok {
				t.Fatalf("ParseStatusKey(%q) returned ok=false, expected ok=true", tt.key)
			}
			if result != tt.expected {
				t.Errorf("ParseStatusKey(%q) = %+v, expected %+v", tt.key, result, tt.expected)
			}
		})
	}
}

func TestParseStatusKey_Whitespace(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		expected Status
	}{
		{name: "leading space", key: " at_risk", expected: AtRisk},
		{name: "trailing space", key: "at_risk ", expected: AtRisk},
		{name: "both sides", key: " on_track ", expected: OnTrack},
		{name: "tabs", key: "\tdone\t", expected: Done},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ok := ParseStatusKey(tt.key)
			if !ok {
				t.Fatalf("ParseStatusKey(%q) returned ok=false, expected ok=true", tt.key)
			}
			if result != tt.expected {
				t.Errorf("ParseStatusKey(%q) = %+v, expected %+v", tt.key, result, tt.expected)
			}
		})
	}
}

func TestStatusKey_Roundtrip(t *testing.T) {
	// Verify that Key() -> ParseStatusKey() round-trips for all known statuses
	statuses := []Status{OnTrack, AtRisk, OffTrack, NotStarted, Done, NeedsUpdate, Unknown}

	for _, status := range statuses {
		key := status.Key()
		parsed, ok := ParseStatusKey(key)
		if !ok {
			t.Fatalf("ParseStatusKey(%q) returned ok=false for key from %+v", key, status)
		}
		if parsed != status {
			t.Errorf("Round-trip failed: %+v -> %q -> %+v", status, key, parsed)
		}
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

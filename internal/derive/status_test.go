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
			name:     "shaping",
			status:   Shaping,
			expected: ":diamond_shape_with_a_dot_inside: Shaping",
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
		{name: "shaping", status: Shaping, expected: "shaping"},
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
		{name: "shaping", key: "shaping", expected: Shaping},
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
	statuses := []Status{OnTrack, AtRisk, OffTrack, NotStarted, Done, NeedsUpdate, Shaping, Unknown}

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

func TestMatchLabelPattern(t *testing.T) {
	tests := []struct {
		name       string
		label      string
		wantStatus Status
		wantOK     bool
	}{
		// Exact matches
		{name: "exact on track", label: "on track", wantStatus: OnTrack, wantOK: true},
		{name: "exact blocked", label: "blocked", wantStatus: OffTrack, wantOK: true},
		{name: "exact done", label: "done", wantStatus: Done, wantOK: true},
		{name: "exact at risk", label: "at risk", wantStatus: AtRisk, wantOK: true},
		{name: "exact not started", label: "not started", wantStatus: NotStarted, wantOK: true},
		{name: "exact complete", label: "complete", wantStatus: Done, wantOK: true},
		{name: "exact completed", label: "completed", wantStatus: Done, wantOK: true},

		// Case insensitivity
		{name: "uppercase ON TRACK", label: "ON TRACK", wantStatus: OnTrack, wantOK: true},
		{name: "mixed case At Risk", label: "At Risk", wantStatus: AtRisk, wantOK: true},
		{name: "uppercase BLOCKED", label: "BLOCKED", wantStatus: OffTrack, wantOK: true},

		// Word boundary: label contains pattern as whole words
		{name: "contains on track", label: "is on track", wantStatus: OnTrack, wantOK: true},
		{name: "contains blocked", label: "currently blocked", wantStatus: OffTrack, wantOK: true},

		// Word boundary: rejects false positives (substring within another word)
		{name: "rejects greenfield", label: "greenfield", wantStatus: Unknown, wantOK: false},
		{name: "rejects deprecated", label: "deprecated", wantStatus: Unknown, wantOK: false},
		{name: "rejects featured", label: "featured", wantStatus: Unknown, wantOK: false},
		{name: "rejects whitelisted", label: "whitelisted", wantStatus: Unknown, wantOK: false},
		{name: "rejects undone", label: "undone", wantStatus: Unknown, wantOK: false},

		// Emoji patterns skipped (labels don't use status emojis)
		{name: "emoji label ignored", label: "🟢", wantStatus: Unknown, wantOK: false},
		{name: "emoji label red ignored", label: "🔴", wantStatus: Unknown, wantOK: false},

		// Edge cases
		{name: "empty label", label: "", wantStatus: Unknown, wantOK: false},
		{name: "whitespace only", label: "   ", wantStatus: Unknown, wantOK: false},
		{name: "unrelated label", label: "epic", wantStatus: Unknown, wantOK: false},
		{name: "unrelated label bug", label: "bug", wantStatus: Unknown, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, ok := matchLabelPattern(tt.label)
			if ok != tt.wantOK {
				t.Errorf("matchLabelPattern(%q) ok = %t, want %t", tt.label, ok, tt.wantOK)
			}
			if status != tt.wantStatus {
				t.Errorf("matchLabelPattern(%q) = %+v, want %+v", tt.label, status, tt.wantStatus)
			}
		})
	}
}

func TestMapLabelsToStatus(t *testing.T) {
	tests := []struct {
		name       string
		labels     []string
		wantStatus Status
		wantOK     bool
	}{
		{
			name:       "single matching label",
			labels:     []string{"on track"},
			wantStatus: OnTrack,
			wantOK:     true,
		},
		{
			name:       "case variations",
			labels:     []string{"At Risk"},
			wantStatus: AtRisk,
			wantOK:     true,
		},
		{
			name:       "multiple labels first match wins",
			labels:     []string{"epic", "at risk"},
			wantStatus: AtRisk,
			wantOK:     true,
		},
		{
			name:       "no matching labels",
			labels:     []string{"epic", "bug", "enhancement"},
			wantStatus: Unknown,
			wantOK:     false,
		},
		{
			name:       "empty labels",
			labels:     []string{},
			wantStatus: Unknown,
			wantOK:     false,
		},
		{
			name:       "nil labels",
			labels:     nil,
			wantStatus: Unknown,
			wantOK:     false,
		},
		{
			name:       "rejects false positives in label set",
			labels:     []string{"greenfield", "deprecated", "featured"},
			wantStatus: Unknown,
			wantOK:     false,
		},
		{
			name:       "matching after non-matching",
			labels:     []string{"greenfield", "blocked"},
			wantStatus: OffTrack,
			wantOK:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, ok := MapLabelsToStatus(tt.labels)
			if ok != tt.wantOK {
				t.Errorf("MapLabelsToStatus(%v) ok = %t, want %t", tt.labels, ok, tt.wantOK)
			}
			if status != tt.wantStatus {
				t.Errorf("MapLabelsToStatus(%v) = %+v, want %+v", tt.labels, status, tt.wantStatus)
			}
		})
	}
}

package derive

import (
	"testing"
	"time"
)

func TestParseTargetDate(t *testing.T) {
	// Helper to create a UTC time for comparison
	utcTime := func(year int, month time.Month, day int) *time.Time {
		t := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
		return &t
	}

	// Helper to create a UTC datetime for comparison
	utcDateTime := func(year int, month time.Month, day, hour, min, sec int) *time.Time {
		t := time.Date(year, month, day, hour, min, sec, 0, time.UTC)
		return &t
	}

	tests := []struct {
		name     string
		input    string
		expected *time.Time
	}{
		// Valid YYYY-MM-DD formats
		{
			name:     "valid YYYY-MM-DD",
			input:    "2025-08-06",
			expected: utcTime(2025, 8, 6),
		},
		{
			name:     "valid YYYY-MM-DD with leading/trailing spaces",
			input:    "  2025-12-25  ",
			expected: utcTime(2025, 12, 25),
		},
		{
			name:     "valid YYYY-MM-DD january",
			input:    "2025-01-15",
			expected: utcTime(2025, 1, 15),
		},

		// Valid RFC3339 formats
		{
			name:     "RFC3339 with Z timezone",
			input:    "2025-08-06T10:30:00Z",
			expected: utcDateTime(2025, 8, 6, 10, 30, 0),
		},
		{
			name:     "RFC3339 with UTC offset",
			input:    "2025-08-06T10:30:00+00:00",
			expected: utcDateTime(2025, 8, 6, 10, 30, 0),
		},
		{
			name:     "RFC3339 with PST offset (converts to UTC)",
			input:    "2025-08-06T10:30:00-08:00",
			expected: utcDateTime(2025, 8, 6, 18, 30, 0), // 10:30 PST = 18:30 UTC
		},

		// Valid datetime formats
		{
			name:     "datetime without timezone",
			input:    "2025-08-06 15:45:30",
			expected: utcDateTime(2025, 8, 6, 15, 45, 30),
		},
		{
			name:     "ISO datetime without timezone",
			input:    "2025-08-06T15:45:30",
			expected: utcDateTime(2025, 8, 6, 15, 45, 30),
		},

		// Invalid or special cases that should return nil
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "whitespace only",
			input:    "   ",
			expected: nil,
		},
		{
			name:     "TBD case insensitive",
			input:    "tbd",
			expected: nil,
		},
		{
			name:     "TBD uppercase",
			input:    "TBD",
			expected: nil,
		},
		{
			name:     "N/A case insensitive",
			input:    "n/a",
			expected: nil,
		},
		{
			name:     "N/A uppercase",
			input:    "N/A",
			expected: nil,
		},
		{
			name:     "invalid date format",
			input:    "08/06/2025",
			expected: nil,
		},
		{
			name:     "invalid date text",
			input:    "next week",
			expected: nil,
		},
		{
			name:     "malformed YYYY-MM-DD",
			input:    "2025-13-45", // Invalid month and day
			expected: nil,
		},
		{
			name:     "incomplete date",
			input:    "2025-08",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseTargetDate(tt.input)
			
			// Both nil
			if tt.expected == nil && result == nil {
				return
			}
			
			// One nil, one not
			if (tt.expected == nil) != (result == nil) {
				t.Errorf("ParseTargetDate(%q) = %v, expected %v", 
					tt.input, result, tt.expected)
				return
			}
			
			// Both not nil - compare times
			if !tt.expected.Equal(*result) {
				t.Errorf("ParseTargetDate(%q) = %v, expected %v", 
					tt.input, result, tt.expected)
			}
		})
	}
}

func TestRenderTargetDate(t *testing.T) {
	// Helper to create a UTC time
	utcTime := func(year int, month time.Month, day int) *time.Time {
		t := time.Date(year, month, day, 15, 30, 45, 0, time.UTC)
		return &t
	}

	// Helper to create a time in different timezone
	pstTime := func(year int, month time.Month, day int) *time.Time {
		loc, _ := time.LoadLocation("America/Los_Angeles")
		t := time.Date(year, month, day, 10, 15, 30, 0, loc)
		return &t
	}

	tests := []struct {
		name     string
		input    *time.Time
		expected string
	}{
		{
			name:     "nil time",
			input:    nil,
			expected: "TBD",
		},
		{
			name:     "valid UTC time",
			input:    utcTime(2025, 8, 6),
			expected: "2025-08-06",
		},
		{
			name:     "time with different timezone (converted to UTC)",
			input:    pstTime(2025, 8, 6),
			expected: "2025-08-06", // Should still render as the same date in UTC
		},
		{
			name:     "january date",
			input:    utcTime(2025, 1, 1),
			expected: "2025-01-01",
		},
		{
			name:     "december date",
			input:    utcTime(2025, 12, 31),
			expected: "2025-12-31",
		},
		{
			name:     "leap year date",
			input:    utcTime(2024, 2, 29),
			expected: "2024-02-29",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RenderTargetDate(tt.input)
			if result != tt.expected {
				t.Errorf("RenderTargetDate(%v) = %q, expected %q", 
					tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsValidDate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Valid dates
		{
			name:     "valid YYYY-MM-DD",
			input:    "2025-08-06",
			expected: true,
		},
		{
			name:     "valid RFC3339",
			input:    "2025-08-06T10:30:00Z",
			expected: true,
		},
		{
			name:     "valid datetime",
			input:    "2025-08-06 15:45:30",
			expected: true,
		},

		// Invalid dates
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "TBD",
			input:    "TBD",
			expected: false,
		},
		{
			name:     "invalid format",
			input:    "08/06/2025",
			expected: false,
		},
		{
			name:     "invalid date",
			input:    "2025-13-45",
			expected: false,
		},
		{
			name:     "text",
			input:    "next week",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidDate(tt.input)
			if result != tt.expected {
				t.Errorf("IsValidDate(%q) = %t, expected %t", 
					tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseTargetDateTimezoneConsistency(t *testing.T) {
	// Test that dates are consistently converted to UTC regardless of input timezone
	inputs := []string{
		"2025-08-06T10:30:00Z",        // UTC
		"2025-08-06T10:30:00+00:00",   // UTC with offset
		"2025-08-06T02:30:00-08:00",   // PST (same as UTC 10:30)
		"2025-08-06T06:30:00-04:00",   // EDT (same as UTC 10:30)
	}

	var results []*time.Time
	for _, input := range inputs {
		result := ParseTargetDate(input)
		if result == nil {
			t.Fatalf("ParseTargetDate(%q) returned nil, expected valid time", input)
		}
		results = append(results, result)
	}

	// All results should be equal when converted to UTC
	expected := results[0]
	for i, result := range results[1:] {
		if !expected.Equal(*result) {
			t.Errorf("ParseTargetDate timezone consistency failed: input %q gave %v, expected %v",
				inputs[i+1], result, expected)
		}
	}

	// All should be in UTC timezone
	for i, result := range results {
		if result.Location() != time.UTC {
			t.Errorf("ParseTargetDate(%q) returned non-UTC timezone: %v", 
				inputs[i], result.Location())
		}
	}
}
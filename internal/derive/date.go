package derive

import (
	"strings"
	"time"
)

// Common date layout formats to try when parsing
var dateLayouts = []string{
	"2006-01-02",                // YYYY-MM-DD (ISO 8601 date)
	time.RFC3339,                // RFC3339 format (e.g., 2025-01-15T10:30:00Z)
	"2006-01-02T15:04:05Z07:00", // RFC3339 alternative
	"2006-01-02 15:04:05",       // Common datetime format
	"2006-01-02T15:04:05",       // ISO 8601 without timezone
}

// ParseTargetDate attempts to parse a target date string into a time.Time pointer
// Returns nil if the date string is empty, invalid, or cannot be parsed
// Tries multiple common date formats: YYYY-MM-DD, RFC3339, and variants
func ParseTargetDate(raw string) *time.Time {
	if raw == "" {
		return nil
	}

	// Normalize whitespace and remove common prefixes/suffixes
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ToLower(raw) == "tbd" || strings.ToLower(raw) == "n/a" {
		return nil
	}

	// Try each layout format
	for _, layout := range dateLayouts {
		if parsed, err := time.Parse(layout, raw); err == nil {
			// Convert to UTC for consistent handling
			utc := parsed.UTC()
			return &utc
		}
	}

	// If no format worked, return nil
	return nil
}

// RenderTargetDate formats a time pointer as a date string
// Returns "TBD" if the time pointer is nil
// Returns YYYY-MM-DD format for valid dates (always in UTC)
func RenderTargetDate(t *time.Time) string {
	if t == nil {
		return "TBD"
	}

	// Format as YYYY-MM-DD in UTC
	return t.UTC().Format("2006-01-02")
}

// IsValidDate checks if a date string can be successfully parsed
// This is a helper function for validation purposes
func IsValidDate(raw string) bool {
	return ParseTargetDate(raw) != nil
}

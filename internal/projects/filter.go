package projects

import (
	"strings"

	"github.com/Attamusc/weekly-report-cli/internal/input"
)

// FilterProjectItems filters project items based on the provided configuration
// Returns only items that match all filter criteria
func FilterProjectItems(items []ProjectItem, config ProjectConfig) []input.IssueRef {
	var issueRefs []input.IssueRef

	for _, item := range items {
		// Skip draft issues (they don't have issue refs)
		if item.ContentType == ContentTypeDraftIssue {
			continue
		}

		// Skip PRs if not included
		if item.ContentType == ContentTypePullRequest && !config.IncludePRs {
			continue
		}

		// Check if item has an issue ref (should always be true for issues/PRs)
		if item.IssueRef == nil {
			continue
		}

		// Apply field filters
		if !MatchesFilters(item, config.FieldFilters) {
			continue
		}

		// Item passes all filters, add to results
		issueRefs = append(issueRefs, *item.IssueRef)
	}

	return issueRefs
}

// MatchesFilters checks if a ProjectItem matches all field filters
// Uses AND logic between filters (all must match)
// Uses OR logic within a filter (any value can match)
func MatchesFilters(item ProjectItem, filters []FieldFilter) bool {
	// If no filters, everything matches
	if len(filters) == 0 {
		return true
	}

	// Check each filter (AND logic)
	for _, filter := range filters {
		// Get the field value for this filter
		fieldValue, exists := item.FieldValues[filter.FieldName]
		if !exists {
			// Field doesn't exist on this item, filter fails
			return false
		}

		// Check if field value matches any of the filter values (OR logic)
		if !matchFieldValue(fieldValue, filter.Values) {
			return false
		}
	}

	// All filters matched
	return true
}

// matchFieldValue checks if a field value matches any of the filter values
// Handles different field types with appropriate matching logic
func matchFieldValue(value FieldValue, filterValues []string) bool {
	// If no filter values, nothing can match
	if len(filterValues) == 0 {
		return false
	}

	switch value.Type {
	case FieldTypeText:
		return matchTextValue(value.Text, filterValues)

	case FieldTypeSingleSelect:
		return matchSingleSelectValue(value.Text, filterValues)

	case FieldTypeDate:
		// For dates, convert to string and do text matching
		if value.Date != nil {
			dateStr := value.Date.Format("2006-01-02")
			return matchTextValue(dateStr, filterValues)
		}
		return false

	case FieldTypeNumber:
		// For numbers, convert to string and do text matching
		numberStr := value.String()
		return matchTextValue(numberStr, filterValues)

	default:
		return false
	}
}

// matchTextValue checks if text matches any filter value (case-insensitive, contains)
func matchTextValue(text string, filterValues []string) bool {
	textLower := strings.ToLower(strings.TrimSpace(text))

	for _, filterValue := range filterValues {
		filterLower := strings.ToLower(strings.TrimSpace(filterValue))

		// Check for exact match first
		if textLower == filterLower {
			return true
		}

		// Check if text contains the filter value
		if strings.Contains(textLower, filterLower) {
			return true
		}
	}

	return false
}

// matchSingleSelectValue checks if single-select value matches any filter value (case-insensitive, exact)
func matchSingleSelectValue(value string, filterValues []string) bool {
	valueLower := strings.ToLower(strings.TrimSpace(value))

	for _, filterValue := range filterValues {
		filterLower := strings.ToLower(strings.TrimSpace(filterValue))

		// Single-select uses exact match only
		if valueLower == filterLower {
			return true
		}
	}

	return false
}

// DeduplicateIssueRefs removes duplicate issue references while preserving order
func DeduplicateIssueRefs(refs []input.IssueRef) []input.IssueRef {
	seen := make(map[string]bool)
	var unique []input.IssueRef

	for _, ref := range refs {
		// Use canonical URL as the key for deduplication
		if !seen[ref.URL] {
			seen[ref.URL] = true
			unique = append(unique, ref)
		}
	}

	return unique
}
